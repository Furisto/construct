use crate::{Code, ConnectError};
use bytes::{Buf, BufMut, Bytes, BytesMut};
use http::HeaderMap;
use prost::Message;
use serde::{de::DeserializeOwned, Deserialize, Serialize};
use std::collections::BTreeMap;

pub(crate) const MAX_MESSAGE_SIZE: usize = 16 * 1024 * 1024;

#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub enum Codec {
    #[default]
    Proto,
    Json,
}

impl Codec {
    pub fn content_type(self, streaming: bool) -> &'static str {
        match (self, streaming) {
            (Self::Proto, false) => "application/proto",
            (Self::Json, false) => "application/json",
            (Self::Proto, true) => "application/connect+proto",
            (Self::Json, true) => "application/connect+json",
        }
    }

    pub(crate) fn from_headers(headers: &HeaderMap, streaming: bool) -> Result<Self, ConnectError> {
        let content_type = headers
            .get("content-type")
            .and_then(|v| v.to_str().ok())
            .unwrap_or("")
            .split(';')
            .next()
            .unwrap_or("")
            .trim();
        [Self::Proto, Self::Json]
            .into_iter()
            .find(|codec| codec.content_type(streaming) == content_type)
            .ok_or_else(|| ConnectError::new(Code::Unimplemented, "unsupported content type"))
    }

    pub(crate) fn encode<T: Message + Serialize>(self, message: &T) -> Result<Bytes, ConnectError> {
        let bytes = match self {
            Self::Proto => message.encode_to_vec(),
            Self::Json => serde_json::to_vec(message).map_err(ConnectError::internal)?,
        };
        if bytes.len() > MAX_MESSAGE_SIZE {
            return Err(ConnectError::new(
                Code::ResourceExhausted,
                "message exceeds 16 MiB",
            ));
        }
        Ok(bytes.into())
    }

    pub(crate) fn decode<T: Message + Default + DeserializeOwned>(
        self,
        bytes: &[u8],
    ) -> Result<T, ConnectError> {
        match self {
            Self::Proto => T::decode(bytes)
                .map_err(|e| ConnectError::new(Code::InvalidArgument, e.to_string())),
            Self::Json => serde_json::from_slice(bytes)
                .map_err(|e| ConnectError::new(Code::InvalidArgument, e.to_string())),
        }
    }
}

pub(crate) fn check_encoding(headers: &HeaderMap, streaming: bool) -> Result<(), ConnectError> {
    for name in if streaming {
        &["content-encoding", "connect-content-encoding"][..]
    } else {
        &["content-encoding"][..]
    } {
        if let Some(value) = headers.get(*name) {
            if value != "identity" {
                return Err(ConnectError::new(
                    Code::Unimplemented,
                    "supported encodings: identity",
                ));
            }
        }
    }
    Ok(())
}

pub(crate) fn envelope(flag: u8, data: &[u8]) -> Bytes {
    let mut bytes = BytesMut::with_capacity(5 + data.len());
    bytes.put_u8(flag);
    bytes.put_u32(data.len() as u32);
    bytes.extend_from_slice(data);
    bytes.freeze()
}

pub(crate) fn take_envelope(buffer: &mut BytesMut) -> Result<Option<(u8, Bytes)>, ConnectError> {
    if buffer.len() < 5 {
        return Ok(None);
    }
    let flag = buffer[0];
    if flag & 1 != 0 {
        return Err(ConnectError::new(
            Code::Unimplemented,
            "compressed envelopes are unsupported",
        ));
    }
    if flag != 0 && flag != 2 {
        return Err(ConnectError::new(
            Code::InvalidArgument,
            "invalid envelope flags",
        ));
    }
    let size = u32::from_be_bytes(buffer[1..5].try_into().unwrap()) as usize;
    if size > MAX_MESSAGE_SIZE {
        return Err(ConnectError::new(
            Code::ResourceExhausted,
            "message exceeds 16 MiB",
        ));
    }
    if buffer.len() < 5 + size {
        return Ok(None);
    }
    buffer.advance(5);
    Ok(Some((flag, buffer.split_to(size).freeze())))
}

#[derive(Default, Serialize, Deserialize)]
pub(crate) struct EndStream {
    #[serde(
        default,
        skip_serializing_if = "Option::is_none",
        deserialize_with = "deserialize_error"
    )]
    pub error: Option<ConnectError>,
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub metadata: BTreeMap<String, Vec<String>>,
}

fn deserialize_error<'de, D: serde::Deserializer<'de>>(
    deserializer: D,
) -> Result<Option<ConnectError>, D::Error> {
    ConnectError::deserialize(deserializer).map(Some)
}

impl EndStream {
    pub fn from_error(error: Option<ConnectError>) -> Self {
        let mut metadata = BTreeMap::new();
        if let Some(error) = &error {
            for (key, value) in error.metadata.iter() {
                if let Ok(value) = value.to_str() {
                    metadata
                        .entry(key.to_string())
                        .or_insert_with(Vec::new)
                        .push(value.to_owned());
                }
            }
        }
        Self { error, metadata }
    }

    pub fn into_error(self) -> Result<Option<ConnectError>, ConnectError> {
        let mut error = self.error;
        let mut headers = HeaderMap::new();
        for (name, values) in self.metadata {
            let name = http::header::HeaderName::from_bytes(name.as_bytes())
                .map_err(ConnectError::internal)?;
            for value in values {
                headers.append(name.clone(), value.parse().map_err(ConnectError::internal)?);
            }
        }
        if let Some(error) = &mut error {
            *error.metadata = headers;
        }
        Ok(error)
    }

    pub fn encode(self) -> Bytes {
        envelope(
            2,
            &serde_json::to_vec(&self).expect("Connect errors are JSON serializable"),
        )
    }
}
