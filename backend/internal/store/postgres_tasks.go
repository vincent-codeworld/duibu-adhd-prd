package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

func (p *Postgres) CreateSyncedTask(ownerID string, in CreateTaskInput) (*TaskView, error) {
	if !in.UploadConsent {
		return nil, fmt.Errorf("uploadConsent required")
	}
	if err := validateTitleNotes(in.Title, in.Notes); err != nil {
		return nil, err
	}
	if err := validateDueAt(in.DueAt); err != nil {
		return nil, err
	}
	if err := validateStages(in.Stages); err != nil {
		return nil, err
	}
	if err := validateRemindAts(in.DueAt.UTC(), in.Stages); err != nil {
		return nil, err
	}
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, ownerID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("unauthorized")
	}
	var verifier *string
	if in.DesignatedVerifierAccountID != nil && strings.TrimSpace(*in.DesignatedVerifierAccountID) != "" {
		vid := strings.TrimSpace(*in.DesignatedVerifierAccountID)
		ok, err := p.friendsInTx(ctx, tx, ownerID, vid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("verifier must be a friend")
		}
		blocked, err := p.blockedInTx(ctx, tx, ownerID, vid)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, fmt.Errorf("blocked")
		}
		verifier = &vid
	}
	now := time.Now().UTC()
	dueAt := in.DueAt.UTC()
	taskID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `
INSERT INTO tasks (id, owner_account_id, sync_state, title, notes, status, designated_verifier_account_id, due_at, created_at, updated_at, upload_consent_at)
VALUES ($1,$2,'synced',$3,$4,'active',$5,$6,$7,$7,$7)`,
		taskID, ownerID, strings.TrimSpace(in.Title), strings.TrimSpace(in.Notes), verifier, dueAt, now)
	if err != nil {
		return nil, err
	}
	for i, s := range in.Stages {
		prog := s.Progress
		if prog == "" {
			prog = string(domain.StagePending)
		}
		stID := uuid.NewString()
		var remind any
		if s.RemindAt != nil {
			remind = s.RemindAt.UTC()
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO stages (id, task_id, position, title, progress, remind_at) VALUES ($1,$2,$3,$4,$5,$6)`,
			stID, taskID, i+1, strings.TrimSpace(s.Title), prog, remind)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d", i+1)
		if rw, ok := in.StageRewards[key]; ok {
			if err := validateRewardText(rw.Text); err != nil {
				return nil, err
			}
			if err := validateLink(rw.Link); err != nil {
				return nil, err
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO rewards (id, scope, task_id, stage_id, text, link, redeem_state)
VALUES ($1,'stage',$2,$3,$4,$5,'unrequested')`,
				uuid.NewString(), taskID, stID, strings.TrimSpace(rw.Text), rw.Link); err != nil {
				return nil, err
			}
		}
	}
	needCompletion := verifier != nil
	if in.CompletionReward != nil && strings.TrimSpace(in.CompletionReward.Text) != "" {
		if err := validateRewardText(in.CompletionReward.Text); err != nil {
			return nil, err
		}
		if err := validateLink(in.CompletionReward.Link); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO rewards (id, scope, task_id, stage_id, text, link, redeem_state)
VALUES ($1,'task_completion',$2,NULL,$3,$4,'unrequested')`,
			uuid.NewString(), taskID, strings.TrimSpace(in.CompletionReward.Text), in.CompletionReward.Link); err != nil {
			return nil, err
		}
		needCompletion = false
	}
	if needCompletion {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO rewards (id, scope, task_id, stage_id, text, link, redeem_state)
VALUES ($1,'task_completion',$2,NULL,$3,NULL,'unrequested')`,
			uuid.NewString(), taskID, defaultCompletionGateText); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetSyncedTask(ownerID, taskID)
}

