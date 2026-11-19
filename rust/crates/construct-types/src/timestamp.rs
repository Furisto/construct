use chrono::{DateTime, NaiveDate, SecondsFormat, Utc};
use serde::{Deserialize, Deserializer, Serialize, Serializer};

/// A UTC instant. Defaults to Go's `0001-01-01T00:00:00Z`, not Unix epoch.
/// Serialization trims fractional trailing zeroes, matching RFC3339Nano.
#[derive(Clone, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub struct Timestamp(
    /// Underlying chrono UTC timestamp.
    pub DateTime<Utc>,
);

impl Default for Timestamp {
    fn default() -> Self {
        Self(
            NaiveDate::from_ymd_opt(1, 1, 1)
                .unwrap()
                .and_hms_opt(0, 0, 0)
                .unwrap()
                .and_utc(),
        )
    }
}

impl Serialize for Timestamp {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        use chrono::Datelike;
        if !(0..=9999).contains(&self.0.year()) {
            return Err(serde::ser::Error::custom(
                "Go JSON timestamp year outside [0,9999]",
            ));
        }
        let mut text = self.0.to_rfc3339_opts(SecondsFormat::Nanos, true);
        text.pop();
        while text.ends_with('0') {
            text.pop();
        }
        if text.ends_with('.') {
            text.pop();
        }
        text.push('Z');
        serializer.serialize_str(&text)
    }
}

impl<'de> Deserialize<'de> for Timestamp {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let text = String::deserialize(deserializer)?;
        let value = DateTime::parse_from_rfc3339(&text).map_err(serde::de::Error::custom)?;
        if value.timestamp_subsec_nanos() >= 1_000_000_000 {
            return Err(serde::de::Error::custom(
                "Go timestamps do not support leap seconds",
            ));
        }
        Ok(Self(value.with_timezone(&Utc)))
    }
}
