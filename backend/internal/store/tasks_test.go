package store

import (
	"testing"
	"time"
)

func testDueAt() time.Time {
	return time.Now().UTC().Add(7 * 24 * time.Hour)
}

func TestCreateSyncedTaskRequiresConsent(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "A")
	_, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		Title: "写报告", UploadConsent: false, DueAt: testDueAt(),
		Stages: []StageInput{{Title: "1"}, {Title: "2"}, {Title: "3"}},
	})
	if err == nil {
		t.Fatal("expected consent")
	}
}

func TestCreateRequiresDueAt(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "A")
	_, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		Title: "写报告", UploadConsent: true,
		Stages: []StageInput{{Title: "1"}, {Title: "2"}, {Title: "3"}},
	})
	if err == nil || err.Error() != "dueAt required" {
		t.Fatalf("want dueAt required, got %v", err)
	}
}

func TestRemindAtMustBeBeforeOrEqualDueAt(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "A")
	due := time.Now().UTC().Add(24 * time.Hour)
	late := due.Add(time.Hour)
	_, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		Title: "写报告", UploadConsent: true, DueAt: due,
		Stages: []StageInput{
			{Title: "1", RemindAt: &late},
			{Title: "2"},
			{Title: "3"},
		},
	})
	if err == nil || err.Error() != "stage 1 remindAt must be ≤ dueAt" {
		t.Fatalf("want remindAt validation, got %v", err)
	}
	okRemind := due.Add(-time.Hour)
	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		Title: "写报告", UploadConsent: true, DueAt: due,
		Stages: []StageInput{
			{Title: "1", RemindAt: &okRemind},
			{Title: "2"},
			{Title: "3"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tv.DueAt.UTC() != due.UTC() {
		t.Fatalf("dueAt=%v", tv.DueAt)
	}
	if tv.Stages[0].RemindAt == nil || !tv.Stages[0].RemindAt.UTC().Equal(okRemind.UTC()) {
		t.Fatalf("remindAt=%v want %v", tv.Stages[0].RemindAt, okRemind)
	}
}

func TestCompleteTaskRules(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "A")
	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		Title: "写报告", UploadConsent: true, DueAt: testDueAt(),
		Stages: []StageInput{
			{Title: "一", Progress: "done"},
			{Title: "二", Progress: "skipped"},
			{Title: "三", Progress: "pending"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompleteTask(a.ID, tv.ID); err == nil {
		t.Fatal("pending should block")
	}
	tv2, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		Title: "另一件", UploadConsent: true, DueAt: testDueAt(),
		Stages: []StageInput{
			{Title: "一", Progress: "done"},
			{Title: "二", Progress: "skipped"},
			{Title: "三", Progress: "skipped"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompleteTask(a.ID, tv2.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteTaskRequiresVerificationWhenVerifierSet(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "Alice")
	b, _, _ := m.UpsertByAppleSubject("b", "Bob")
	_, _, _ = m.AddFriendByInvite(a.ID, b.InviteCode)
	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		Title: "有验收人", UploadConsent: true, DueAt: testDueAt(),
		Stages: []StageInput{
			{Title: "一", Progress: "done"},
			{Title: "二", Progress: "skipped"},
			{Title: "三", Progress: "skipped"},
		},
		DesignatedVerifierAccountID: &b.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tv.CompletionReward == nil {
		t.Fatal("expected auto task_completion gate reward")
	}
	if _, err := m.CompleteTask(a.ID, tv.ID); err == nil || err.Error() != "verification required" {
		t.Fatalf("want verification required, got %v", err)
	}
	item, err := m.SubmitVerification(a.ID, tv.CompletionReward.ID, SubmitVerificationInput{Note: "请验收完成"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Approve(b.ID, item.RequestID); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetSyncedTask(a.ID, tv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "completed" {
		t.Fatalf("status=%s", got.Status)
	}
	if got.CompletedAt == nil {
		t.Fatal("completedAt missing")
	}
}

func TestPatchStageProgress(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "A")
	b, _, _ := m.UpsertByAppleSubject("b", "B")
	_, _, _ = m.AddFriendByInvite(a.ID, b.InviteCode)
	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		Title: "写报告", UploadConsent: true, DueAt: testDueAt(),
		Stages:                      []StageInput{{Title: "一"}, {Title: "二"}, {Title: "三"}},
		StageRewards:                map[string]RewardInput{"1": {Text: "剧"}},
		DesignatedVerifierAccountID: &b.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	patched, err := m.PatchSyncedTask(a.ID, tv.ID, PatchTaskInput{
		Stages: []StageInput{
			{Title: "一", Progress: "done"},
			{Title: "二", Progress: "pending"},
			{Title: "三", Progress: "pending"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if patched.Stages[0].Progress != "done" {
		t.Fatalf("got %s", patched.Stages[0].Progress)
	}
	if _, err := m.SubmitVerification(a.ID, patched.Stages[0].Reward.ID, SubmitVerificationInput{Note: "ok"}); err != nil {
		t.Fatal(err)
	}
	_, err = m.PatchSyncedTask(a.ID, tv.ID, PatchTaskInput{
		Stages: []StageInput{
			{Title: "一", Progress: "pending"},
			{Title: "二", Progress: "pending"},
			{Title: "三", Progress: "pending"},
		},
	})
	if err == nil {
		t.Fatal("expected lock on uncheck while pending verification")
	}
}
