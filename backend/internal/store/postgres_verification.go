package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

func (p *Postgres) SubmitVerification(requesterID, rewardID string, in SubmitVerificationInput) (*InboxItem, error) {
	if utf8.RuneCountInString(in.Note) > 200 {
		return nil, fmt.Errorf("note too long")
	}
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var scope, taskID string
	var stageID sql.NullString
	var redeemState, rewardText string
	var rewardLink sql.NullString
	var owner string
	var verifier sql.NullString
	var taskStatus, taskTitle string
	err = tx.QueryRowContext(ctx, `
SELECT r.scope, r.task_id, r.stage_id, r.redeem_state, r.text, r.link,
       t.owner_account_id, t.designated_verifier_account_id, t.status, t.title
FROM rewards r
JOIN tasks t ON t.id = r.task_id
WHERE r.id=$1`, rewardID).Scan(
		&scope, &taskID, &stageID, &redeemState, &rewardText, &rewardLink,
		&owner, &verifier, &taskStatus, &taskTitle)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	if owner != requesterID {
		return nil, fmt.Errorf("forbidden")
	}
	if !verifier.Valid {
		return nil, fmt.Errorf("verifier not designated")
	}
	verifierID := verifier.String
	ok, err := p.friendsInTx(ctx, tx, requesterID, verifierID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("verifier must be a friend")
	}
	blocked, err := p.blockedInTx(ctx, tx, requesterID, verifierID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, fmt.Errorf("blocked")
	}
	var stageTitle *string
	if scope == string(domain.RewardStage) {
		if !stageID.Valid {
			return nil, fmt.Errorf("invalid reward")
		}
		var prog, stTitle string
		err := tx.QueryRowContext(ctx, `SELECT progress, title FROM stages WHERE id=$1`, stageID.String).
			Scan(&prog, &stTitle)
		if err != nil {
			return nil, fmt.Errorf("stage not done")
		}
		if prog != string(domain.StageDone) {
			return nil, fmt.Errorf("stage not done")
		}
		stageTitle = &stTitle
	} else if taskStatus != string(domain.TaskCompleted) {
		// PRD v1.1 complete gate: submit task_completion while active once stages ready
		srows, err := tx.QueryContext(ctx, `SELECT progress FROM stages WHERE task_id=$1`, taskID)
		if err != nil {
			return nil, err
		}
		var stages []domain.Stage
		for srows.Next() {
			var prog string
			if err := srows.Scan(&prog); err != nil {
				srows.Close()
				return nil, err
			}
			stages = append(stages, domain.Stage{Progress: domain.StageProgress(prog)})
		}
		srows.Close()
		if err := stagesReadyForCompletion(stages); err != nil {
			return nil, err
		}
	}

	rw := &domain.Reward{RedeemState: domain.RedeemState(redeemState)}
	if err := domain.ApplyVerificationEvent(rw, "submit"); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE rewards SET redeem_state=$2 WHERE id=$1`, rewardID, rw.RedeemState); err != nil {
		return nil, err
	}

	var maxAttempt sql.NullInt64
	_ = tx.QueryRowContext(ctx, `SELECT MAX(attempt) FROM verification_requests WHERE reward_id=$1`, rewardID).Scan(&maxAttempt)
	attempt := 1
	if maxAttempt.Valid {
		attempt = int(maxAttempt.Int64) + 1
	}
	now := time.Now().UTC()
	reqID := uuid.NewString()
	note := strings.TrimSpace(in.Note)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO verification_requests
  (id, reward_id, attempt, requester_account_id, verifier_account_id, status, note, created_at)
VALUES ($1,$2,$3,$4,$5,'pending',$6,$7)`,
		reqID, rewardID, attempt, requesterID, verifierID, note, now); err != nil {
		return nil, err
	}
	shot := strings.TrimSpace(in.ScreenshotKey)
	if err := p.attachScreenshotTx(ctx, tx, requesterID, shot, reqID); err != nil {
		return nil, err
	}
	if err := p.insertNoticeTx(ctx, tx, verifierID, domain.NoticeVerificationPending, reqID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	nick := ""
	_ = p.db.QueryRow(`SELECT nickname FROM accounts WHERE id=$1`, requesterID).Scan(&nick)
	var link *string
	if rewardLink.Valid {
		s := rewardLink.String
		link = &s
	}
	shotKey, shotURL := withScreenshotURL(shot)
	return &InboxItem{
		RequestID:         reqID,
		RewardID:          rewardID,
		Attempt:           attempt,
		Status:            string(domain.VerifPending),
		Note:              note,
		ScreenshotKey:     shotKey,
		ScreenshotURL:     shotURL,
		ApplicantNickname: nick,
		TaskTitle:         taskTitle,
		StageTitle:        stageTitle,
		RewardText:        rewardText,
		RewardLink:        link,
		CreatedAt:         now,
	}, nil
}

