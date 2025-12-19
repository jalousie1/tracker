package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type R2Simulator struct {
	bucket   string
	endpoint string
}

func NewR2Simulator(bucket, endpoint string) *R2Simulator {
	return &R2Simulator{
		bucket:   strings.TrimSpace(bucket),
		endpoint: strings.TrimSpace(endpoint),
	}
}

// UploadAvatar simula upload e retorna uma url determinística.
// Implementa StorageClient interface (retorna empty string on error for compatibility)
func (r *R2Simulator) UploadAvatar(userID, avatarHash string, imageData []byte) (string, error) {
	if len(imageData) == 0 {
		return "", fmt.Errorf("empty image data")
	}
	return r.UploadAvatarSimulated(userID, avatarHash), nil
}

func (r *R2Simulator) UploadAvatarSimulated(userID, avatarHash string) string {
	sum := sha256.Sum256([]byte(userID + ":" + avatarHash))
	key := hex.EncodeToString(sum[:])

	ep := r.endpoint
	if ep == "" {
		ep = "https://r2.example.invalid"
	}
	bucket := r.bucket
	if bucket == "" {
		bucket = "identity-archive"
	}

	return fmt.Sprintf("%s/%s/avatars/%s.webp", strings.TrimRight(ep, "/"), bucket, key)
}


