use crate::Result;
use serde::Serialize;
use serde_yaml::Value;

pub fn to_string<T: ?Sized + Serialize>(value: &T) -> Result<String> {
    fn scalar(value: &Value, indent: usize, step: usize) -> Result<String> {
        if let Value::Number(number) = value {
            if number.is_f64() && number.as_f64().is_some_and(|n| n.fract() == 0.0) {
                return Ok(format!("{:.0}", number.as_f64().unwrap()));
            }
        }
        if let Value::String(text) = value {
            if text.is_empty() {
                return Ok("\"\"".into());
            }
            if chrono::NaiveDateTime::parse_from_str(text, "%Y-%m-%d %H:%M:%S").is_ok() {
                return Ok(serde_json::to_string(text)?);
            }
            if matches!(
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
            ) {
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
    fn compound(value: &Value) -> bool {
        matches!(value, Value::Mapping(v) if !v.is_empty())
            || matches!(value, Value::Sequence(v) if !v.is_empty())
    }
    fn emit(value: &Value, indent: usize, step: usize, output: &mut String) -> Result<()> {
        let pad = " ".repeat(indent);
        match value {
            Value::Mapping(map) if !map.is_empty() => {
                for (key, value) in map {
                    output.push_str(&pad);
                    output.push_str(&scalar(key, indent + step, step)?);
                    output.push(':');
                    if compound(value) {
                        output.push('\n');
                        emit(value, indent + step, 4, output)?;
                    } else {
                        output.push(' ');
                        output.push_str(&scalar(value, indent + step, step)?);
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
                        emit(item, indent + 2, 2, &mut nested)?;
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
    emit(&serde_yaml::to_value(value)?, 0, 4, &mut output)?;
    Ok(output)
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
        assert_eq!(to_string(&"").unwrap(), "\"\"\n");
        assert_eq!(to_string(&0.0).unwrap(), "0\n");
        assert_eq!(
            to_string(&"2026-11-19 15:18:13").unwrap(),
            "\"2026-11-19 15:18:13\"\n"
        );
        assert_eq!(
            to_string(&serde_json::json!([{"usage": {"cost": 0.0}}])).unwrap(),
            "- usage:\n    cost: 0\n"
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
