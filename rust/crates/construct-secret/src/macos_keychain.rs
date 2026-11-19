//! macOS Keychain access through `/usr/bin/security`, as zalando/go-keyring does.
//! Items written by the Go and Rust binaries then share the same access list
//! (the `security` tool), so neither binary triggers a Keychain password prompt.
use base64::{engine::general_purpose::STANDARD, Engine};
use std::{
    io::Write,
    process::{Command, Output, Stdio},
};

const SECURITY: &str = "/usr/bin/security";
const HEX_PREFIX: &str = "go-keyring-encoded:";
const BASE64_PREFIX: &str = "go-keyring-base64:";
/// go-keyring rejects `security -i` command lines longer than this.
const MAX_COMMAND_LEN: usize = 4096;

#[derive(Debug, PartialEq, Eq)]
pub enum KeychainError {
    NotFound,
    TooLarge,
    Other(String),
}

fn failure(output: &Output) -> KeychainError {
    let text = format!(
        "{}{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    if text.contains("could not be found") {
        KeychainError::NotFound
    } else {
        KeychainError::Other(format!("{SECURITY}: {}: {}", output.status, text.trim()))
    }
}

fn run(args: &[&str]) -> Result<Output, KeychainError> {
    Command::new(SECURITY)
        .args(args)
        .output()
        .map_err(|e| KeychainError::Other(e.to_string()))
}

/// Shell quoting identical to al.essio.dev/pkg/shellescape.Quote.
fn quote(value: &str) -> String {
    if value.is_empty() {
        return "''".into();
    }
    let safe = |c: char| c.is_ascii_alphanumeric() || "_@%+=:,./-".contains(c);
    if value.chars().all(safe) {
        value.into()
    } else {
        format!("'{}'", value.replace('\'', "'\"'\"'"))
    }
}

fn decode(stored: &str) -> Result<String, KeychainError> {
    let invalid = |e: String| KeychainError::Other(e);
    let bytes = if let Some(encoded) = stored.strip_prefix(HEX_PREFIX) {
        (0..encoded.len())
            .step_by(2)
            .map(|i| {
                encoded
                    .get(i..i + 2)
                    .and_then(|pair| u8::from_str_radix(pair, 16).ok())
                    .ok_or_else(|| invalid("invalid hex secret".into()))
            })
            .collect::<Result<Vec<_>, _>>()?
    } else if let Some(encoded) = stored.strip_prefix(BASE64_PREFIX) {
        STANDARD
            .decode(encoded)
            .map_err(|e| invalid(e.to_string()))?
    } else {
        return Ok(stored.into());
    };
    String::from_utf8(bytes).map_err(|e| invalid(e.to_string()))
}

pub fn get(service: &str, account: &str) -> Result<String, KeychainError> {
    let output = run(&["find-generic-password", "-s", service, "-wa", account])?;
    if !output.status.success() {
        return Err(failure(&output));
    }
    decode(String::from_utf8_lossy(&output.stdout).trim())
}

pub fn set(service: &str, account: &str, value: &str) -> Result<(), KeychainError> {
    // `-i` reads the command from stdin so the secret never appears in argv.
    let value = format!("{BASE64_PREFIX}{}", STANDARD.encode(value));
    let command = format!(
        "add-generic-password -U -s {} -a {} -w {}\n",
        quote(service),
        quote(account),
        quote(&value)
    );
    if command.len() > MAX_COMMAND_LEN {
        return Err(KeychainError::TooLarge);
    }
    let other = |e: std::io::Error| KeychainError::Other(e.to_string());
    let mut child = Command::new(SECURITY)
        .arg("-i")
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(other)?;
    let mut stdin = child.stdin.take().expect("piped stdin");
    stdin.write_all(command.as_bytes()).map_err(other)?;
    drop(stdin);
    let output = child.wait_with_output().map_err(other)?;
    if output.status.success() {
        Ok(())
    } else {
        Err(failure(&output))
    }
}

pub fn delete(service: &str, account: &str) -> Result<(), KeychainError> {
    let output = run(&["delete-generic-password", "-s", service, "-a", account])?;
    if output.status.success() {
        Ok(())
    } else {
        Err(failure(&output))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn quote_matches_shellescape() {
        assert_eq!(quote(""), "''");
        assert_eq!(quote("construct"), "construct");
        assert_eq!(quote("go-keyring-base64:YQ=="), "go-keyring-base64:YQ==");
        assert_eq!(quote("a b"), "'a b'");
        assert_eq!(quote("it's"), "'it'\"'\"'s'");
    }

    #[test]
    fn decode_matches_go_keyring() {
        assert_eq!(decode("plain").unwrap(), "plain");
        assert_eq!(decode("go-keyring-base64:4pyT").unwrap(), "✓");
        assert_eq!(decode("go-keyring-encoded:6869").unwrap(), "hi");
        assert!(decode("go-keyring-encoded:6").is_err());
    }

    #[test]
    fn oversized_values_are_rejected_before_running_security() {
        assert_eq!(
            set("construct", "too-large", &"x".repeat(4096)),
            Err(KeychainError::TooLarge)
        );
    }
}
