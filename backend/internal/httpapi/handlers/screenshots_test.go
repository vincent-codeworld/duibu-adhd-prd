package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vincent-codeworld/duibu-adhd/backend/internal/config"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/httpapi"
)

func TestUploadSubmitGetScreenshotFlow(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("STORE_DRIVER", "memory")
	os.Setenv("AUTH_MODE", "stub")
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("UPLOAD_DIR", dir)
	defer func() {
		os.Unsetenv("STORE_DRIVER")
		os.Unsetenv("AUTH_MODE")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("UPLOAD_DIR")
	}()

	cfg := config.Load()
	srv := httpapi.NewRouter(cfg)

	login := func(subject, nick string) string {
		body, _ := json.Marshal(map[string]string{"subject": subject, "nickname": nick})
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/dev", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("login %s: %d %s", subject, rr.Code, rr.Body.String())
		}
		var out struct {
			AccessToken string `json:"accessToken"`
			Account     struct {
				ID         string `json:"id"`
				InviteCode string `json:"inviteCode"`
			} `json:"account"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &out)
		return out.AccessToken
	}
	aliceTok := login("shot-alice", "Alice")
	bobTok := login("shot-bob", "Bob")

	// get bob invite
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+bobTok)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	var bobMe struct {
		ID         string `json:"id"`
		InviteCode string `json:"inviteCode"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &bobMe)

	// alice adds bob
	body, _ := json.Marshal(map[string]string{"inviteCode": bobMe.InviteCode})
	req = httptest.NewRequest(http.MethodPost, "/v1/friends", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != 200 && rr.Code != 201 {
		t.Fatalf("friends: %d %s", rr.Code, rr.Body.String())
	}

	// create task with stage done + reward + verifier
	taskBody := map[string]any{
		"title": "报告", "notes": "", "uploadConsent": true,
		"dueAt": "2026-12-01T12:00:00Z",
		"stages": []map[string]any{
			{"title": "一步", "progress": "done"},
			{"title": "二"},
			{"title": "三"},
		},
		"stageRewards":                map[string]any{"1": map[string]string{"text": "奶茶"}},
		"designatedVerifierAccountId": bobMe.ID,
	}
	raw, _ := json.Marshal(taskBody)
	req = httptest.NewRequest(http.MethodPost, "/v1/tasks", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != 201 && rr.Code != 200 {
		t.Fatalf("create task: %d %s", rr.Code, rr.Body.String())
	}
	var tv struct {
		Stages []struct {
			Reward *struct {
				ID string `json:"id"`
			} `json:"reward"`
		} `json:"stages"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &tv)
	if len(tv.Stages) == 0 || tv.Stages[0].Reward == nil {
		t.Fatalf("no reward: %s", rr.Body.String())
	}
	rewardID := tv.Stages[0].Reward.ID

	// oversize reject
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "big.jpg")
	huge := make([]byte, (4<<20)+16)
	huge[0], huge[1], huge[2] = 0xff, 0xd8, 0xff
	_, _ = fw.Write(huge)
	_ = w.Close()
	req = httptest.NewRequest(http.MethodPost, "/v1/uploads/screenshots", &buf)
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge && rr.Code != http.StatusBadRequest {
		t.Fatalf("oversize: %d %s", rr.Code, rr.Body.String())
	}

	// tiny png upload
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	buf.Reset()
	w = multipart.NewWriter(&buf)
	fw, _ = w.CreateFormFile("file", "shot.png")
	_, _ = fw.Write(png)
	_ = w.Close()
	req = httptest.NewRequest(http.MethodPost, "/v1/uploads/screenshots", &buf)
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rr.Code, rr.Body.String())
	}
	var up struct {
		ScreenshotKey string `json:"screenshotKey"`
		ContentType   string `json:"contentType"`
		ByteSize      int    `json:"byteSize"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &up)
	if up.ScreenshotKey == "" || up.ContentType != "image/png" || up.ByteSize != len(png) {
		t.Fatalf("%+v", up)
	}
	if _, err := os.Stat(filepath.Join(dir, up.ScreenshotKey)); err != nil {
		t.Fatal(err)
	}

	// submit with key
	sub, _ := json.Marshal(map[string]string{"note": "ok", "screenshotKey": up.ScreenshotKey})
	req = httptest.NewRequest(http.MethodPost, "/v1/rewards/"+rewardID+"/verifications", bytes.NewReader(sub))
	req.Header.Set("Authorization", "Bearer "+aliceTok)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("submit: %d %s", rr.Code, rr.Body.String())
	}

	// bob gets bytes
	req = httptest.NewRequest(http.MethodGet, "/v1/screenshots/"+up.ScreenshotKey, nil)
	req.Header.Set("Authorization", "Bearer "+bobTok)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("get as verifier: %d %s", rr.Code, rr.Body.String())
	}
	got, _ := io.ReadAll(rr.Body)
	if !bytes.Equal(got, png) {
		t.Fatalf("bytes mismatch")
	}

	// stranger forbidden
	carolTok := login("shot-carol", "Carol")
	req = httptest.NewRequest(http.MethodGet, "/v1/screenshots/"+up.ScreenshotKey, nil)
	req.Header.Set("Authorization", "Bearer "+carolTok)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("stranger: %d", rr.Code)
	}
}
