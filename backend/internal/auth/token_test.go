package auth

import (
	"testing"
	"time"
)

func TestIssueAndParseToken(t *testing.T) {
	tok, _, err := IssueToken("secret", "acc-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseToken("secret", tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.AccountID != "acc-1" {
		t.Fatalf("got %s", c.AccountID)
	}
}
