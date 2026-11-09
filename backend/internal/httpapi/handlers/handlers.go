package handlers

import (
	"encoding/json"
	"io"

	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vincent-codeworld/duibu-adhd/backend/internal/ai"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/auth"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/config"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/httpapi/middleware"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/store"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/upload"
)

type Handler struct {
	cfg            config.Config
	limiter        *auth.InviteFailLimiter
	accounts       store.Store
	disk           *upload.Local
	aiProvider     ai.Provider
	appleVerifier  *auth.AppleVerifier
	googleVerifier *auth.GoogleVerifier
}

func New(cfg config.Config, limiter *auth.InviteFailLimiter, accounts store.Store, disk *upload.Local) *Handler {
	var apple *auth.AppleVerifier
	var google *auth.GoogleVerifier
	if cfg.AuthMode == "apple" {
		apple = auth.NewAppleVerifier(cfg.AppleAudiences()...)
		google = auth.NewGoogleVerifier(cfg.GoogleAudiences()...)
	}
	return &Handler{
		cfg:            cfg,
		limiter:        limiter,
		accounts:       accounts,
		disk:           disk,
		aiProvider:     ai.NewProvider(cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel),
		appleVerifier:  apple,
		googleVerifier: google,
	}
}

func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"authMode":    h.cfg.AuthMode,
		"storeDriver": h.cfg.StoreDriver,
	})
}

func (h *Handler) NotImplemented(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "not_implemented",
		"hint":  "skeleton route; implement in module order auth→friends→tasks→verification→ai",
	})
}

type authResponse struct {
	AccessToken string         `json:"accessToken"`
	ExpiresAt   time.Time      `json:"expiresAt"`
	Account     accountPayload `json:"account"`
	Created     bool           `json:"created"`
}

type accountPayload struct {
	ID         string    `json:"id"`
	Nickname   string    `json:"nickname"`
	InviteCode string    `json:"inviteCode"`
	CreatedAt  time.Time `json:"createdAt"`
}

// AuthApple: stub mode treats identityToken as a stable Apple subject string.
// apple mode verifies the JWT against Apple JWKS (aud = APPLE_CLIENT_ID[/S]).
func (h *Handler) AuthApple(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IdentityToken string `json:"identityToken"`
		Nickname      string `json:"nickname,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.IdentityToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identityToken required"})
		return
	}
	token := strings.TrimSpace(body.IdentityToken)
	nickname := strings.TrimSpace(body.Nickname)

	if h.cfg.AuthMode == "stub" {
		h.issueForAppleSubject(w, token, nickname)
		return
	}

	if h.appleVerifier == nil || !h.appleVerifier.Configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "apple_auth_not_configured",
			"hint":  "set APPLE_CLIENT_ID to the iOS Bundle ID (and optional APPLE_CLIENT_IDS)",
		})
		return
	}
	id, err := h.appleVerifier.Verify(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "invalid_apple_token",
			"hint":  err.Error(),
		})
		return
	}
	h.issueForAppleSubject(w, id.Subject, nickname)
}

// AuthGoogle: stub mode treats identityToken as a stable Google subject string.
// apple mode (the non-stub mode) verifies the Google ID token.
// The account is keyed only by Google sub, never by email or by an Apple subject.
func (h *Handler) AuthGoogle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IdentityToken string `json:"identityToken"`
		Nickname      string `json:"nickname,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.IdentityToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identityToken required"})
		return
	}
	token := strings.TrimSpace(body.IdentityToken)
	nickname := strings.TrimSpace(body.Nickname)

	if h.cfg.AuthMode == "stub" {
		h.issueForGoogleSubject(w, token, nickname)
		return
	}

	if h.googleVerifier == nil || !h.googleVerifier.Configured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "google_auth_not_configured",
			"hint":  "set GOOGLE_CLIENT_ID to the iOS OAuth client ID (and optional GOOGLE_CLIENT_IDS)",
		})
		return
	}
	id, err := h.googleVerifier.Verify(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "invalid_google_token",
			"hint":  err.Error(),
		})
		return
	}
	h.issueForGoogleSubject(w, id.Subject, nickname)
}

