package secret

import (
	"bytes"
	"fmt"

	"github.com/tink-crypto/tink-go/aead"
	"github.com/tink-crypto/tink-go/keyset"
	"github.com/tink-crypto/tink-go/tink"
	"github.com/tink-crypto/tink-go/insecurecleartextkeyset"
)

func GenerateKeyset() (*keyset.Handle, error) {
	return keyset.NewHandle(aead.AES256GCMKeyTemplate())
}

type Client struct {
	keyset *keyset.Handle
	aead   tink.AEAD
}

func NewClient(keysetHandle *keyset.Handle) (*Client, error) {
	aeadPrimitive, err := aead.New(keysetHandle)
	if err != nil {
		return nil, fmt.Errorf("aead.New failed: %v", err)
	}

	return &Client{
		keyset: keysetHandle,
		aead:   aeadPrimitive,
	}, nil
}

func (c *Client) Encrypt(plaintext []byte, associatedData []byte) ([]byte, error) {
	if plaintext == nil {
		return nil, fmt.Errorf("plaintext cannot be nil")
	}

	ciphertext, err := c.aead.Encrypt(plaintext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("encryption failed: %v", err)
	}

	return ciphertext, nil
}

func (c *Client) Decrypt(ciphertext []byte, associatedData []byte) ([]byte, error) {
	if ciphertext == nil {
		return nil, fmt.Errorf("ciphertext cannot be nil")
	}

	plaintext, err := c.aead.Decrypt(ciphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %v", err)
	}

	return plaintext, nil
}

func KeysetToJSON(keysetHandle *keyset.Handle) (string, error) {
	buf := new(bytes.Buffer)
	writer := keyset.NewJSONWriter(buf)

	if err := insecurecleartextkeyset.Write(keysetHandle, writer); err != nil {
		return "", fmt.Errorf("failed to write keyset to JSON: %v", err)
	}

	return buf.String(), nil
}

func KeysetFromJSON(jsonKeyset string) (*keyset.Handle, error) {
	reader := keyset.NewJSONReader(bytes.NewBufferString(jsonKeyset))

	keysetHandle, err := insecurecleartextkeyset.Read(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read keyset from JSON: %v", err)
	}

	return keysetHandle, nil
}
