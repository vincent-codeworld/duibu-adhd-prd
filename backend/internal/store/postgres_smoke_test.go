package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresPersistsTask(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://duibu:duibu@localhost:5432/duibu?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer p.Close()

	subj := "pg-smoke-" + time.Now().UTC().Format("150405.000")
	acc, _, err := p.UpsertByAppleSubject(subj, "Persist")
	if err != nil {
		t.Fatal(err)
	}
	tv, err := p.CreateSyncedTask(acc.ID, CreateTaskInput{
		DueAt: testDueAt(),
		Title: "重启仍在", Notes: "", UploadConsent: true,
		Stages: []StageInput{{Title: "一"}, {Title: "二"}, {Title: "三"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	taskID := tv.ID

	// reopen connection to simulate process restart (same DB)
	_ = p.Close()
	p2, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	got, err := p2.GetSyncedTask(acc.ID, taskID)
	if err != nil {
		t.Fatalf("task missing after reopen: %v", err)
	}
	if got.Title != "重启仍在" || len(got.Stages) != 3 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestPostgresPersistsInviteCodeFailures(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://duibu:duibu@localhost:5432/duibu?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer p.Close()

	subj := "pg-invite-fail-" + time.Now().UTC().Format("150405.000")
	acc, _, err := p.UpsertByAppleSubject(subj, "Failer")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := p.RecordInviteCodeFailure(acc.ID, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	_ = p.Close()
	p2, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	n, err := p2.CountInviteCodeFailures(acc.ID, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("want 3 failures after reopen, got %d", n)
	}
}

func TestPostgresGoogleSubjectSeparateFromApple(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://duibu:duibu@localhost:5432/duibu?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer p.Close()

	sub := "pg-google-" + time.Now().UTC().Format("150405.000000")
	apple, appleCreated, err := p.UpsertByAppleSubject(sub, "Apple")
	if err != nil || !appleCreated {
		t.Fatalf("apple created=%v err=%v", appleCreated, err)
	}
	google, googleCreated, err := p.UpsertByGoogleSubject(sub, "Google")
	if err != nil || !googleCreated {
		t.Fatalf("google created=%v err=%v", googleCreated, err)
	}
	if apple.ID == google.ID {
		t.Fatal("postgres linked Apple and Google on the same subject string")
	}
	if apple.GoogleSubject != "" || google.AppleSubject != "" || google.GoogleSubject != sub {
		t.Fatalf("subjects apple=%+v google=%+v", apple, google)
	}
	again, created, err := p.UpsertByGoogleSubject(sub, "")
	if err != nil || created || again.ID != google.ID {
		t.Fatalf("relogin created=%v id=%s err=%v", created, again.ID, err)
	}
	if _, _, err := p.UpsertByGoogleSubject("  ", "x"); err == nil {
		t.Fatal("blank google subject should fail")
	}

	_ = p.Close()
	p2, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	got, err := p2.GetByID(google.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GoogleSubject != sub || got.AppleSubject != "" || got.ID == apple.ID {
		t.Fatalf("reopen got=%+v apple=%s", got, apple.ID)
	}
	appleGot, err := p2.GetByID(apple.ID)
	if err != nil {
		t.Fatal(err)
	}
	if appleGot.AppleSubject != sub || appleGot.GoogleSubject != "" {
		t.Fatalf("apple reopen=%+v", appleGot)
	}
}
