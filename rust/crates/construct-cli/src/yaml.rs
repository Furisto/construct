use crate::Result;
use serde::Serialize;
use serde_yaml::Value;

pub fn to_string<T: ?Sized + Serialize>(value: &T) -> Result<String> {
    fn scalar(value: &Value, indent: usize, step: usize) -> Result<String> {
        if let Value::Number(number) = value {
            if let (true, Some(float)) = (number.is_f64(), number.as_f64()) {
                return Ok(go_float(float));
            }
        }
        if let Value::String(text) = value {
            if text.is_empty() {
                return Ok("\"\"".into());
            }
            let timestamp =
                chrono::NaiveDateTime::parse_from_str(text, "%Y-%m-%d %H:%M:%S").is_ok();
            if timestamp
                || matches!(
                    text.as_str(),
                    "y" | "Y"
                        | "yes"
                        | "Yes"
                        | "YES"
                        | "on"
                        | "On"
                        | "ON"
                        | "n"
                        | "N"
                        | "no"
                        | "No"
                        | "NO"
                        | "off"
                        | "Off"
                        | "OFF"
                )
            {
                return Ok(serde_json::to_string(text)?);
            }
            if !text.contains('\n') && resolves_to_non_string(text) {
                return Ok(serde_json::to_string(text)?);
            }
        }
        let encoded = serde_yaml::to_string(value)?;
        let encoded = encoded.strip_suffix('\n').unwrap_or(&encoded);
        let mut lines = encoded.split('\n');
        let first = lines.next().unwrap_or_default();
        let mut output = if first.starts_with("|2") {
            first.replacen("|2", &format!("|{step}"), 1)
        } else {
            first.to_string()
        };
        for line in lines {
            output.push('\n');
            if !line.is_empty() {
                output.push_str(&" ".repeat(indent));
                output.push_str(line.strip_prefix("  ").unwrap_or(line));
            }
        }
        Ok(output)
    }
    /// yaml.v3 indents block content under a mapping to the next multiple of 4.
    fn mapping_child(indent: usize) -> usize {
        4 * ((indent + 4) / 4)
    }
    fn compound(value: &Value) -> bool {
        matches!(value, Value::Mapping(v) if !v.is_empty())
            || matches!(value, Value::Sequence(v) if !v.is_empty())
    }
    fn emit(value: &Value, indent: usize, output: &mut String) -> Result<()> {
        let pad = " ".repeat(indent);
        match value {
            Value::Mapping(map) if !map.is_empty() => {
                let child = mapping_child(indent);
                for (key, value) in map {
                    output.push_str(&pad);
                    output.push_str(&scalar(key, child, child - indent)?);
                    output.push(':');
                    if compound(value) {
                        output.push('\n');
                        emit(value, child, output)?;
                    } else {
                        output.push(' ');
                        output.push_str(&scalar(value, child, child - indent)?);
                        output.push('\n');
                    }
                }
            }
            Value::Sequence(seq) if !seq.is_empty() => {
                for item in seq {
                    output.push_str(&pad);
                    output.push_str("- ");
                    if compound(item) {
                        let mut nested = String::new();
                        emit(item, indent + 2, &mut nested)?;
                        output.push_str(
                            nested
                                .strip_prefix(&" ".repeat(indent + 2))
                                .unwrap_or(&nested),
                        );
                    } else {
                        output.push_str(&scalar(item, indent + 2, 2)?);
                        output.push('\n');
                    }
                }
            }
            _ => {
                output.push_str(&pad);
                output.push_str(&scalar(value, indent + 4, 4)?);
                output.push('\n');
            }
        }
        Ok(())
    }
    let mut output = String::new();
    emit(&serde_yaml::to_value(value)?, 0, &mut output)?;
    Ok(output)
}

/// yaml.v3 double-quotes strings that would otherwise resolve to null, bool, int, float,
/// timestamp or a YAML 1.1 sexagesimal number.
fn resolves_to_non_string(text: &str) -> bool {
    use std::sync::OnceLock;
    static PATTERNS: OnceLock<[regex::Regex; 3]> = OnceLock::new();
    let [float, base60, timestamp] = PATTERNS.get_or_init(|| {
        [
            r"^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$",
            r"^[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+(?:\.[0-9_]*)?$",
            r"^[0-9]{4}-[0-9]{1,2}-[0-9]{1,2}(?:[Tt ][0-9]{1,2}:[0-9]{1,2}:[0-9]{1,2}(?:\.[0-9]*)?(?:Z|[+-][0-9]{1,2}(?::[0-9]{2})?)?)?$",
        ]
        .map(|p| regex::Regex::new(p).unwrap())
    });
    let plain = text.replace('_', "");
    matches!(
        text,
        "" | "~"
            | "null"
            | "Null"
            | "NULL"
            | "true"
            | "True"
            | "TRUE"
            | "false"
            | "False"
            | "FALSE"
            | ".inf"
            | ".Inf"
            | ".INF"
            | "+.inf"
            | "+.Inf"
            | "+.INF"
            | "-.inf"
            | "-.Inf"
            | "-.INF"
            | ".nan"
            | ".NaN"
            | ".NAN"
    ) || is_go_int(&plain)
        || float.is_match(&plain)
        || base60.is_match(text)
        || timestamp.is_match(text)
}

