package handlers

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/auth"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/config"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/store"
)

func TestAuthGoogleStubSessionSeparateFromApple(t *testing.T) {
	h := New(config.Config{AuthMode: "stub", JWTSecret: "test-secret"}, nil, store.NewMemory(), nil)

	first := postIdentity(t, h.AuthGoogle, "same-person", "Google")
	if !first.Created || first.AccessToken == "" || first.Account.ID == "" || first.Account.Nickname != "Google" || first.Account.InviteCode == "" || first.Account.CreatedAt.IsZero() || first.ExpiresAt.IsZero() {
		t.Fatalf("session=%+v", first)
	}
	claims, err := auth.ParseToken("test-secret", first.AccessToken)
	if err != nil || claims.AccountID != first.Account.ID {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}

	apple := postIdentity(t, h.AuthApple, "same-person", "Apple")
	if !apple.Created || apple.Account.ID == first.Account.ID {
		t.Fatalf("apple linked to google: apple=%s google=%s", apple.Account.ID, first.Account.ID)
	}

	again := postIdentity(t, h.AuthGoogle, "same-person", "")
	if again.Created || again.Account.ID != first.Account.ID || again.Account.Nickname != "Google" {
		t.Fatalf("relogin=%+v", again)
	}

	def := postIdentity(t, h.AuthGoogle, "brand-new-google", "")
	if !def.Created || def.Account.Nickname != "用户" {
		t.Fatalf("default nick=%+v", def.Account)
	}

	rr := callIdentity(h.AuthGoogle, "", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty token: %d %s", rr.Code, rr.Body.String())
	}
}

func TestAuthGoogleFailClosedUnlessTokenVerified(t *testing.T) {
	mem := store.NewMemory()
	unconfigured := New(config.Config{AuthMode: "apple", JWTSecret: "test-secret"}, nil, mem, nil)
	rr := callIdentity(unconfigured.AuthGoogle, "not-a-real-token", "Ada")
	if rr.Code != http.StatusServiceUnavailable || !bytes.Contains(rr.Body.Bytes(), []byte("google_auth_not_configured")) {
		t.Fatalf("fail closed: %d %s", rr.Code, rr.Body.String())
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	h := New(config.Config{
		AuthMode:       "apple",
		JWTSecret:      "test-secret",
		GoogleClientID: "ios-client",
	}, nil, mem, nil)
	h.googleVerifier.Keys = auth.StaticKeySource{"kid": &priv.PublicKey}
	h.googleVerifier.Now = func() time.Time { return now }

	raw := callIdentity(h.AuthGoogle, "google-alice", "Ada")
	if raw.Code != http.StatusUnauthorized || !bytes.Contains(raw.Body.Bytes(), []byte("invalid_google_token")) {
		t.Fatalf("raw subject in real mode: %d %s", raw.Code, raw.Body.String())
	}

	badAud := signGoogleID(t, priv, jwt.MapClaims{
		"sub": "fresh-sub", "aud": "other-client", "iss": auth.GoogleIssuer,
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
		"email": "ada@example.com", "email_verified": true,
	})
	bad := callIdentity(h.AuthGoogle, badAud, "Ada")
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad aud: %d %s", bad.Code, bad.Body.String())
	}

	apple, _, err := mem.UpsertByAppleSubject("ada@example.com", "Ada Apple")
	if err != nil {
		t.Fatal(err)
	}
	good := signGoogleID(t, priv, jwt.MapClaims{
		"sub": "fresh-sub", "aud": "ios-client", "azp": "ios-client", "iss": auth.GoogleIssuer,
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
		"email": "ada@example.com", "email_verified": true,
	})
	ok := decodeAuth(t, callIdentity(h.AuthGoogle, good, "Ada Google"))
	if !ok.Created || ok.Account.ID == apple.ID || ok.Account.Nickname != "Ada Google" {
		t.Fatalf("verified session linked or not created: %+v apple=%s", ok, apple.ID)
	}

	expired := signGoogleID(t, priv, jwt.MapClaims{
		"sub": "fresh-sub", "aud": "ios-client", "iss": auth.GoogleIssuer,
		"iat": now.Add(-2 * time.Hour).Unix(), "exp": now.Add(-time.Hour).Unix(),
	})
	if rr := callIdentity(h.AuthGoogle, expired, ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("expired: %d %s", rr.Code, rr.Body.String())
	}

	wrongIss := signGoogleID(t, priv, jwt.MapClaims{
		"sub": "other-sub", "aud": "ios-client", "iss": "https://appleid.apple.com",
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	if rr := callIdentity(h.AuthGoogle, wrongIss, ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("issuer: %d %s", rr.Code, rr.Body.String())
	}

	again := decodeAuth(t, callIdentity(h.AuthGoogle, good, ""))
	if again.Created || again.Account.ID != ok.Account.ID {
		t.Fatalf("second verify=%+v", again)
	}
}

type authBody struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Created     bool      `json:"created"`
	Account     struct {
		ID         string    `json:"id"`
		Nickname   string    `json:"nickname"`
		InviteCode string    `json:"inviteCode"`
		CreatedAt  time.Time `json:"createdAt"`
	} `json:"account"`
}

func postIdentity(t *testing.T, fn http.HandlerFunc, identityToken, nickname string) authBody {
	t.Helper()
	rr := callIdentity(fn, identityToken, nickname)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	return decodeAuth(t, rr)
}

func decodeAuth(t *testing.T, rr *httptest.ResponseRecorder) authBody {
	t.Helper()
	var out authBody
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func callIdentity(fn http.HandlerFunc, identityToken, nickname string) *httptest.ResponseRecorder {
	payload := map[string]string{}
	if identityToken != "" {
		payload["identityToken"] = identityToken
	}
	if nickname != "" {
		payload["nickname"] = nickname
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	fn(rr, req)
	return rr
}

func signGoogleID(t *testing.T, priv *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "kid"
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
