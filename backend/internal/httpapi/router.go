package httpapi

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/auth"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/config"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/httpapi/handlers"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/httpapi/middleware"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/store"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/upload"
)

func NewRouter(cfg config.Config) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	accounts := mustOpenStore(cfg)
	limiter := auth.NewInviteFailLimiter(cfg.InviteCodeMaxFailsPerHour, accounts)
	disk, err := upload.NewLocal(cfg.UploadDir)
	if err != nil {
		log.Fatalf("upload dir: %v", err)
	}
	log.Printf("uploads: dir=%s", disk.Dir)
	h := handlers.New(cfg, limiter, accounts, disk)

	r.Get("/healthz", h.Healthz)

	r.Route("/v1", func(r chi.Router) {
		r.Post("/auth/apple", h.AuthApple)
		r.Post("/auth/google", h.AuthGoogle)
		if cfg.AuthMode == "stub" {
			r.Post("/auth/dev", h.AuthDev)
		}

		r.Group(func(r chi.Router) {
			r.Use(middleware.BearerAuth(cfg.JWTSecret))
			r.Get("/me", h.Me)
			r.Patch("/me", h.PatchMe)
			r.Get("/me/invite-code", h.GetInviteCode)
			r.Post("/me/invite-code/reset", h.ResetInviteCode)

			r.Get("/friends", h.ListFriends)
			r.Post("/friends", h.AddFriend)
			r.Delete("/friends/{friendshipID}", h.RemoveFriend)
			r.Post("/blocks", h.CreateBlock)

			r.Get("/tasks", h.ListTasks)
			r.Post("/tasks", h.CreateTask)
			r.Get("/tasks/{taskID}", h.GetTask)
			r.Patch("/tasks/{taskID}", h.PatchTask)
			r.Delete("/tasks/{taskID}", h.DeleteTask)
			r.Post("/tasks/{taskID}/complete", h.CompleteTask)
			r.Put("/tasks/{taskID}/verifier", h.SetTaskVerifier)

			r.Post("/uploads/screenshots", h.UploadScreenshot)
			r.Get("/screenshots/{screenshotKey}", h.GetScreenshot)

			r.Post("/rewards/{rewardID}/verifications", h.SubmitVerification)
			r.Get("/verifications/inbox", h.VerificationInbox)
			r.Get("/verifications/outbox", h.VerificationOutbox)
			r.Post("/verifications/{requestID}/approve", h.ApproveVerification)
			r.Post("/verifications/{requestID}/reject", h.RejectVerification)
			r.Post("/verifications/{requestID}/withdraw", h.WithdrawVerification)

			r.Get("/notices", h.ListNotices)
			r.Post("/notices/{noticeID}/read", h.MarkNoticeRead)
			r.Post("/reports", h.CreateReport)
		})

		r.Post("/ai/decompose", h.AIDecompose)
	})

	return r
}

func mustOpenStore(cfg config.Config) store.Store {
	switch cfg.StoreDriver {
	case "memory":
		log.Printf("store: memory (ephemeral; restarts wipe data)")
		return store.NewMemory()
	default:
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pg, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("store postgres: %v", err)
		}
		log.Printf("store: postgres (migrations applied)")
		return pg
	}
}
