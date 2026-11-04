package auth

import (
	"testing"
	"time"
)

func TestInviteFailLimiter(t *testing.T) {
	l := NewInviteFailLimiter(2, NewMemoryInviteFailLog())
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	id := "acc1"
	if !l.Allowed(id, now) {
		t.Fatal("should allow")
	}
	l.RecordFail(id, now)
	l.RecordFail(id, now.Add(time.Minute))
	if l.Allowed(id, now.Add(2*time.Minute)) {
		t.Fatal("should block after max fails")
	}
	if !l.Allowed(id, now.Add(61*time.Minute)) {
		t.Fatal("should allow after window")
	}
}

func TestInviteFailLimiterNilLogDefaultsToMemory(t *testing.T) {
	l := NewInviteFailLimiter(1, nil)
	now := time.Now().UTC()
	if !l.Allowed("x", now) {
		t.Fatal("should allow")
	}
	l.RecordFail("x", now)
	if l.Allowed("x", now.Add(time.Second)) {
		t.Fatal("should block")
	}
}
