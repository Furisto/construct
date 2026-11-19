use base64::{engine::general_purpose::STANDARD, Engine};
use construct_secret::{Encryption, KeyringProvider, Keyset, Provider};
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
struct Vector {
    keyset: serde_json::Value,
    binary: String,
    plaintext: String,
    ad: String,
    ciphertext: String,
}
fn main() {
    let args: Vec<_> = std::env::args().collect();
    if args[1] == "keyring" {
        let key = &args[2];
        assert_eq!(KeyringProvider.get(key).unwrap(), "Go→Rust keyring ✓");
        KeyringProvider.delete(key).unwrap();
        assert!(matches!(
            KeyringProvider.get(key),
            Err(construct_secret::Error::NotFound(_))
        ));
        assert!(matches!(
            KeyringProvider.delete(key),
            Err(construct_secret::Error::NotFound(_))
        ));
        KeyringProvider.set(key, "").unwrap();
        assert_eq!(KeyringProvider.get(key).unwrap(), "");
        KeyringProvider.set(key, "Rust→Go keyring ✓").unwrap();
        return;
    }
    let vectors: Vec<Vector> =
        serde_json::from_str(include_str!("../tests/fixtures/go.json")).unwrap();
    let mut out = vec![];
    for v in vectors {
        let key = Keyset::from_json(&v.keyset.to_string()).unwrap();
        let c = Encryption::new(key.clone()).unwrap();
        let p = STANDARD.decode(&v.plaintext).unwrap();
        let ad = STANDARD.decode(&v.ad).unwrap();
        let ct = STANDARD.decode(&v.ciphertext).unwrap();
        assert_eq!(c.decrypt(&ct, &ad).unwrap(), p);
        let generated = Keyset::generate();
        for k in [key, generated] {
            let encryption = Encryption::new(k.clone()).unwrap();
            out.push(Vector {
                keyset: serde_json::from_str(&k.to_json().unwrap()).unwrap(),
                binary: STANDARD.encode(k.to_binary()),
                plaintext: STANDARD.encode(&p),
                ad: STANDARD.encode(&ad),
                ciphertext: STANDARD.encode(encryption.encrypt(&p, &ad).unwrap()),
            });
        }
    }
    std::fs::write(&args[1], serde_json::to_vec_pretty(&out).unwrap()).unwrap();
    println!(
        "Rust decrypted 8 Go vectors; emitted {} Rust vectors",
        out.len()
    );
}
