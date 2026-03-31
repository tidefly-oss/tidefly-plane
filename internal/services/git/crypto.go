package git

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32
	saltLen       = 16
)

// HashSecret hashes a secret using Argon2id.
// Used for verification only — the hash cannot be reversed.
func HashSecret(secret string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey([]byte(secret), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// Format: base64(salt)$base64(hash)
	encoded := base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
	return encoded, nil
}

// EncryptSecret encrypts a secret using AES-256-GCM with the app key.
// The encrypted value can be decrypted later for use in API calls.
func EncryptSecret(secret, appKey string) (string, error) {
	key := deriveKey(appKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(secret), nil)
	return base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSecret decrypts an AES-256-GCM encrypted secret.
func DecryptSecret(encrypted, appKey string) (string, error) {
	key := deriveKey(appKey)

	data, err := base64.RawStdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("decoding secret: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting secret: %w", err)
	}

	return string(plaintext), nil
}

// deriveKey derives a 32-byte AES key from the app secret using Argon2id.
func deriveKey(appKey string) []byte {
	// Fixed salt for key derivation (not for password hashing)
	salt := []byte("tidefly-plane-git-key-v1")
	return argon2.IDKey([]byte(appKey), salt, 1, 64*1024, 4, 32)
}
