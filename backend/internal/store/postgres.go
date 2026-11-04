package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Postgres is the durable Store backed by PostgreSQL.
type Postgres struct {
	db *sql.DB
}

// OpenPostgres connects, pings, and runs migrations.
func OpenPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Postgres{db: db}, nil
}

func (p *Postgres) Close() error {
	if p == nil || p.db == nil {
		return nil
	}
	return p.db.Close()
}

func (p *Postgres) DB() *sql.DB { return p.db }

var _ Store = (*Postgres)(nil)

func scanAccount(row interface {
	Scan(dest ...any) error
}) (*domain.Account, error) {
	var a domain.Account
	if err := row.Scan(&a.ID, &a.Nickname, &a.AppleSubject, &a.InviteCode, &a.CreatedAt); err != nil {
		return nil, err
	}
	a.CreatedAt = a.CreatedAt.UTC()
	return &a, nil
}

func (p *Postgres) UpsertByAppleSubject(appleSubject, nickname string) (*domain.Account, bool, error) {
	ctx := context.Background()
	var existing domain.Account
	err := p.db.QueryRowContext(ctx, `
SELECT id, nickname, apple_subject, invite_code, created_at
FROM accounts WHERE apple_subject=$1`, appleSubject).
		Scan(&existing.ID, &existing.Nickname, &existing.AppleSubject, &existing.InviteCode, &existing.CreatedAt)
	if err == nil {
		if nickname != "" && existing.Nickname != nickname {
			if len([]rune(nickname)) > 20 {
				return nil, false, fmt.Errorf("nickname too long")
			}
			if _, err := p.db.ExecContext(ctx, `UPDATE accounts SET nickname=$2 WHERE id=$1`, existing.ID, nickname); err != nil {
				return nil, false, err
			}
			existing.Nickname = nickname
		}
		existing.CreatedAt = existing.CreatedAt.UTC()
		return &existing, false, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, err
	}
	if nickname == "" {
		nickname = "用户"
	}
	if len([]rune(nickname)) > 20 {
		return nil, false, fmt.Errorf("nickname too long")
	}
	code, err := randomInviteCode()
	if err != nil {
		return nil, false, err
	}
	var a domain.Account
	err = p.db.QueryRowContext(ctx, `
INSERT INTO accounts (nickname, apple_subject, invite_code)
VALUES ($1,$2,$3)
RETURNING id, nickname, apple_subject, invite_code, created_at`, nickname, appleSubject, code).
		Scan(&a.ID, &a.Nickname, &a.AppleSubject, &a.InviteCode, &a.CreatedAt)
	if err != nil {
		return nil, false, err
	}
	a.CreatedAt = a.CreatedAt.UTC()
	return &a, true, nil
}

func (p *Postgres) GetByID(id string) (*domain.Account, error) {
	a, err := scanAccount(p.db.QueryRow(`
SELECT id, nickname, apple_subject, invite_code, created_at FROM accounts WHERE id=$1`, id))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
	}
	return a, err
}

func (p *Postgres) GetByInviteCode(code string) (*domain.Account, error) {
	a, err := scanAccount(p.db.QueryRow(`
SELECT id, nickname, apple_subject, invite_code, created_at FROM accounts WHERE invite_code=$1`, code))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid invite code")
	}
	return a, err
}

func (p *Postgres) UpdateNickname(id, nickname string) (*domain.Account, error) {
	n := len([]rune(nickname))
	if n < 1 || n > 20 {
		return nil, fmt.Errorf("nickname must be 1–20 characters")
	}
	res, err := p.db.Exec(`UPDATE accounts SET nickname=$2 WHERE id=$1`, id, nickname)
	if err != nil {
		return nil, err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil, fmt.Errorf("not found")
	}
	return p.GetByID(id)
}

func (p *Postgres) ResetInviteCode(id string) (*domain.Account, error) {
	code, err := randomInviteCode()
	if err != nil {
		return nil, err
	}
	res, err := p.db.Exec(`UPDATE accounts SET invite_code=$2 WHERE id=$1`, id, code)
	if err != nil {
		return nil, err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil, fmt.Errorf("not found")
	}
	return p.GetByID(id)
}

func (p *Postgres) IsBlockedEitherWay(a, b string) bool {
	var ok bool
	_ = p.db.QueryRow(`
SELECT EXISTS(
  SELECT 1 FROM blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)
)`, a, b).Scan(&ok)
	return ok
}

