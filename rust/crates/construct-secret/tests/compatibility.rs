use base64::{engine::general_purpose::STANDARD, Engine};
use construct_secret::*;
use serde_json::{json, Value};

fn vectors() -> Vec<Value> {
    serde_json::from_str(include_str!("fixtures/go.json")).unwrap()
}
fn bytes(v: &Value, key: &str) -> Vec<u8> {
    STANDARD.decode(v[key].as_str().unwrap()).unwrap()
}

#[test]
fn go_vectors_json_binary_prefixes_and_authentication() {
    for v in vectors() {
        for k in [
            Keyset::from_json(&v["keyset"].to_string()).unwrap(),
            Keyset::from_binary(&bytes(&v, "binary")).unwrap(),
        ] {
            let k = Keyset::from_json(&k.to_json().unwrap()).unwrap();
            let c = Encryption::new(k).unwrap();
            let ct = bytes(&v, "ciphertext");
            let ad = bytes(&v, "ad");
            let p = bytes(&v, "plaintext");
            assert_eq!(c.decrypt(&ct, &ad).unwrap(), p);
            assert!(c.decrypt(&ct, b"wrong").is_err());
            let mut bad = ct.clone();
            *bad.last_mut().unwrap() ^= 1;
            assert!(c.decrypt(&bad, &ad).is_err());
            for end in 0..ct.len() {
                assert!(c.decrypt(&ct[..end], &ad).is_err());
            }
            let encrypted = c.encrypt(&p, &ad).unwrap();
            assert_eq!(c.decrypt(&encrypted, &ad).unwrap(), p);
            let empty = c.encrypt(b"", b"").unwrap();
            assert_eq!(c.decrypt(&empty, b"").unwrap(), b"");
        }
    }
}

#[test]
fn rotation_and_invalid_keysets() {
    let vs = vectors();
    let old = &vs[0];
    let new = &vs[2];
    let mut rotated = old["keyset"].clone();
    rotated["key"]
        .as_array_mut()
        .unwrap()
        .push(new["keyset"]["key"][0].clone());
    rotated["primaryKeyId"] = new["keyset"]["primaryKeyId"].clone();
    let c = Encryption::new(Keyset::from_json(&rotated.to_string()).unwrap()).unwrap();
    assert_eq!(
        c.decrypt(&bytes(old, "ciphertext"), &bytes(old, "ad"))
            .unwrap(),
        bytes(old, "plaintext")
    );
    rotated["key"][0]["status"] = json!("DISABLED");
    let c = Encryption::new(Keyset::from_json(&rotated.to_string()).unwrap()).unwrap();
    assert!(c
        .decrypt(&bytes(old, "ciphertext"), &bytes(old, "ad"))
        .is_err());
    for input in ["", "{}", "null", r#"{"primaryKeyId":1,"key":[]}"#] {
        assert!(Keyset::from_json(input).is_err());
    }
    assert!(Keyset::from_binary(b"").is_err());
    assert!(Keyset::from_binary(b"garbage").is_err());
    for (field, value) in [
        ("outputPrefixType", json!("UNKNOWN_PREFIX")),
        ("status", json!("DISABLED")),
    ] {
        let mut k = old["keyset"].clone();
        k["key"][0][field] = value;
        assert!(Keyset::from_json(&k.to_string()).is_err());
    }
    for (field, value) in [
        ("typeUrl", json!("unsupported")),
        ("value", json!("bad base64!")),
        ("value", json!("")),
        ("keyMaterialType", json!("REMOTE")),
    ] {
        let mut k = old["keyset"].clone();
        k["key"][0]["keyData"][field] = value;
        assert!(Keyset::from_json(&k.to_string()).is_err());
    }
}

#[test]
fn file_provider_and_key_loading() {
    let temp = tempfile::tempdir().unwrap();
    let p = FileProvider::from_data_home(temp.path()).unwrap();
    assert!(matches!(p.get("missing"), Err(Error::NotFound(_))));
    p.delete("missing").unwrap();
    p.set("test", "hello\0✓").unwrap();
    assert_eq!(p.get("test").unwrap(), "hello\0✓");
    p.set("test", "").unwrap();
    assert_eq!(p.get("test").unwrap(), "");
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let dir = temp.path().join("construct/secrets");
        assert_eq!(
            std::fs::metadata(&dir).unwrap().permissions().mode() & 0o777,
            0o700
        );
        assert_eq!(
            std::fs::metadata(dir.join("test"))
                .unwrap()
                .permissions()
                .mode()
                & 0o777,
            0o600
        );
    }
    p.delete("test").unwrap();
    assert!(p.set("missing/child", "value").is_err());
    let c = load_or_generate(&p).unwrap();
    let ct = c.encrypt(b"hello", b"ad").unwrap();
    assert_eq!(
        load_or_generate(&p).unwrap().decrypt(&ct, b"ad").unwrap(),
        b"hello"
    );
    p.set(encryption_key_secret(), "corrupt").unwrap();
    assert!(load_or_generate(&p).is_err());
    assert_eq!(p.get(encryption_key_secret()).unwrap(), "corrupt");
    assert_eq!(
        model_provider_associated(uuid::Uuid::nil()),
        b"model_provider:00000000-0000-0000-0000-000000000000"
    );
}
