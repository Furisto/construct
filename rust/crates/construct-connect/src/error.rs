use http::{HeaderMap, StatusCode};
use serde::{Deserialize, Serialize};
use std::fmt;

macro_rules! codes {
    ($($variant:ident => ($name:literal, $status:literal)),+ $(,)?) => {
        #[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
        pub enum Code {
            $(#[serde(rename = $name)] $variant,)+
        }
        impl Code {
            pub const ALL: [Self; 16] = [$(Self::$variant,)+];

            pub const fn as_str(self) -> &'static str {
                match self { $(Self::$variant => $name,)+ }
            }

            pub fn http_status(self) -> StatusCode {
                StatusCode::from_u16(match self { $(Self::$variant => $status,)+ }).unwrap()
            }
        }
    };
}

codes! {
    Canceled => ("canceled", 499),
    Unknown => ("unknown", 500),
    InvalidArgument => ("invalid_argument", 400),
    DeadlineExceeded => ("deadline_exceeded", 504),
    NotFound => ("not_found", 404),
    AlreadyExists => ("already_exists", 409),
    PermissionDenied => ("permission_denied", 403),
    ResourceExhausted => ("resource_exhausted", 429),
    FailedPrecondition => ("failed_precondition", 400),
    Aborted => ("aborted", 409),
    OutOfRange => ("out_of_range", 400),
    Unimplemented => ("unimplemented", 501),
    Internal => ("internal", 500),
    Unavailable => ("unavailable", 503),
    DataLoss => ("data_loss", 500),
    Unauthenticated => ("unauthenticated", 401),
}

impl fmt::Display for Code {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConnectError {
    pub code: Code,
    #[serde(default)]
    pub message: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub details: Vec<serde_json::Value>,
    #[serde(skip)]
    /// Response headers and error trailers, boxed to keep the error inexpensive to return.
    pub metadata: Box<HeaderMap>,
}

impl ConnectError {
    pub fn new(code: Code, message: impl Into<String>) -> Self {
        Self {
            code,
            message: message.into(),
            details: Vec::new(),
            metadata: Box::default(),
        }
    }

    pub(crate) fn internal(error: impl fmt::Display) -> Self {
        Self::new(Code::Internal, error.to_string())
    }

    pub(crate) fn deadline() -> Self {
        Self::new(Code::DeadlineExceeded, "deadline exceeded")
    }

    pub(crate) fn from_status(status: StatusCode) -> Self {
        let code = match status.as_u16() {
            400 => Code::Internal,
            401 => Code::Unauthenticated,
            403 => Code::PermissionDenied,
            404 => Code::Unimplemented,
            429 | 502 | 503 | 504 => Code::Unavailable,
            _ => Code::Unknown,
        };
        Self::new(code, format!("HTTP {status}"))
    }
}

impl fmt::Display for ConnectError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}: {}", self.code, self.message)
    }
}

impl std::error::Error for ConnectError {}
