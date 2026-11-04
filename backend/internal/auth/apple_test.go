package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAppleVerifier_OK(t *testing.T) {
	priv, pub := mustRSA(t)
	kid := "test-kid"
	now := time.Date(2026, 11, 3, 14, 0, 0, 0, time.UTC)
	tok := signApple(t, priv, kid, appleTokenOpts{
		sub:           "apple-sub-1",
		aud:           "com.example.duibu",
		iss:           AppleIssuer,
		iat:           now.Add(-time.Minute),
		exp:           now.Add(10 * time.Minute),
		email:         "a@privaterelay.appleid.com",
		emailVerified: true,
	})
	v := &AppleVerifier{
		Audiences: []string{"com.example.duibu", "com.example.duibu.siwa"},
		Keys:      StaticKeySource{kid: pub},
		Issuer:    AppleIssuer,
		Now:       func() time.Time { return now },
	}
	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "apple-sub-1" {
		t.Fatalf("sub=%s", got.Subject)
	}
	if got.Email != "a@privaterelay.appleid.com" || !got.EmailVerified {
		t.Fatalf("email=%+v", got)
	}
}

func TestAppleVerifier_RejectsBadAud(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC()
	tok := signApple(t, priv, "k1", appleTokenOpts{
		sub: "s",
		aud: "wrong.bundle",
		iss: AppleIssuer,
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	v := &AppleVerifier{
		Audiences: []string{"com.example.duibu"},
		Keys:      StaticKeySource{"k1": pub},
		Now:       func() time.Time { return now },
	}
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected audience error")
	}
}

func TestAppleVerifier_RejectsExpired(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC()
	tok := signApple(t, priv, "k1", appleTokenOpts{
		sub: "s",
		aud: "com.example.duibu",
		iss: AppleIssuer,
		iat: now.Add(-2 * time.Hour),
		exp: now.Add(-time.Hour),
	})
	v := &AppleVerifier{
		Audiences: []string{"com.example.duibu"},
		Keys:      StaticKeySource{"k1": pub},
		Now:       func() time.Time { return now },
	}
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected expiry error")
	}
}

func TestAppleVerifier_RejectsBadIssuer(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC()
	tok := signApple(t, priv, "k1", appleTokenOpts{
		sub: "s",
		aud: "com.example.duibu",
		iss: "https://evil.example",
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	v := &AppleVerifier{
		Audiences: []string{"com.example.duibu"},
		Keys:      StaticKeySource{"k1": pub},
		Now:       func() time.Time { return now },
	}
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatal("expected issuer error")
	}
}

func TestJWKSKeySource_Fetches(t *testing.T) {
	priv, pub := mustRSA(t)
	kid := "jwks-kid"
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{
				{"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256", "n": n, "e": e},
			},
		})
	}))
	defer srv.Close()

	src := NewJWKSKeySource(srv.URL, srv.Client())
	got, err := src.PublicKey(context.Background(), kid)
	if err != nil {
		t.Fatal(err)
	}
	if got.N.Cmp(pub.N) != 0 || got.E != pub.E {
		t.Fatal("key mismatch")
	}

	now := time.Now().UTC()
	tok := signApple(t, priv, kid, appleTokenOpts{
		sub: "from-jwks",
		aud: "com.example.duibu",
		iss: AppleIssuer,
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	v := &AppleVerifier{
		Audiences: []string{"com.example.duibu"},
		Keys:      src,
		Now:       func() time.Time { return now },
	}
	id, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "from-jwks" {
		t.Fatalf("sub=%s", id.Subject)
	}
}

func TestAppleVerifier_NotConfigured(t *testing.T) {
	v := NewAppleVerifier()
	if v.Configured() {
		t.Fatal("expected not configured")
	}
	if _, err := v.Verify(context.Background(), "x"); err != ErrAppleConfig {
		t.Fatalf("err=%v", err)
	}
}

type appleTokenOpts struct {
	sub, aud, iss, email string
	emailVerified        bool
	iat, exp             time.Time
}

func signApple(t *testing.T, priv *rsa.PrivateKey, kid string, o appleTokenOpts) string {
	t.Helper()
	claims := appleClaims{
		Email:         o.email,
		EmailVerified: o.emailVerified,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   o.sub,
			Issuer:    o.iss,
			Audience:  jwt.ClaimStrings{o.aud},
			IssuedAt:  jwt.NewNumericDate(o.iat),
			ExpiresAt: jwt.NewNumericDate(o.exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustRSA(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return priv, &priv.PublicKey
}
