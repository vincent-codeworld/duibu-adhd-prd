package store

import (
	"testing"
)

func TestCreateReportThin(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("rep-a", "Alice")
	b, _, _ := m.UpsertByAppleSubject("rep-b", "Bob")
	c, _, _ := m.UpsertByAppleSubject("rep-c", "Carol")
	_, _, _ = m.AddFriendByInvite(a.ID, b.InviteCode)
	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		DueAt: testDueAt(),
		Title: "寄材料", Notes: "秘密备注", UploadConsent: true,
		Stages:                      []StageInput{{Title: "一步", Progress: "done"}, {Title: "二"}, {Title: "三"}},
		StageRewards:                map[string]RewardInput{"1": {Text: "奶茶"}},
		DesignatedVerifierAccountID: &b.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := m.SubmitVerification(a.ID, tv.Stages[0].Reward.ID, SubmitVerificationInput{Note: "截图说明"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := m.CreateReport(b.ID, CreateReportInput{
		RequestID: item.RequestID,
		Reason:    "harassment",
		Detail:    "备注不当",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Message != "已收到" || got.RequestID != item.RequestID || got.Reason != "harassment" || got.Detail != "备注不当" {
		t.Fatalf("receipt=%+v", got)
	}
	if got.ID == "" {
		t.Fatal("missing report id")
	}

	// stranger cannot report
	if _, err := m.CreateReport(c.ID, CreateReportInput{RequestID: item.RequestID, Reason: "other"}); err == nil {
		t.Fatal("expected forbidden for stranger")
	}
	// bad reason
	if _, err := m.CreateReport(a.ID, CreateReportInput{RequestID: item.RequestID, Reason: "spam"}); err == nil {
		t.Fatal("expected invalid reason")
	}
	// missing request
	if _, err := m.CreateReport(a.ID, CreateReportInput{RequestID: "nope", Reason: "privacy"}); err == nil {
		t.Fatal("expected not found")
	}
	// progress untouched: reward still pending, stage still done
	tv2, err := m.GetSyncedTask(a.ID, tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tv2.Stages[0].Progress != "done" {
		t.Fatalf("progress changed: %s", tv2.Stages[0].Progress)
	}
	if tv2.Stages[0].Reward == nil || tv2.Stages[0].Reward.RedeemState != "pending" {
		t.Fatalf("reward changed: %+v", tv2.Stages[0].Reward)
	}
}
