//! Go JSON serialization conventions shared by the domain modules.
use serde::Serialize;
use serde_json::ser::Formatter;
use std::io::{self, Write};

pub(crate) trait Empty {
    fn empty(&self) -> bool;
}
impl<T> Empty for Vec<T> {
    fn empty(&self) -> bool {
        self.is_empty()
    }
}
impl<K, V> Empty for std::collections::BTreeMap<K, V> {
    fn empty(&self) -> bool {
        self.is_empty()
    }
}
pub(crate) fn is_empty<T: Empty>(value: &Option<T>) -> bool {
    value.as_ref().is_none_or(Empty::empty)
}

pub(crate) fn is_zero<T: Default + PartialEq>(value: &T) -> bool {
    value == &T::default()
}

struct GoFormatter;

impl Formatter for GoFormatter {
    fn write_f64<W: ?Sized + Write>(&mut self, writer: &mut W, value: f64) -> io::Result<()> {
        write_number(writer, &value.to_string(), value.abs())
    }

    fn write_f32<W: ?Sized + Write>(&mut self, writer: &mut W, value: f32) -> io::Result<()> {
        write_number(writer, &value.to_string(), f64::from(value.abs()))
    }
}

fn write_number<W: ?Sized + Write>(writer: &mut W, number: &str, absolute: f64) -> io::Result<()> {
    if absolute == 0.0 || (1e-6..1e21).contains(&absolute) {
        return writer.write_all(number.as_bytes());
    }
    let (sign, number) = number.strip_prefix('-').map_or(("", number), |v| ("-", v));
    let (mantissa, exponent) = number.split_once('e').unwrap_or((number, "0"));
    let exponent: i32 = exponent.parse().map_err(io::Error::other)?;
    let point = mantissa.find('.').unwrap_or(mantissa.len()) as i32;
    let digits = mantissa.replace('.', "");
    let first = digits.find(|c| c != '0').unwrap_or(0);
    let significant = digits[first..].trim_end_matches('0');
    let exponent = exponent + point - first as i32 - 1;
    write!(writer, "{sign}{}", &significant[..1])?;
    if significant.len() > 1 {
        write!(writer, ".{}", &significant[1..])?;
    }
    if exponent >= 0 {
        write!(writer, "e+{exponent}")
    } else {
        write!(writer, "e{exponent}")
    }
}

/// Encode compact JSON like Go's `json.Marshal`, retaining struct/raw-object order.
///
/// This escapes HTML and U+2028/U+2029, and compacts embedded `RawValue`s as Go
/// does. Use sorted maps, not `HashMap`, in caller-defined serializable values.
/// Domain float fields reject NaN/infinity; arbitrary caller-defined serde types
/// remain responsible for rejecting non-finite values (serde_json maps them to null).
pub fn to_go_json<T: ?Sized + Serialize>(value: &T) -> serde_json::Result<Vec<u8>> {
    let mut serializer = serde_json::Serializer::with_formatter(Vec::new(), GoFormatter);
    value.serialize(&mut serializer)?;
    let serialized = String::from_utf8(serializer.into_inner()).expect("serde emits UTF-8");
    let mut output = String::with_capacity(serialized.len());
    let (mut quoted, mut escaped) = (false, false);
    for c in serialized.chars() {
        if !quoted && matches!(c, ' ' | '\n' | '\r' | '\t') {
            continue;
        }
        match c {
            '<' => output.push_str("\\u003c"),
            '>' => output.push_str("\\u003e"),
            '&' => output.push_str("\\u0026"),
            '\u{2028}' => output.push_str("\\u2028"),
            '\u{2029}' => output.push_str("\\u2029"),
            _ => output.push(c),
        }
        if !escaped && c == '"' {
            quoted = !quoted;
        }
        escaped = !escaped && quoted && c == '\\';
    }
    Ok(output.into_bytes())
}

pub(crate) mod finite_float {
    use serde::{Deserialize, Deserializer, Serializer};
    pub fn serialize<S: Serializer>(value: &f64, serializer: S) -> Result<S::Ok, S::Error> {
        if !value.is_finite() {
            return Err(serde::ser::Error::custom("unsupported non-finite Go float"));
        }
        serializer.serialize_f64(*value)
    }
    pub fn deserialize<'de, D: Deserializer<'de>>(deserializer: D) -> Result<f64, D::Error> {
        f64::deserialize(deserializer)
    }
}