func (p *Postgres) AreFriends(a, b string) bool {
	var ok bool
	_ = p.db.QueryRow(`
SELECT EXISTS(
  SELECT 1 FROM friendships
  WHERE removed_at IS NULL
    AND ((account_a=$1 AND account_b=$2) OR (account_a=$2 AND account_b=$1))
)`, a, b).Scan(&ok)
	return ok
}

func (p *Postgres) Block(blockerID, blockedID string) error {
	if blockerID == blockedID {
		return fmt.Errorf("cannot block yourself")
	}
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, blockedID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("account not found")
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1,$2)
ON CONFLICT DO NOTHING`, blockerID, blockedID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE friendships SET removed_at=now()
WHERE removed_at IS NULL
  AND ((account_a=$1 AND account_b=$2) OR (account_a=$2 AND account_b=$1))`, blockerID, blockedID); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) ListFriends(accountID string) ([]FriendListItem, error) {
	ctx := context.Background()
	var exists bool
	if err := p.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, accountID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	rows, err := p.db.QueryContext(ctx, `
SELECT f.id,
       CASE WHEN f.account_a=$1 THEN f.account_b ELSE f.account_a END AS other_id,
       a.nickname,
       f.created_at
FROM friendships f
JOIN accounts a ON a.id = CASE WHEN f.account_a=$1 THEN f.account_b ELSE f.account_a END
WHERE f.removed_at IS NULL AND (f.account_a=$1 OR f.account_b=$1)
ORDER BY f.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FriendListItem{}
	for rows.Next() {
		var it FriendListItem
		if err := rows.Scan(&it.FriendshipID, &it.AccountID, &it.Nickname, &it.Since); err != nil {
			return nil, err
		}
		it.Since = it.Since.UTC()
		out = append(out, it)
	}
	return out, rows.Err()
}

func (p *Postgres) AddFriendByInvite(requesterID, inviteCode string) (FriendListItem, bool, error) {
	ctx := context.Background()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return FriendListItem{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var meID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1`, requesterID).Scan(&meID); err != nil {
		if err == sql.ErrNoRows {
			return FriendListItem{}, false, fmt.Errorf("not found")
		}
		return FriendListItem{}, false, err
	}
	var otherID, otherNick string
	err = tx.QueryRowContext(ctx, `SELECT id, nickname FROM accounts WHERE invite_code=$1`, inviteCode).
		Scan(&otherID, &otherNick)
	if err == sql.ErrNoRows {
		return FriendListItem{}, false, fmt.Errorf("invalid invite code")
	}
	if err != nil {
		return FriendListItem{}, false, err
	}
	if otherID == meID {
		return FriendListItem{}, false, fmt.Errorf("cannot add yourself")
	}
	var blocked bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)
)`, meID, otherID).Scan(&blocked); err != nil {
		return FriendListItem{}, false, err
	}
	if blocked {
		return FriendListItem{}, false, fmt.Errorf("blocked")
	}
	var fid string
	var since time.Time
	err = tx.QueryRowContext(ctx, `
SELECT id, created_at FROM friendships
WHERE removed_at IS NULL
  AND ((account_a=$1 AND account_b=$2) OR (account_a=$2 AND account_b=$1))`, meID, otherID).
		Scan(&fid, &since)
	if err == nil {
		return FriendListItem{FriendshipID: fid, AccountID: otherID, Nickname: otherNick, Since: since.UTC()}, true, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return FriendListItem{}, false, err
	}
	err = tx.QueryRowContext(ctx, `
INSERT INTO friendships (account_a, account_b) VALUES ($1,$2)
RETURNING id, created_at`, meID, otherID).Scan(&fid, &since)
	if err != nil {
		return FriendListItem{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return FriendListItem{}, false, err
	}
	return FriendListItem{FriendshipID: fid, AccountID: otherID, Nickname: otherNick, Since: since.UTC()}, false, nil
}

func (p *Postgres) RemoveFriendship(requesterID, friendshipID string) error {
	res, err := p.db.Exec(`
UPDATE friendships SET removed_at=now()
WHERE id=$1 AND removed_at IS NULL AND (account_a=$2 OR account_b=$2)`, friendshipID, requesterID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		// distinguish forbidden vs not found
		var ownerA, ownerB string
		var removed sql.NullTime
		err := p.db.QueryRow(`SELECT account_a, account_b, removed_at FROM friendships WHERE id=$1`, friendshipID).
			Scan(&ownerA, &ownerB, &removed)
		if err == sql.ErrNoRows || (err == nil && removed.Valid) {
			return fmt.Errorf("not found")
		}
		if err != nil {
			return err
		}
		if ownerA != requesterID && ownerB != requesterID {
			return fmt.Errorf("forbidden")
		}
		return fmt.Errorf("not found")
	}
	return nil
}
