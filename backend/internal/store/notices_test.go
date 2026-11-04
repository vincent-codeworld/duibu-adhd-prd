package store

import (
	"testing"
)

func TestNoticesFromVerificationEvents(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "Alice")
	b, _, _ := m.UpsertByAppleSubject("b", "Bob")
	_, _, _ = m.AddFriendByInvite(a.ID, b.InviteCode)
	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		DueAt: testDueAt(),
		Title: "报告", Notes: "秘密", UploadConsent: true,
		Stages:                      []StageInput{{Title: "一步", Progress: "done"}, {Title: "二"}, {Title: "三"}},
		StageRewards:                map[string]RewardInput{"1": {Text: "奶茶"}},
		DesignatedVerifierAccountID: &b.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rewardID := tv.Stages[0].Reward.ID
	item, err := m.SubmitVerification(a.ID, rewardID, SubmitVerificationInput{Note: "写完了"})
	if err != nil {
		t.Fatal(err)
	}

	bobNotices, err := m.ListNotices(b.ID)
	if err != nil || len(bobNotices) != 1 || bobNotices[0].Kind != "verification_pending" {
		t.Fatalf("bob notices=%+v err=%v", bobNotices, err)
	}
	if bobNotices[0].RequestID != item.RequestID {
		t.Fatalf("requestId mismatch")
	}
	aliceNotices, _ := m.ListNotices(a.ID)
	if len(aliceNotices) != 0 {
		t.Fatalf("alice should have none yet: %+v", aliceNotices)
	}

	if err := m.Approve(b.ID, item.RequestID); err != nil {
		t.Fatal(err)
	}
	aliceNotices, err = m.ListNotices(a.ID)
	if err != nil || len(aliceNotices) != 1 || aliceNotices[0].Kind != "verification_approved" {
		t.Fatalf("alice after approve=%+v err=%v", aliceNotices, err)
	}
	got, err := m.MarkNoticeRead(a.ID, aliceNotices[0].ID)
	if err != nil || got.ReadAt == nil {
		t.Fatalf("mark read=%+v err=%v", got, err)
	}
	// idempotent
	got2, err := m.MarkNoticeRead(a.ID, aliceNotices[0].ID)
	if err != nil || got2.ReadAt == nil {
		t.Fatal(err)
	}
	if _, err := m.MarkNoticeRead(b.ID, aliceNotices[0].ID); err == nil {
		t.Fatal("other user should not mark foreign notice")
	}
}

func TestNoticeWithdrawnToVerifier(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a2", "Alice")
	b, _, _ := m.UpsertByAppleSubject("b2", "Bob")
	_, _, _ = m.AddFriendByInvite(a.ID, b.InviteCode)
	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		DueAt: testDueAt(),
		Title: "寄材料", Notes: "", UploadConsent: true,
		Stages:                      []StageInput{{Title: "一步", Progress: "done"}, {Title: "二"}, {Title: "三"}},
		StageRewards:                map[string]RewardInput{"1": {Text: "咖啡"}},
		DesignatedVerifierAccountID: &b.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := m.SubmitVerification(a.ID, tv.Stages[0].Reward.ID, SubmitVerificationInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Withdraw(a.ID, item.RequestID); err != nil {
		t.Fatal(err)
	}
	bob, err := m.ListNotices(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, n := range bob {
		kinds[n.Kind]++
	}
	if kinds["verification_pending"] != 1 || kinds["verification_withdrawn"] != 1 {
		t.Fatalf("kinds=%v", kinds)
	}
}
