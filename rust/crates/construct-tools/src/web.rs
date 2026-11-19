//! HTTP fetching, article extraction, and Markdown conversion.
use crate::{custom, decode, description, schema};
use construct_core::tool::{Tool, ToolContext, ToolError};
use construct_types::tools::{FetchInput, FetchResult, ToolOutput};
use futures_util::StreamExt;
use serde_json::Value;
use std::time::Duration;

/// Go's five-MiB response limit, applied before HTML extraction.
pub const MAX_CONTENT_SIZE: usize = 5 * 1024 * 1024;
/// Fetch tool with an injectable HTTP client.
#[derive(Clone, Debug, Default)]
pub struct Fetch {
    client: reqwest::Client,
}

impl Fetch {
    /// Use an existing client (proxy/TLS/redirect policy belongs to the caller).
    pub fn new(client: reqwest::Client) -> Self {
        Self { client }
    }
}

#[construct_core::async_trait]
impl Tool for Fetch {
    fn name(&self) -> &str {
        "fetch"
    }
    fn description(&self) -> &str {
        description(self.name())
    }
    fn input_schema(&self) -> Value {
        schema(self.name())
    }
    async fn execute(&self, ctx: &ToolContext, value: Value) -> Result<ToolOutput, ToolError> {
        let input: FetchInput = decode(value)?;
        if input.url.is_empty() {
            return Err(custom(
                "URL is required",
                &["Provide a valid HTTP or HTTPS URL"],
                &[],
            ));
        }
        let scheme = input
            .url
            .split_once(':')
            .map(|(s, _)| s)
            .unwrap_or("")
            .to_ascii_lowercase();
        if scheme != "http" && scheme != "https" {
            return Err(custom(
                "Invalid URL scheme",
                &["Only http:// and https:// URLs are supported"],
                &[("scheme", scheme.into())],
            ));
        }
        let url = reqwest::Url::parse(&input.url).map_err(|e| {
            custom(
                "Invalid URL",
                &[
                    "Ensure the URL is properly formatted",
                    "Include the protocol (http:// or https://)",
                ],
                &[
                    ("url", input.url.clone().into()),
                    ("error", e.to_string().into()),
                ],
            )
        })?;
        let timeout = if input.timeout <= 0 {
            30
        } else {
            input.timeout
        };
        let work = async {
            let mut request = self.client.get(url.clone()).header("User-Agent", "Mozilla/5.0 (compatible; ConstructBot/1.0)")
                .header("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json;q=0.8,*/*;q=0.7")
                .header("Accept-Language", "en-US,en;q=0.5");
            for (key, value) in input.headers.as_ref().into_iter().flatten() {
                let mut headers = reqwest::header::HeaderMap::new();
                let invalid = |error: String| {
                    custom(
                        "Failed to fetch URL",
                        &[
                            "Check that the URL is accessible",
                            "Verify your network connection",
                        ],
                        &[("url", input.url.clone().into()), ("error", error.into())],
                    )
                };
                headers.insert(
                    reqwest::header::HeaderName::from_bytes(key.as_bytes())
                        .map_err(|e| invalid(e.to_string()))?,
                    reqwest::header::HeaderValue::from_str(value)
                        .map_err(|e| invalid(e.to_string()))?,
                );
                request = request.headers(headers);
            }
            let response = request.send().await.map_err(|e| {
                custom(
                    "Failed to fetch URL",
                    &[
                        "Check that the URL is accessible",
                        "Verify your network connection",
                    ],
                    &[
                        ("url", input.url.clone().into()),
                        ("error", e.to_string().into()),
                    ],
                )
            })?;
            let status = response.status();
            if !status.is_success() {
                return Err(custom(
                    "HTTP request failed",
                    &[
                        "The server returned an error status",
                        "Check that the URL is correct and accessible",
                    ],
                    &[
                        ("status_code", status.as_u16().into()),
                        ("status", status.to_string().into()),
                    ],
                ));
            }
            let content_type = response
                .headers()
                .get("Content-Type")
                .and_then(|h| h.to_str().ok())
                .unwrap_or("")
                .to_owned();
            let html =
                content_type.contains("text/html") || content_type.contains("application/xhtml");
            let json = content_type.contains("application/json");
            if !html && !json {
                return Err(custom(
                    "Unsupported content type",
                    &[
                        "This tool supports HTML web pages and JSON responses",
                        &format!("The server returned content type: {content_type}"),
                    ],
                    &[("content_type", content_type.into())],
                ));
            }
            let mut body = Vec::new();
            let mut stream = response.bytes_stream();
            while let Some(chunk) = stream.next().await {
                let chunk = chunk.map_err(|e| {
                    custom(
                        "Failed to read response body",
                        &["The server response could not be read"],
                        &[("error", e.to_string().into())],
                    )
                })?;
                let remaining = (MAX_CONTENT_SIZE + 1).saturating_sub(body.len());
                body.extend_from_slice(&chunk[..remaining.min(chunk.len())]);
                if body.len() > MAX_CONTENT_SIZE {
                    break;
                }
            }
            let truncated = body.len() > MAX_CONTENT_SIZE;
            body.truncate(MAX_CONTENT_SIZE);
            let size = body.len();
            let authority_and_path = input.url.split_once("://").map(|(_, s)| s).unwrap_or("");
            let authority = authority_and_path
                .split(['/', '?', '#'])
                .next()
                .unwrap_or("");
            let host = authority.rsplit('@').next().unwrap_or("");
            let path = if authority_and_path
                .get(authority.len()..)
                .is_some_and(|s| s.starts_with('/'))
            {
                url.path()
            } else {
                ""
            };
            let fallback_title = format!("{host}{path}");
            let (content, title) = if json {
                (crate::go_string(&body), fallback_title)
            } else {
                let url = url.clone();
                tokio::task::spawn_blocking(move || {
                    let source = crate::go_string(&body);
                    let heading = regex::Regex::new(r"(?i)(</?)h1(\s|>)").expect("heading pattern");
                    let source = heading.replace_all(&source, "${1}h2$2");
                    let article = readability::extractor::extract(
                        &mut std::io::Cursor::new(source.as_bytes()),
                        &url,
                    )
                    .map_err(|e| {
                        custom(
                            "Failed to extract content from page",
                            &[
                                "The page content could not be parsed",
                                "This may happen with non-standard HTML or JavaScript-heavy pages",
                            ],
                            &[("error", e.to_string().into())],
                        )
                    })?;
                    let markdown = markdown(&article.content);
                    Ok::<_, ToolError>((
                        cleanup(&markdown),
                        if article.title.is_empty() {
                            fallback_title
                        } else {
                            article.title
                        },
                    ))
                })
                .await
                .map_err(|e| custom(e.to_string(), &[], &[]))??
            };
            Ok(ToolOutput {
                fetch: Some(Box::new(FetchResult {
                    url: input.url.clone(),
                    title,
                    content,
                    content_type: if json { "json" } else { "html" }.into(),
                    byte_size: size as i64,
                    truncated,
                })),
                ..Default::default()
            })
        };
        tokio::select! {
            result = tokio::time::timeout(Duration::from_secs(timeout as u64),work) => result.unwrap_or_else(|_| Err(custom("Request timed out", &["The server took too long to respond", "Try increasing the timeout parameter"], &[("timeout_seconds",timeout.into())]))),
            () = ctx.cancellation.cancelled() => Err(custom("Failed to fetch URL", &["Check that the URL is accessible", "Verify your network connection"], &[("url",input.url.clone().into()), ("error","context canceled".into())])),
        }
    }
}

