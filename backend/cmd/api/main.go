package main

import (
	"log"
	"net/http"
	"time"

	"github.com/vincent-codeworld/duibu-adhd/backend/internal/config"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/httpapi"
)

func main() {
	cfg := config.Load()
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("duibu api listening on %s", cfg.HTTPAddr)
	log.Fatal(srv.ListenAndServe())
}
