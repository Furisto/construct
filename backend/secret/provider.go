package secret

import (
	"fmt"

	"github.com/google/uuid"
)

// Provider defines the interface for secret storage backends.
type Provider interface {
	// Get retrieves a secret by key.
	Get(key string) (string, error)

	// Set stores a secret with the given key.
	Set(key string, value string) error

	// Delete removes a secret by key.
	Delete(key string) error
}

func ModelProviderAssociated(id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("model_provider:%s", id))
}

func EncryptionKeySecret() string {
	return "encryption_key"
}
