use crate::{
    paths::{write_private, UserInfo},
    Result,
};
use serde_yaml::{Mapping, Value};
use std::{
    fs,
    path::{Path, PathBuf},
};

pub const SUPPORTED_KEYS: &[&str] = &[
    "secret",
    "secret.provider",
    "secret.path",
    "cmd",
    "cmd.new",
    "cmd.new.agent",
    "cmd.exec",
    "cmd.exec.agent",
    "cmd.exec.max-turns",
    "cmd.resume",
    "cmd.resume.recent_task_limit",
    "log",
    "log.level",
    "log.file",
    "log.format",
    "editor",
    "output",
    "output.format",
    "output.no-headers",
    "output.wide",
];

#[derive(Debug)]
pub struct ConfigStore {
    pub settings: Value,
    path: PathBuf,
}
impl ConfigStore {
    pub fn load(user: &UserInfo) -> Result<Self> {
        Self::at(user.config.join("config.yaml"))
    }
    pub fn at(path: impl AsRef<Path>) -> Result<Self> {
        let path = path.as_ref().to_path_buf();
        if let Some(parent) = path.parent() {
            UserInfo::ensure(parent)?;
        }
        let settings = match fs::read(&path) {
            Ok(bytes) => serde_yaml::from_slice(&bytes)
                .map_err(|e| format!("failed to parse config file: {e}"))?,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Value::Mapping(Mapping::new()),
            Err(e) => return Err(format!("failed to read config file: {e}").into()),
        };
        Ok(Self { settings, path })
    }
    pub fn get(&self, key: &str) -> Option<&Value> {
        let mut value = &self.settings;
        for part in key.split('.') {
            value = value.as_mapping()?.get(Value::String(part.into()))?;
        }
        Some(value)
    }
    pub fn get_or_default<'a>(&'a self, key: &str, default: &'a Value) -> (&'a Value, bool) {
        self.get(key).map_or((default, false), |v| (v, true))
    }
    pub fn set(&mut self, key: &str, value: Value) -> Result<()> {
        let parts: Vec<_> = key.split('.').collect();
        for (i, part) in parts.iter().enumerate() {
            if part.is_empty() {
                return Err(format!("invalid key: empty path segment at position {i}").into());
            }
        }
        if self.settings.is_null() {
            self.settings = Value::Mapping(Mapping::new());
        }
        let mut current = self
            .settings
            .as_mapping_mut()
            .ok_or("config root must be an object")?;
        for (i, part) in parts[..parts.len() - 1].iter().enumerate() {
            let entry = current
                .entry(Value::String((*part).into()))
                .or_insert_with(|| Value::Mapping(Mapping::new()));
            current = entry.as_mapping_mut().ok_or_else(|| {
                format!(
                    "key '{}' already exists as a non-object value",
                    parts[..=i].join(".")
                )
            })?;
        }
        current.insert(Value::String(parts.last().unwrap().to_string()), value);
        Ok(())
    }
    pub fn delete(&mut self, key: &str) {
        fn remove(value: &mut Value, parts: &[&str]) {
            if let Some(map) = value.as_mapping_mut() {
                let key = Value::String(parts[0].into());
                if parts.len() == 1 {
                    map.remove(&key);
                } else if let Some(child) = map.get_mut(&key) {
                    remove(child, &parts[1..]);
                    if child.as_mapping().is_some_and(Mapping::is_empty) {
                        map.remove(&key);
                    }
                }
            }
        }
        remove(&mut self.settings, &key.split('.').collect::<Vec<_>>());
    }
    pub fn flush(&self) -> Result<()> {
        write_private(
            &self.path,
            marshal_yaml_with_spacing(&self.settings)?.as_bytes(),
        )?;
        Ok(())
    }
}

