package store

import "testing"

func TestAddFriendByInvite(t *testing.T) {
	m := NewMemory()
	a, _, err := m.UpsertByAppleSubject("sub-a", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := m.UpsertByAppleSubject("sub-b", "Bob")
	if err != nil {
		t.Fatal(err)
	}
	item, already, err := m.AddFriendByInvite(a.ID, b.InviteCode)
	if err != nil || already {
		t.Fatalf("err=%v already=%v", err, already)
	}
	if item.AccountID != b.ID {
		t.Fatalf("got %s", item.AccountID)
	}
	_, already, err = m.AddFriendByInvite(a.ID, b.InviteCode)
	if err != nil || !already {
		t.Fatalf("expected already friends")
	}
	if _, _, err := m.AddFriendByInvite(a.ID, a.InviteCode); err == nil {
		t.Fatal("cannot add self")
	}
}

func TestBlockPreventsAdd(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("sub-a", "Alice")
	b, _, _ := m.UpsertByAppleSubject("sub-b", "Bob")
	if err := m.Block(a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.AddFriendByInvite(a.ID, b.InviteCode); err == nil {
		t.Fatal("expected block")
	}
}
