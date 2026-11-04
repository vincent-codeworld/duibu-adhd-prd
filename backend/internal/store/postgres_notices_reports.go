package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

func (p *Postgres) ListNotices(accountID string) ([]NoticeView, error) {
	var exists bool
	if err := p.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, accountID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	rows, err := p.db.Query(`
SELECT id, kind, request_id, created_at, read_at
FROM in_app_notices WHERE account_id=$1
ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NoticeView, 0)
	for rows.Next() {
		var n NoticeView
		var readAt sql.NullTime
		if err := rows.Scan(&n.ID, &n.Kind, &n.RequestID, &n.CreatedAt, &readAt); err != nil {
			return nil, err
		}
		n.CreatedAt = n.CreatedAt.UTC()
		if readAt.Valid {
			t := readAt.Time.UTC()
			n.ReadAt = &t
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (p *Postgres) MarkNoticeRead(accountID, noticeID string) (*NoticeView, error) {
	res, err := p.db.Exec(`
UPDATE in_app_notices SET read_at=COALESCE(read_at, now())
WHERE id=$1 AND account_id=$2`, noticeID, accountID)
	if err != nil {
		return nil, err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil, fmt.Errorf("not found")
	}
	var n NoticeView
	var readAt sql.NullTime
	err = p.db.QueryRow(`
SELECT id, kind, request_id, created_at, read_at FROM in_app_notices WHERE id=$1`, noticeID).
		Scan(&n.ID, &n.Kind, &n.RequestID, &n.CreatedAt, &readAt)
	if err != nil {
		return nil, err
	}
	n.CreatedAt = n.CreatedAt.UTC()
	if readAt.Valid {
		t := readAt.Time.UTC()
		n.ReadAt = &t
	}
	return &n, nil
}

func (p *Postgres) CreateReport(reporterID string, in CreateReportInput) (*ReportView, error) {
	reason, ok := domain.ParseReportReason(strings.TrimSpace(in.Reason))
	if !ok {
		return nil, fmt.Errorf("invalid reason")
	}
	requestID := strings.TrimSpace(in.RequestID)
	if requestID == "" {
		return nil, fmt.Errorf("requestId required")
	}
	detail := strings.TrimSpace(in.Detail)
	if utf8.RuneCountInString(detail) > maxReportDetailRunes {
		return nil, fmt.Errorf("detail too long")
	}
	var exists bool
	if err := p.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, reporterID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	var reqAcc, verAcc string
	err := p.db.QueryRow(`
SELECT requester_account_id, verifier_account_id FROM verification_requests WHERE id=$1`, requestID).
		Scan(&reqAcc, &verAcc)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	if reporterID != reqAcc && reporterID != verAcc {
		return nil, fmt.Errorf("forbidden")
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	if _, err := p.db.Exec(`
INSERT INTO reports (id, reporter_account_id, request_id, reason, detail, created_at)
VALUES ($1,$2,$3,$4,$5,$6)`, id, reporterID, requestID, string(reason), detail, now); err != nil {
		return nil, err
	}
	return &ReportView{
		ID:        id,
		RequestID: requestID,
		Reason:    string(reason),
		Detail:    detail,
		CreatedAt: now,
		Message:   "已收到",
	}, nil
}
