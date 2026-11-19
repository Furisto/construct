use crate::Result;
use std::io::{Read, Write};

pub fn confirm(stdin: &mut dyn Read, stdout: &mut dyn Write, message: &str) -> bool {
    if write!(stdout, "{message} (y/n): ")
        .and_then(|_| stdout.flush())
        .is_err()
    {
        return false;
    }
    let mut bytes = Vec::new();
    let mut byte = [0];
    loop {
        match stdin.read(&mut byte) {
            Ok(0) | Err(_) => break,
            Ok(_) => {
                if byte[0].is_ascii_whitespace() {
                    if bytes.is_empty() {
                        continue;
                    }
                    break;
                }
                bytes.push(byte[0]);
            }
        }
    }
    String::from_utf8(bytes).is_ok_and(|s| matches!(s.to_lowercase().as_str(), "y" | "yes"))
}
pub fn confirm_deletion(
    stdin: &mut dyn Read,
    stdout: &mut dyn Write,
    kind: &str,
    names: &[String],
) -> bool {
    if names.is_empty() {
        return false;
    }
    confirm(
        stdin,
        stdout,
        &format!(
            "Are you sure you want to delete {kind}{} {}?",
            if names.len() > 1 { "s" } else { "" },
            names.join(" ")
        ),
    )
}
/// Signed nanoseconds, like Go's time.Duration.
pub fn parse_duration(value: &str) -> Result<i64> {
    let value = value.trim();
    if let Some(days) = value.strip_suffix('d') {
        return crate::config::parse_go_float(days)
            .map(|d| {
                let nanos = d * 86_400_000_000_000.;
                if !nanos.is_finite()
                    || !(-9_223_372_036_854_775_808. ..9_223_372_036_854_775_808.).contains(&nanos)
                {
                    i64::MIN
                } else {
                    nanos as i64
                }
            })
            .ok_or_else(|| format!("invalid duration format: {value}").into());
    }
    let invalid = || format!("time: invalid duration {value:?}");
    let (negative, rest) = match value.as_bytes().first() {
        Some(b'-') => (true, &value[1..]),
        Some(b'+') => (false, &value[1..]),
        _ => (false, value),
    };
    if rest == "0" {
        return Ok(0);
    }
    let mut rest = rest;
    let mut total = 0u128;
    if rest.is_empty() {
        return Err(invalid().into());
    }
    while !rest.is_empty() {
        let integer_len = rest.bytes().take_while(u8::is_ascii_digit).count();
        let integer = if integer_len == 0 {
            0
        } else {
            rest[..integer_len].parse::<u64>().map_err(|_| invalid())?
        };
        if integer > 1 << 63 {
            return Err(invalid().into());
        }
        rest = &rest[integer_len..];
        let (mut fraction, mut scale, mut fraction_len) = (0u64, 1f64, 0usize);
        if let Some(after) = rest.strip_prefix('.') {
            fraction_len = after.bytes().take_while(u8::is_ascii_digit).count();
            let mut overflow = false;
            for digit in after[..fraction_len].bytes() {
                if overflow {
                    continue;
                }
                if fraction > (i64::MAX as u64) / 10 {
                    overflow = true;
                    continue;
                }
                let next = fraction * 10 + u64::from(digit - b'0');
                if next > 1 << 63 {
                    overflow = true;
                    continue;
                }
                fraction = next;
                scale *= 10.;
            }
            rest = &after[fraction_len..];
        }
        if integer_len == 0 && fraction_len == 0 {
            return Err(invalid().into());
        }
        let unit_len = rest
            .find(|c: char| c == '.' || c.is_ascii_digit())
            .unwrap_or(rest.len());
        if unit_len == 0 {
            return Err(format!("time: missing unit in duration {value:?}").into());
        }
        let name = &rest[..unit_len];
        let unit = match name {
            "ns" => 1u64,
            "us" | "µs" | "μs" => 1_000,
            "ms" => 1_000_000,
            "s" => 1_000_000_000,
            "m" => 60_000_000_000,
            "h" => 3_600_000_000_000,
            _ => return Err(format!("time: unknown unit {name:?} in duration {value:?}").into()),
        };
        total += u128::from(integer) * u128::from(unit)
            + ((fraction as f64) * (unit as f64 / scale)) as u128;
        if total > 1 << 63 {
            return Err(invalid().into());
        }
        rest = &rest[unit_len..];
    }
    if negative {
        Ok((total as i64).wrapping_neg())
    } else {
        i64::try_from(total).map_err(|_| invalid().into())
    }
}
pub fn validate_token_expiry(duration: i64) -> Result<()> {
    if duration > 365 * 86_400_000_000_000 {
        Err("expiry duration exceeds maximum of 365 days".into())
    } else if duration <= 0 {
        Err("expiry duration must be positive".into())
    } else {
        Ok(())
    }
}
pub fn validate_setup_code_expiry(duration: i64) -> Result<()> {
    if duration > 72 * 3_600_000_000_000 {
        Err("code expiry exceeds maximum of 72 hours".into())
    } else if duration <= 0 {
        Err("code expiry must be positive".into())
    } else {
        Ok(())
    }
}
pub fn format_relative_time_at(
    time: chrono::DateTime<chrono::Utc>,
    now: chrono::DateTime<chrono::Utc>,
) -> String {
    let duration = time - now;
    let nanos = duration
        .num_nanoseconds()
        .unwrap_or(if time < now { i64::MIN } else { i64::MAX });
    let seconds = nanos as f64 / 1_000_000_000.;
    let abs = seconds.abs();
    let (value, unit) = if abs < 3600. {
        ((abs / 60.).round(), "minute")
    } else if abs < 86400. {
        ((abs / 3600.).round(), "hour")
    } else {
        ((abs / 86400.).round(), "day")
    };
    let unit = format!("{unit}{}", if value == 1. { "" } else { "s" });
    if seconds < 0. {
        format!("{value} {unit} ago")
    } else {
        format!("in {value} {unit}")
    }
}
pub fn format_relative_time(time: chrono::DateTime<chrono::Utc>) -> String {
    format_relative_time_at(time, chrono::Utc::now())
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn durations() {
        assert_eq!(parse_duration(" 1.5d ").unwrap(), 129_600_000_000_000);
        assert_eq!(parse_duration("-1h30m").unwrap(), -5_400_000_000_000);
        assert!(parse_duration("").is_err());
        assert_eq!(
            parse_duration("123").unwrap_err().to_string(),
            "time: missing unit in duration \"123\""
        );
        assert_eq!(
            parse_duration("1q").unwrap_err().to_string(),
            "time: unknown unit \"q\" in duration \"1q\""
        );
        assert_eq!(parse_duration("9223372036854775807ns").unwrap(), i64::MAX);
        assert_eq!(parse_duration("-9223372036854775808ns").unwrap(), i64::MIN);
        assert!(parse_duration("9223372036854775808ns").is_err());
        assert!(validate_token_expiry(0).is_err());
    }
    #[test]
    fn confirmation() {
        let mut out = Vec::new();
        assert!(confirm(&mut &b"yes\n"[..], &mut out, "Continue?"));
        assert_eq!(out, b"Continue? (y/n): ");
        assert!(!confirm_deletion(&mut &b"y"[..], &mut out, "task", &[]));
    }
    #[test]
    fn relative_times() {
        let now = chrono::DateTime::from_timestamp(1_000_000, 0).unwrap();
        assert_eq!(
            format_relative_time_at(now - chrono::Duration::nanoseconds(1), now),
            "0 minutes ago"
        );
        assert_eq!(
            format_relative_time_at(now + chrono::Duration::seconds(90), now),
            "in 2 minutes"
        );
        assert_eq!(
            format_relative_time_at(now - chrono::Duration::hours(48), now),
            "2 days ago"
        );
    }
}
