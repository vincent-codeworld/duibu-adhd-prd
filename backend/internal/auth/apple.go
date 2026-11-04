package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AppleIssuer     = "https://appleid.apple.com"
	AppleJWKSURL    = "https://appleid.apple.com/auth/keys"
	defaultJWKSTTL  = time.Hour
	maxIdentitySize = 1 << 14
)

var (
	ErrAppleInvalidToken = errors.New("invalid_apple_token")
	ErrAppleConfig       = errors.New("apple_auth_not_configured")
)

// AppleIdentity holds verified claims from an Apple identityToken.
type AppleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
}

// KeySource resolves an RSA public key by JWT kid.
type KeySource interface {
	PublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error)
}

// AppleVerifier validates Sign in with Apple identity tokens (RS256 + Apple JWKS).
type AppleVerifier struct {
	Audiences []string
	Keys      KeySource
	Issuer    string
	Now       func() time.Time
}

// NewAppleVerifier builds a verifier for the given audience values (Bundle ID and/or Services ID).
func NewAppleVerifier(audiences ...string) *AppleVerifier {
	clean := make([]string, 0, len(audiences))
	for _, a := range audiences {
		a = strings.TrimSpace(a)
		if a != "" {
			clean = append(clean, a)
		}
	}
	return &AppleVerifier{
		Audiences: clean,
		Keys:      NewJWKSKeySource(AppleJWKSURL, nil),
		Issuer:    AppleIssuer,
		Now:       func() time.Time { return time.Now().UTC() },
	}
}

func (v *AppleVerifier) Configured() bool {
	return v != nil && len(v.Audiences) > 0 && v.Keys != nil
}

// Verify parses and validates an Apple identityToken, returning the stable Apple subject (sub).
func (v *AppleVerifier) Verify(ctx context.Context, identityToken string) (*AppleIdentity, error) {
	if !v.Configured() {
		return nil, ErrAppleConfig
	}
	identityToken = strings.TrimSpace(identityToken)
	if identityToken == "" || len(identityToken) > maxIdentitySize {
		return nil, fmt.Errorf("%w: empty_or_too_large", ErrAppleInvalidToken)
	}

	// Audience is checked against all configured Client IDs below (Bundle ID and/or Services ID).
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.issuer()),
		jwt.WithLeeway(30*time.Second),
		jwt.WithTimeFunc(v.now),
	)

	claims := &appleClaims{}
	tok, err := parser.ParseWithClaims(identityToken, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("%w: missing_kid", ErrAppleInvalidToken)
		}
		return v.Keys.PublicKey(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAppleInvalidToken, err)
	}
	if !tok.Valid {
		return nil, fmt.Errorf("%w: not_valid", ErrAppleInvalidToken)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: missing_sub", ErrAppleInvalidToken)
	}
	if !audienceOK(claims.Audience, v.Audiences) {
		return nil, fmt.Errorf("%w: audience_mismatch", ErrAppleInvalidToken)
	}

	out := &AppleIdentity{Subject: claims.Subject, Email: claims.Email}
	if claims.EmailVerified != nil {
		switch x := claims.EmailVerified.(type) {
		case bool:
			out.EmailVerified = x
		case string:
			out.EmailVerified = x == "true"
		}
	}
	return out, nil
}

func (v *AppleVerifier) issuer() string {
	if v.Issuer != "" {
		return v.Issuer
	}
	return AppleIssuer
}

func (v *AppleVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now().UTC()
}

type appleClaims struct {
	Email         string `json:"email,omitempty"`
	EmailVerified any    `json:"email_verified,omitempty"`
	jwt.RegisteredClaims
}

func audienceOK(got jwt.ClaimStrings, want []string) bool {
	allowed := map[string]struct{}{}
	for _, a := range want {
		allowed[a] = struct{}{}
	}
	for _, g := range got {
		if _, ok := allowed[g]; ok {
			return true
		}
	}
	return false
}

// JWKSKeySource fetches and caches Apple (or test) JWKS.
type JWKSKeySource struct {
	URL        string
	HTTPClient *http.Client
	TTL        time.Duration

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewJWKSKeySource(url string, client *http.Client) *JWKSKeySource {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &JWKSKeySource{
		URL:        url,
		HTTPClient: client,
		TTL:        defaultJWKSTTL,
		keys:       map[string]*rsa.PublicKey{},
	}
}

func (s *JWKSKeySource) PublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if k := s.cached(kid); k != nil {
		return k, nil
	}
	if err := s.refresh(ctx); err != nil {
		return nil, err
	}
	if k := s.cached(kid); k != nil {
		return k, nil
	}
	// One forced refresh for key rotation mid-cache.
	s.mu.Lock()
	s.fetchedAt = time.Time{}
	s.mu.Unlock()
	if err := s.refresh(ctx); err != nil {
		return nil, err
	}
	if k := s.cached(kid); k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("%w: kid_not_found", ErrAppleInvalidToken)
}

func (s *JWKSKeySource) cached(kid string) *rsa.PublicKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.TTL > 0 && !s.fetchedAt.IsZero() && time.Since(s.fetchedAt) > s.TTL {
		return nil
	}
	return s.keys[kid]
}

func (s *JWKSKeySource) refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TTL > 0 && !s.fetchedAt.IsZero() && time.Since(s.fetchedAt) <= s.TTL && len(s.keys) > 0 {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return err
	}
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	parsed, err := parseJWKS(body)
	if err != nil {
		return err
	}
	s.keys = parsed
	s.fetchedAt = time.Now().UTC()
	return nil
}

type jwksDoc struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func parseJWKS(raw []byte) (map[string]*rsa.PublicKey, error) {
	var doc jwksDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("jwks empty")
	}
	return out, nil
}

func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	if len(eb) == 0 {
		return nil, fmt.Errorf("empty exponent")
	}
	var eInt int
	for _, b := range eb {
		eInt = eInt<<8 + int(b)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nb),
		E: eInt,
	}, nil
}

// StaticKeySource is a test helper mapping kid -> public key.
type StaticKeySource map[string]*rsa.PublicKey

func (s StaticKeySource) PublicKey(_ context.Context, kid string) (*rsa.PublicKey, error) {
	k, ok := s[kid]
	if !ok {
		return nil, fmt.Errorf("%w: kid_not_found", ErrAppleInvalidToken)
	}
	return k, nil
}
