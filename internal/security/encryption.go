package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	nonceSize = 12 // 96 bits for GCM
)

// EncryptToken encrypts a plaintext token using AES-256-GCM
// Returns base64 encoded string containing nonce + ciphertext
func EncryptToken(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("encryption key must be 32 bytes (256 bits)")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	// Concatenate nonce + ciphertext
	combined := append(nonce, ciphertext...)

	// Base64 encode
	return base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptToken decrypts an encrypted token using AES-256-GCM
// Expects base64 encoded string containing nonce + ciphertext
func DecryptToken(encrypted string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("decryption key must be 32 bytes (256 bits)")
	}

	// Base64 decode
	combined, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	if len(combined) < nonceSize {
		return "", errors.New("encrypted data too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce and ciphertext
	nonce := combined[:nonceSize]
	ciphertext := combined[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

