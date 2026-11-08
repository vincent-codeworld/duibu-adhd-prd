package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vincent-codeworld/duibu-adhd/backend/internal/config"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/httpapi"
)

func TestGoogleAuthRouteStubSessionAndAppleUnchanged(t *testing.T) {
	t.Setenv("STORE_DRIVER", "memory")
	t.Setenv("AUTH_MODE", "stub")
	t.Setenv("JWT_SECRET", "route-secret")
	t.Setenv("UPLOAD_DIR", t.TempDir())
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("APPLE_CLIENT_ID", "")

	srv := httpapi.NewRouter(config.Load())
	google := postRoute(t, srv, "/v1/auth/google", `{"identityToken":"route-user","nickname":"G"}`, http.StatusOK)
	if !google.Created || google.AccessToken == "" || google.Account.Nickname != "G" {
		t.Fatalf("google=%+v", google)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+google.AccessToken)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte(google.Account.ID)) {
		t.Fatalf("me: %d %s", rr.Code, rr.Body.String())
	}

	apple := postRoute(t, srv, "/v1/auth/apple", `{"identityToken":"route-user","nickname":"A"}`, http.StatusOK)
	if !apple.Created || apple.Account.ID == google.Account.ID {
		t.Fatalf("apple stub linked or failed: %+v google=%s", apple, google.Account.ID)
	}

	again := postRoute(t, srv, "/v1/auth/google", `{"identityToken":"route-user"}`, http.StatusOK)
	if again.Created || again.Account.ID != google.Account.ID {
		t.Fatalf("second google=%+v", again)
	}
}

func TestGoogleAuthRouteFailClosedWhenUnconfigured(t *testing.T) {
	t.Setenv("STORE_DRIVER", "memory")
	t.Setenv("AUTH_MODE", "apple")
	t.Setenv("JWT_SECRET", "route-secret")
	t.Setenv("UPLOAD_DIR", t.TempDir())
	t.Setenv("GOOGLE_CLIENT_ID", "")
	t.Setenv("GOOGLE_CLIENT_IDS", "")
	t.Setenv("APPLE_CLIENT_ID", "")
	t.Setenv("APPLE_CLIENT_IDS", "")

	srv := httpapi.NewRouter(config.Load())
	rr := postRaw(srv, "/v1/auth/google", `{"identityToken":"anything"}`)
	if rr.Code != http.StatusServiceUnavailable || !bytes.Contains(rr.Body.Bytes(), []byte("google_auth_not_configured")) {
		t.Fatalf("google: %d %s", rr.Code, rr.Body.String())
	}
	rr = postRaw(srv, "/v1/auth/apple", `{"identityToken":"anything"}`)
	if rr.Code != http.StatusServiceUnavailable || !bytes.Contains(rr.Body.Bytes(), []byte("apple_auth_not_configured")) {
		t.Fatalf("apple: %d %s", rr.Code, rr.Body.String())
	}
	rr = postRaw(srv, "/v1/auth/dev", `{"subject":"x"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("dev: %d %s", rr.Code, rr.Body.String())
	}
}

type routeAuth struct {
	AccessToken string `json:"accessToken"`
	Created     bool   `json:"created"`
	Account     struct {
		ID       string `json:"id"`
		Nickname string `json:"nickname"`
	} `json:"account"`
}

func postRoute(t *testing.T, srv http.Handler, path, body string, want int) routeAuth {
	t.Helper()
	rr := postRaw(srv, path, body)
	if rr.Code != want {
		t.Fatalf("%s: %d %s", path, rr.Code, rr.Body.String())
	}
	var out routeAuth
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postRaw(srv http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}
