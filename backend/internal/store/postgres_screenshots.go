package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (p *Postgres) RegisterScreenshotUpload(uploaderID, key, contentType string, byteSize int) error {
	key = strings.TrimSpace(key)
	if key == "" || uploaderID == "" || byteSize <= 0 {
		return fmt.Errorf("invalid upload")
	}
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return fmt.Errorf("unsupported image type")
	}
	ctx := context.Background()
	var exists bool
	if err := p.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, uploaderID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("not found")
	}
	_, err := p.db.ExecContext(ctx, `
INSERT INTO screenshot_uploads (key, uploader_account_id, content_type, byte_size, created_at)
VALUES ($1,$2,$3,$4,now())`, key, uploaderID, contentType, byteSize)
	if err != nil {
		return fmt.Errorf("conflict")
	}
	return nil
}

func (p *Postgres) GetScreenshotMeta(key string) (*ScreenshotUploadMeta, error) {
	return p.loadScreenshotMeta(context.Background(), strings.TrimSpace(key))
}

func (p *Postgres) loadScreenshotMeta(ctx context.Context, key string) (*ScreenshotUploadMeta, error) {
	var meta ScreenshotUploadMeta
	var attached sql.NullString
	var created time.Time
	err := p.db.QueryRowContext(ctx, `
SELECT key, uploader_account_id, content_type, byte_size, created_at, attached_request_id
FROM screenshot_uploads WHERE key=$1`, key).
		Scan(&meta.Key, &meta.UploaderAccountID, &meta.ContentType, &meta.ByteSize, &created, &attached)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	meta.CreatedAt = created.UTC()
	if attached.Valid {
		s := attached.String
		meta.AttachedRequestID = &s
		_ = p.db.QueryRowContext(ctx, `
SELECT requester_account_id, verifier_account_id FROM verification_requests WHERE id=$1`, s).
			Scan(&meta.RequesterAccountID, &meta.VerifierAccountID)
	}
	return &meta, nil
}

func (p *Postgres) CanFetchScreenshot(viewerID, key string) (*ScreenshotUploadMeta, error) {
	meta, err := p.loadScreenshotMeta(context.Background(), strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}
	if meta.AttachedRequestID == nil {
		if meta.UploaderAccountID != viewerID {
			return nil, fmt.Errorf("forbidden")
		}
		return meta, nil
	}
	if viewerID == meta.RequesterAccountID || viewerID == meta.VerifierAccountID {
		return meta, nil
	}
	return nil, fmt.Errorf("forbidden")
}

func (p *Postgres) ScreenshotKeysForTask(ownerID, taskID string) ([]string, error) {
	ctx := context.Background()
	var owner string
	err := p.db.QueryRowContext(ctx, `SELECT owner_account_id FROM tasks WHERE id=$1`, taskID).Scan(&owner)
	if err == sql.ErrNoRows || (err == nil && owner != ownerID) {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	rows, err := p.db.QueryContext(ctx, `
SELECT DISTINCT sc.storage_key
FROM screenshots sc
JOIN verification_requests v ON v.id = sc.request_id
JOIN rewards r ON r.id = v.reward_id
WHERE r.task_id=$1`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (p *Postgres) attachScreenshotTx(ctx context.Context, tx *sql.Tx, uploaderID, key, requestID string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	var uploader string
	var attached sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT uploader_account_id, attached_request_id FROM screenshot_uploads WHERE key=$1 FOR UPDATE`, key).
		Scan(&uploader, &attached)
	if err == sql.ErrNoRows {
		return fmt.Errorf("screenshot not found")
	}
	if err != nil {
		return err
	}
	if uploader != uploaderID {
		return fmt.Errorf("screenshot not owned")
	}
	if attached.Valid {
		return fmt.Errorf("screenshot already attached")
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE screenshot_uploads SET attached_request_id=$2 WHERE key=$1`, key, requestID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO screenshots (request_id, storage_key) VALUES ($1,$2)`, requestID, key); err != nil {
		return err
	}
	return nil
}
