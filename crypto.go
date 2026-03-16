package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"syscall"

	"fiatjaf.com/nostr"
)

// encryptedFile holds the result of encrypting a file for Blossom upload.
// Ciphertext is written to a temporary file (CiphertextPath) instead of
// being held in memory, so the plaintext buffer can be freed immediately.
type encryptedFile struct {
	CiphertextPath string // path to temp file containing the ciphertext
	Size           int64  // size of the ciphertext in bytes
	SHA256Hex      string // hex-encoded SHA-256 of the ciphertext
	KeyHex         string // hex-encoded 256-bit AES key
	NonceHex       string // hex-encoded 12-byte GCM nonce
	OxHex          string // hex-encoded SHA-256 of the plaintext (pre-encryption hash)
}

// encryptFileForUpload mmaps the file at srcPath, encrypts it with a fresh
// AES-256-GCM key and nonce, and writes the ciphertext to a temporary file.
// The mmap is released immediately after gcm.Seal so plaintext pages are
// returned to the OS while the ciphertext is written to disk. The caller is
// responsible for removing CiphertextPath when done.
func encryptFileForUpload(srcPath string) (*encryptedFile, error) {
	plaintext, err := mmapFile(srcPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if len(plaintext) > 0 {
			_ = syscall.Munmap(plaintext)
		}
	}()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating AES key: %w", err)
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	oxHash := sha256.Sum256(plaintext)
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	ctHash := sha256.Sum256(ciphertext)

	ctPath, ctSize, err := writeTemp(ciphertext)
	if err != nil {
		return nil, err
	}

	return &encryptedFile{
		CiphertextPath: ctPath,
		Size:           ctSize,
		SHA256Hex:      hex.EncodeToString(ctHash[:]),
		KeyHex:         hex.EncodeToString(key),
		NonceHex:       hex.EncodeToString(nonce),
		OxHex:          hex.EncodeToString(oxHash[:]),
	}, nil
}

// mmapFile maps a file read-only into memory via the page cache.
// Returns an empty slice (not mmap-backed) for zero-length files.
func mmapFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if fi.Size() == 0 {
		return []byte{}, nil
	}

	return syscall.Mmap(int(f.Fd()), 0, int(fi.Size()), syscall.PROT_READ, syscall.MAP_PRIVATE)
}

// writeTemp writes data to a new temporary file and returns its path and size.
// The caller is responsible for removing the file.
func writeTemp(data []byte) (string, int64, error) {
	tmp, err := os.CreateTemp("", "nitrous-upload-*")
	if err != nil {
		return "", 0, fmt.Errorf("creating temp file: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", 0, fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", 0, fmt.Errorf("closing temp file: %w", err)
	}
	return name, int64(len(data)), nil
}

// decryptAESGCM decrypts ciphertext using AES-GCM with the given key and
// nonce. Uses NewGCMWithNonceSize to accept non-standard nonce lengths
// that some Nostr clients send in the wild.
func decryptAESGCM(key, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCMWithNonceSize(block, len(nonce))
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("AES-GCM decryption: %w", err)
	}

	return plaintext, nil
}

// parseDecryptionParams extracts and decodes the AES key and nonce from tags.
// It accepts 16- or 32-byte keys (AES-128/256) and does not validate nonce
// length here; decryptAESGCM uses NewGCMWithNonceSize to tolerate non-standard
// nonce lengths that some Nostr clients send in the wild.
func parseDecryptionParams(tags nostr.Tags) ([]byte, []byte, error) {
	keyHex := tagValue(tags, "decryption-key")
	nonceHex := tagValue(tags, "decryption-nonce")

	if keyHex == "" || nonceHex == "" {
		return nil, nil, errors.New("missing decryption-key or decryption-nonce tags")
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding decryption key: %w", err)
	}

	if len(key) != 16 && len(key) != 32 {
		return nil, nil, fmt.Errorf("decryption key must be 16 or 32 bytes (AES-128/256), got %d", len(key))
	}

	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding decryption nonce: %w", err)
	}

	return key, nonce, nil
}

// decryptFileInPlace reads the file at path, decrypts it using AES-GCM with
// the key and nonce from the rumor tags, verifies the SHA-256 hash against
// the "ox" tag (pre-encryption hash), and writes the plaintext back.
func decryptFileInPlace(filePath string, tags nostr.Tags) error {
	algo := tagValue(tags, "encryption-algorithm")
	if algo != "aes-gcm" {
		return fmt.Errorf("unsupported encryption algorithm: %q", algo)
	}

	key, nonce, err := parseDecryptionParams(tags)
	if err != nil {
		return err
	}

	ciphertext, err := mmapFile(filePath)
	if err != nil {
		return fmt.Errorf("reading encrypted file: %w", err)
	}

	plaintext, err := decryptAESGCM(key, nonce, ciphertext)
	// Unmap before writing back to the same file.
	if len(ciphertext) > 0 {
		_ = syscall.Munmap(ciphertext)
	}
	if err != nil {
		return err
	}

	// Verify against the pre-encryption hash if provided.
	if oxHex := tagValue(tags, "ox"); oxHex != "" {
		hash := sha256.Sum256(plaintext)
		if hex.EncodeToString(hash[:]) != oxHex {
			return errors.New("SHA-256 mismatch after decryption")
		}
	}

	if err := os.WriteFile(filePath, plaintext, 0o600); err != nil {
		return fmt.Errorf("writing decrypted file: %w", err)
	}

	return nil
}

// tagValue returns the value of the first tag with the given key, or "".
func tagValue(tags nostr.Tags, key string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == key {
			return tag[1]
		}
	}

	return ""
}
