package keyring

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

const ServiceName = "construct"

type ErrSecretNotFound struct {
	Key string
	Err error
}

func (e *ErrSecretNotFound) Error() string {
	return fmt.Sprintf("secret %q not found: %s", e.Key, e.Err)
}

func (e *ErrSecretNotFound) Is(target error) bool {
	_, ok := target.(*ErrSecretNotFound)
	return ok
}

func (e *ErrSecretNotFound) Unwrap() error {
	return e.Err
}

type ErrSecretTooLarge struct {
	Key string
	Err error
}

func (e *ErrSecretTooLarge) Error() string {
	return fmt.Sprintf("secret %q is too large: %s", e.Key, e.Err)
}

func (e *ErrSecretTooLarge) Is(target error) bool {
	_, ok := target.(*ErrSecretTooLarge)
	return ok
}

func (e *ErrSecretTooLarge) Unwrap() error {
	return e.Err
}

//go:generate mockgen -destination=../mocks/keyring_provider_mock.go -package=mocks . Provider
type Provider interface {
	Get(key string) (string, error)
	Set(key string, value string) error
	Delete(key string) error
}

type KeyringProvider struct {
	service string
}

func NewKeyringProvider() *KeyringProvider {
	return &KeyringProvider{
		service: ServiceName,
	}
}

func (k *KeyringProvider) Get(key string) (string, error) {
	secret, err := keyring.Get(k.service, key)
	if err != nil {
		return "", toError(key, err)
	}
	return secret, nil
}

func (k *KeyringProvider) Set(key string, value string) error {
	err := keyring.Set(k.service, key, value)
	if err != nil {
		return toError(key, err)
	}
	return nil
}

func (k *KeyringProvider) Delete(key string) error {
	err := keyring.Delete(k.service, key)
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

var _ Provider = (*KeyringProvider)(nil)
