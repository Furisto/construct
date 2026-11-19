// Interoperability fixture generator and verifier. All keys are test-only.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/furisto/construct/backend/secret"
	"github.com/tink-crypto/tink-go/insecurecleartextkeyset"
	"github.com/tink-crypto/tink-go/keyset"
)

type vector struct {
	Keyset     json.RawMessage `json:"keyset"`
	Binary     []byte          `json:"binary"`
	Plaintext  []byte          `json:"plaintext"`
	AD         []byte          `json:"ad"`
	Ciphertext []byte          `json:"ciphertext"`
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
func run() error {
	if os.Args[1] == "keyring" {
		p := secret.NewKeyringProvider()
		key := os.Args[3]
		switch os.Args[2] {
		case "set":
			return p.Set(key, "Go→Rust keyring ✓")
		case "get":
			s, err := p.Get(key)
			if err != nil {
				return err
			}
			if s != "Rust→Go keyring ✓" {
				return fmt.Errorf("unexpected value %q", s)
			}
			return p.Delete(key)
		}
	}
	path := os.Args[2]
	if os.Args[1] == "generate" {
		var vectors []vector
		for _, prefix := range []string{"TINK", "RAW", "LEGACY", "CRUNCHY"} {
			h, err := secret.GenerateKeyset()
			if err != nil {
				return err
			}
			s, err := secret.KeysetToJSON(h)
			if err != nil {
				return err
			}
			var object map[string]interface{}
			if err := json.Unmarshal([]byte(s), &object); err != nil {
				return err
			}
			object["key"].([]interface{})[0].(map[string]interface{})["outputPrefixType"] = prefix
			j, err := json.Marshal(object)
			if err != nil {
				return err
			}
			h, err = secret.KeysetFromJSON(string(j))
			if err != nil {
				return err
			}
			var binary bytes.Buffer
			if err := insecurecleartextkeyset.Write(h, keyset.NewBinaryWriter(&binary)); err != nil {
				return err
			}
			c, err := secret.NewClient(h)
			if err != nil {
				return err
			}
			for _, plain := range [][]byte{[]byte("Go→Rust secret\x00✓"), {}} {
				ad := []byte("model_provider:00000000-0000-0000-0000-000000000001")
				ct, err := c.Encrypt(plain, ad)
				if err != nil {
					return err
				}
				vectors = append(vectors, vector{j, binary.Bytes(), plain, ad, ct})
			}
		}
		b, err := json.MarshalIndent(vectors, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(path, append(b, '\n'), 0600)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var vectors []vector
	if err := json.Unmarshal(b, &vectors); err != nil {
		return err
	}
	for i, v := range vectors {
		h, err := secret.KeysetFromJSON(string(v.Keyset))
		if err != nil {
			return err
		}
		for _, handle := range []*keyset.Handle{h, nil} {
			if handle == nil {
				handle, err = insecurecleartextkeyset.Read(keyset.NewBinaryReader(bytes.NewReader(v.Binary)))
				if err != nil {
					return err
				}
			}
			c, err := secret.NewClient(handle)
			if err != nil {
				return err
			}
			p, err := c.Decrypt(v.Ciphertext, v.AD)
			if err != nil {
				return fmt.Errorf("vector %d: %w", i, err)
			}
			if !bytes.Equal(p, v.Plaintext) {
				return fmt.Errorf("vector %d mismatch: %s", i, base64.StdEncoding.EncodeToString(p))
			}
		}
	}
	fmt.Printf("Go decrypted %d Rust vectors (JSON and binary keysets) from %s\n", len(vectors), filepath.Base(path))
	return nil
}
