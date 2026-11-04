package upload

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const MaxBytes = 4 << 20 // 4 MiB

// Local is a disk-backed object store for verification screenshots (MVP / local联调).
type Local struct {
	Dir string
}

func NewLocal(dir string) (*Local, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "./data/screenshots"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("upload dir: %w", err)
	}
	return &Local{Dir: dir}, nil
}

// SaveValidated reads at most MaxBytes+1, sniffs image type, writes as a new UUID key.
// Does not trust client Content-Type.
func (l *Local) SaveValidated(r io.Reader) (key, contentType string, byteSize int, err error) {
	limited := io.LimitReader(r, MaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", "", 0, err
	}
	if len(data) == 0 {
		return "", "", 0, fmt.Errorf("empty file")
	}
	if len(data) > MaxBytes {
		return "", "", 0, fmt.Errorf("file too large")
	}
	ct, ok := SniffImage(data)
	if !ok {
		return "", "", 0, fmt.Errorf("unsupported image type")
	}
	key = uuid.NewString()
	path := l.path(key)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", "", 0, err
	}
	return key, ct, len(data), nil
}

func (l *Local) Open(key string) (*os.File, error) {
	if !validKey(key) {
		return nil, fmt.Errorf("not found")
	}
	f, err := os.Open(l.path(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not found")
		}
		return nil, err
	}
	return f, nil
}

func (l *Local) Remove(key string) error {
	if !validKey(key) {
		return nil
	}
	err := os.Remove(l.path(key))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *Local) path(key string) string {
	return filepath.Join(l.Dir, key)
}

func validKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || strings.Contains(key, "/") || strings.Contains(key, "..") || strings.Contains(key, `\`) {
		return false
	}
	// UUID shape (accept with or without hyphens via uuid parse)
	if _, err := uuid.Parse(key); err != nil {
		return false
	}
	return true
}

// SniffImage returns a canonical content-type for JPEG/PNG/WebP, or false.
func SniffImage(data []byte) (string, bool) {
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg", true
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) {
		return "image/png", true
	}
	// RIFF....WEBP
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return "image/webp", true
	}
	return "", false
}
