package secret

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func TestKeyringProvider(t *testing.T) {
	provider := NewKeyringProvider()
	testKey := "test_keyring_key"
	testValue := "test_value_123"

	_ = provider.Delete(testKey)

	// Test Set
	err := provider.Set(testKey, testValue)
	if err != nil {
		t.Fatalf("Failed to set secret: %v", err)
	}

	// Test Get
	retrieved, err := provider.Get(testKey)
	if err != nil {
		t.Fatalf("Failed to get secret: %v", err)
	}

	if retrieved != testValue {
		t.Errorf("Expected %q, got %q", testValue, retrieved)
	}

	// Test Delete
	err = provider.Delete(testKey)
	if err != nil {
		t.Fatalf("Failed to delete secret: %v", err)
	}

	// Verify deletion
	_, err = provider.Get(testKey)
	if _, ok := err.(*ErrSecretNotFound); !ok {
		t.Errorf("Expected ErrSecretNotFound after deletion, got %v", err)
	}
}

func TestFileProvider(t *testing.T) {
	fs := afero.NewMemMapFs()
	basePath := "/secrets"

	provider, err := NewFileProvider(basePath, fs)
	if err != nil {
		t.Fatalf("Failed to create file provider: %v", err)
	}

	testKey := "test_file_key"
	testValue := "test_file_value_123"

	// Test Set
	err = provider.Set(testKey, testValue)
	if err != nil {
		t.Fatalf("Failed to set secret: %v", err)
	}

	// Verify file exists with correct permissions
	filePath := filepath.Join(basePath, testKey)
	info, err := fs.Stat(filePath)
	if err != nil {
		t.Fatalf("Secret file not created: %v", err)
	}

	// Check permissions (0600 = -rw-------)
	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %v", info.Mode().Perm())
	}

	// Test Get
	retrieved, err := provider.Get(testKey)
	if err != nil {
		t.Fatalf("Failed to get secret: %v", err)
	}

	if retrieved != testValue {
		t.Errorf("Expected %q, got %q", testValue, retrieved)
	}

	// Test Delete
	err = provider.Delete(testKey)
	if err != nil {
		t.Fatalf("Failed to delete secret: %v", err)
	}

	// Verify deletion
	_, err = provider.Get(testKey)
	if _, ok := err.(*ErrSecretNotFound); !ok {
		t.Errorf("Expected ErrSecretNotFound after deletion, got %v", err)
	}
}

func TestFileProviderNonExistentKey(t *testing.T) {
	fs := afero.NewMemMapFs()
	basePath := "/secrets"
	provider, err := NewFileProvider(basePath, fs)
	if err != nil {
		t.Fatalf("Failed to create file provider: %v", err)
	}

	_, err = provider.Get("nonexistent")
	if _, ok := err.(*ErrSecretNotFound); !ok {
		t.Errorf("Expected ErrSecretNotFound for non-existent key, got %v", err)
	}
}
