package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/vincent-codeworld/duibu-adhd/backend/internal/auth"
)

type ctxKey string

const AccountIDKey ctxKey = "accountID"

func BearerAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			claims, err := auth.ParseToken(secret, strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), AccountIDKey, claims.AccountID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AccountID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(AccountIDKey).(string)
	return v, ok && v != ""
}