func (p *Postgres) ListSyncedTasks(ownerID string) ([]TaskView, error) {
	rows, err := p.db.Query(`SELECT id FROM tasks WHERE owner_account_id=$1 ORDER BY created_at`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []TaskView{}
	for _, id := range ids {
		tv, err := p.GetSyncedTask(ownerID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *tv)
	}
	return out, nil
}

func (p *Postgres) GetSyncedTask(ownerID, taskID string) (*TaskView, error) {
	return p.loadTaskView(context.Background(), p.db, ownerID, taskID)
}

func (p *Postgres) DeleteSyncedTask(ownerID, taskID string) error {
	res, err := p.db.Exec(`DELETE FROM tasks WHERE id=$1 AND owner_account_id=$2`, taskID, ownerID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

func (p *Postgres) SetVerifier(ownerID, taskID, verifierID string) (*TaskView, error) {
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var owner string
	err = tx.QueryRowContext(ctx, `SELECT owner_account_id FROM tasks WHERE id=$1`, taskID).Scan(&owner)
	if err == sql.ErrNoRows || (err == nil && owner != ownerID) {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	ok, err := p.friendsInTx(ctx, tx, ownerID, verifierID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("verifier must be a friend")
	}
	blocked, err := p.blockedInTx(ctx, tx, ownerID, verifierID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, fmt.Errorf("blocked")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE tasks SET designated_verifier_account_id=$2, updated_at=now() WHERE id=$1`, taskID, verifierID); err != nil {
		return nil, err
	}
	var hasCompletion bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM rewards WHERE task_id=$1 AND scope='task_completion')`, taskID).Scan(&hasCompletion); err != nil {
		return nil, err
	}
	if !hasCompletion {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO rewards (id, scope, task_id, stage_id, text, link, redeem_state)
VALUES ($1,'task_completion',$2,NULL,$3,NULL,'unrequested')`,
			uuid.NewString(), taskID, defaultCompletionGateText); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetSyncedTask(ownerID, taskID)
}

func (p *Postgres) CompleteTask(ownerID, taskID string) (*TaskView, error) {
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var owner string
	var verifier sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT owner_account_id, designated_verifier_account_id FROM tasks WHERE id=$1`, taskID).Scan(&owner, &verifier)
	if err == sql.ErrNoRows || (err == nil && owner != ownerID) {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	if verifier.Valid {
		return nil, fmt.Errorf("verification required")
	}
	rows, err := tx.QueryContext(ctx, `SELECT progress FROM stages WHERE task_id=$1`, taskID)
	if err != nil {
		return nil, err
	}
	var stages []domain.Stage
	for rows.Next() {
		var prog string
		if err := rows.Scan(&prog); err != nil {
			rows.Close()
			return nil, err
		}
		stages = append(stages, domain.Stage{Progress: domain.StageProgress(prog)})
	}
	rows.Close()
	if err := stagesReadyForCompletion(stages); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
UPDATE tasks SET status='completed', completed_at=$2, updated_at=$2 WHERE id=$1`, taskID, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetSyncedTask(ownerID, taskID)
}

func (p *Postgres) PatchSyncedTask(ownerID, taskID string, in PatchTaskInput) (*TaskView, error) {
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var title, notes string
	var owner string
	var dueAt time.Time
	var hasVerifier bool
	err = tx.QueryRowContext(ctx, `
SELECT owner_account_id, title, notes, due_at, designated_verifier_account_id IS NOT NULL
FROM tasks WHERE id=$1`, taskID).
		Scan(&owner, &title, &notes, &dueAt, &hasVerifier)
	if err == sql.ErrNoRows || (err == nil && owner != ownerID) {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	if in.Title != nil {
		if err := validateTitleNotes(*in.Title, notes); err != nil {
			return nil, err
		}
		title = strings.TrimSpace(*in.Title)
	}
	if in.Notes != nil {
		if err := validateTitleNotes(title, *in.Notes); err != nil {
			return nil, err
		}
		notes = strings.TrimSpace(*in.Notes)
	}
	if in.DueAt != nil {
		if err := validateDueAt(*in.DueAt); err != nil {
			return nil, err
		}
		dueAt = in.DueAt.UTC()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET title=$2, notes=$3, due_at=$4, updated_at=now() WHERE id=$1`, taskID, title, notes, dueAt); err != nil {
		return nil, err
	}

	if in.Stages != nil {
		if err := validateStages(in.Stages); err != nil {
			return nil, err
		}
		if err := validateRemindAts(dueAt, in.Stages); err != nil {
			return nil, err
		}
		type oldStage struct {
			id       string
			position int
			progress string
		}
		oldStages := map[int]oldStage{}
		srows, err := tx.QueryContext(ctx, `SELECT id, position, progress FROM stages WHERE task_id=$1`, taskID)
		if err != nil {
			return nil, err
		}
		for srows.Next() {
			var s oldStage
			if err := srows.Scan(&s.id, &s.position, &s.progress); err != nil {
				srows.Close()
				return nil, err
			}
			oldStages[s.position] = s
		}
		srows.Close()

		type oldRew struct {
			id, stageID, text string
			link              sql.NullString
			state             string
		}
		oldByPos := map[int]oldRew{}
		rrows, err := tx.QueryContext(ctx, `
SELECT r.id, r.stage_id, r.text, r.link, r.redeem_state, s.position
FROM rewards r JOIN stages s ON s.id = r.stage_id
WHERE r.task_id=$1 AND r.scope='stage'`, taskID)
		if err != nil {
			return nil, err
		}
		for rrows.Next() {
			var rw oldRew
			var pos int
			if err := rrows.Scan(&rw.id, &rw.stageID, &rw.text, &rw.link, &rw.state, &pos); err != nil {
				rrows.Close()
				return nil, err
			}
			oldByPos[pos] = rw
		}
		rrows.Close()

		now := time.Now().UTC()
		keepPos := map[int]struct{}{}
		for i, s := range in.Stages {
			pos := i + 1
			keepPos[pos] = struct{}{}
			prog := s.Progress
			if prog == "" {
				prog = string(domain.StagePending)
			}
			if old, ok := oldStages[pos]; ok {
				if old.progress == string(domain.StageDone) && domain.StageProgress(prog) == domain.StagePending {
					if rw, ok := oldByPos[pos]; ok {
						r := &domain.Reward{Text: rw.text, RedeemState: domain.RedeemState(rw.state)}
						if !domain.CanUncheckStage(domain.StageDone, r) {
							return nil, fmt.Errorf("cannot uncheck stage %d: reward locked", pos)
						}
					}
				}
			}
			stID := uuid.NewString()
			if old, ok := oldStages[pos]; ok {
				stID = old.id
			}
			var doneAt, skippedAt *time.Time
			if domain.StageProgress(prog) == domain.StageDone {
				doneAt = &now
			}
			if domain.StageProgress(prog) == domain.StageSkipped {
				skippedAt = &now
			}
			var remind any
			if s.RemindAt != nil {
				remind = s.RemindAt.UTC()
			}
			if _, ok := oldStages[pos]; ok {
				if _, err := tx.ExecContext(ctx, `
UPDATE stages SET title=$2, progress=$3, done_at=$4, skipped_at=$5, remind_at=$6 WHERE id=$1`,
					stID, strings.TrimSpace(s.Title), prog, doneAt, skippedAt, remind); err != nil {
					return nil, err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `
INSERT INTO stages (id, task_id, position, title, progress, done_at, skipped_at, remind_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
					stID, taskID, pos, strings.TrimSpace(s.Title), prog, doneAt, skippedAt, remind); err != nil {
					return nil, err
				}
			}

			if in.StageRewards != nil {
				key := fmt.Sprintf("%d", pos)
				if rwIn, ok := in.StageRewards[key]; ok {
					if err := validateRewardText(rwIn.Text); err != nil {
						return nil, err
					}
					if err := validateLink(rwIn.Link); err != nil {
						return nil, err
					}
					if old, ok := oldByPos[pos]; ok {
						if _, err := tx.ExecContext(ctx, `
UPDATE rewards SET text=$2, link=$3, stage_id=$4 WHERE id=$1`,
							old.id, strings.TrimSpace(rwIn.Text), rwIn.Link, stID); err != nil {
							return nil, err
						}
					} else {
						if _, err := tx.ExecContext(ctx, `
INSERT INTO rewards (id, scope, task_id, stage_id, text, link, redeem_state)
VALUES ($1,'stage',$2,$3,$4,$5,'unrequested')`,
							uuid.NewString(), taskID, stID, strings.TrimSpace(rwIn.Text), rwIn.Link); err != nil {
							return nil, err
						}
					}
				} else if old, ok := oldByPos[pos]; ok {
					// Explicit map provided but this position omitted → drop reward
					if _, err := tx.ExecContext(ctx, `DELETE FROM rewards WHERE id=$1`, old.id); err != nil {
						return nil, err
					}
				}
			}
		}
		// Remove stages (and cascading rewards) for positions no longer present.
		for pos, old := range oldStages {
			if _, ok := keepPos[pos]; ok {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM stages WHERE id=$1`, old.id); err != nil {
				return nil, err
			}
		}
	}

	if in.ClearCompletionReward {
		if hasVerifier {
			return nil, fmt.Errorf("completion reward required when verifier designated")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rewards WHERE task_id=$1 AND scope='task_completion'`, taskID); err != nil {
			return nil, err
		}
	} else if in.CompletionReward != nil && strings.TrimSpace(in.CompletionReward.Text) != "" {
		if err := validateRewardText(in.CompletionReward.Text); err != nil {
			return nil, err
		}
		if err := validateLink(in.CompletionReward.Link); err != nil {
			return nil, err
		}
		var rid string
		err := tx.QueryRowContext(ctx, `SELECT id FROM rewards WHERE task_id=$1 AND scope='task_completion'`, taskID).Scan(&rid)
		if err == sql.ErrNoRows {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO rewards (id, scope, task_id, text, link, redeem_state)
VALUES ($1,'task_completion',$2,$3,$4,'unrequested')`,
				uuid.NewString(), taskID, strings.TrimSpace(in.CompletionReward.Text), in.CompletionReward.Link); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else {
			if _, err := tx.ExecContext(ctx, `
UPDATE rewards SET text=$2, link=$3 WHERE id=$1`,
				rid, strings.TrimSpace(in.CompletionReward.Text), in.CompletionReward.Link); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.GetSyncedTask(ownerID, taskID)
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (p *Postgres) loadTaskView(ctx context.Context, q queryer, ownerID, taskID string) (*TaskView, error) {
	var v TaskView
	var verifier sql.NullString
	var completedAt sql.NullTime
	var owner string
	err := q.QueryRowContext(ctx, `
SELECT owner_account_id, title, notes, status, designated_verifier_account_id,
       due_at, upload_consent_at, created_at, updated_at, completed_at
FROM tasks WHERE id=$1`, taskID).Scan(
		&owner, &v.Title, &v.Notes, &v.Status, &verifier,
		&v.DueAt, &v.UploadConsentAt, &v.CreatedAt, &v.UpdatedAt, &completedAt)
	if err == sql.ErrNoRows || (err == nil && owner != ownerID) {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	v.ID = taskID
	v.DueAt = v.DueAt.UTC()
	v.CreatedAt = v.CreatedAt.UTC()
	v.UpdatedAt = v.UpdatedAt.UTC()
	v.UploadConsentAt = v.UploadConsentAt.UTC()
	if verifier.Valid {
		s := verifier.String
		v.DesignatedVerifierAccountID = &s
	}
	if completedAt.Valid {
		t := completedAt.Time.UTC()
		v.CompletedAt = &t
	}

	type rw struct {
		id, text, state string
		link            sql.NullString
		stageID         sql.NullString
		scope           string
	}
	rewards := []rw{}
	rrows, err := q.QueryContext(ctx, `
SELECT id, scope, stage_id, text, link, redeem_state FROM rewards WHERE task_id=$1`, taskID)
	if err != nil {
		return nil, err
	}
	for rrows.Next() {
		var r rw
		if err := rrows.Scan(&r.id, &r.scope, &r.stageID, &r.text, &r.link, &r.state); err != nil {
			rrows.Close()
			return nil, err
		}
		rewards = append(rewards, r)
	}
	rrows.Close()
	byStage := map[string]*RewardView{}
	for _, r := range rewards {
		var link *string
		if r.link.Valid {
			s := r.link.String
			link = &s
		}
		rv := &RewardView{ID: r.id, Text: r.text, Link: link, RedeemState: r.state}
		if r.scope == string(domain.RewardTaskCompletion) {
			v.CompletionReward = rv
		} else if r.stageID.Valid {
			byStage[r.stageID.String] = rv
		}
	}

	srows, err := q.QueryContext(ctx, `
SELECT id, position, title, progress, remind_at FROM stages WHERE task_id=$1 ORDER BY position`, taskID)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var s StageView
		var remind sql.NullTime
		if err := srows.Scan(&s.ID, &s.Position, &s.Title, &s.Progress, &remind); err != nil {
			return nil, err
		}
		if remind.Valid {
			t := remind.Time.UTC()
			s.RemindAt = &t
		}
		if rw := byStage[s.ID]; rw != nil {
			s.Reward = rw
		}
		v.Stages = append(v.Stages, s)
	}
	return &v, srows.Err()
}

func (p *Postgres) friendsInTx(ctx context.Context, tx *sql.Tx, a, b string) (bool, error) {
	var ok bool
	err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM friendships
  WHERE removed_at IS NULL
    AND ((account_a=$1 AND account_b=$2) OR (account_a=$2 AND account_b=$1))
)`, a, b).Scan(&ok)
	return ok, err
}

func (p *Postgres) blockedInTx(ctx context.Context, tx *sql.Tx, a, b string) (bool, error) {
	var ok bool
	err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)
)`, a, b).Scan(&ok)
	return ok, err
}
