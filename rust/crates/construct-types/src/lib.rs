//! Shared Construct domain records and Go-compatible stored JSON.
//!
//! Use [`to_go_json`] for byte identity with Go's `encoding/json`: ordinary
//! `serde_json::to_string` does not provide Go's HTML escaping or float spelling.
//! Struct fields retain Go declaration order; maps use `BTreeMap` (sorted keys).
//! Non-omitempty slices/maps use `Option`: `None` is Go nil (`null`), whereas
//! `Some(Vec::new())` is `[]`. Omitempty collections omit both nil and empty.
//! Pointer fields preserve presence, including pointers to zero values.
//! String enums are open newtypes, preserving Go's zero and unknown strings.
#![deny(missing_docs)]

macro_rules! string_type {
    ($name:ident, $($constant:ident = $value:literal),* $(,)?) => {
        #[doc = concat!("Go string-backed `", stringify!($name), "`; unknown values are preserved.")]
        #[derive(Clone, Debug, Default, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
        #[serde(transparent)]
        pub struct $name(#[doc = "Underlying Go string value."] pub String);
        impl $name {
            $(#[doc = concat!("Go value `", $value, "`.")]
            pub const $constant: &'static str = $value;)*
        }
        impl From<&str> for $name {
            fn from(value: &str) -> Self { Self(value.to_owned()) }
        }
        impl AsRef<str> for $name {
            fn as_ref(&self) -> &str { &self.0 }
        }
        impl std::fmt::Display for $name {
            fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                self.0.fmt(f)
            }
        }
    };
}

pub mod entity;
pub mod event;
pub mod json;
pub mod memory;
pub mod provider;
pub mod skill;
mod timestamp;
pub mod tools;

/// Serialize compact JSON using Go's escaping and float conventions.
pub use json::to_go_json;
/// UTC timestamp with Go's zero value and RFC3339Nano encoding.
pub use timestamp::Timestamp;
