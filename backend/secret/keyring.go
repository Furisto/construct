package secret

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

const keychainService = "construct"

func ModelProviderSecret(id uuid.UUID) string {
	return fmt.Sprintf("model_provider/%s", id.String())
}

func GetSecret[T any](key string) (*T, error) {
	secret, err := keyring.Get(keychainService, key)
	if err != nil {
		return nil, err
	}
	var result T
	if err := json.Unmarshal([]byte(secret), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func SetSecret[T any](key string, secret *T) error {
	secretBytes, err := json.Marshal(secret)
	if err != nil {
		return err
	}
	return keyring.Set(keychainService, key, string(secretBytes))
}

func DeleteSecret(key string) error {
	return keyring.Delete(keychainService, key)
}
