package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/disintegration/imaging"
)

type S3Client struct {
	client     *s3.Client
	bucket     string
	publicURL  string
	httpClient *http.Client
}

type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PublicURL       string
	Region          string
}

func NewS3Client(cfg S3Config) (*S3Client, error) {
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	// Custom endpoint resolver for R2
	if cfg.Endpoint != "" {
		awsCfg.BaseEndpoint = aws.String(cfg.Endpoint)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})

	return &S3Client{
		client:     client,
		bucket:     cfg.Bucket,
		publicURL:  cfg.PublicURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (s *S3Client) UploadAvatar(userID string, avatarHash string, imageData []byte) (string, error) {
	// Validate image
	if len(imageData) == 0 {
		return "", fmt.Errorf("empty image data")
	}

	if len(imageData) > 5*1024*1024 { // 5MB max
		return "", fmt.Errorf("image too large: %d bytes", len(imageData))
	}

	// Calculate SHA-256 for deduplication
	hash := sha256.Sum256(imageData)
	hashHex := hex.EncodeToString(hash[:])

	// Check if already exists in DB (this should be done by caller)
	// For now, proceed with upload

	// Resize image to max 512x512
	resized, err := imaging.Decode(bytes.NewReader(imageData))
	if err != nil {
		return "", fmt.Errorf("failed to decode image: %w", err)
	}

	resized = imaging.Fit(resized, 512, 512, imaging.Lanczos)

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, resized, imaging.PNG); err != nil {
		return "", fmt.Errorf("failed to encode image: %w", err)
	}

	imageData = buf.Bytes()

	// Generate object key
	timestamp := time.Now().Unix()
	objectKey := fmt.Sprintf("avatars/%s/%d_%s.png", userID, timestamp, avatarHash)

	// Upload to S3/R2
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(imageData),
		ContentType: aws.String("image/png"),
		Metadata: map[string]string{
			"user_id":     userID,
			"avatar_hash": avatarHash,
			"image_hash":  hashHex,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Construct public URL
	if s.publicURL != "" {
		return fmt.Sprintf("%s/%s", s.publicURL, objectKey), nil
	}

	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.bucket, objectKey), nil
}

func (s *S3Client) DownloadAvatarFromDiscord(userID, avatarHash string) ([]byte, error) {
	url := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png?size=1024", userID, avatarHash)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download avatar: status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
		return nil, fmt.Errorf("invalid content type: %s", contentType)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if len(data) > 5*1024*1024 {
		return nil, fmt.Errorf("image too large: %d bytes", len(data))
	}

	return data, nil
}

// UploadAvatar implements StorageClient interface
func (s *S3Client) UploadAvatarFromDiscord(userID, avatarHash string) (string, error) {
	// Download from Discord CDN
	imageData, err := s.DownloadAvatarFromDiscord(userID, avatarHash)
	if err != nil {
		return "", fmt.Errorf("failed to download avatar: %w", err)
	}

	// Upload to R2
	return s.UploadAvatar(userID, avatarHash, imageData)
}

