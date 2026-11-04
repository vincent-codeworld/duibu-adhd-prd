package auth

import (
	"sync"
	"time"
)

// InviteFailLog persists (or holds) wrong-invite timestamps for rate limiting.
// Postgres store implements this with invite_code_failures; memory keeps an in-process log.
type InviteFailLog interface {
	CountInviteCodeFailures(accountID string, since time.Time) (int, error)
	RecordInviteCodeFailure(accountID string, at time.Time) error
}

// InviteFailLimiter rate-limits wrong invite-code attempts per account.
type InviteFailLimiter struct {
	maxPerHour int
	log        InviteFailLog
}

func NewInviteFailLimiter(maxPerHour int, log InviteFailLog) *InviteFailLimiter {
	if maxPerHour <= 0 {
		maxPerHour = 10
	}
	if log == nil {
		log = NewMemoryInviteFailLog()
	}
	return &InviteFailLimiter{maxPerHour: maxPerHour, log: log}
}

func (l *InviteFailLimiter) Allowed(accountID string, now time.Time) bool {
	n, err := l.log.CountInviteCodeFailures(accountID, now.Add(-time.Hour))
	if err != nil {
		// Fail open on read errors so a transient DB blip does not lock everyone out.
		return true
	}
	return n < l.maxPerHour
}

func (l *InviteFailLimiter) RecordFail(accountID string, now time.Time) {
	_ = l.log.RecordInviteCodeFailure(accountID, now)
}

// MemoryInviteFailLog is the in-process backend used by unit tests (and as nil-log fallback).
type MemoryInviteFailLog struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

func NewMemoryInviteFailLog() *MemoryInviteFailLog {
	return &MemoryInviteFailLog{fails: map[string][]time.Time{}}
}

func (m *MemoryInviteFailLog) CountInviteCodeFailures(accountID string, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.fails[accountID][:0]
	for _, ts := range m.fails[accountID] {
		if ts.After(since) {
			kept = append(kept, ts)
		}
	}
	if len(kept) == 0 {
		delete(m.fails, accountID)
		return 0, nil
	}
	m.fails[accountID] = kept
	return len(kept), nil
}

func (m *MemoryInviteFailLog) RecordInviteCodeFailure(accountID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := at.Add(-time.Hour)
	kept := m.fails[accountID][:0]
	for _, ts := range m.fails[accountID] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	m.fails[accountID] = append(kept, at)
	return nil
}
