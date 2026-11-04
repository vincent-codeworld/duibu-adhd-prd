package store

import (
	"testing"
	"time"
)

func TestMemoryInviteCodeFailures(t *testing.T) {
	m := NewMemory()
	acc, _, err := m.UpsertByAppleSubject("invite-fail-mem", "A")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	if n, err := m.CountInviteCodeFailures(acc.ID, now.Add(-time.Hour)); err != nil || n != 0 {
		t.Fatalf("want 0, got %d %v", n, err)
	}
	_ = m.RecordInviteCodeFailure(acc.ID, now)
	_ = m.RecordInviteCodeFailure(acc.ID, now.Add(time.Minute))
	n, err := m.CountInviteCodeFailures(acc.ID, now.Add(-time.Hour))
	if err != nil || n != 2 {
		t.Fatalf("want 2, got %d %v", n, err)
	}
	n, err = m.CountInviteCodeFailures(acc.ID, now.Add(30*time.Minute))
	if err != nil || n != 0 {
		t.Fatalf("window elapsed want 0, got %d %v", n, err)
	}
}
