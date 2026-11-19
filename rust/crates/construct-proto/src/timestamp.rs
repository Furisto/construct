use chrono::{DateTime, SecondsFormat, Utc};
use serde::{ser::Error, Serialize, Serializer};

pub(crate) struct Timestamp<'a>(pub &'a pbjson_types::Timestamp);

impl Serialize for Timestamp<'_> {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        let value = self.0;
        if !(-62_135_596_800..=253_402_300_799).contains(&value.seconds)
            || !(0..=999_999_999).contains(&value.nanos)
        {
            return Err(S::Error::custom("protobuf Timestamp out of range"));
        }
        let time = DateTime::<Utc>::from_timestamp(value.seconds, value.nanos as u32)
            .ok_or_else(|| S::Error::custom("invalid protobuf Timestamp"))?;
        serializer.serialize_str(&time.to_rfc3339_opts(SecondsFormat::AutoSi, true))
    }
}
