package secret

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

type ErrSecretNotFound struct {
	Key string
	Err error
}

type ErrSecretMarshal struct {
	Key string
	Err error
}

type ErrSecretTooLarge struct {
	Key string
	Err error
}

func (e *ErrSecretNotFound) Error() string {
	return fmt.Sprintf("key %s not found: %s", e.Key, e.Err)
}

func (e *ErrSecretNotFound) Is(target error) bool {
	_, ok := target.(*ErrSecretNotFound)
	return ok
}

func (e *ErrSecretMarshal) Error() string {
	return fmt.Sprintf("failed to marshal secret %s: %s", e.Key, e.Err)
}

func (e *ErrSecretTooLarge) Error() string {
	return fmt.Sprintf("secret %s is too large: %s", e.Key, e.Err)
}

const keychainService = "construct"

func ModelProviderSecret(id uuid.UUID) string {
	return fmt.Sprintf("model_provider/%s", id.String())
}

func ModelProviderEncryptionKey() string {
	return "model_provider_encryption"
}

func GetSecret[T any](key string) (*T, error) {
	secret, err := keyring.Get(keychainService, key)
	if err != nil {
		return nil, toError(key, err)
	}
	var result T
	if err := json.Unmarshal([]byte(secret), &result); err != nil {
		return nil, &ErrSecretMarshal{Key: key, Err: err}
	}
	return &result, nil
}

func SetSecret[T any](key string, secret *T) error {
	secretBytes, err := json.Marshal(secret)
	if err != nil {
		return &ErrSecretMarshal{Key: key, Err: err}
	}
	err = keyring.Set(keychainService, key, string(secretBytes))
	if err != nil {
		return toError(key, err)
	}
	return nil
}

func DeleteSecret(key string) error {
	err := keyring.Delete(keychainService, key)
	if err != nil {
		return toError(key, err)
	}
	return nil
}

func toError(key string, err error) error {
	if err == keyring.ErrNotFound {
		return &ErrSecretNotFound{Key: key, Err: err}
	}

	if err == keyring.ErrSetDataTooBig {
		return &ErrSecretTooLarge{Key: key, Err: err}
	}

	return err
}
