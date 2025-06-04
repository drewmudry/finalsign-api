// internal/storage/s3.go
package storage

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

type S3Service struct {
	client        *s3.Client
	uploader      *manager.Uploader
	downloader    *manager.Downloader
	bucket        string
	region        string
	encryptionKey []byte // 32-byte AES-256 key
}

type UploadResult struct {
	S3Key      string
	S3Bucket   string
	FileHash   string // SHA-256 hash of original file
	FileSize   int64
	MimeType   string
	UploadedAt time.Time
}

type DownloadResult struct {
	Data     []byte
	FileHash string
	FileSize int64
	MimeType string
}

// NewS3Service creates a new S3 service instance with MinIO support
func NewS3Service() (*S3Service, error) {
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("AWS_S3_BUCKET environment variable is required")
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1" // default region
	}

	encryptionKeyHex := os.Getenv("DOCUMENT_ENCRYPTION_KEY")
	if encryptionKeyHex == "" {
		return nil, fmt.Errorf("DOCUMENT_ENCRYPTION_KEY environment variable is required (64 hex characters)")
	}

	encryptionKey, err := hex.DecodeString(encryptionKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid encryption key format: %w", err)
	}

	if len(encryptionKey) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes (64 hex characters)")
	}

	// Load AWS config with custom endpoint for MinIO
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with custom endpoint for MinIO
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		endpointURL := os.Getenv("AWS_ENDPOINT_URL")
		if endpointURL != "" {
			o.BaseEndpoint = aws.String(endpointURL)
			o.UsePathStyle = true // MinIO requires path-style addressing
		}
	})

	return &S3Service{
		client:        client,
		uploader:      manager.NewUploader(client),
		downloader:    manager.NewDownloader(client),
		bucket:        bucket,
		region:        region,
		encryptionKey: encryptionKey,
	}, nil
}

// UploadTemplate uploads a PDF template to S3 with encryption
func (s *S3Service) UploadTemplate(ctx context.Context, file multipart.File, header *multipart.FileHeader, userID int, workspaceID uuid.UUID) (*UploadResult, error) {
	// Validate file type
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		return nil, fmt.Errorf("only PDF files are allowed")
	}

	// Read file data
	fileData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Reset file position for potential reuse
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// Calculate hash of original file
	hash := sha256.Sum256(fileData)
	fileHash := hex.EncodeToString(hash[:])

	// Encrypt file data
	encryptedData, err := s.encryptData(fileData)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt file: %w", err)
	}

	// Generate S3 key
	templateID := uuid.New()
	s3Key := fmt.Sprintf("templates/%d/%s/%s.pdf", userID, workspaceID.String(), templateID.String())

	// Upload to S3
	uploadInput := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader(encryptedData),
		ContentType: aws.String("application/pdf"),
		Metadata: map[string]string{
			"original-filename": header.Filename,
			"user-id":           fmt.Sprintf("%d", userID),
			"workspace-id":      workspaceID.String(),
			"template-id":       templateID.String(),
			"original-hash":     fileHash,
			"encrypted":         "true",
		},
		ServerSideEncryption: types.ServerSideEncryptionAes256, // Additional S3-level encryption
	}

	_, err = s.uploader.Upload(ctx, uploadInput)
	if err != nil {
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	return &UploadResult{
		S3Key:      s3Key,
		S3Bucket:   s.bucket,
		FileHash:   fileHash,
		FileSize:   int64(len(fileData)),
		MimeType:   "application/pdf",
		UploadedAt: time.Now().UTC(),
	}, nil
}

// UploadSignedDocument uploads a completed/signed document to S3
func (s *S3Service) UploadSignedDocument(ctx context.Context, documentData []byte, userID int, workspaceID uuid.UUID, documentID uuid.UUID) (*UploadResult, error) {
	// Calculate hash of document
	hash := sha256.Sum256(documentData)
	fileHash := hex.EncodeToString(hash[:])

	// Encrypt document data
	encryptedData, err := s.encryptData(documentData)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt document: %w", err)
	}

	// Generate S3 key for signed document
	s3Key := fmt.Sprintf("documents/%d/%s/%s-signed.pdf", userID, workspaceID.String(), documentID.String())

	// Upload to S3
	uploadInput := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader(encryptedData),
		ContentType: aws.String("application/pdf"),
		Metadata: map[string]string{
			"user-id":       fmt.Sprintf("%d", userID),
			"workspace-id":  workspaceID.String(),
			"document-id":   documentID.String(),
			"document-hash": fileHash,
			"encrypted":     "true",
			"document-type": "signed",
		},
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	}

	_, err = s.uploader.Upload(ctx, uploadInput)
	if err != nil {
		return nil, fmt.Errorf("failed to upload signed document to S3: %w", err)
	}

	return &UploadResult{
		S3Key:      s3Key,
		S3Bucket:   s.bucket,
		FileHash:   fileHash,
		FileSize:   int64(len(documentData)),
		MimeType:   "application/pdf",
		UploadedAt: time.Now().UTC(),
	}, nil
}

// DownloadFile downloads and decrypts a file from S3
func (s *S3Service) DownloadFile(ctx context.Context, s3Key string) (*DownloadResult, error) {
	// Create a buffer to write the downloaded data
	buf := manager.NewWriteAtBuffer([]byte{})

	// Download from S3
	_, err := s.downloader.Download(ctx, buf, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download from S3: %w", err)
	}

	// Decrypt the data
	decryptedData, err := s.decryptData(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt file: %w", err)
	}

	// Calculate hash of decrypted data
	hash := sha256.Sum256(decryptedData)
	fileHash := hex.EncodeToString(hash[:])

	return &DownloadResult{
		Data:     decryptedData,
		FileHash: fileHash,
		FileSize: int64(len(decryptedData)),
		MimeType: "application/pdf",
	}, nil
}

// GeneratePresignedURL generates a presigned URL for temporary access (for previews, etc.)
func (s *S3Service) GeneratePresignedURL(ctx context.Context, s3Key string, expiration time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s.client)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return request.URL, nil
}

// DeleteFile deletes a file from S3
func (s *S3Service) DeleteFile(ctx context.Context, s3Key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete file from S3: %w", err)
	}

	return nil
}

// CheckFileExists checks if a file exists in S3
func (s *S3Service) CheckFileExists(ctx context.Context, s3Key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})

	if err != nil {
		// Check if it's a "not found" error
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}

// encryptData encrypts data using AES-256-GCM
func (s *S3Service) encryptData(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decryptData decrypts data using AES-256-GCM
func (s *S3Service) decryptData(encryptedData []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, fmt.Errorf("encrypted data too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]

	// Decrypt the data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}

// ValidateFileIntegrity validates a file against its stored hash
func (s *S3Service) ValidateFileIntegrity(data []byte, expectedHash string) error {
	hash := sha256.Sum256(data)
	actualHash := hex.EncodeToString(hash[:])

	if actualHash != expectedHash {
		return fmt.Errorf("file integrity check failed: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}
