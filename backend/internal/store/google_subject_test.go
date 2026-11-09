package store

import "testing"

func TestGoogleSubjectDoesNotLinkApple(t *testing.T) {
	m := NewMemory()
	apple, created, err := m.UpsertByAppleSubject("same-sub", "AppleUser")
	if err != nil || !created {
		t.Fatalf("apple: created=%v err=%v", created, err)
	}
	google, created, err := m.UpsertByGoogleSubject("same-sub", "GoogleUser")
	if err != nil || !created {
		t.Fatalf("google: created=%v err=%v", created, err)
	}
	if apple.ID == google.ID {
		t.Fatal("same subject string linked Apple and Google accounts")
	}
	if apple.GoogleSubject != "" || google.AppleSubject != "" {
		t.Fatalf("subjects crossed: apple=%+v google=%+v", apple, google)
	}
	if google.GoogleSubject != "same-sub" || apple.AppleSubject != "same-sub" {
		t.Fatalf("provider subject not stored: apple=%+v google=%+v", apple, google)
	}

	// An email-shaped Apple subject must not become the Google account.
	appleEmail, _, err := m.UpsertByAppleSubject("ada@example.com", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	byEmailSub, created, err := m.UpsertByGoogleSubject("google-sub-ada", "Ada Google")
	if err != nil || !created {
		t.Fatalf("google email case: created=%v err=%v", created, err)
	}
	if appleEmail.ID == byEmailSub.ID {
		t.Fatal("linked Google account to Apple account that used an email-like subject")
	}

	again, created, err := m.UpsertByGoogleSubject("same-sub", "Renamed")
	if err != nil || created {
		t.Fatalf("relogin: created=%v err=%v", created, err)
	}
	if again.ID != google.ID || again.Nickname != "Renamed" || again.AppleSubject != "" {
		t.Fatalf("relogin account=%+v", again)
	}
	got, err := m.GetByID(google.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GoogleSubject != "same-sub" || got.AppleSubject != "" || got.Nickname != "Renamed" {
		t.Fatalf("stored=%+v", got)
	}

	if _, _, err := m.UpsertByGoogleSubject("   ", "x"); err == nil {
		t.Fatal("blank google subject should fail")
	}
	if _, _, err := m.UpsertByGoogleSubject("ok", "nickname-longer-than-20"); err == nil {
		t.Fatal("long nickname should fail")
	}
}
