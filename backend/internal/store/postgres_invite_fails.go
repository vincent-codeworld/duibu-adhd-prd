package store

import (
	"context"
	"time"
)

func (p *Postgres) RecordInviteCodeFailure(accountID string, at time.Time) error {
	ctx := context.Background()
	_, err := p.db.ExecContext(ctx, `
INSERT INTO invite_code_failures (account_id, failed_at)
VALUES ($1, $2)`, accountID, at.UTC())
	return err
}

func (p *Postgres) CountInviteCodeFailures(accountID string, since time.Time) (int, error) {
	ctx := context.Background()
	var n int
	err := p.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM invite_code_failures
WHERE account_id=$1 AND failed_at > $2`, accountID, since.UTC()).Scan(&n)
	return n, err
}
