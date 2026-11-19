use crate::{Error, Result};
use aes_gcm::{
    aead::{rand_core::RngCore, Aead, AeadCore, KeyInit, OsRng, Payload},
    Aes128Gcm, Aes256Gcm, Nonce,
};
use base64::{engine::general_purpose::STANDARD, Engine};
use prost::Message;
use serde::{Deserialize, Serialize};

const TYPE_URL: &str = "type.googleapis.com/google.crypto.tink.AesGcmKey";

#[derive(Clone, PartialEq, Message)]
struct AesKey {
    #[prost(uint32, tag = "1")]
    version: u32,
    #[prost(bytes = "vec", tag = "3")]
    key_value: Vec<u8>,
}
#[derive(Clone, PartialEq, Message)]
struct KeyData {
    #[prost(string, tag = "1")]
    type_url: String,
    #[prost(bytes = "vec", tag = "2")]
    value: Vec<u8>,
    #[prost(int32, tag = "3")]
    key_material_type: i32,
}
#[derive(Clone, PartialEq, Message)]
struct Key {
    #[prost(message, optional, tag = "1")]
    key_data: Option<KeyData>,
    #[prost(int32, tag = "2")]
    status: i32,
    #[prost(uint32, tag = "3")]
    key_id: u32,
    #[prost(int32, tag = "4")]
    output_prefix_type: i32,
}
#[derive(Clone, PartialEq, Message)]
pub struct Keyset {
    #[prost(uint32, tag = "1")]
    primary_key_id: u32,
    #[prost(message, repeated, tag = "2")]
    key: Vec<Key>,
}
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct JsonKeyset {
    primary_key_id: u32,
    key: Vec<JsonKey>,
}
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct JsonKey {
    #[serde(default)]
    key_data: Option<JsonData>,
    status: String,
    key_id: u32,
    output_prefix_type: String,
}
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct JsonData {
    type_url: String,
    value: String,
    key_material_type: String,
}
fn invalid(e: impl std::fmt::Display) -> Error {
    Error::InvalidKeyset(e.to_string())
}
fn enum_name(value: i32, names: &[&str]) -> Result<String> {
    names
        .get(value as usize)
        .map(|s| s.to_string())
        .ok_or_else(|| invalid("unknown enum"))
}
fn enum_value(value: &str, names: &[&str]) -> Result<i32> {
    names
        .iter()
        .position(|n| *n == value)
        .map(|i| i as i32)
        .ok_or_else(|| invalid("unknown enum"))
}
const STATUS: &[&str] = &["UNKNOWN_STATUS", "ENABLED", "DISABLED", "DESTROYED"];
const PREFIX: &[&str] = &["UNKNOWN_PREFIX", "TINK", "LEGACY", "RAW", "CRUNCHY"];
const MATERIAL: &[&str] = &[
    "UNKNOWN_KEYMATERIAL",
    "SYMMETRIC",
    "ASYMMETRIC_PRIVATE",
    "ASYMMETRIC_PUBLIC",
    "REMOTE",
];

