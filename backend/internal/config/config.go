package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr                  string
	DatabaseURL               string
	JWTSecret                 string
	AuthMode                  string // stub | apple
	AppleClientID             string // primary audience (usually iOS Bundle ID)
	AppleClientIDs            string // optional comma-separated extra audiences (e.g. Services ID)
	AppleTeamID               string // for client_secret / future token exchange; not required to verify identityToken
	AppleKeyID                string
	ApplePrivateKeyPEM        string // inline .p8 PEM (optional)
	ApplePrivateKeyPath       string // path to .p8 (optional)
	AIBaseURL                 string
	AIAPIKey                  string
	AIModel                   string
	InviteCodeMaxFailsPerHour int
	StoreDriver               string // memory | postgres (default postgres)
	UploadDir                 string // local screenshot files (default ./data/screenshots)
}

func Load() Config {
	mode := getenv("AUTH_MODE", "stub")
	if mode != "stub" && mode != "apple" {
		mode = "stub"
	}
	storeDriver := strings.ToLower(getenv("STORE_DRIVER", "postgres"))
	if storeDriver != "memory" && storeDriver != "postgres" {
		storeDriver = "postgres"
	}
	return Config{
		HTTPAddr:                  getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:               getenv("DATABASE_URL", "postgres://duibu:duibu@localhost:5432/duibu?sslmode=disable"),
		JWTSecret:                 getenv("JWT_SECRET", "change-me-in-production"),
		AuthMode:                  mode,
		AppleClientID:             os.Getenv("APPLE_CLIENT_ID"),
		AppleClientIDs:            os.Getenv("APPLE_CLIENT_IDS"),
		AppleTeamID:               os.Getenv("APPLE_TEAM_ID"),
		AppleKeyID:                os.Getenv("APPLE_KEY_ID"),
		ApplePrivateKeyPEM:        os.Getenv("APPLE_PRIVATE_KEY_PEM"),
		ApplePrivateKeyPath:       os.Getenv("APPLE_PRIVATE_KEY_PATH"),
		AIBaseURL:                 os.Getenv("AI_BASE_URL"),
		AIAPIKey:                  os.Getenv("AI_API_KEY"),
		AIModel:                   os.Getenv("AI_MODEL"),
		InviteCodeMaxFailsPerHour: getenvInt("INVITE_CODE_MAX_FAILS_PER_HOUR", 10),
		StoreDriver:               storeDriver,
		UploadDir:                 getenv("UPLOAD_DIR", "./data/screenshots"),
	}
}

// AppleAudiences returns Bundle ID / Services IDs used as JWT aud.
func (c Config) AppleAudiences() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(c.AppleClientID)
	for _, part := range strings.Split(c.AppleClientIDs, ",") {
		add(part)
	}
	return out
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
