use crate::{app::App, Result};
use clap::{ArgMatches, Command};
use flate2::read::GzDecoder;
use reqwest::Client;
use semver::Version;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::{
    io::{Read, Write},
    path::Path,
    time::Duration,
};
use url::{Host, Url};

const RELEASE_URL: &str = "https://us-construct-releases.s3.us-east-1.amazonaws.com";
const MAX_METADATA: usize = 1024 * 1024;
const MAX_ARTIFACT: usize = 128 * 1024 * 1024;
const MAX_ARCHIVE: u64 = 2 * MAX_ARTIFACT as u64;

pub fn command() -> Command {
    Command::new("update")
        .about("Update Construct to the latest version")
        .display_order(13)
}

#[derive(Deserialize)]
struct Manifest {
    version: String,
    artifacts: Vec<Artifact>,
}

#[derive(Deserialize)]
struct Artifact {
    platform: String,
    url: Url,
}

pub async fn run(app: &mut App, _: &ArgMatches) -> Result<()> {
    let config = |key, default: &'static str| -> Result<&str> {
        match app.config.get(key) {
            Some(value) => value
                .as_str()
                .ok_or_else(|| format!("{key} is not a string").into()),
            None => Ok(default),
        }
    };
    let release = Url::parse(config("update.url", RELEASE_URL)?)?;
    let channel = config("update.channel", "latest")?;
    if channel.is_empty()
        || !channel
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"._-".contains(&b))
    {
        return Err("invalid update.channel".into());
    }
    let client = update_client()?;
    let destination = std::env::current_exe()?.canonicalize()?;
    update(
        &client,
        &release,
        channel,
        super::info::VERSION,
        &destination,
        &mut app.stdout,
    )
    .await
}

async fn update(
    client: &Client,
    release: &Url,
    channel: &str,
    current: &str,
    destination: &Path,
    output: &mut (dyn Write + Send),
) -> Result<()> {
    let manifest_url = append_segments(release, &[&format!("{channel}.json")])?;
    let manifest: Manifest =
        serde_json::from_slice(&download(client, manifest_url, "manifest", MAX_METADATA).await?)?;
    if !is_newer(&manifest.version, current)? {
        writeln!(output, "Already up to date! Current version: {current}")?;
        return Ok(());
    }
    let platform = platform(std::env::consts::OS, std::env::consts::ARCH)?;
    let artifact = manifest
        .artifacts
        .into_iter()
        .find(|artifact| artifact.platform == platform)
        .ok_or_else(|| format!("artifact not found for platform: {platform}"))?;
    let checksum_url = append_segments(
        release,
        &[
            "versions",
            &manifest.version,
            &format!("{platform}_checksum.txt"),
        ],
    )?;
    let checksum = download(client, checksum_url, "checksum", MAX_METADATA).await?;
    let mut expected = [0u8; 32];
    hex::decode_to_slice(std::str::from_utf8(&checksum)?.trim(), &mut expected)
        .map_err(|_| "invalid SHA-256 checksum format")?;
    writeln!(output, "Updating from {current} to {}", manifest.version)?;
    let archive = download(client, artifact.url, "artifact", MAX_ARTIFACT).await?;
    replace_binary(&archive[..], &expected, destination)
}

fn append_segments(base: &Url, segments: &[&str]) -> Result<Url> {
    let mut url = base.clone();
    url.path_segments_mut()
        .map_err(|_| "invalid update URL")?
        .pop_if_empty()
        .extend(segments);
    Ok(url)
}

fn is_newer(latest: &str, current: &str) -> Result<bool> {
    let semver = |v: &str| Version::parse(v.strip_prefix('v').unwrap_or(v));
    let latest = semver(latest)?;
    Ok(current != "unknown" && latest.cmp_precedence(&semver(current)?).is_gt())
}

fn platform(os: &str, arch: &str) -> Result<String> {
    let os = match os {
        "linux" => "linux",
        "macos" => "darwin",
        os => return Err(format!("unsupported operating system: {os}").into()),
    };
    let arch = match arch {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        arch => return Err(format!("unsupported architecture: {arch}").into()),
    };
    Ok(format!("{os}_{arch}"))
}

fn permitted(url: &Url) -> bool {
    if !url.username().is_empty() || url.password().is_some() {
        return false;
    }
    match (url.scheme(), url.host()) {
        ("https", Some(_)) => true,
        ("http", Some(Host::Ipv4(ip))) => ip.is_loopback(),
        ("http", Some(Host::Ipv6(ip))) => ip.is_loopback(),
        ("http", Some(Host::Domain("localhost"))) => true,
        _ => false,
    }
}

fn update_client() -> Result<Client> {
    Ok(Client::builder()
        .connect_timeout(Duration::from_secs(10))
        .timeout(Duration::from_secs(180))
        .redirect(reqwest::redirect::Policy::custom(|attempt| {
            if attempt.previous().len() >= 10 || !permitted(attempt.url()) {
                attempt.error("unsafe update redirect or too many redirects")
            } else {
                attempt.follow()
            }
        }))
        .build()?)
}

async fn download(client: &Client, url: Url, kind: &str, limit: usize) -> Result<Vec<u8>> {
    if !permitted(&url) {
        return Err(
            format!("{kind} URL must use HTTPS (plain HTTP is only allowed on loopback)").into(),
        );
    }
    let error = |err: reqwest::Error| format!("failed to download {kind}: {}", err.without_url());
    let mut response = client.get(url).send().await.map_err(error)?;
    if response.status() != reqwest::StatusCode::OK {
        return Err(format!("failed to download {kind}: {}", response.status()).into());
    }
    let too_large = || format!("downloaded {kind} exceeds {limit} bytes");
    if response
        .content_length()
        .is_some_and(|len| len > limit as u64)
    {
        return Err(too_large().into());
    }
    let mut bytes = Vec::new();
    while let Some(chunk) = response.chunk().await.map_err(error)? {
        if chunk.len() > limit - bytes.len() {
            return Err(too_large().into());
        }
        bytes.extend_from_slice(&chunk);
    }
    Ok(bytes)
}

