package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Storage defines the interface for file storage operations
type Storage interface {
	UploadFile(ctx context.Context, key string, body io.Reader, contentType string) (string, error)
	DeleteFile(ctx context.Context, key string) error
	GetSignedURL(ctx context.Context, key string) (string, error)
}

// S3Storage implements the Storage interface using AWS S3
type S3Storage struct {
	client *s3.Client
	bucket string
	region string
}

// NewS3Storage initializes a new S3 storage client
func NewS3Storage() (*S3Storage, error) {
	region := os.Getenv("AWS_DEFAULT_REGION")
	if region == "" {
		region = os.Getenv("AWS_S3_REGION")
	}
	bucket := os.Getenv("AWS_BUCKET")
	if bucket == "" {
		bucket = os.Getenv("AWS_S3_BUCKET")
	}
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if region == "" || bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("missing S3 configuration in environment")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}

	client := s3.NewFromConfig(cfg)

	return &S3Storage{
		client: client,
		bucket: bucket,
		region: region,
	}, nil
}

// UploadFile uploads a file to S3
func (s *S3Storage) UploadFile(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload: %w", err)
	}

	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key)
	return url, nil
}

// DeleteFile deletes a file from S3
func (s *S3Storage) DeleteFile(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// GetSignedURL generates a short-lived presigned URL for reading a private object.
// This lets the browser fetch the file directly from S3 without the bucket being
// public — the signature in the query string authorises the single GET.
func (s *S3Storage) GetSignedURL(ctx context.Context, key string) (string, error) {
	presigner := s3.NewPresignClient(s.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return "", fmt.Errorf("failed to presign url: %w", err)
	}
	return req.URL, nil
}
