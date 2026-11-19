//! Shared CLI foundation. Resource commands live in independent `commands` modules.
pub mod app;
pub mod cli;
pub mod commands;
pub mod config;
pub mod context;
pub mod credentials;
pub mod errors;
pub mod helpers;
pub mod logging;
pub mod paths;
pub mod render;
pub mod yaml;

pub type Error = Box<dyn std::error::Error + Send + Sync>;
pub type Result<T> = std::result::Result<T, Error>;
