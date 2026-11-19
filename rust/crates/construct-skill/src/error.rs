use crate::ParseError;

/// Errors from installing, updating, deleting, or listing skills. Messages match Go.
#[derive(Debug, thiserror::Error)]
pub enum SkillError {
    /// The install source string could not be parsed.
    #[error("{0}")]
    InvalidSource(String),
    /// A skill with this name is installed and `force` was not set.
    #[error("skill {0:?} already exists, use --force to overwrite")]
    AlreadyExists(String),
    /// No installed skill has this name.
    #[error("skill {0:?} not found")]
    NotFound(String),
    /// The skill has no remote source in the lock file.
    #[error("skill {0:?} was manually installed and cannot be updated")]
    NotUpdatable(String),
    /// The name cannot be used as a skill directory.
    #[error("invalid skill name {0:?}")]
    InvalidName(String),
    /// The fetched SKILL.md could not be parsed or validated.
    #[error("failed to parse SKILL.md: {0}")]
    Parse(#[from] ParseError),
    /// Any other failure, with Go's wording.
    #[error("{0}")]
    Other(String),
}

impl SkillError {
    pub(crate) fn other(context: &str, err: impl std::fmt::Display) -> Self {
        Self::Other(format!("{context}: {err}"))
    }
}
