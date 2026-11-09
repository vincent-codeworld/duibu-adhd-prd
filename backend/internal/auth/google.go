package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// GoogleIssuer and GoogleIssuerBare are the only issuers Google documents for ID tokens.
	GoogleIssuer     = "https://accounts.google.com"
	GoogleIssuerBare = "accounts.google.com"
	GoogleJWKSURL    = "https://www.googleapis.com/oauth2/v3/certs"
)

var (
	ErrGoogleInvalidToken = errors.New("invalid_google_token")
	ErrGoogleConfig       = errors.New("google_auth_not_configured")
)

// GoogleIdentity is the verified Google account key.
// Email is informational only and must never be used to find or merge an app account.
type GoogleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
}

// GoogleVerifier validates Google ID tokens (RS256 + Google JWKS).
type GoogleVerifier struct {
	Audiences []string
	Keys      KeySource
	Now       func() time.Time
}

// NewGoogleVerifier builds a verifier for the given OAuth client IDs (iOS, and optional extras).
func NewGoogleVerifier(audiences ...string) *GoogleVerifier {
	clean := make([]string, 0, len(audiences))
	for _, a := range audiences {
		a = strings.TrimSpace(a)
		if a != "" {
			clean = append(clean, a)
		}
	}
	return &GoogleVerifier{
		Audiences: clean,
		Keys:      NewJWKSKeySource(GoogleJWKSURL, nil),
		Now:       func() time.Time { return time.Now().UTC() },
	}
}

func (v *GoogleVerifier) Configured() bool {
	return v != nil && len(v.Audiences) > 0 && v.Keys != nil
}

// Verify parses and validates a Google ID token.
// The returned subject is Google's sub claim. Callers must persist it as a Google identity,
// not as an Apple subject and not looked up by email.
func (v *GoogleVerifier) Verify(ctx context.Context, idToken string) (*GoogleIdentity, error) {
	if !v.Configured() {
		return nil, ErrGoogleConfig
	}
	idToken = strings.TrimSpace(idToken)
	if idToken == "" || len(idToken) > maxIdentitySize {
		return nil, fmt.Errorf("%w: empty_or_too_large", ErrGoogleInvalidToken)
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
		jwt.WithTimeFunc(v.now),
	)

	claims := &googleClaims{}
	tok, err := parser.ParseWithClaims(idToken, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("%w: unexpected_alg", ErrGoogleInvalidToken)
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("%w: missing_kid", ErrGoogleInvalidToken)
		}
		return v.Keys.PublicKey(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGoogleInvalidToken, err)
	}
	if !tok.Valid {
		return nil, fmt.Errorf("%w: not_valid", ErrGoogleInvalidToken)
	}
	if !googleIssuerOK(claims.Issuer) {
		return nil, fmt.Errorf("%w: issuer_mismatch", ErrGoogleInvalidToken)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: missing_sub", ErrGoogleInvalidToken)
	}
	if !audienceOK(claims.Audience, v.Audiences) {
		return nil, fmt.Errorf("%w: audience_mismatch", ErrGoogleInvalidToken)
	}
	// azp is the OAuth client that requested the token. When Google includes it,
	// it must be one of our client IDs (fail closed). aud remains the audience check.
	if claims.Azp != "" && !audienceOK(jwt.ClaimStrings{claims.Azp}, v.Audiences) {
		return nil, fmt.Errorf("%w: azp_mismatch", ErrGoogleInvalidToken)
	}

	out := &GoogleIdentity{Subject: claims.Subject, Email: claims.Email}
	if claims.EmailVerified != nil {
		switch x := claims.EmailVerified.(type) {
		case bool:
			out.EmailVerified = x
		case string:
			out.EmailVerified = strings.EqualFold(x, "true")
		}
	}
	return out, nil
}

func googleIssuerOK(iss string) bool {
	return iss == GoogleIssuer || iss == GoogleIssuerBare
}

func (v *GoogleVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now().UTC()
}

type googleClaims struct {
	Email         string `json:"email,omitempty"`
	EmailVerified any    `json:"email_verified,omitempty"`
	Azp           string `json:"azp,omitempty"`
	jwt.RegisteredClaims
}
