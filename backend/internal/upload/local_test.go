package upload

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSniffImage(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0}
	ct, ok := SniffImage(png)
	if !ok || ct != "image/png" {
		t.Fatalf("png: %v %v", ct, ok)
	}
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0}
	ct, ok = SniffImage(jpeg)
	if !ok || ct != "image/jpeg" {
		t.Fatalf("jpeg: %v %v", ct, ok)
	}
	webp := append([]byte("RIFF"), make([]byte, 4)...)
	webp = append(webp, []byte("WEBP")...)
	ct, ok = SniffImage(webp)
	if !ok || ct != "image/webp" {
		t.Fatalf("webp: %v %v", ct, ok)
	}
	if _, ok := SniffImage([]byte("not an image")); ok {
		t.Fatal("expected reject")
	}
}

func TestSaveValidatedAndOpen(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	key, ct, n, err := l.SaveValidated(bytes.NewReader(png))
	if err != nil || ct != "image/png" || n != len(png) || key == "" {
		t.Fatalf("save: key=%s ct=%s n=%d err=%v", key, ct, n, err)
	}
	f, err := l.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, _ := os.ReadFile(f.Name())
	if !bytes.Equal(got, png) {
		t.Fatalf("bytes mismatch")
	}
	if _, _, _, err := l.SaveValidated(bytes.NewReader(make([]byte, MaxBytes+1))); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("want too large, got %v", err)
	}
}
