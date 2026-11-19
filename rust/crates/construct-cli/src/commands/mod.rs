pub mod agent;
pub mod config;
pub mod context;
pub mod daemon;
pub mod exec;
pub mod info;
pub mod message;
pub mod model;
pub mod modelprovider;
pub mod new_session;
pub mod resume;
pub mod skill;
pub mod task;
pub mod update;

pub fn stub(name: &str) -> crate::Result<()> {
    Err(format!("{name}: not yet implemented").into())
}
