package secret

import (
	"fmt"

	"github.com/tink-crypto/tink-go/aead"
	"github.com/tink-crypto/tink-go/core/registry"
	"github.com/tink-crypto/tink-go/integration/gcpkms"
	"github.com/tink-crypto/tink-go/keyset"
	"github.com/tink-crypto/tink-go/tink"
)

type Client struct {
	keyset *keyset.Handle
}

func NewClient(keyset *keyset.Handle) *Client {
	return &Client{keyset: keyset}
}

func GenerateKeyAndEncrypt(plaintext []byte) ([]byte, *keyset.Handle, error) {
	// Generate a new keyset handle for AEAD (Authenticated Encryption with Associated Data)
	keysetHandle, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return nil, nil, fmt.Errorf("keyset.NewHandle failed: %v", err)
	}

	// Get the AEAD primitive
	aeadPrimitive, err := aead.New(keysetHandle)
	if err != nil {
		return nil, nil, fmt.Errorf("aead.New failed: %v", err)
	}

	// Encrypt the plaintext
	ciphertext, err := aeadPrimitive.Encrypt(plaintext, nil) // nil is the additional authenticated data (AAD)
	if err != nil {
		return nil, nil, fmt.Errorf("encryption failed: %v", err)
	}

	return ciphertext, keysetHandle, nil
}

// Decrypt decrypts data using the provided keyset handle
func Decrypt(ciphertext []byte, keysetHandle *keyset.Handle) ([]byte, error) {
	// Get the AEAD primitive from the keyset handle
	aeadPrimitive, err := aead.New(keysetHandle)
	if err != nil {
		return nil, fmt.Errorf("aead.New failed: %v", err)
	}

	// Decrypt the ciphertext
	plaintext, err := aeadPrimitive.Decrypt(ciphertext, nil) // nil is the additional authenticated data (AAD)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %v", err)
	}

	return plaintext, nil
}

func SaveKeysetToFile(keysetHandle *keyset.Handle, filename string) error {
	// Create a writer for the file
	writer, err := keyset.JSONWriter.Write()
	if err != nil {
		return fmt.Errorf("keyset.NewJSONWriter failed: %v", err)
	}

	// Write the keyset
	if err := keysetHandle.Write(writer, nil); err != nil {
		return fmt.Errorf("keysetHandle.Write failed: %v", err)
	}

	return nil
}

// LoadKeysetFromFile loads a keyset from a file
func LoadKeysetFromFile(filename string) (*keyset.Handle, error) {
	// Create a reader for the file
	reader, err := keyset.NewJSONReader(openFile(filename))
	if err != nil {
		return nil, fmt.Errorf("keyset.NewJSONReader failed: %v", err)
	}

	// Read the keyset
	keysetHandle, err := keyset.Read(reader, nil)
	if err != nil {
		return nil, fmt.Errorf("keyset.Read failed: %v", err)
	}

	return keysetHandle, nil
}

