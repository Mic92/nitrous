package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"fiatjaf.com/nostr"
)

func TestEncryptFileForUpload_RoundTrip(t *testing.T) {
	t.Parallel()

	plaintext := []byte("confidential file content 🔒")

	enc, err := encryptFileForUpload(plaintext)
	if err != nil {
		t.Fatalf("encryptFileForUpload: %v", err)
	}

	// Ciphertext must differ from plaintext.
	if string(enc.Ciphertext) == string(plaintext) {
		t.Fatal("ciphertext equals plaintext")
	}

	// Decrypt and verify round-trip.
	key, err := hex.DecodeString(enc.KeyHex)
	if err != nil {
		t.Fatalf("decoding key: %v", err)
	}

	nonce, err := hex.DecodeString(enc.NonceHex)
	if err != nil {
		t.Fatalf("decoding nonce: %v", err)
	}

	got, err := decryptAESGCM(key, nonce, enc.Ciphertext)
	if err != nil {
		t.Fatalf("decryptAESGCM: %v", err)
	}

	if string(got) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", got, plaintext)
	}

	// Verify ox hash matches plaintext SHA-256.
	wantHash := sha256.Sum256(plaintext)
	if enc.OxHex != hex.EncodeToString(wantHash[:]) {
		t.Errorf("OxHex = %s, want %s", enc.OxHex, hex.EncodeToString(wantHash[:]))
	}
}

func TestDecryptAESGCM_InvalidKeyLength(t *testing.T) {
	t.Parallel()

	// 15-byte key is invalid for AES.
	key := make([]byte, 15)
	nonce := make([]byte, 12)
	ciphertext := []byte("fake")

	_, err := decryptAESGCM(key, nonce, ciphertext)
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
}

func TestParseDecryptionParams_InvalidKeyLength(t *testing.T) {
	t.Parallel()

	// 24-byte key (not 16 or 32) should be rejected.
	key24 := make([]byte, 24)
	tags := nostr.Tags{
		{"decryption-key", hex.EncodeToString(key24)},
		{"decryption-nonce", hex.EncodeToString(make([]byte, 12))},
	}

	_, _, err := parseDecryptionParams(tags)
	if err == nil {
		t.Fatal("expected error for 24-byte key")
	}
}

func TestParseDecryptionParams_ValidKeys(t *testing.T) {
	t.Parallel()

	for _, keyLen := range []int{16, 32} {
		key := make([]byte, keyLen)
		nonce := make([]byte, 12)
		tags := nostr.Tags{
			{"decryption-key", hex.EncodeToString(key)},
			{"decryption-nonce", hex.EncodeToString(nonce)},
		}

		gotKey, gotNonce, err := parseDecryptionParams(tags)
		if err != nil {
			t.Fatalf("keyLen=%d: unexpected error: %v", keyLen, err)
		}
		if len(gotKey) != keyLen {
			t.Errorf("keyLen=%d: got key length %d", keyLen, len(gotKey))
		}
		if len(gotNonce) != 12 {
			t.Errorf("keyLen=%d: got nonce length %d", keyLen, len(gotNonce))
		}
	}
}

func TestDecryptAESGCM_NonStandardNonceLength(t *testing.T) {
	t.Parallel()

	// Encrypt with a 16-byte nonce (non-standard for GCM which defaults to 12).
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("non-standard nonce test")
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	got, err := decryptAESGCM(key, nonce, ciphertext)
	if err != nil {
		t.Fatalf("decryptAESGCM with 16-byte nonce: %v", err)
	}

	if string(got) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", got, plaintext)
	}
}

func TestDecryptFileInPlace_OxMismatch(t *testing.T) {
	t.Parallel()

	plaintext := []byte("original content")

	enc, err := encryptFileForUpload(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	fpath := filepath.Join(dir, "encrypted.bin")
	if err := os.WriteFile(fpath, enc.Ciphertext, 0o644); err != nil {
		t.Fatal(err)
	}

	tags := nostr.Tags{
		{"encryption-algorithm", "aes-gcm"},
		{"decryption-key", enc.KeyHex},
		{"decryption-nonce", enc.NonceHex},
		{"ox", "0000000000000000000000000000000000000000000000000000000000000000"}, // wrong hash
	}

	err = decryptFileInPlace(fpath, tags)
	if err == nil {
		t.Fatal("expected error for ox hash mismatch")
	}
}

func TestTagValue(t *testing.T) {
	t.Parallel()

	tags := nostr.Tags{
		{"p", "abc123"},
		{"file-type", "image/png"},
		{"encryption-algorithm", "aes-gcm"},
	}

	if got := tagValue(tags, "file-type"); got != "image/png" {
		t.Errorf("tagValue(file-type) = %q, want %q", got, "image/png")
	}

	if got := tagValue(tags, "missing"); got != "" {
		t.Errorf("tagValue(missing) = %q, want empty", got)
	}
}
