package secret

import (
	"bytes"
	"testing"
)

func TestClientEncryptDecrypt(t *testing.T) {
	keysetHandle, err := GenerateKeyset()
	if err != nil {
		t.Fatalf("Failed to generate keyset: %v", err)
	}

	client, err := NewClient(keysetHandle)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	plaintext := []byte("This is a secret message")
	associatedData := []byte("additional data")

	ciphertext, err := client.Encrypt(plaintext, associatedData)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("Ciphertext should be different from plaintext")
	}

	decrypted, err := client.Decrypt(ciphertext, associatedData)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("Decrypted data doesn't match original plaintext")
	}


	_, err = client.Decrypt(ciphertext, []byte("wrong data"))
	if err == nil {
		t.Fatal("Decryption with wrong associated data should fail")
	}
}

func TestKeysetSerialization(t *testing.T) {
	keysetHandle, err := GenerateKeyset()
	if err != nil {
		t.Fatalf("Failed to generate keyset: %v", err)
	}

	client, err := NewClient(keysetHandle)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	jsonStr, err := client.KeysetToJSON()
	if err != nil {
		t.Fatalf("Failed to serialize keyset: %v", err)
	}

	deserializedKeyset, err := client.KeysetFromJSON(jsonStr)
	if err != nil {
		t.Fatalf("Failed to deserialize keyset: %v", err)
	}

	newClient, err := NewClient(deserializedKeyset)
	if err != nil {
		t.Fatalf("Failed to create client with deserialized keyset: %v", err)
	}

	plaintext := []byte("Test message for serialization")
	associatedData := []byte("associated data")

	ciphertext, err := client.Encrypt(plaintext, associatedData)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	decrypted, err := newClient.Decrypt(ciphertext, associatedData)
	if err != nil {
		t.Fatalf("Decryption with deserialized keyset failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("Decrypted data doesn't match original plaintext")
	}
}
