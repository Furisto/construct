# construct-secret

Port of `backend/secret`. `Keyset::generate`, `Keyset::{from_json,to_json,
from_binary,to_binary}`, and `Encryption::{new,encrypt,decrypt}` implement the
cleartext Tink AES-GCM format. **Keyset serialization exposes secret key material**;
never log it. `load_or_generate` stores JSON as `encryption_key`, generating only
on `Error::NotFound` (never on a malformed key or storage failure).

AES-128/256-GCM version 0 key protos, standard base64 JSON bytes, protobuf binary
keysets, enabled-key rotation, TINK (`01 || big-endian key ID`), LEGACY/CRUNCHY
(`00 || big-endian key ID`) and RAW prefixes are supported. The body is a random
12-byte nonce followed by ciphertext and a 16-byte tag. Associated data is passed
unchanged, including for LEGACY. Other Tink algorithms are intentionally rejected.
Rust slices cannot represent Go's nil-vs-empty distinction: empty plaintext and
associated data are accepted; empty ciphertext fails authentication.

`Provider` exposes string get/set/delete. `FileProvider::new` takes the secret
directory directly; `from_data_home` appends `construct/secrets` to XDG_DATA_HOME
(normally `~/.local/share`). New directories/files use Unix modes 0700/0600;
existing permissions are preserved, like Go. Values are raw UTF-8, without
base64 or newline. Keys must be trusted filenames, as in the Go API.

`KeyringProvider` uses the service `construct` and the key as account. On Linux
it uses Secret Service directly because Go uses the attribute `username`, not
the `user` attribute used by Rust keyring implementations. Both use the login
collection. Missing file deletion is successful; missing keyring deletion is
`Error::NotFound`, matching Go. macOS/Windows use native `keyring` backends;
these have not been exercised on Linux.

Run `bash rust/scripts/secret-interop.sh` from the repository root for Go↔Rust
interoperability (also part of `go-interop.sh` CI). To include the live Linux
keyring, run it in an unlocked D-Bus session with `CONSTRUCT_TEST_KEYRING=1`.
Fixtures contain **test-only randomly generated keys**. Regenerate with:

```sh
go run ./backend/secret/testdata/interop generate rust/crates/construct-secret/tests/fixtures/go.json
```

The Go helper changes no production behavior. Rust emits fresh ciphertexts and
keysets at test time and Go checks both binary and JSON serialization.
