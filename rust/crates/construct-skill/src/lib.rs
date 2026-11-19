//! Agent skills: SKILL.md parsing, workspace discovery, and git/URL installs.
//!
//! Ports Go `backend/skill`. On-disk formats (`skills/<name>/SKILL.md`,
//! `skills.lock.json` and its `.lock` flock file) are shared with the Go daemon.
#![deny(missing_docs)]

mod dirs;
mod discovery;
mod error;
mod git;
mod lockfile;
mod manager;
mod parser;
mod prompt;
pub mod service;
mod source;

pub use dirs::UserDirs;
pub use discovery::{default_discovery_paths, Discoverer, DiscoveryPath};
pub use error::SkillError;
pub use lockfile::{LOCK_FILE_NAME, LOCK_FILE_VERSION};
pub use manager::{InstallOptions, InstalledSkill, SkillManager};
pub use parser::{parse, validate, ParseError};
pub use prompt::format_available_skills;
pub use source::{parse_source, Source};

pub use construct_types::skill::{GitProvider, GitSource, LockEntry, LockFile, URLSource};

/// File name of a skill's instructions inside its directory.
pub const SKILL_FILE_NAME: &str = "SKILL.md";
/// Maximum skill name length in bytes.
pub const MAX_NAME_LENGTH: usize = 64;
/// Maximum description length (declared by Go, not enforced by validation).
pub const MAX_DESCRIPTION_LENGTH: usize = 1024;
/// Minimum description length.
pub const MIN_DESCRIPTION_LENGTH: usize = 1;

/// Where a discovered skill was found.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum SkillScope {
    /// Inside the workspace repository.
    Repo,
    /// In the user's home or config directory.
    User,
    /// Machine-wide (`/etc/construct/skills`).
    System,
}

impl SkillScope {
    /// Go string value (`repo`, `user`, `system`).
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Repo => "repo",
            Self::User => "user",
            Self::System => "system",
        }
    }
}

/// A parsed skill.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Skill {
    /// Frontmatter `name`.
    pub name: String,
    /// Frontmatter `description`.
    pub description: String,
    /// Path of the SKILL.md file, or the source URL for a just-fetched URL skill.
    pub location: String,
    /// Discovery scope; `None` for skills read by the manager.
    pub scope: Option<SkillScope>,
}
