package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	AccountID string `json:"aid"`
	jwt.RegisteredClaims
}

func IssueToken(secret, accountID string, ttl time.Duration) (string, time.Time, error) {
	exp := time.Now().UTC().Add(ttl)
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		AccountID: accountID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			Subject:   accountID,
		},
	})
	s, err := t.SignedString([]byte(secret))
	return s, exp, err
}

func ParseToken(secret, token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
