package discord

import (
	"crypto/sha256"
	"encoding/hex"
)

// TokenFingerprintForAPI returns a stable fingerprint for a plaintext token.
// It is safe to store in DB and compare for deduplication.
func TokenFingerprintForAPI(tokenString string) string {
	sum := sha256.Sum256([]byte(tokenString))
	return hex.EncodeToString(sum[:])
}
