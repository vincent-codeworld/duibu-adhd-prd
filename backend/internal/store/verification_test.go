package store

import (
	"testing"
)

func TestVerificationInboxOmitsNotes(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "Alice")
	b, _, _ := m.UpsertByAppleSubject("b", "Bob")
	_, _, _ = m.AddFriendByInvite(a.ID, b.InviteCode)
	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		DueAt: testDueAt(),
		Title: "报告", Notes: "秘密备注不要给验收人", UploadConsent: true,
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
	inbox, err := m.ListInbox(b.ID)
	if err != nil || len(inbox) != 1 {
		t.Fatalf("inbox=%v err=%v", inbox, err)
	}
	if inbox[0].TaskTitle != "报告" || inbox[0].Note != "写完了" {
		t.Fatalf("%+v", inbox[0])
	}
	// ensure notes not present on struct JSON side — field doesn't exist
	_ = item
	if err := m.Reject(b.ID, inbox[0].RequestID, "看不清"); err != nil {
		t.Fatal(err)
	}
	// stage progress unchanged: still done via task view
	got, _ := m.GetSyncedTask(a.ID, tv.ID)
	if got.Stages[0].Progress != "done" {
		t.Fatalf("progress rolled back: %s", got.Stages[0].Progress)
	}
	if got.Stages[0].Reward.RedeemState != "rejected" {
		t.Fatalf("redeem=%s", got.Stages[0].Reward.RedeemState)
	}
}
