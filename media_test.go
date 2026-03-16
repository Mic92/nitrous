package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fiatjaf.com/nostr"
)

func TestDetectContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filePath string
		data     []byte
		want     string
	}{
		{"png by extension", "photo.png", nil, "image/png"},
		{"jpeg by extension", "photo.jpg", nil, "image/jpeg"},
		{"gif by extension", "anim.gif", nil, "image/gif"},
		{"unknown extension uses sniffing", "data.zzz", []byte("<html>"), "text/html; charset=utf-8"},
		{"no extension uses sniffing", "noext", []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, "image/png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectContentType(tt.filePath, tt.data)
			if got != tt.want {
				t.Errorf("detectContentType(%q, ...) = %q, want %q", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"normal.txt", "normal.txt"},
		{"unsafe<>:\"/\\|?*.txt", "unsafe_________.txt"},
		{"", "attachment"},
		{".", "attachment"},
		{"/", "_"},
		{strings.Repeat("a", 300) + ".png", strings.Repeat("a", 196) + ".png"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleFileMessage_Unencrypted(t *testing.T) {
	t.Parallel()

	content := []byte("hello world file")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	peerPK := "deadbeef01234567deadbeef01234567deadbeef01234567deadbeef01234567"

	rumor := nostr.Event{
		Kind:    KindFileMessage,
		Content: srv.URL + "/test.txt",
		Tags: nostr.Tags{
			{"file-type", "text/plain"},
		},
	}

	result := handleFileMessage(context.Background(), rumor, cacheDir, peerPK)

	if !strings.Contains(result, "📎 file:") {
		t.Errorf("expected file display string, got %q", result)
	}

	// Verify the file was downloaded (directory uses truncated 8-char pubkey).
	attachDir := filepath.Join(cacheDir, "attachments", peerPK[:8])
	entries, err := os.ReadDir(attachDir)
	if err != nil {
		t.Fatalf("reading attachments dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no files downloaded")
	}

	data, err := os.ReadFile(filepath.Join(attachDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Errorf("downloaded content = %q, want %q", data, content)
	}
}

func TestHandleFileMessage_Encrypted(t *testing.T) {
	t.Parallel()

	plaintext := []byte("secret image data 🔐")

	enc, err := encryptFileForUpload(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	xHash := sha256.Sum256(enc.Ciphertext)
	xHex := hex.EncodeToString(xHash[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(enc.Ciphertext)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	peerPK := "aaaa000011112222aaaa000011112222aaaa000011112222aaaa000011112222"

	rumor := nostr.Event{
		Kind:    KindFileMessage,
		Content: srv.URL + "/encrypted.bin",
		Tags: nostr.Tags{
			{"file-type", "image/png"},
			{"encryption-algorithm", "aes-gcm"},
			{"decryption-key", enc.KeyHex},
			{"decryption-nonce", enc.NonceHex},
			{"x", xHex},
			{"ox", enc.OxHex},
		},
	}

	result := handleFileMessage(context.Background(), rumor, cacheDir, peerPK)

	if !strings.Contains(result, "📎 image:") {
		t.Errorf("expected image display string, got %q", result)
	}

	// Verify the file was decrypted (directory uses truncated 8-char pubkey).
	attachDir := filepath.Join(cacheDir, "attachments", peerPK[:8])
	entries, err := os.ReadDir(attachDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no files downloaded")
	}

	data, err := os.ReadFile(filepath.Join(attachDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(plaintext) {
		t.Errorf("decrypted content = %q, want %q", data, plaintext)
	}
}

func TestDownloadURL_SizeLimit(t *testing.T) {
	t.Parallel()

	// Serve a response larger than maxDownloadSize.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write maxDownloadSize + 2 bytes.
		buf := make([]byte, 8192)
		written := int64(0)
		for written < maxDownloadSize+2 {
			n := int64(len(buf))
			if written+n > maxDownloadSize+2 {
				n = maxDownloadSize + 2 - written
			}
			_, _ = w.Write(buf[:n])
			written += n
		}
	}))
	defer srv.Close()

	cacheDir := t.TempDir()

	_, err := downloadURL(context.Background(), srv.URL+"/big.bin", cacheDir, "testpeer")
	if err == nil {
		t.Fatal("expected error for oversized download")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify no partial file remains.
	attachDir := filepath.Join(cacheDir, "attachments", "testpeer"[:8])
	entries, _ := os.ReadDir(attachDir)
	if len(entries) > 0 {
		t.Errorf("partial file not cleaned up: found %d entries", len(entries))
	}
}

func TestHandleFileMessage_DownloadFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	peerPK := "bbbb000011112222bbbb000011112222bbbb000011112222bbbb000011112222"

	rumor := nostr.Event{
		Kind:    KindFileMessage,
		Content: srv.URL + "/missing.txt",
		Tags: nostr.Tags{
			{"file-type", "text/plain"},
		},
	}

	result := handleFileMessage(context.Background(), rumor, cacheDir, peerPK)

	if !strings.Contains(result, "download failed") {
		t.Errorf("expected download failed message, got %q", result)
	}
}

func TestHandleFileMessage_EmptyContent(t *testing.T) {
	t.Parallel()

	rumor := nostr.Event{
		Kind:    KindFileMessage,
		Content: "",
	}

	result := handleFileMessage(context.Background(), rumor, t.TempDir(), "peer")
	if !strings.Contains(result, "no URL") {
		t.Errorf("expected no URL message, got %q", result)
	}
}

func TestDownloadURL_PreservesExtension(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "data")
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	localPath, err := downloadURL(context.Background(), srv.URL+"/my%20file.txt", cacheDir, "peer1234")
	if err != nil {
		t.Fatal(err)
	}

	if ext := filepath.Ext(localPath); ext != ".txt" {
		t.Errorf("extension = %q, want .txt", ext)
	}

	// Verify truncated peer directory.
	if !strings.Contains(localPath, "peer1234") {
		t.Errorf("path %q does not contain truncated peer pk", localPath)
	}
}