/// Go `strconv.ParseInt(s, 0, 64)` syntax.
fn is_go_int(s: &str) -> bool {
    let digits = s.strip_prefix(['+', '-']).unwrap_or(s);
    let (radix, body) = match digits.get(..2) {
        Some("0x" | "0X") => (16, &digits[2..]),
        Some("0o" | "0O") => (8, &digits[2..]),
        Some("0b" | "0B") => (2, &digits[2..]),
        _ if digits.len() > 1 && digits.starts_with('0') => (8, &digits[1..]),
        _ => (10, digits),
    };
    !body.is_empty() && body.chars().all(|c| c.is_digit(radix))
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn go_indent_and_yaml_11_strings() {
        let value: Value =
            serde_yaml::from_str("contexts:\n  local:\n    address: /tmp/test\n    kind: unix\n")
                .unwrap();
        assert_eq!(
            to_string(&value).unwrap(),
            "contexts:\n    local:\n        address: /tmp/test\n        kind: unix\n"
        );
        assert_eq!(to_string(&"yes").unwrap(), "\"yes\"\n");
        assert_eq!(
            to_string(&"2026-11-19 15:14:28").unwrap(),
            "\"2026-11-19 15:14:28\"\n"
        );
    }
    #[test]
    fn sequence_items_and_empty_strings_match_yaml_v3() {
        let value = serde_json::json!([{"agent": "", "usage": {"cost": 0.0, "inner": {"a": 1}}}]);
        assert_eq!(
            to_string(&value).unwrap(),
            "- agent: \"\"\n  usage:\n    cost: 0\n    inner:\n        a: 1\n"
        );
    }
    /// Expected output is Go's `yaml.Marshal` of the same values.
    #[test]
    fn matches_go_yaml_v3_output() {
        let value = serde_json::json!([{"a": "x\ny\n", "b": "123", "c": "", "d": {"e": "p\nq", "f": ["r\ns", {"g": "t\nu"}]}, "h": "true", "i": "1.5", "j": "~", "k": "null", "l": "0x1F", "m": "1_000", "n": "It's: x", "o": "-", "p": ".inf", "q": "1e3", "r": "+1"}]);
        assert_eq!(
            to_string(&value).unwrap(),
            "- a: |\n    x\n    y\n  b: \"123\"\n  c: \"\"\n  d:\n    e: |-\n        p\n        q\n    f:\n        - |-\n          r\n          s\n        - g: |-\n            t\n            u\n  h: \"true\"\n  i: \"1.5\"\n  j: \"~\"\n  k: \"null\"\n  l: \"0x1F\"\n  m: \"1_000\"\n  \"n\": 'It''s: x'\n  o: '-'\n  p: \".inf\"\n  q: \"1e3\"\n  r: \"+1\"\n"
        );
        let value = serde_json::json!({"s": ["a\nb", ["c\nd"]], "top": "a\nb"});
        assert_eq!(
            to_string(&value).unwrap(),
            "s:\n    - |-\n      a\n      b\n    - - |-\n        c\n        d\ntop: |-\n    a\n    b\n"
        );
        let value = serde_json::json!([
            "a: b",
            "#x",
            " lead",
            "trail ",
            "é",
            "tab\there",
            "1:20",
            "2024-01-02",
            "2024-01-02T03:04:05Z",
            "<<",
            "0b101",
            "y"
        ]);
        assert_eq!(
            to_string(&value).unwrap(),
            "- 'a: b'\n- '#x'\n- ' lead'\n- 'trail '\n- é\n- \"tab\\there\"\n- \"1:20\"\n- \"2024-01-02\"\n- \"2024-01-02T03:04:05Z\"\n- <<\n- \"0b101\"\n- \"y\"\n"
        );
    }
    #[test]
    fn multiline_scalars_preserve_contents_and_nesting() {
        for text in ["a\nb", "a\nb\n", "a\nb\n\n", "  a\n  b", ""] {
            let value = serde_json::json!({"nested": {"text": text}, "list": [text]});
            let encoded = to_string(&value).unwrap();
            let decoded: serde_json::Value = serde_yaml::from_str(&encoded).unwrap();
            assert_eq!(decoded, value, "{encoded}");
        }
        let value = serde_json::json!({"nested": {"text": "a\nb"}});
        assert_eq!(
            to_string(&value).unwrap(),
            "nested:\n    text: |-\n        a\n        b\n"
        );
    }
}

/// Formats a float like yaml.v3, i.e. Go's strconv.FormatFloat(v, 'g', -1, 64).
pub fn go_float(value: f64) -> String {
    if value.is_nan() {
        return ".nan".into();
    }
    if value.is_infinite() {
        return if value > 0. { ".inf" } else { "-.inf" }.into();
    }
    if value == 0. {
        return if value.is_sign_negative() { "-0" } else { "0" }.into();
    }
    let scientific = format!("{value:e}");
    let (mantissa, exponent) = scientific.split_once('e').unwrap_or((&scientific, "0"));
    let exponent: i32 = exponent.parse().unwrap_or(0);
    // Shortest 'g' switches to %e below 1e-4 and at or above 1e6.
    if !(-4..6).contains(&exponent) {
        let sign = if exponent < 0 { '-' } else { '+' };
        return format!("{mantissa}e{sign}{:02}", exponent.abs());
    }
    format!("{value}")
}

#[cfg(test)]
mod float_tests {
    use super::go_float;

    #[test]
    fn matches_go_shortest_g() {
        for (value, expected) in [
            (0., "0"),
            (1., "1"),
            (0.00033585, "0.00033585"),
            (0.0001, "0.0001"),
            (0.00001, "1e-05"),
            (123456., "123456"),
            (1e6, "1e+06"),
            (1234567.5, "1.2345675e+06"),
            (-2.5, "-2.5"),
            (f64::INFINITY, ".inf"),
        ] {
            assert_eq!(go_float(value), expected, "{value}");
        }
    }
}