// AuthDev is stub-only convenience: POST {"subject":"...","nickname":"..."}.
func (h *Handler) AuthDev(w http.ResponseWriter, r *http.Request) {
	if h.cfg.AuthMode != "stub" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	var body struct {
		Subject  string `json:"subject"`
		Nickname string `json:"nickname,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Subject) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subject required"})
		return
	}
	h.issueForAppleSubject(w, strings.TrimSpace(body.Subject), strings.TrimSpace(body.Nickname))
}

func (h *Handler) issueForAppleSubject(w http.ResponseWriter, subject, nickname string) {
	acc, created, err := h.accounts.UpsertByAppleSubject(subject, nickname)
	h.writeAuthSession(w, acc, created, err)
}

func (h *Handler) issueForGoogleSubject(w http.ResponseWriter, subject, nickname string) {
	acc, created, err := h.accounts.UpsertByGoogleSubject(subject, nickname)
	h.writeAuthSession(w, acc, created, err)
}

func (h *Handler) writeAuthSession(w http.ResponseWriter, acc *domain.Account, created bool, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tok, exp, err := auth.IssueToken(h.cfg.JWTSecret, acc.ID, 7*24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_issue_failed"})
		return
	}
	writeJSON(w, http.StatusOK, authResponse{
		AccessToken: tok,
		ExpiresAt:   exp,
		Created:     created,
		Account: accountPayload{
			ID:         acc.ID,
			Nickname:   acc.Nickname,
			InviteCode: acc.InviteCode,
			CreatedAt:  acc.CreatedAt,
		},
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	acc, err := h.accounts.GetByID(id)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, accountPayload{
		ID:         acc.ID,
		Nickname:   acc.Nickname,
		InviteCode: acc.InviteCode,
		CreatedAt:  acc.CreatedAt,
	})
}

func (h *Handler) PatchMe(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	acc, err := h.accounts.UpdateNickname(id, strings.TrimSpace(body.Nickname))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, accountPayload{
		ID:         acc.ID,
		Nickname:   acc.Nickname,
		InviteCode: acc.InviteCode,
		CreatedAt:  acc.CreatedAt,
	})
}

func (h *Handler) GetInviteCode(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	acc, err := h.accounts.GetByID(id)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"inviteCode": acc.InviteCode})
}

func (h *Handler) ResetInviteCode(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	acc, err := h.accounts.ResetInviteCode(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"inviteCode": acc.InviteCode})
}

func (h *Handler) ListFriends(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := h.accounts.ListFriends(id)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"friends": items})
}

func (h *Handler) AddFriend(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		InviteCode string `json:"inviteCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.InviteCode) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "inviteCode required"})
		return
	}
	now := time.Now().UTC()
	if !h.limiter.Allowed(id, now) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too_many_failed_invite_attempts"})
		return
	}
	item, already, err := h.accounts.AddFriendByInvite(id, strings.TrimSpace(body.InviteCode))
	if err != nil {
		msg := err.Error()
		if msg == "invalid invite code" {
			h.limiter.RecordFail(id, now)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid_invite_code"})
			return
		}
		if msg == "cannot add yourself" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_add_yourself"})
			return
		}
		if msg == "blocked" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "blocked"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	status := http.StatusCreated
	payload := map[string]any{"friend": item, "alreadyFriends": already}
	if already {
		status = http.StatusOK
	}
	writeJSON(w, status, payload)
}

