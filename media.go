package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fiatjaf.com/nostr"
)

// KindFileMessage is the NIP-17 kind for file messages (not yet in the nostr library).
const KindFileMessage nostr.Kind = 15

// maxDownloadSize caps the amount of data downloadURL will save to disk.
// 50 MiB is generous for images and voice memos while preventing abuse from
// multi-gigabyte payloads that could exhaust disk space or memory.
const maxDownloadSize = 50 << 20 // 50 MiB

// detectContentType determines the MIME type for a file from its extension,
// falling back to http.DetectContentType for byte sniffing.
func detectContentType(filePath string, data []byte) string {
	if ext := filepath.Ext(filePath); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			return ct
		}
	}

	return http.DetectContentType(data)
}

// detectContentTypeFromFile is like detectContentType but reads only the
// first 512 bytes for sniffing, avoiding loading the entire file.
func detectContentTypeFromFile(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return detectContentType(filePath, nil)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return detectContentType(filePath, buf[:n])
}

// downloadURL downloads a URL to the per-peer attachments directory.
// Returns the local file path. Files are stored in
// <cacheDir>/attachments/<peerPK>/.
func downloadURL(ctx context.Context, rawURL, cacheDir, peerPK string) (string, error) {
	// Extract filename from URL path.
	filename := "attachment"
	if parsed, parseErr := url.Parse(rawURL); parseErr == nil {
		filename = path.Base(parsed.Path)
	}
	filename = sanitizeFilename(filename)
	ext := filepath.Ext(filename)

	// Truncate peer pubkey to 8 hex chars for shorter directory names.
	dirPK := peerPK
	if len(dirPK) > 8 {
		dirPK = dirPK[:8]
	}
	downloadDir := filepath.Join(cacheDir, "attachments", dirPK)

	// Use a hash of the URL as a stable cache key so the same file
	// is not downloaded twice.
	urlHash := sha256.Sum256([]byte(rawURL))
	cachedName := hex.EncodeToString(urlHash[:8]) + ext
	cachedPath := filepath.Join(downloadDir, cachedName)
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return "", fmt.Errorf("creating attachments dir: %w", err)
	}

	f, err := os.Create(cachedPath)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Limit download size to prevent disk exhaustion.
	limited := io.LimitReader(resp.Body, maxDownloadSize+1)

	n, err := io.Copy(f, limited)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(cachedPath)
		return "", fmt.Errorf("writing file: %w", err)
	}

	if n > maxDownloadSize {
		_ = f.Close()
		_ = os.Remove(cachedPath)
		return "", fmt.Errorf("download exceeds maximum size of %d bytes", maxDownloadSize)
	}

	return cachedPath, nil
}

// unsafeFilenameChars matches characters that are unsafe in filenames across
// common filesystems (NTFS, ext4, HFS+, etc.).
var unsafeFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// sanitizeFilename removes filesystem-unsafe characters, trims the result,
// caps length, and falls back to "attachment" if nothing useful remains.
func sanitizeFilename(name string) string {
	name = unsafeFilenameChars.ReplaceAllString(name, "_")
	name = strings.TrimSpace(name)

	const maxLen = 200
	if len(name) > maxLen {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if len(base) > maxLen-len(ext) {
			base = base[:maxLen-len(ext)]
		}
		name = base + ext
	}

	if name == "" || name == "." || name == "/" {
		name = "attachment"
	}

	return name
}

// handleFileMessage processes a NIP-17 kind 15 file message. The rumor's
// content is the file URL and tags carry metadata (file-type, encryption params).
// The file is downloaded, decrypted if needed, and a display string is returned.
func handleFileMessage(ctx context.Context, rumor nostr.Event, cacheDir, peerPK string) string {
	fileURL := strings.TrimSpace(rumor.Content)
	if fileURL == "" {
		log.Printf("media: kind 15 file message with empty content from %s", peerPK)
		return "[file message with no URL]"
	}

	mimeType := tagValue(rumor.Tags, "file-type")

	log.Printf("media: downloading file message url=%s mime=%s sender=%s", fileURL, mimeType, peerPK)

	localPath, err := downloadURL(ctx, fileURL, cacheDir, peerPK)
	if err != nil {
		log.Printf("media: failed to download file message url=%s err=%v", fileURL, err)
		return fmt.Sprintf("📎 %s (download failed)", fileURL)
	}

	// Decrypt if the file was encrypted.
	if algo := tagValue(rumor.Tags, "encryption-algorithm"); algo != "" {
		if err := decryptFileInPlace(localPath, rumor.Tags); err != nil {
			log.Printf("media: failed to decrypt file message url=%s err=%v", fileURL, err)
			_ = os.Remove(localPath)
			return "📎 [encrypted file — decryption failed]"
		}
	}

	desc := "file"
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		desc = "image"
	case strings.HasPrefix(mimeType, "audio/"):
		desc = "audio"
	case strings.HasPrefix(mimeType, "video/"):
		desc = "video"
	}

	return fmt.Sprintf("📎 %s: %s", desc, localPath)
}