fn replace_binary(archive: impl Read, expected: &[u8; 32], destination: &Path) -> Result<()> {
    let mut archive = tar::Archive::new(GzDecoder::new(archive).take(MAX_ARCHIVE));
    for entry in archive.entries()? {
        let mut entry = entry?;
        if entry.path()?.as_ref() != Path::new("construct") {
            continue;
        }
        if !entry.header().entry_type().is_file()
            || entry.size() == 0
            || entry.size() > MAX_ARTIFACT as u64
        {
            return Err("archive contains an invalid construct executable".into());
        }
        let mut file = tempfile::NamedTempFile::new_in(
            destination
                .parent()
                .ok_or("executable directory not found")?,
        )?;
        let mut hasher = Sha256::new();
        let mut buffer = [0; 64 * 1024];
        loop {
            let read = entry.read(&mut buffer)?;
            if read == 0 {
                break;
            }
            file.write_all(&buffer[..read])?;
            hasher.update(&buffer[..read]);
        }
        if &hasher.finalize()[..] != expected {
            return Err("construct binary SHA-256 checksum mismatch".into());
        }
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mode = destination.metadata()?.permissions().mode() & 0o777;
            file.as_file()
                .set_permissions(std::fs::Permissions::from_mode(mode))?;
        }
        file.as_file().sync_all()?;
        file.persist(destination)?;
        return Ok(());
    }
    Err("construct binary not found in archive".into())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn semver_and_release_url_policy() {
        assert!(is_newer("v2.0.0", "2.0.0-rc.1").unwrap());
        assert!(!is_newer("2.0.0+new", "2.0.0+old").unwrap());
        assert!(!is_newer("2.0.0", "unknown").unwrap());
        assert!(is_newer("bad", "unknown").is_err());
        assert_eq!(platform("macos", "aarch64").unwrap(), "darwin_arm64");
        assert_eq!(platform("linux", "x86_64").unwrap(), "linux_amd64");
        assert!(permitted(&Url::parse("https://example.test").unwrap()));
        assert!(permitted(&Url::parse("http://[::1]").unwrap()));
        assert!(!permitted(&Url::parse("http://example.test").unwrap()));
        assert!(!permitted(
            &Url::parse("https://user:password@example.test").unwrap()
        ));
    }

    #[cfg(any(target_os = "linux", target_os = "macos"))]
    #[tokio::test]
    async fn update_downloads_release_and_only_replaces_after_checksum_verification() {
        use construct_connect::axum::{self, routing::get, Json, Router};
        use std::{fs, os::unix::fs::PermissionsExt};
        let new = b"new binary";
        let mut header = tar::Header::new_gnu();
        header.set_mode(0o755);
        header.set_size(new.len() as u64);
        let mut archive = tar::Builder::new(flate2::write::GzEncoder::new(
            Vec::new(),
            flate2::Compression::default(),
        ));
        archive
            .append_data(&mut header, "construct", &new[..])
            .unwrap();
        let archive = archive.into_inner().unwrap().finish().unwrap();
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let base = format!("http://{}/releases/", listener.local_addr().unwrap());
        let release = Url::parse(&base).unwrap();
        let platform = platform(std::env::consts::OS, std::env::consts::ARCH).unwrap();
        let manifest = serde_json::json!({ "version": "2.0.0", "artifacts": [
            {"platform": platform, "url": format!("{base}artifact.tar.gz")}
        ] });
        let checksum = std::sync::Arc::new(std::sync::Mutex::new("00".repeat(32)));
        let checksum_state = checksum.clone();
        let router = Router::new()
            .route(
                "/releases/beta.json",
                get(move || async move { Json(manifest) }),
            )
            .route(
                &format!("/releases/versions/2.0.0/{platform}_checksum.txt"),
                get(move || async move { format!("{}\n", checksum_state.lock().unwrap()) }),
            )
            .route(
                "/releases/artifact.tar.gz",
                get(move || async move { archive }),
            );
        let server = tokio::spawn(async move { axum::serve(listener, router).await.unwrap() });
        let dir = tempfile::tempdir().unwrap();
        let binary = dir.path().join("construct");
        fs::write(&binary, b"original").unwrap();
        fs::set_permissions(&binary, fs::Permissions::from_mode(0o751)).unwrap();
        let client = update_client().unwrap();
        let mut output = Vec::new();
        update(&client, &release, "beta", "2.0.0", &binary, &mut output)
            .await
            .unwrap();
        assert!(String::from_utf8_lossy(&output).contains("Already up to date"));
        assert!(
            update(&client, &release, "beta", "1.0.0", &binary, &mut output)
                .await
                .unwrap_err()
                .to_string()
                .contains("SHA-256 checksum mismatch")
        );
        assert_eq!(fs::read(&binary).unwrap(), b"original");
        assert_eq!(fs::read_dir(dir.path()).unwrap().count(), 1);
        *checksum.lock().unwrap() = hex::encode(Sha256::digest(new));
        update(&client, &release, "beta", "1.0.0", &binary, &mut output)
            .await
            .unwrap();
        assert_eq!(fs::read(&binary).unwrap(), new);
        assert_eq!(
            binary.metadata().unwrap().permissions().mode() & 0o777,
            0o751
        );
        server.abort();
    }
}
