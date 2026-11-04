package store

import (
	"bytes"
	"testing"

	"github.com/vincent-codeworld/duibu-adhd/backend/internal/upload"
)

func TestScreenshotUploadAttachAndAuth(t *testing.T) {
	m := NewMemory()
	a, _, _ := m.UpsertByAppleSubject("a", "Alice")
	b, _, _ := m.UpsertByAppleSubject("b", "Bob")
	c, _, _ := m.UpsertByAppleSubject("c", "Carol")
	_, _, _ = m.AddFriendByInvite(a.ID, b.InviteCode)

	dir := t.TempDir()
	disk, err := upload.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 9, 9}
	key, ct, n, err := disk.SaveValidated(bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterScreenshotUpload(a.ID, key, ct, n); err != nil {
		t.Fatal(err)
	}

	// uploader can fetch before attach
	if _, err := m.CanFetchScreenshot(a.ID, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CanFetchScreenshot(b.ID, key); err == nil || err.Error() != "forbidden" {
		t.Fatalf("want forbidden for verifier before attach, got %v", err)
	}

	tv, err := m.CreateSyncedTask(a.ID, CreateTaskInput{
		DueAt: testDueAt(),
		Title: "报告", Notes: "secret", UploadConsent: true,
		Stages:                      []StageInput{{Title: "一步", Progress: "done"}, {Title: "二"}, {Title: "三"}},
		StageRewards:                map[string]RewardInput{"1": {Text: "奶茶"}},
		DesignatedVerifierAccountID: &b.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	rewardID := tv.Stages[0].Reward.ID
	item, err := m.SubmitVerification(a.ID, rewardID, SubmitVerificationInput{Note: "done", ScreenshotKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if item.ScreenshotKey != key || item.ScreenshotURL != "/v1/screenshots/"+key {
		t.Fatalf("%+v", item)
	}

	// after attach: requester + verifier ok, stranger forbidden
	if _, err := m.CanFetchScreenshot(a.ID, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CanFetchScreenshot(b.ID, key); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CanFetchScreenshot(c.ID, key); err == nil || err.Error() != "forbidden" {
		t.Fatalf("stranger: %v", err)
	}

	// reuse key rejected
	if _, err := m.SubmitVerification(a.ID, rewardID, SubmitVerificationInput{ScreenshotKey: key}); err == nil {
		t.Fatal("expected already attached / cannot submit")
	}

	// missing key
	if _, err := m.SubmitVerification(a.ID, rewardID, SubmitVerificationInput{ScreenshotKey: "00000000-0000-4000-8000-000000000099"}); err == nil {
		t.Fatal("expected screenshot not found")
	}
}

func TestScreenshotOversizeRejectedByDisk(t *testing.T) {
	disk, err := upload.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	huge := make([]byte, upload.MaxBytes+10)
	huge[0], huge[1], huge[2] = 0xff, 0xd8, 0xff
	if _, _, _, err := disk.SaveValidated(bytes.NewReader(huge)); err == nil {
		t.Fatal("expected too large")
	}
}
