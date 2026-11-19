use crate::{paths::UserInfo, Result};
use std::{
    fs::{self, OpenOptions},
    io::{self, Write},
    path::PathBuf,
    sync::{Arc, Mutex},
};
use tracing_subscriber::fmt::MakeWriter;

pub fn parse_level(value: &str) -> std::result::Result<String, String> {
    if matches!(value, "debug" | "info" | "warn" | "error") {
        Ok(value.into())
    } else {
        Err("must be one of \"debug\", \"info\", \"warn\", or \"error\"".into())
    }
}
pub fn resolve_level(flag: Option<&str>, environment: Option<&str>) -> String {
    flag.map(str::to_owned)
        .or_else(|| environment.and_then(|v| parse_level(v).ok()))
        .unwrap_or_else(|| "info".into())
}
#[derive(Clone)]
struct LogWriter {
    path: Option<PathBuf>,
    lock: Arc<Mutex<()>>,
}
impl Write for LogWriter {
    fn write(&mut self, bytes: &[u8]) -> io::Result<usize> {
        let _guard = self
            .lock
            .lock()
            .map_err(|_| io::Error::other("log lock poisoned"))?;
        io::stdout().write_all(bytes)?;
        if let Some(path) = &self.path {
            if fs::metadata(path).is_ok_and(|m| m.len() + bytes.len() as u64 > 50 * 1024 * 1024) {
                for i in (1..=3).rev() {
                    let from = if i == 1 {
                        path.clone()
                    } else {
                        path.with_extension(format!("json.{}", i - 1))
                    };
                    let to = path.with_extension(format!("json.{i}"));
                    if from.exists() {
                        fs::rename(from, to)?;
                    }
                }
            }
            OpenOptions::new()
                .create(true)
                .append(true)
                .open(path)?
                .write_all(bytes)?;
        }
        Ok(bytes.len())
    }
    fn flush(&mut self) -> io::Result<()> {
        io::stdout().flush()
    }
}
impl<'a> MakeWriter<'a> for LogWriter {
    type Writer = Self;
    fn make_writer(&'a self) -> Self {
        self.clone()
    }
}
pub fn init(user: &UserInfo, level: &str, disable_file: bool) -> Result<()> {
    let path = if !disable_file && UserInfo::ensure(&user.log).is_ok() {
        Some(user.log.join("construct.json"))
    } else {
        None
    };
    tracing_subscriber::fmt()
        .json()
        .with_max_level(level.parse::<tracing::Level>()?)
        .with_writer(LogWriter {
            path,
            lock: Arc::new(Mutex::new(())),
        })
        .try_init()
        .map_err(|e| e.to_string())?;
    Ok(())
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn levels() {
        assert_eq!(resolve_level(Some("debug"), Some("error")), "debug");
        assert_eq!(resolve_level(None, Some("bad")), "info");
        assert_eq!(
            parse_level("bad").unwrap_err(),
            "must be one of \"debug\", \"info\", \"warn\", or \"error\""
        );
    }
}
