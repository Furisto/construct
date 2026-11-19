//! `skills.lock.json` access, serialized with the Go daemon through flock on
//! `skills.lock.json.lock` and written atomically via a temp file and rename.
use crate::{LockEntry, LockFile, SkillError};
use std::collections::BTreeMap;
use std::fs::{File, OpenOptions};
use std::io::{ErrorKind, Write};
use std::path::{Path, PathBuf};

/// Lock file name inside the Construct config directory.
pub const LOCK_FILE_NAME: &str = "skills.lock.json";
/// Current lock file `version`.
pub const LOCK_FILE_VERSION: i64 = 1;

fn empty() -> LockFile {
    LockFile {
        version: LOCK_FILE_VERSION,
        skills: Some(BTreeMap::new()),
    }
}

fn open_flock(lock_path: &Path) -> Result<File, SkillError> {
    let mut path = lock_path.as_os_str().to_owned();
    path.push(".lock");
    let mut options = OpenOptions::new();
    options.create(true).truncate(false).read(true).write(true);
    #[cfg(unix)]
    std::os::unix::fs::OpenOptionsExt::mode(&mut options, 0o600);
    options
        .open(PathBuf::from(path))
        .map_err(|e| SkillError::other("failed to open lock file", e))
}

fn read(lock_path: &Path) -> Result<Option<LockFile>, SkillError> {
    let content = match std::fs::read(lock_path) {
        Ok(content) => content,
        Err(e) if e.kind() == ErrorKind::NotFound => return Ok(None),
        Err(e) => return Err(SkillError::other("failed to read lock file", e)),
    };
    serde_json::from_slice(&content)
        .map(Some)
        .map_err(|e| SkillError::other("failed to parse lock file", e))
}

/// Read the lock file under a shared lock; a missing file is empty.
pub(crate) fn load(lock_path: &Path) -> Result<LockFile, SkillError> {
    let flock = open_flock(lock_path)?;
    flock
        .lock_shared()
        .map_err(|e| SkillError::other("failed to acquire read lock", e))?;
    let mut lock_file = read(lock_path)?.unwrap_or_else(empty);
    lock_file.skills.get_or_insert_with(BTreeMap::new);
    Ok(lock_file)
}

/// Insert or replace one entry under an exclusive lock.
pub(crate) fn upsert(lock_path: &Path, name: &str, entry: LockEntry) -> Result<(), SkillError> {
    let flock = open_flock(lock_path)?;
    flock
        .lock()
        .map_err(|e| SkillError::other("failed to acquire write lock", e))?;
    let mut lock_file = read(lock_path)?.unwrap_or_else(empty);
    lock_file
        .skills
        .get_or_insert_with(BTreeMap::new)
        .insert(name.to_owned(), Some(Box::new(entry)));
    write_atomic(lock_path, &lock_file)
}

/// Remove one entry under an exclusive lock; a missing file is a no-op.
pub(crate) fn remove(lock_path: &Path, name: &str) -> Result<(), SkillError> {
    let flock = open_flock(lock_path)?;
    flock
        .lock()
        .map_err(|e| SkillError::other("failed to acquire write lock", e))?;
    let Some(mut lock_file) = read(lock_path)? else {
        return Ok(());
    };
    if let Some(skills) = lock_file.skills.as_mut() {
        skills.remove(name);
    }
    write_atomic(lock_path, &lock_file)
}

fn write_atomic(lock_path: &Path, lock_file: &LockFile) -> Result<(), SkillError> {
    let compact = construct_types::to_go_json(lock_file)
        .map_err(|e| SkillError::other("failed to marshal lock file", e))?;
    let content = indent(&compact);
    let dir = lock_path.parent().unwrap_or(Path::new("."));
    let mut temp = tempfile::Builder::new()
        .prefix("skills.lock.")
        .suffix(".tmp")
        .tempfile_in(dir)
        .map_err(|e| SkillError::other("failed to create temp file", e))?;
    temp.write_all(&content)
        .map_err(|e| SkillError::other("failed to write temp file", e))?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        // Go's afero.TempFile creates 0600 and keeps it across the rename.
        let _ = temp
            .as_file()
            .set_permissions(std::fs::Permissions::from_mode(0o600));
    }
    temp.persist(lock_path)
        .map_err(|e| SkillError::other("failed to rename temp file", e.error))?;
    Ok(())
}

