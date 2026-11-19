//! Rustls HTTP client, Go-compatible error mapping and incremental SSE decoding.
use bytes::Bytes;
use construct_core::CancellationToken;
use construct_types::provider::{ProviderError, ProviderErrorKind};
use futures_util::{Stream, StreamExt};
use reqwest::{header::HeaderMap, StatusCode};
use std::time::Duration;

/// Build an HTTP client without SDK retries or a whole-stream timeout.
/// Per-profile timeouts should be supplied by the provider.
pub fn client_builder() -> reqwest::ClientBuilder {
    reqwest::Client::builder().use_rustls_tls()
}

/// Map an HTTP API failure using Go Anthropic's status classification.
/// Retry-After accepts signed fractional seconds, not HTTP dates, matching Go.
pub fn status_error(
    provider: &str,
    status: StatusCode,
    headers: &HeaderMap,
    detail: impl Into<String>,
) -> ProviderError {
    let kind = match status.as_u16() {
        400 | 401 | 403 | 404 | 413 => ProviderErrorKind::INVALID_REQUEST,
        429 => ProviderErrorKind::RATE_LIMIT_EXCEEDED,
        529 => ProviderErrorKind::OVERLOADED,
        500..=599 => ProviderErrorKind::INTERNAL,
        _ => ProviderErrorKind::UNKNOWN,
    };
    let retry_after = if status == StatusCode::TOO_MANY_REQUESTS {
        headers
            .get("retry-after")
            .and_then(|v| v.to_str().ok())
            .and_then(parse_retry_after)
            .unwrap_or(0)
    } else {
        0
    };
    ProviderError {
        provider: provider.into(),
        kind: kind.into(),
        retry_after,
        err: Some(detail.into()),
    }
}

/// Parse Go's Retry-After seconds into signed nanoseconds.
pub fn parse_retry_after(value: &str) -> Option<i64> {
    if value.trim() != value || value.is_empty() {
        return None;
    }
    let negative = value.starts_with('-');
    let unsigned = value.strip_prefix(['-', '+']).unwrap_or(value);
    let (whole, fraction) = unsigned.split_once('.').unwrap_or((unsigned, ""));
    if (whole.is_empty() && fraction.is_empty())
        || !whole.bytes().all(|b| b.is_ascii_digit())
        || !fraction.bytes().all(|b| b.is_ascii_digit())
    {
        return None;
    }
    let seconds = if whole.is_empty() {
        0
    } else {
        whole.parse::<i128>().ok()?
    };
    let mut nanos = seconds.checked_mul(1_000_000_000)?;
    let digits = &fraction[..fraction.len().min(9)];
    if !digits.is_empty() {
        nanos = nanos
            .checked_add(digits.parse::<i128>().ok()? * 10_i128.pow(9 - digits.len() as u32))?;
    }
    i64::try_from(if negative { -nanos } else { nanos }).ok()
}

/// Map non-HTTP transport errors. Go Anthropic maps these to unknown,
/// including timeouts; explicit context cancellation is handled separately.
pub fn transport_error(provider: &str, error: impl std::fmt::Display) -> ProviderError {
    ProviderError {
        provider: provider.into(),
        kind: ProviderErrorKind::UNKNOWN.into(),
        err: Some(error.to_string()),
        ..Default::default()
    }
}

/// Execute a request with explicit cancellation and Go-style transport errors.
/// Non-success statuses are left to the caller to inspect with status_error.
pub async fn send(
    provider: &str,
    request: reqwest::RequestBuilder,
    cancellation: &CancellationToken,
) -> Result<reqwest::Response, ProviderError> {
    crate::resilience::cancelable(provider, cancellation, async {
        request
            .send()
            .await
            .map_err(|error| transport_error(provider, error))
    })
    .await
}

/// One SSE event. Data lines are joined with newlines; comments are ignored.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct SseEvent {
    /// Event type; empty is the default message event.
    pub event: String,
    /// Event payload without its final newline.
    pub data: String,
    /// Last event ID, persisting across events.
    pub id: String,
    /// Optional reconnection interval supplied by the stream.
    pub retry: Option<Duration>,
}

#[derive(Default)]
struct Decoder {
    line: Vec<u8>,
    event: SseEvent,
    has_data: bool,
    skip_lf: bool,
    first_line: bool,
}

impl Decoder {
    fn line(&mut self) -> Option<SseEvent> {
        let bytes = std::mem::take(&mut self.line);
        let line = String::from_utf8_lossy(&bytes);
        let line = if !self.first_line {
            self.first_line = true;
            line.trim_start_matches('\u{feff}')
        } else {
            &line
        };
        if line.is_empty() {
            let result = self.has_data.then(|| {
                self.event.data.pop();
                self.event.clone()
            });
            self.event.event.clear();
            self.event.data.clear();
            self.has_data = false;
            return result;
        }
        let (field, value) = line.split_once(':').unwrap_or((line, ""));
        let value = value.strip_prefix(' ').unwrap_or(value);
        match field {
            "event" => self.event.event = value.into(),
            "data" => {
                self.event.data.push_str(value);
                self.event.data.push('\n');
                self.has_data = true;
            }
            "id" if !value.contains('\0') => self.event.id = value.into(),
            "retry" if !value.is_empty() && value.bytes().all(|b| b.is_ascii_digit()) => {
                if let Ok(ms) = value.parse() {
                    self.event.retry = Some(Duration::from_millis(ms));
                }
            }
            _ => {}
        }
        None
    }

