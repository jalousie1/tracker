package storage

// StorageClient interface for avatar storage
type StorageClient interface {
	UploadAvatar(userID string, avatarHash string, imageData []byte) (string, error)
}