pub fn marshal_yaml_with_spacing(value: &Value) -> Result<String> {
    fn sorted(value: &Value) -> Value {
        match value {
            Value::Mapping(map) => {
                let mut entries: Vec<_> = map.iter().collect();
                entries.sort_by_key(|(k, _)| k.as_str().unwrap_or_default());
                Value::Mapping(
                    entries
                        .into_iter()
                        .map(|(k, v)| (k.clone(), sorted(v)))
                        .collect(),
                )
            }
            Value::Sequence(seq) => Value::Sequence(seq.iter().map(sorted).collect()),
            _ => value.clone(),
        }
    }
    let yaml = crate::yaml::to_string(&sorted(value))?;
    let mut output = String::new();
    for (i, line) in yaml.split_inclusive('\n').enumerate() {
        if i > 0 && !line.trim().is_empty() && !line.starts_with([' ', '\t']) {
            output.push('\n');
        }
        output.push_str(line);
    }
    Ok(output)
}

pub fn parse_value(value: &str) -> Value {
    match value {
        "1" | "t" | "T" | "TRUE" | "true" | "True" => return Value::Bool(true),
        "0" | "f" | "F" | "FALSE" | "false" | "False" => return Value::Bool(false),
        _ => {}
    }
    if !value.trim().eq(value) {
        return Value::String(value.into());
    }
    if let Ok(n) = value.parse::<i64>() {
        return Value::Number(n.into());
    }
    if let Some(n) = parse_go_float(value) {
        return Value::Number(n.into());
    }
    Value::String(value.into())
}
pub(crate) fn parse_go_float(value: &str) -> Option<f64> {
    match value.to_ascii_lowercase().as_str() {
        "nan" => return Some(f64::NAN),
        "inf" | "+inf" | "infinity" | "+infinity" => return Some(f64::INFINITY),
        "-inf" | "-infinity" => return Some(f64::NEG_INFINITY),
        _ => {}
    }
    static DECIMAL: std::sync::OnceLock<regex::Regex> = std::sync::OnceLock::new();
    let decimal=DECIMAL.get_or_init(||regex::Regex::new(r"^[+-]?(?:[0-9](?:_?[0-9])*(?:\.(?:[0-9](?:_?[0-9])*)?)?|\.[0-9](?:_?[0-9])*)(?:[eE][+-]?[0-9](?:_?[0-9])*)?$").unwrap());
    if decimal.is_match(value) {
        let n = value.replace('_', "").parse::<f64>().ok()?;
        return n.is_finite().then_some(n);
    }
    static HEX: std::sync::OnceLock<regex::Regex> = std::sync::OnceLock::new();
    let hex=HEX.get_or_init(||regex::Regex::new(r"^[+-]?0[xX]_?(?:[0-9a-fA-F](?:_?[0-9a-fA-F])*(?:\.(?:[0-9a-fA-F](?:_?[0-9a-fA-F])*)?)?|\.[0-9a-fA-F](?:_?[0-9a-fA-F])*)[pP][+-]?[0-9](?:_?[0-9])*$").unwrap());
    if !hex.is_match(value) {
        return None;
    }
    let clean = value.replace('_', "");
    let negative = clean.starts_with('-');
    let unsigned = clean.trim_start_matches(['+', '-']);
    let (mantissa, exponent) = unsigned[2..].split_once(['p', 'P'])?;
    let exponent: i32 = exponent.parse().ok()?;
    let mut n = 0.;
    let mut after_dot = false;
    let mut scale = 1.;
    for c in mantissa.chars() {
        if c == '.' {
            after_dot = true;
            continue;
        }
        let digit = f64::from(c.to_digit(16)?);
        if after_dot {
            scale /= 16.;
            n += digit * scale;
        } else {
            n = n * 16. + digit;
        }
    }
    n *= 2f64.powi(exponent);
    if negative {
        n = -n;
    }
    n.is_finite().then_some(n)
}
pub fn is_section_key(key: &str) -> bool {
    SUPPORTED_KEYS
        .iter()
        .any(|k| k.starts_with(&format!("{key}.")))
}
pub fn keys_under_section(section: &str) -> Vec<&'static str> {
    let prefix = format!("{section}.");
    SUPPORTED_KEYS
        .iter()
        .copied()
        .filter(|k| k.strip_prefix(&prefix).is_some_and(|s| !s.contains('.')))
        .collect()
}
pub fn suggestions(input: &str) -> Vec<&'static str> {
    if input.is_empty() || !input.is_ascii() {
        return Vec::new();
    }
    let pattern = input.as_bytes();
    let mut matches = Vec::new();
    for (index, key) in SUPPORTED_KEYS.iter().copied().enumerate() {
        let candidate = key.as_bytes();
        let mut matched = Vec::new();
        let (mut total, mut pi, mut best, mut matched_index, mut adjacent) = (0, 0, -1, None, 0);
        for (j, &c) in candidate.iter().enumerate() {
            if pi < pattern.len() && c.eq_ignore_ascii_case(&pattern[pi]) {
                let mut score = if j == 0 { 10 } else { 0 };
                if j > 0 && b"/-_ .\\".contains(&candidate[j - 1]) {
                    score += 20;
                }
                if let Some(last) = matched.last() {
                    if j > 0 && *last == j - 1 {
                        let bonus = adjacent * 2 + 5;
                        score += bonus;
                        adjacent += bonus;
                    }
                }
                if score > best {
                    best = score;
                    matched_index = Some(j);
                }
            }
            let next_pattern = pattern.get(pi + 1).copied().unwrap_or(0);
            let next_candidate = candidate.get(j + 1).copied().unwrap_or(0);
            if next_pattern.eq_ignore_ascii_case(&next_candidate) || next_candidate == 0 {
                if let Some(position) = matched_index.take() {
                    if matched.is_empty() {
                        best += (-5 * position as i32).max(-15);
                    }
                    total += best;
                    matched.push(position);
                    best = -1;
                    pi += 1;
                }
            }
        }
        total += matched.len() as i32 - candidate.len() as i32;
        if matched.len() == pattern.len() {
            matches.push((total, index, key));
        }
    }
    matches.sort_by(|a, b| b.0.cmp(&a.0).then_with(|| b.1.cmp(&a.1)));
    matches.truncate(3);
    matches.into_iter().map(|(_, _, key)| key).collect()
}
pub fn validate_key(key: &str) -> Result<()> {
    if SUPPORTED_KEYS.contains(&key) {
        return Ok(());
    }
    let matches = suggestions(key);
    let mut message = format!("unsupported configuration key: '{key}'");
    if !matches.is_empty() {
        message.push_str("\n\nDid you mean one of these?\n");
        for suggestion in matches {
            message.push_str(&format!(" - {suggestion}\n"));
        }
    }
    Err(message.into())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn nested_roundtrip_and_errors() {
        let dir = tempfile::tempdir().unwrap();
        let mut store = ConfigStore::at(dir.path().join("config.yaml")).unwrap();
        assert!(store.get("cmd.new.agent").is_none());
        store.set("cmd.new.agent", parse_value("edit")).unwrap();
        store.set("editor", parse_value("vim")).unwrap();
        store.flush().unwrap();
        assert_eq!(
            ConfigStore::at(&store.path).unwrap().get("cmd.new.agent"),
            Some(&Value::String("edit".into()))
        );
        assert!(store
            .set("editor.name", Value::Null)
            .unwrap_err()
            .to_string()
            .contains("already exists as a non-object"));
        assert_eq!(
            store.set("a..b", Value::Null).unwrap_err().to_string(),
            "invalid key: empty path segment at position 1"
        );
        store.delete("cmd.new.agent");
        assert!(store.get("cmd").is_none());
    }
    #[test]
    fn go_scalar_order() {
        assert_eq!(parse_value("1"), Value::Bool(true));
        assert_eq!(parse_value("42"), Value::Number(42.into()));
        assert!(parse_value("1.5").is_number());
        assert_eq!(parse_value(" true"), Value::String(" true".into()));
        assert_eq!(parse_value("1e400"), Value::String("1e400".into()));
        assert_eq!(parse_value("0x1.8p2").as_f64(), Some(6.));
        assert_eq!(parse_value("1_234").as_f64(), Some(1234.));
    }
}