impl Keyset {
    pub fn generate() -> Self {
        let key_value = Aes256Gcm::generate_key(&mut OsRng).to_vec();
        let primary_key_id = (OsRng.next_u32() & 0x7fff_ffff).max(1);
        Self {
            primary_key_id,
            key: vec![Key {
                key_data: Some(KeyData {
                    type_url: TYPE_URL.into(),
                    value: AesKey {
                        version: 0,
                        key_value,
                    }
                    .encode_to_vec(),
                    key_material_type: 1,
                }),
                status: 1,
                key_id: primary_key_id,
                output_prefix_type: 1,
            }],
        }
    }
    pub fn from_binary(bytes: &[u8]) -> Result<Self> {
        let k = Self::decode(bytes).map_err(invalid)?;
        k.validate()?;
        Ok(k)
    }
    pub fn to_binary(&self) -> Vec<u8> {
        self.encode_to_vec()
    }
    pub fn from_json(json: &str) -> Result<Self> {
        let j: JsonKeyset = serde_json::from_str(json).map_err(invalid)?;
        let key = j
            .key
            .into_iter()
            .map(|k| {
                Ok(Key {
                    key_data: k
                        .key_data
                        .map(|d| {
                            Ok::<_, Error>(KeyData {
                                type_url: d.type_url,
                                value: STANDARD.decode(d.value).map_err(invalid)?,
                                key_material_type: enum_value(&d.key_material_type, MATERIAL)?,
                            })
                        })
                        .transpose()?,
                    status: enum_value(&k.status, STATUS)?,
                    key_id: k.key_id,
                    output_prefix_type: enum_value(&k.output_prefix_type, PREFIX)?,
                })
            })
            .collect::<Result<_>>()?;
        let k = Self {
            primary_key_id: j.primary_key_id,
            key,
        };
        k.validate()?;
        Ok(k)
    }
    pub fn to_json(&self) -> Result<String> {
        let key = self
            .key
            .iter()
            .map(|k| {
                Ok(JsonKey {
                    key_data: k
                        .key_data
                        .as_ref()
                        .map(|d| {
                            Ok::<_, Error>(JsonData {
                                type_url: d.type_url.clone(),
                                value: STANDARD.encode(&d.value),
                                key_material_type: enum_name(d.key_material_type, MATERIAL)?,
                            })
                        })
                        .transpose()?,
                    status: enum_name(k.status, STATUS)?,
                    key_id: k.key_id,
                    output_prefix_type: enum_name(k.output_prefix_type, PREFIX)?,
                })
            })
            .collect::<Result<_>>()?;
        serde_json::to_string(&JsonKeyset {
            primary_key_id: self.primary_key_id,
            key,
        })
        .map_err(invalid)
    }
    fn validate(&self) -> Result<()> {
        let mut primary = 0;
        let mut enabled = 0;
        for k in self.key.iter().filter(|k| k.status == 1) {
            enabled += 1;
            if k.key_id == self.primary_key_id {
                primary += 1;
            }
            if !(1..=4).contains(&k.output_prefix_type) {
                return Err(invalid("unknown prefix"));
            }
            key_bytes(k)?;
        }
        if enabled == 0 || primary != 1 {
            return Err(invalid("missing or duplicate enabled primary key"));
        }
        Ok(())
    }
}
fn key_bytes(k: &Key) -> Result<Vec<u8>> {
    let d = k
        .key_data
        .as_ref()
        .ok_or_else(|| invalid("missing key data"))?;
    if d.type_url != TYPE_URL || d.key_material_type != 1 {
        return Err(invalid("unsupported key type"));
    }
    let a = AesKey::decode(d.value.as_slice()).map_err(invalid)?;
    if a.version != 0 || ![16, 32].contains(&a.key_value.len()) {
        return Err(invalid("invalid AES-GCM key"));
    }
    Ok(a.key_value)
}
fn prefix(k: &Key) -> Vec<u8> {
    if k.output_prefix_type == 3 {
        return vec![];
    }
    let mut p = vec![u8::from(k.output_prefix_type == 1)];
    p.extend(k.key_id.to_be_bytes());
    p
}
pub struct Encryption {
    keyset: Keyset,
}
impl Encryption {
    pub fn new(keyset: Keyset) -> Result<Self> {
        keyset.validate()?;
        Ok(Self { keyset })
    }
    pub fn encrypt(&self, plaintext: &[u8], associated_data: &[u8]) -> Result<Vec<u8>> {
        let k = self
            .keyset
            .key
            .iter()
            .find(|k| k.status == 1 && k.key_id == self.keyset.primary_key_id)
            .ok_or(Error::Crypto)?;
        let nonce = Aes256Gcm::generate_nonce(&mut OsRng);
        let key = key_bytes(k)?;
        let payload = Payload {
            msg: plaintext,
            aad: associated_data,
        };
        let body = if key.len() == 16 {
            Aes128Gcm::new_from_slice(&key)
                .map_err(|_| Error::Crypto)?
                .encrypt(&nonce, payload)
        } else {
            Aes256Gcm::new_from_slice(&key)
                .map_err(|_| Error::Crypto)?
                .encrypt(&nonce, payload)
        }
        .map_err(|_| Error::Crypto)?;
        let mut out = prefix(k);
        out.extend(nonce);
        out.extend(body);
        Ok(out)
    }
    pub fn decrypt(&self, ciphertext: &[u8], associated_data: &[u8]) -> Result<Vec<u8>> {
        for k in self.keyset.key.iter().filter(|k| k.status == 1) {
            let p = prefix(k);
            if !ciphertext.starts_with(&p) {
                continue;
            }
            let body = &ciphertext[p.len()..];
            if body.len() < 28 {
                continue;
            }
            let key = key_bytes(k)?;
            let payload = Payload {
                msg: &body[12..],
                aad: associated_data,
            };
            let nonce = Nonce::from_slice(&body[..12]);
            let result = if key.len() == 16 {
                Aes128Gcm::new_from_slice(&key)
                    .map_err(|_| Error::Crypto)?
                    .decrypt(nonce, payload)
            } else {
                Aes256Gcm::new_from_slice(&key)
                    .map_err(|_| Error::Crypto)?
                    .decrypt(nonce, payload)
            };
            if let Ok(plain) = result {
                return Ok(plain);
            }
        }
        Err(Error::Crypto)
    }
}
