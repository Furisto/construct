//! Go skill domain values; declaration order is JSON field order.
use crate::Timestamp;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
string_type!(GitProvider, GIT_HUB = "github", GIT_LAB = "gitlab");

/// Go `backend/skill/manager.go` `LockFile`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct LockFile {
    /// Go `Version` field.
    #[serde(rename = "version")]
    pub version: i64,
    /// Go `Skills` field.
    #[serde(rename = "skills")]
    pub skills: Option<BTreeMap<String, Option<Box<LockEntry>>>>,
}

/// Go `backend/skill/manager.go` `LockEntry`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct LockEntry {
    /// Go `InstalledAt` field.
    #[serde(rename = "installed_at")]
    pub installed_at: Timestamp,
    /// Go `UpdatedAt` field.
    #[serde(rename = "updated_at")]
    pub updated_at: Timestamp,
    /// Go `Git` field.
    #[serde(rename = "git", skip_serializing_if = "Option::is_none")]
    pub git: Option<Box<GitSource>>,
    /// Go `URL` field.
    #[serde(rename = "url", skip_serializing_if = "Option::is_none")]
    pub url: Option<Box<URLSource>>,
}

/// Go `backend/skill/manager.go` `GitSource`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct GitSource {
    /// Go `Provider` field.
    #[serde(rename = "provider")]
    pub provider: GitProvider,
    /// Go `CloneURL` field.
    #[serde(rename = "clone_url")]
    pub clone_url: String,
    /// Go `Path` field.
    #[serde(rename = "path")]
    pub path: String,
    /// Go `Ref` field.
    #[serde(rename = "ref")]
    pub r#ref: String,
    /// Go `TreeHash` field.
    #[serde(rename = "tree_hash")]
    pub tree_hash: String,
}

/// Go `backend/skill/manager.go` `URLSource`.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct URLSource {
    /// Go `URL` field.
    #[serde(rename = "url")]
    pub url: String,
}
