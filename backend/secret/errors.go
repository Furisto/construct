package secret

import "fmt"

type ErrSecretNotFound struct {
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

type ErrSecretMarshal struct {
	Key string
	Err error
}

func (e *ErrSecretMarshal) Error() string {
	return fmt.Sprintf("failed to marshal secret %s: %s", e.Key, e.Err)
}

type ErrSecretTooLarge struct {
	Key string
	Err error
}

func (e *ErrSecretTooLarge) Error() string {
	return fmt.Sprintf("secret %s is too large: %s", e.Key, e.Err)
}