    fn push(&mut self, bytes: &[u8]) -> Vec<SseEvent> {
        let mut events = Vec::new();
        for &byte in bytes {
            if self.skip_lf {
                self.skip_lf = false;
                if byte == b'\n' {
                    continue;
                }
            }
            if byte == b'\r' || byte == b'\n' {
                if let Some(event) = self.line() {
                    events.push(event);
                }
                self.skip_lf = byte == b'\r';
            } else {
                self.line.push(byte);
            }
        }
        events
    }
}

/// Decode a reqwest byte stream into SSE events, honoring cancellation.
/// Supports arbitrary chunk boundaries, UTF-8, CR/LF/CRLF, comments and BOM.
/// Incomplete events at EOF are discarded per the SSE specification.
pub fn event_stream<S, E>(
    provider: &str,
    stream: S,
    cancellation: CancellationToken,
) -> impl Stream<Item = Result<SseEvent, ProviderError>> + Send
where
    S: Stream<Item = Result<Bytes, E>> + Send,
    E: std::fmt::Display + Send,
{
    let provider = provider.to_owned();
    async_stream::stream! {
        futures_util::pin_mut!(stream);
        let mut decoder = Decoder::default();
        loop {
            let next = tokio::select! {
                biased;
                _ = cancellation.cancelled() => {
                    yield Err(crate::resilience::canceled(&provider));
                    break;
                }
                next = stream.next() => next,
            };
            match next {
                Some(Ok(bytes)) => for event in decoder.push(&bytes) { yield Ok(event); },
                Some(Err(error)) => { yield Err(transport_error(&provider, error)); break; }
                None => break,
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use futures_util::stream;
    #[tokio::test]
    async fn chunked_sse() {
        let input = "\u{feff}:comment\r\nid: abc\revent: delta\ndata: hé\r\ndata: world\nretry: 42\n\ndata:\n\ndata: unfinished";
        let chunks = input
            .as_bytes()
            .iter()
            .map(|b| Ok::<_, &str>(Bytes::from(vec![*b])))
            .collect::<Vec<_>>();
        let events = event_stream("test", stream::iter(chunks), CancellationToken::new())
            .collect::<Vec<_>>()
            .await;
        assert_eq!(events.len(), 2);
        assert_eq!(
            events[0].as_ref().unwrap(),
            &SseEvent {
                event: "delta".into(),
                data: "hé\nworld".into(),
                id: "abc".into(),
                retry: Some(Duration::from_millis(42))
            }
        );
        assert_eq!(events[1].as_ref().unwrap().data, "");
        assert_eq!(events[1].as_ref().unwrap().id, "abc");
        assert_eq!(events[1].as_ref().unwrap().event, "");
    }
    #[tokio::test]
    async fn empty_error_and_cancellation() {
        assert!(event_stream(
            "test",
            stream::empty::<Result<Bytes, &str>>(),
            CancellationToken::new()
        )
        .collect::<Vec<_>>()
        .await
        .is_empty());
        let errors = event_stream(
            "test",
            stream::iter([Err::<Bytes, _>("broken")]),
            CancellationToken::new(),
        )
        .collect::<Vec<_>>()
        .await;
        assert_eq!(
            errors[0].as_ref().unwrap_err().err.as_deref(),
            Some("broken")
        );
        let token = CancellationToken::new();
        token.cancel();
        let errors = event_stream("test", stream::pending::<Result<Bytes, &str>>(), token)
            .collect::<Vec<_>>()
            .await;
        assert_eq!(
            errors[0].as_ref().unwrap_err().kind.as_ref(),
            ProviderErrorKind::CANCELED
        );
    }
    #[test]
    fn status_mapping() {
        for (status, kind) in [
            (400, ProviderErrorKind::INVALID_REQUEST),
            (401, ProviderErrorKind::INVALID_REQUEST),
            (403, ProviderErrorKind::INVALID_REQUEST),
            (404, ProviderErrorKind::INVALID_REQUEST),
            (413, ProviderErrorKind::INVALID_REQUEST),
            (429, ProviderErrorKind::RATE_LIMIT_EXCEEDED),
            (529, ProviderErrorKind::OVERLOADED),
            (503, ProviderErrorKind::INTERNAL),
            (408, ProviderErrorKind::UNKNOWN),
        ] {
            let mut headers = HeaderMap::new();
            headers.insert("retry-after", "1.25".parse().unwrap());
            let error = status_error(
                "test",
                StatusCode::from_u16(status).unwrap(),
                &headers,
                "detail",
            );
            assert_eq!(error.kind.as_ref(), kind);
            assert_eq!(
                error.retry_after,
                if status == 429 { 1_250_000_000 } else { 0 }
            );
        }
        for invalid in [
            "",
            "NaN",
            "inf",
            " 1",
            "Wed, 21 Oct 2015 07:28:00 GMT",
            "1e100",
            "1e1",
        ] {
            assert_eq!(parse_retry_after(invalid), None);
        }
        assert_eq!(parse_retry_after("-2"), Some(-2_000_000_000));
        assert_eq!(
            transport_error("test", "timeout").kind.as_ref(),
            ProviderErrorKind::UNKNOWN
        );
    }
}