/// Go `json.Indent(dst, src, "", "  ")` over compact Go JSON, as `json.MarshalIndent` does.
fn indent(src: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(src.len() * 2);
    let mut depth = 0usize;
    let mut in_string = false;
    let mut escaped = false;
    let mut need_indent = false;
    let newline = |out: &mut Vec<u8>, depth: usize| {
        out.push(b'\n');
        out.extend(std::iter::repeat_n(b' ', depth * 2));
    };
    for &c in src {
        if in_string {
            out.push(c);
            if escaped {
                escaped = false;
            } else if c == b'\\' {
                escaped = true;
            } else if c == b'"' {
                in_string = false;
            }
            continue;
        }
        if need_indent && c != b']' && c != b'}' {
            need_indent = false;
            depth += 1;
            newline(&mut out, depth);
        }
        match c {
            b'"' => {
                in_string = true;
                out.push(c);
            }
            b'{' | b'[' => {
                out.push(c);
                need_indent = true;
            }
            b',' => {
                out.push(c);
                newline(&mut out, depth);
            }
            b':' => out.extend_from_slice(b": "),
            b'}' | b']' => {
                if need_indent {
                    need_indent = false;
                } else {
                    depth -= 1;
                    newline(&mut out, depth);
                }
                out.push(c);
            }
            _ => out.push(c),
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{GitSource, URLSource};
    use construct_types::Timestamp;

    fn ts(s: &str) -> Timestamp {
        serde_json::from_value(serde_json::Value::String(s.into())).unwrap()
    }

    #[test]
    fn missing_file_loads_empty() {
        let tmp = tempfile::tempdir().unwrap();
        let lock = load(&tmp.path().join(LOCK_FILE_NAME)).unwrap();
        assert_eq!(lock.version, 1);
        assert!(lock.skills.unwrap().is_empty());
        remove(&tmp.path().join(LOCK_FILE_NAME), "absent").unwrap();
        assert!(!tmp.path().join(LOCK_FILE_NAME).exists());
    }

    #[test]
    fn writes_go_marshal_indent_bytes() {
        let tmp = tempfile::tempdir().unwrap();
        let path = tmp.path().join(LOCK_FILE_NAME);
        let at = ts("2024-01-02T03:04:05.5Z");
        upsert(
            &path,
            "git-skill",
            LockEntry {
                installed_at: at.clone(),
                updated_at: at.clone(),
                git: Some(Box::new(GitSource {
                    provider: "github".into(),
                    clone_url: "https://github.com/o/r.git".into(),
                    path: "skills/a<b>".into(),
                    r#ref: "main".into(),
                    tree_hash: "abc".into(),
                })),
                url: None,
            },
        )
        .unwrap();
        upsert(
            &path,
            "url-skill",
            LockEntry {
                installed_at: at.clone(),
                updated_at: at,
                git: None,
                url: Some(Box::new(URLSource {
                    url: "https://x/SKILL.md".into(),
                })),
            },
        )
        .unwrap();
        let want = r#"{
  "version": 1,
  "skills": {
    "git-skill": {
      "installed_at": "2024-01-02T03:04:05.5Z",
      "updated_at": "2024-01-02T03:04:05.5Z",
      "git": {
        "provider": "github",
        "clone_url": "https://github.com/o/r.git",
        "path": "skills/a\u003cb\u003e",
        "ref": "main",
        "tree_hash": "abc"
      }
    },
    "url-skill": {
      "installed_at": "2024-01-02T03:04:05.5Z",
      "updated_at": "2024-01-02T03:04:05.5Z",
      "url": {
        "url": "https://x/SKILL.md"
      }
    }
  }
}"#;
        assert_eq!(std::fs::read_to_string(&path).unwrap(), want);
        remove(&path, "git-skill").unwrap();
        remove(&path, "url-skill").unwrap();
        assert_eq!(
            std::fs::read_to_string(&path).unwrap(),
            "{\n  \"version\": 1,\n  \"skills\": {}\n}"
        );
        let leftovers: Vec<_> = std::fs::read_dir(tmp.path())
            .unwrap()
            .flatten()
            .map(|e| e.file_name())
            .collect();
        assert_eq!(
            leftovers.len(),
            2,
            "only the lock file and its flock file remain: {leftovers:?}"
        );
    }

    #[test]
    fn reads_go_written_file_with_local_offsets_and_null_skills() {
        let tmp = tempfile::tempdir().unwrap();
        let path = tmp.path().join(LOCK_FILE_NAME);
        std::fs::write(&path, r#"{"version":1,"skills":{"a":{"installed_at":"2024-05-01T10:00:00.123456789+02:00","updated_at":"2024-05-01T10:00:00+02:00","url":{"url":"u"}}}}"#).unwrap();
        let lock = load(&path).unwrap();
        let entry = lock.skills.unwrap()["a"].clone().unwrap();
        assert_eq!(entry.url.unwrap().url, "u");
        assert_eq!(entry.installed_at, ts("2024-05-01T08:00:00.123456789Z"));
        std::fs::write(&path, r#"{"version":1,"skills":null}"#).unwrap();
        assert!(load(&path).unwrap().skills.unwrap().is_empty());
        std::fs::write(&path, "{").unwrap();
        assert!(load(&path)
            .unwrap_err()
            .to_string()
            .starts_with("failed to parse lock file"));
    }

    #[test]
    fn indent_matches_go() {
        assert_eq!(indent(br#"{"a":[],"b":{},"c":[1,{"d":"x,:{"}]}"#), b"{\n  \"a\": [],\n  \"b\": {},\n  \"c\": [\n    1,\n    {\n      \"d\": \"x,:{\"\n    }\n  ]\n}");
    }
}