fn cleanup(content: &str) -> String {
    let mut empty = 0;
    content
        .split('\n')
        .filter(|line| {
            if line.trim().is_empty() {
                empty += 1;
                empty <= 2
            } else {
                empty = 0;
                true
            }
        })
        .collect::<Vec<_>>()
        .into_iter()
        .map(|line| if line.trim().is_empty() { "" } else { line })
        .collect::<Vec<_>>()
        .join("\n")
        .trim()
        .into()
}

#[derive(Default)]
struct Heading;
impl html2md::TagHandler for Heading {
    fn handle(&mut self, tag: &html2md::Handle, printer: &mut html2md::StructuredPrinter) {
        if let html2md::NodeData::Element { name, .. } = &tag.data {
            let level = name
                .local
                .to_string()
                .trim_start_matches('h')
                .parse::<usize>()
                .unwrap_or(2)
                .max(2);
            printer.append_str(&format!("\n\n{} ", "#".repeat(level)));
        }
    }
    fn after_handle(&mut self, printer: &mut html2md::StructuredPrinter) {
        printer.append_str("\n\n");
    }
}

fn markdown(content: &str) -> String {
    let mut handlers: std::collections::HashMap<String, Box<dyn html2md::TagHandlerFactory>> =
        std::collections::HashMap::new();
    for level in 1..=6 {
        handlers.insert(format!("h{level}"), Box::new(Heading::default));
    }
    for tag in ["head", "script", "style"] {
        handlers.insert(tag.into(), Box::new(Skip::default));
    }
    html2md::parse_html_custom(content, &handlers)
}

#[derive(Default)]
struct Skip;
impl html2md::TagHandler for Skip {
    fn handle(&mut self, _tag: &html2md::Handle, _printer: &mut html2md::StructuredPrinter) {}
    fn after_handle(&mut self, _printer: &mut html2md::StructuredPrinter) {}
    fn skip_descendants(&self) -> bool {
        true
    }
}
