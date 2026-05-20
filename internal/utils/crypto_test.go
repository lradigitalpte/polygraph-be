package utils

import (
	"os"
	"testing"
)

func TestEncryptionDecryption(t *testing.T) {
	// Setup test environment variable
	os.Setenv("ENCRYPTION_KEY", "v7fX2pL9mK4nB8jW3hQ1sD5gR6tY0uE2")

	originalText := "Top Secret Polygraph Result"

	// Test Encrypt
	encrypted, err := Encrypt(originalText)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if encrypted == originalText {
		t.Fatal("Encrypted text is same as original")
	}

	// Test Decrypt
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if decrypted != originalText {
		t.Errorf("Decrypted text '%s' does not match original '%s'", decrypted, originalText)
	}
}

func TestEncryptionKeyLength(t *testing.T) {
	// Test with invalid key length
	os.Setenv("ENCRYPTION_KEY", "too-short")
	_, err := Encrypt("test")
	if err == nil {
		t.Fatal("Should have failed with short key")
	}

	os.Setenv("ENCRYPTION_KEY", "v7fX2pL9mK4nB8jW3hQ1sD5gR6tY0uE2") // Restore
}
