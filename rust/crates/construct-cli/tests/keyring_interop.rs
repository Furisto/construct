//! Opt-in real system keyring (Linux Secret Service, macOS Keychain) shared with Go.
#![cfg(any(target_os = "linux", target_os = "macos"))]
use construct::credentials::{Provider, SystemProvider};
use std::{fs, path::Path, process::Command};

#[test]
fn real_keyring_is_go_compatible() {
    if std::env::var("CONSTRUCT_KEYRING_INTEROP").as_deref() != Ok("1") {
        eprintln!(
            "SKIP real keyring: set CONSTRUCT_KEYRING_INTEROP=1 with an unlocked system keyring"
        );
        return;
    }
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../..");
    let dir = tempfile::tempdir().unwrap();
    let source = dir.path().join("keyring.go");
    fs::write(
        &source,
        include_str!("../../../scripts/keyring-interop.go.txt"),
    )
    .unwrap();
    let helper = dir.path().join("keyring-helper");
    assert!(Command::new("go")
        .current_dir(&root)
        .args(["build", "-o"])
        .arg(&helper)
        .arg(&source)
        .status()
        .unwrap()
        .success());
    let key = format!("rust-cli-interop-{}", std::process::id());
    let provider = SystemProvider;
    let go = |action: &str, value: Option<&str>| {
        let mut cmd = Command::new(&helper);
        cmd.args([action, &key]);
        if let Some(value) = value {
            cmd.arg(value);
        }
        let output = cmd.output().unwrap();
        assert!(
            output.status.success(),
            "{}",
            String::from_utf8_lossy(&output.stderr)
        );
        String::from_utf8(output.stdout).unwrap()
    };
    go("set", Some("go-test-token"));
    assert_eq!(provider.get(&key).unwrap(), "go-test-token");
    provider.set(&key, "rust-test-token").unwrap();
    assert_eq!(go("get", None), "rust-test-token");
    provider.delete(&key).unwrap();
    provider.set(&key, "new-rust-entry").unwrap();
    assert_eq!(go("get", None), "new-rust-entry");
    go("delete", None);
    eprintln!("[cli/keyring] real system keyring Go -> Rust, Rust -> Go, overwrite and delete verified (no tokens logged)");
}
