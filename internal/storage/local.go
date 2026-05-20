package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage persists files on disk for development when S3 is unavailable.
type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) *LocalStorage {
	if baseDir == "" {
		baseDir = "uploads"
	}
	return &LocalStorage{baseDir: baseDir}
}

func (l *LocalStorage) UploadFile(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	_ = ctx
	_ = contentType
	target := filepath.Join(l.baseDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	file, err := os.Create(target)
	if err != nil {
		return "", fmt.Errorf("create upload file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, body); err != nil {
		return "", fmt.Errorf("write upload file: %w", err)
	}

	return "/uploads/" + filepath.ToSlash(key), nil
}

func (l *LocalStorage) DeleteFile(ctx context.Context, key string) error {
	_ = ctx
	target := filepath.Join(l.baseDir, filepath.FromSlash(key))
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *LocalStorage) GetSignedURL(ctx context.Context, key string) (string, error) {
	_ = ctx
	return "/uploads/" + filepath.ToSlash(key), nil
}
