package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGoogleVerifier_OK(t *testing.T) {
	priv, pub := mustRSA(t)
	kid := "google-kid"
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	tok := signGoogle(t, priv, kid, googleTokenOpts{
		sub:           "google-sub-1",
		aud:           "ios-client.apps.googleusercontent.com",
		azp:           "ios-client.apps.googleusercontent.com",
		iss:           GoogleIssuer,
		iat:           now.Add(-time.Minute),
		exp:           now.Add(time.Hour),
		email:         "ada@example.com",
		emailVerified: true,
	})
	v := &GoogleVerifier{
		Audiences: []string{"ios-client.apps.googleusercontent.com", "web-client.apps.googleusercontent.com"},
		Keys:      StaticKeySource{kid: pub},
		Now:       func() time.Time { return now },
	}
	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "google-sub-1" {
		t.Fatalf("sub=%s", got.Subject)
	}
	if got.Email != "ada@example.com" || !got.EmailVerified {
		t.Fatalf("email=%+v", got)
	}
}

func TestGoogleVerifier_AcceptsBareIssuer(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC().Truncate(time.Second)
	tok := signGoogle(t, priv, "k1", googleTokenOpts{
		sub: "sub-bare",
		aud: "client",
		iss: GoogleIssuerBare,
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	v := testGoogleVerifier(pub, now, "client")
	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "sub-bare" {
		t.Fatalf("sub=%s", got.Subject)
	}
}

func TestGoogleVerifier_RejectsBadAudience(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC()
	tok := signGoogle(t, priv, "k1", googleTokenOpts{
		sub: "s",
		aud: "other-client",
		iss: GoogleIssuer,
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	if _, err := testGoogleVerifier(pub, now, "our-client").Verify(context.Background(), tok); !errors.Is(err, ErrGoogleInvalidToken) {
		t.Fatalf("err=%v", err)
	}
}

func TestGoogleVerifier_RejectsExpired(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC()
	tok := signGoogle(t, priv, "k1", googleTokenOpts{
		sub: "s",
		aud: "our-client",
		iss: GoogleIssuer,
		iat: now.Add(-2 * time.Hour),
		exp: now.Add(-time.Hour),
	})
	if _, err := testGoogleVerifier(pub, now, "our-client").Verify(context.Background(), tok); !errors.Is(err, ErrGoogleInvalidToken) {
		t.Fatalf("err=%v", err)
	}
}

func TestGoogleVerifier_RejectsMissingExpiry(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC()
	claims := googleClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  "s",
			Issuer:   GoogleIssuer,
			Audience: jwt.ClaimStrings{"our-client"},
			IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testGoogleVerifier(pub, now, "our-client").Verify(context.Background(), s); !errors.Is(err, ErrGoogleInvalidToken) {
		t.Fatalf("err=%v", err)
	}
}

func TestGoogleVerifier_RejectsBadIssuer(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC()
	tok := signGoogle(t, priv, "k1", googleTokenOpts{
		sub: "s",
		aud: "our-client",
		iss: "https://appleid.apple.com",
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	if _, err := testGoogleVerifier(pub, now, "our-client").Verify(context.Background(), tok); !errors.Is(err, ErrGoogleInvalidToken) {
		t.Fatalf("err=%v", err)
	}
}

func TestGoogleVerifier_RejectsBadSignature(t *testing.T) {
	priv, _ := mustRSA(t)
	_, otherPub := mustRSA(t)
	now := time.Now().UTC()
	tok := signGoogle(t, priv, "k1", googleTokenOpts{
		sub: "s",
		aud: "our-client",
		iss: GoogleIssuer,
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	if _, err := testGoogleVerifier(otherPub, now, "our-client").Verify(context.Background(), tok); !errors.Is(err, ErrGoogleInvalidToken) {
		t.Fatalf("err=%v", err)
	}
}

func TestGoogleVerifier_RejectsAzpOutsideConfiguredClients(t *testing.T) {
	priv, pub := mustRSA(t)
	now := time.Now().UTC()
	tok := signGoogle(t, priv, "k1", googleTokenOpts{
		sub: "s",
		aud: "our-client",
		azp: "attacker-client",
		iss: GoogleIssuer,
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	if _, err := testGoogleVerifier(pub, now, "our-client").Verify(context.Background(), tok); !errors.Is(err, ErrGoogleInvalidToken) {
		t.Fatalf("err=%v", err)
	}
}

func TestGoogleVerifier_RejectsNoneAlg(t *testing.T) {
	now := time.Now().UTC()
	claims := googleClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "s",
			Issuer:    GoogleIssuer,
			Audience:  jwt.ClaimStrings{"our-client"},
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	priv, pub := mustRSA(t)
	_ = priv
	if _, err := testGoogleVerifier(pub, now, "our-client").Verify(context.Background(), s); !errors.Is(err, ErrGoogleInvalidToken) {
		t.Fatalf("err=%v", err)
	}
}

func TestGoogleJWKSKeySource_Verifies(t *testing.T) {
	priv, pub := mustRSA(t)
	kid := "jwks-google"
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

	now := time.Now().UTC()
	tok := signGoogle(t, priv, kid, googleTokenOpts{
		sub: "from-jwks",
		aud: "our-client",
		iss: GoogleIssuer,
		iat: now.Add(-time.Minute),
		exp: now.Add(time.Hour),
	})
	v := &GoogleVerifier{
		Audiences: []string{"our-client"},
		Keys:      NewJWKSKeySource(srv.URL, srv.Client()),
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

func TestGoogleVerifier_NotConfigured(t *testing.T) {
	v := NewGoogleVerifier()
	if v.Configured() {
		t.Fatal("expected not configured")
	}
	_, err := v.Verify(context.Background(), "header.payload.sig")
	if err != ErrGoogleConfig {
		t.Fatalf("err=%v", err)
	}
	blank := NewGoogleVerifier("  ", "")
	if blank.Configured() {
		t.Fatal("whitespace audiences should not configure")
	}
}

func testGoogleVerifier(pub *rsa.PublicKey, now time.Time, audiences ...string) *GoogleVerifier {
	return &GoogleVerifier{
		Audiences: audiences,
		Keys:      StaticKeySource{"k1": pub},
		Now:       func() time.Time { return now },
	}
}

type googleTokenOpts struct {
	sub, aud, azp, iss, email string
	emailVerified             bool
	iat, exp                  time.Time
}

func signGoogle(t *testing.T, priv *rsa.PrivateKey, kid string, o googleTokenOpts) string {
	t.Helper()
	claims := googleClaims{
		Email:         o.email,
		EmailVerified: o.emailVerified,
		Azp:           o.azp,
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
