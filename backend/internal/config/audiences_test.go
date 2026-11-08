package config

import "testing"

func TestGoogleAudiencesTrimsAndDedupes(t *testing.T) {
	c := Config{
		GoogleClientID:  " ios.apps.googleusercontent.com ",
		GoogleClientIDs: "ios.apps.googleusercontent.com, web.apps.googleusercontent.com, ,web.apps.googleusercontent.com",
	}
	got := c.GoogleAudiences()
	if len(got) != 2 || got[0] != "ios.apps.googleusercontent.com" || got[1] != "web.apps.googleusercontent.com" {
		t.Fatalf("audiences=%q", got)
	}
	if len(Config{}.GoogleAudiences()) != 0 {
		t.Fatal("empty config should have no Google audiences")
	}
}
