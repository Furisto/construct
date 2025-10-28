package secret

import (
	"github.com/zalando/go-keyring"
)

const keychainService = "construct"

type KeyringProvider struct{}

func NewKeyringProvider() *KeyringProvider {
	return &KeyringProvider{}
}

func (k *KeyringProvider) Get(key string) (string, error) {
	secret, err := keyring.Get(keychainService, key)
	if err != nil {
		return "", toError(key, err)
	}
	return secret, nil
}

func (k *KeyringProvider) Set(key string, value string) error {
	err := keyring.Set(keychainService, key, value)
	if err != nil {
		return toError(key, err)
	}
	return nil
}

func (k *KeyringProvider) Delete(key string) error {
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
