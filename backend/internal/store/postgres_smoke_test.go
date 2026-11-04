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