func (h *Handler) RemoveFriend(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	fid := chi.URLParam(r, "friendshipID")
	if err := h.accounts.RemoveFriendship(id, fid); err != nil {
		if err.Error() == "forbidden" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateBlock(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		AccountID string `json:"accountId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.AccountID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "accountId required"})
		return
	}
	if err := h.accounts.Block(id, strings.TrimSpace(body.AccountID)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "blocked"})
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := h.accounts.ListSyncedTasks(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items})
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body store.CreateTaskInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	tv, err := h.accounts.CreateSyncedTask(id, body)
	if err != nil {
		msg := err.Error()
		status := http.StatusBadRequest
		if msg == "uploadConsent required" {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	writeJSON(w, http.StatusCreated, tv)
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	tv, err := h.accounts.GetSyncedTask(id, chi.URLParam(r, "taskID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, tv)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	taskID := chi.URLParam(r, "taskID")
	keys, _ := h.accounts.ScreenshotKeysForTask(id, taskID)
	if err := h.accounts.DeleteSyncedTask(id, taskID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if h.disk != nil {
		for _, k := range keys {
			_ = h.disk.Remove(k)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	tv, err := h.accounts.CompleteTask(id, chi.URLParam(r, "taskID"))
	if err != nil {
		status := http.StatusBadRequest
		msg := err.Error()
		if msg == "not found" {
			status = http.StatusNotFound
		} else if msg == "verification required" {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	writeJSON(w, http.StatusOK, tv)
}

func (h *Handler) SetTaskVerifier(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		AccountID string `json:"accountId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.AccountID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "accountId required"})
		return
	}
	tv, err := h.accounts.SetVerifier(id, chi.URLParam(r, "taskID"), strings.TrimSpace(body.AccountID))
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "not found" {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tv)
}

func (h *Handler) SubmitVerification(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body store.SubmitVerificationInput
	_ = json.NewDecoder(r.Body).Decode(&body)
	item, err := h.accounts.SubmitVerification(id, chi.URLParam(r, "rewardID"), body)
	if err != nil {
		status := http.StatusBadRequest
		switch err.Error() {
		case "not found":
			status = http.StatusNotFound
		case "forbidden":
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) VerificationInbox(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := h.accounts.ListInbox(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) VerificationOutbox(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := h.accounts.ListOutbox(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ApproveVerification(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err := h.accounts.Approve(id, chi.URLParam(r, "requestID")); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "not found" {
			status = http.StatusNotFound
		} else if err.Error() == "forbidden" {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (h *Handler) RejectVerification(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		ResolutionNote string `json:"resolutionNote"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := h.accounts.Reject(id, chi.URLParam(r, "requestID"), body.ResolutionNote); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "not found" {
			status = http.StatusNotFound
		} else if err.Error() == "forbidden" {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

func (h *Handler) WithdrawVerification(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err := h.accounts.Withdraw(id, chi.URLParam(r, "requestID")); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "not found" {
			status = http.StatusNotFound
		} else if err.Error() == "forbidden" {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "withdrawn"})
}

func (h *Handler) PatchTask(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body store.PatchTaskInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	tv, err := h.accounts.PatchSyncedTask(id, chi.URLParam(r, "taskID"), body)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "not found" {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tv)
}

func (h *Handler) ListNotices(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := h.accounts.ListNotices(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	unread := 0
	for _, n := range items {
		if n.ReadAt == nil {
			unread++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "unreadCount": unread})
}

func (h *Handler) MarkNoticeRead(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	n, err := h.accounts.MarkNoticeRead(id, chi.URLParam(r, "noticeID"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (h *Handler) AIDecompose(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	req, err := ai.ValidateRequest(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	resp := ai.RunDecompose(r.Context(), h.aiProvider, req)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateReport(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		RequestID string `json:"requestId"`
		Reason    string `json:"reason"`
		Detail    string `json:"detail,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	view, err := h.accounts.CreateReport(id, store.CreateReportInput{
		RequestID: body.RequestID,
		Reason:    body.Reason,
		Detail:    body.Detail,
	})
	if err != nil {
		msg := err.Error()
		status := http.StatusBadRequest
		switch msg {
		case "not found":
			status = http.StatusNotFound
		case "forbidden":
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (h *Handler) UploadScreenshot(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.disk == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "uploads_unavailable"})
		return
	}
	const maxMem = upload.MaxBytes + (1 << 20)
	if err := r.ParseMultipartForm(maxMem); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_multipart"})
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file required"})
		return
	}
	defer f.Close()
	key, ct, n, err := h.disk.SaveValidated(f)
	if err != nil {
		msg := err.Error()
		status := http.StatusBadRequest
		if msg == "file too large" {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	if err := h.accounts.RegisterScreenshotUpload(id, key, ct, n); err != nil {
		_ = h.disk.Remove(key)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"screenshotKey": key,
		"contentType":   ct,
		"byteSize":      n,
	})
}

func (h *Handler) GetScreenshot(w http.ResponseWriter, r *http.Request) {
	id, ok := middleware.AccountID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	key := chi.URLParam(r, "screenshotKey")
	meta, err := h.accounts.CanFetchScreenshot(id, key)
	if err != nil {
		status := http.StatusNotFound
		if err.Error() == "forbidden" {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if h.disk == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "uploads_unavailable"})
		return
	}
	f, err := h.disk.Open(key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(meta.ByteSize))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