func (p *Postgres) ListInbox(verifierID string) ([]InboxItem, error) {
	rows, err := p.db.Query(`
SELECT v.id, v.reward_id, v.attempt, v.status, v.note, v.created_at,
       a.nickname, t.title, r.text, r.link, r.scope, s.title, sc.storage_key
FROM verification_requests v
JOIN accounts a ON a.id = v.requester_account_id
JOIN rewards r ON r.id = v.reward_id
JOIN tasks t ON t.id = r.task_id
LEFT JOIN stages s ON s.id = r.stage_id
LEFT JOIN screenshots sc ON sc.request_id = v.id
WHERE v.verifier_account_id=$1 AND v.status='pending'
ORDER BY v.created_at DESC`, verifierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InboxItem{}
	for rows.Next() {
		var it InboxItem
		var link, stageTitle, shot sql.NullString
		var scope string
		if err := rows.Scan(&it.RequestID, &it.RewardID, &it.Attempt, &it.Status, &it.Note, &it.CreatedAt,
			&it.ApplicantNickname, &it.TaskTitle, &it.RewardText, &link, &scope, &stageTitle, &shot); err != nil {
			return nil, err
		}
		it.CreatedAt = it.CreatedAt.UTC()
		if link.Valid {
			s := link.String
			it.RewardLink = &s
		}
		if scope == string(domain.RewardStage) && stageTitle.Valid {
			s := stageTitle.String
			it.StageTitle = &s
		}
		if shot.Valid {
			it.ScreenshotKey, it.ScreenshotURL = withScreenshotURL(shot.String)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (p *Postgres) ListOutbox(requesterID string) ([]OutboxItem, error) {
	rows, err := p.db.Query(`
SELECT v.id, v.reward_id, v.attempt, v.status, v.note, v.resolution_note, v.created_at, v.resolved_at,
       t.title, r.text, r.scope, s.title
FROM verification_requests v
JOIN rewards r ON r.id = v.reward_id
JOIN tasks t ON t.id = r.task_id
LEFT JOIN stages s ON s.id = r.stage_id
WHERE v.requester_account_id=$1
ORDER BY v.created_at DESC`, requesterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OutboxItem{}
	for rows.Next() {
		var it OutboxItem
		var resolved sql.NullTime
		var stageTitle sql.NullString
		var scope string
		if err := rows.Scan(&it.RequestID, &it.RewardID, &it.Attempt, &it.Status, &it.Note, &it.ResolutionNote,
			&it.CreatedAt, &resolved, &it.TaskTitle, &it.RewardText, &scope, &stageTitle); err != nil {
			return nil, err
		}
		it.CreatedAt = it.CreatedAt.UTC()
		if resolved.Valid {
			t := resolved.Time.UTC()
			it.ResolvedAt = &t
		}
		if scope == string(domain.RewardStage) && stageTitle.Valid {
			s := stageTitle.String
			it.StageTitle = &s
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (p *Postgres) Approve(verifierID, requestID string) error {
	return p.resolve(verifierID, requestID, "approve", "")
}

func (p *Postgres) Reject(verifierID, requestID, resolutionNote string) error {
	if utf8.RuneCountInString(resolutionNote) > 80 {
		return fmt.Errorf("resolution note too long")
	}
	return p.resolve(verifierID, requestID, "reject", strings.TrimSpace(resolutionNote))
}

func (p *Postgres) Withdraw(requesterID, requestID string) error {
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var status, rewardID, verifierID, reqAcc string
	err = tx.QueryRowContext(ctx, `
SELECT status, reward_id, verifier_account_id, requester_account_id
FROM verification_requests WHERE id=$1`, requestID).
		Scan(&status, &rewardID, &verifierID, &reqAcc)
	if err == sql.ErrNoRows {
		return fmt.Errorf("not found")
	}
	if err != nil {
		return err
	}
	if reqAcc != requesterID {
		return fmt.Errorf("forbidden")
	}
	if status != string(domain.VerifPending) {
		return fmt.Errorf("not pending")
	}
	var redeemState string
	if err := tx.QueryRowContext(ctx, `SELECT redeem_state FROM rewards WHERE id=$1`, rewardID).Scan(&redeemState); err != nil {
		return err
	}
	rw := &domain.Reward{RedeemState: domain.RedeemState(redeemState)}
	if err := domain.ApplyVerificationEvent(rw, "withdraw"); err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE rewards SET redeem_state=$2 WHERE id=$1`, rewardID, rw.RedeemState); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE verification_requests SET status='withdrawn', resolved_at=$2 WHERE id=$1`, requestID, now); err != nil {
		return err
	}
	if err := p.insertNoticeTx(ctx, tx, verifierID, domain.NoticeVerificationWithdrawn, requestID); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) resolve(verifierID, requestID, event, resolutionNote string) error {
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var status, rewardID, verAcc, reqAcc string
	err = tx.QueryRowContext(ctx, `
SELECT status, reward_id, verifier_account_id, requester_account_id
FROM verification_requests WHERE id=$1`, requestID).
		Scan(&status, &rewardID, &verAcc, &reqAcc)
	if err == sql.ErrNoRows {
		return fmt.Errorf("not found")
	}
	if err != nil {
		return err
	}
	if verAcc != verifierID {
		return fmt.Errorf("forbidden")
	}
	if status != string(domain.VerifPending) {
		return fmt.Errorf("not pending")
	}
	var redeemState string
	if err := tx.QueryRowContext(ctx, `SELECT redeem_state FROM rewards WHERE id=$1`, rewardID).Scan(&redeemState); err != nil {
		return err
	}
	rw := &domain.Reward{RedeemState: domain.RedeemState(redeemState)}
	if err := domain.ApplyVerificationEvent(rw, event); err != nil {
		return err
	}
	now := time.Now().UTC()
	if event == "approve" {
		if _, err := tx.ExecContext(ctx, `
UPDATE rewards SET redeem_state=$2, redeemed_at=$3 WHERE id=$1`, rewardID, rw.RedeemState, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE verification_requests SET status='approved', resolved_at=$2, resolution_note=$3 WHERE id=$1`,
			requestID, now, resolutionNote); err != nil {
			return err
		}
		if err := p.insertNoticeTx(ctx, tx, reqAcc, domain.NoticeVerificationApproved, requestID); err != nil {
			return err
		}
		var scope string
		var taskIDForComplete string
		if err := tx.QueryRowContext(ctx, `SELECT scope, task_id FROM rewards WHERE id=$1`, rewardID).Scan(&scope, &taskIDForComplete); err != nil {
			return err
		}
		if scope == string(domain.RewardTaskCompletion) {
			if _, err := tx.ExecContext(ctx, `
UPDATE tasks SET status='completed', completed_at=$2, updated_at=$2
WHERE id=$1 AND status<>'completed'`, taskIDForComplete, now); err != nil {
				return err
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE rewards SET redeem_state=$2 WHERE id=$1`, rewardID, rw.RedeemState); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE verification_requests SET status='rejected', resolved_at=$2, resolution_note=$3 WHERE id=$1`,
			requestID, now, resolutionNote); err != nil {
			return err
		}
		if err := p.insertNoticeTx(ctx, tx, reqAcc, domain.NoticeVerificationRejected, requestID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Postgres) insertNoticeTx(ctx context.Context, tx *sql.Tx, accountID string, kind domain.NoticeKind, requestID string) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO in_app_notices (id, account_id, kind, request_id, created_at)
VALUES ($1,$2,$3,$4,now())`, uuid.NewString(), accountID, string(kind), requestID)
	return err
}
