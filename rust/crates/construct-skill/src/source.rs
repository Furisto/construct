use crate::{GitProvider, SkillError, SKILL_FILE_NAME};
use regex::Regex;
use std::sync::LazyLock;

/// A parsed install source (Go `Source`): a git repository or a direct SKILL.md URL.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Source {
    /// Hosting provider for git sources.
    pub provider: GitProvider,
    /// HTTPS clone URL; non-empty for git sources.
    pub clone_url: String,
    /// Path within the repository to search for skills.
    pub path: String,
    /// Branch or tag; defaults to `main`.
    pub r#ref: String,
    /// Direct URL to a SKILL.md; non-empty for URL sources.
    pub direct_url: String,
}

impl Source {
    /// Whether this is a git repository source.
    pub fn is_git(&self) -> bool {
        !self.clone_url.is_empty()
    }

    /// Whether this is a direct URL source.
    pub fn is_url(&self) -> bool {
        !self.direct_url.is_empty()
    }
}

static SHORTHAND: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"^([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)(?:/(.+))?$").unwrap());
static GITHUB_URL: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^(?:https?://)?github\.com/([^/]+)/([^/]+)(?:/tree/([^/]+)(?:/(.+))?)?$").unwrap()
});
static GITLAB_URL: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^(?:https?://)?gitlab\.com/([^/]+)/([^/]+)(?:/-/tree/([^/]+)(?:/(.+))?)?$")
        .unwrap()
});

/// Go `ParseSource`: `owner/repo[/path]`, `github.com/owner/repo/tree/ref/path`,
/// `gitlab.com/owner/repo/-/tree/ref/path`, or a URL ending in `/SKILL.md`.
pub fn parse_source(source: &str) -> Result<Source, SkillError> {
    let source = source.trim();
    if source.is_empty() {
        return Err(SkillError::InvalidSource("source cannot be empty".into()));
    }
    if source.ends_with(&format!("/{SKILL_FILE_NAME}"))
        || source.ends_with(&format!("/{}", SKILL_FILE_NAME.to_lowercase()))
    {
        let url = if source.starts_with("http://") || source.starts_with("https://") {
            source.to_owned()
        } else {
            format!("https://{source}")
        };
        return Ok(Source {
            direct_url: url,
            ..Source::default()
        });
    }
    for (pattern, provider, host) in [
        (&GITHUB_URL, GitProvider::GIT_HUB, "github.com"),
        (&GITLAB_URL, GitProvider::GIT_LAB, "gitlab.com"),
    ] {
        if let Some(caps) = pattern.captures(source) {
            let group = |i| caps.get(i).map_or("", |m| m.as_str());
            let r#ref = if group(3).is_empty() {
                "main"
            } else {
                group(3)
            };
            return Ok(Source {
                provider: provider.into(),
                clone_url: format!("https://{host}/{}/{}.git", group(1), group(2)),
                path: group(4).to_owned(),
                r#ref: r#ref.to_owned(),
                direct_url: String::new(),
            });
        }
    }
    if let Some(caps) = SHORTHAND.captures(source) {
        if source.contains("://") {
            return Err(SkillError::InvalidSource(format!(
                "unrecognized source format: {source}"
            )));
        }
        return Ok(Source {
            provider: GitProvider::GIT_HUB.into(),
            clone_url: format!("https://github.com/{}/{}.git", &caps[1], &caps[2]),
            path: caps.get(3).map_or("", |m| m.as_str()).to_owned(),
            r#ref: "main".into(),
            direct_url: String::new(),
        });
    }
    Err(SkillError::InvalidSource(format!(
        "unrecognized source format: {source}"
    )))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn git(provider: &str, clone_url: &str, path: &str, r#ref: &str) -> Source {
        Source {
            provider: provider.into(),
            clone_url: clone_url.into(),
            path: path.into(),
            r#ref: r#ref.into(),
            direct_url: String::new(),
        }
    }

    #[test]
    fn parses_go_test_cases() {
        let cases = [
            (
                "anthropics/skills",
                git(
                    "github",
                    "https://github.com/anthropics/skills.git",
                    "",
                    "main",
                ),
            ),
            (
                "anthropics/skills/skills/pdf",
                git(
                    "github",
                    "https://github.com/anthropics/skills.git",
                    "skills/pdf",
                    "main",
                ),
            ),
            (
                "github.com/anthropics/skills",
                git(
                    "github",
                    "https://github.com/anthropics/skills.git",
                    "",
                    "main",
                ),
            ),
            (
                "https://github.com/anthropics/skills/tree/develop/skills/pdf",
                git(
                    "github",
                    "https://github.com/anthropics/skills.git",
                    "skills/pdf",
                    "develop",
                ),
            ),
            (
                "gitlab.com/org/repo/-/tree/v1.0/skills",
                git(
                    "gitlab",
                    "https://gitlab.com/org/repo.git",
                    "skills",
                    "v1.0",
                ),
            ),
            (
                "  owner/repo  ",
                git("github", "https://github.com/owner/repo.git", "", "main"),
            ),
        ];
        for (input, want) in cases {
            assert_eq!(parse_source(input).unwrap(), want, "{input}");
        }
    }

    #[test]
    fn parses_direct_urls() {
        let url = |s: &str| parse_source(s).unwrap().direct_url;
        assert_eq!(
            url("https://example.com/skills/my-skill/SKILL.md"),
            "https://example.com/skills/my-skill/SKILL.md"
        );
        assert_eq!(
            url("example.com/skills/skill.md"),
            "https://example.com/skills/skill.md"
        );
        assert_eq!(
            url("http://example.com/SKILL.md"),
            "http://example.com/SKILL.md"
        );
        assert!(parse_source("https://x.com/a/SKILL.md").unwrap().is_url());
    }

    #[test]
    fn rejects_bad_sources() {
        for input in [
            "",
            "   ",
            "just-a-name",
            "https://example.com/owner/repo",
            "a/b c",
        ] {
            assert!(
                matches!(parse_source(input), Err(SkillError::InvalidSource(_))),
                "{input}"
            );
        }
        assert_eq!(
            parse_source("").unwrap_err().to_string(),
            "source cannot be empty"
        );
        assert_eq!(
            parse_source("x").unwrap_err().to_string(),
            "unrecognized source format: x"
        );
    }
}
