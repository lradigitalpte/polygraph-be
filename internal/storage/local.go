package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// resolve maps a storage key to an on-disk path and guarantees the result stays
// inside baseDir, preventing directory traversal via "../" in user-controlled
// keys/filenames (gosec G304).
func (l *LocalStorage) resolve(key string) (string, error) {
	root, err := filepath.Abs(l.baseDir)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(key))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("invalid storage key")
	}
	return target, nil
}

func (l *LocalStorage) UploadFile(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	_ = ctx
	_ = contentType
	target, err := l.resolve(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	// #nosec G304 -- target is produced by resolve(), which rejects any key that
	// escapes baseDir, so this is not attacker-controlled path traversal.
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
	target, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *LocalStorage) GetSignedURL(ctx context.Context, key string) (string, error) {
	_ = ctx
	return "/uploads/" + filepath.ToSlash(key), nil
}
