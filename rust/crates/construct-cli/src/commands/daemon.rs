mod foreground;
mod service;
mod token;

use crate::{app::App, Result};
use clap::{ArgMatches, Args, Command, FromArgMatches};

pub fn command() -> Command {
    Command::new("daemon")
        .about("Run and manage the daemon")
        .display_order(11)
        .subcommands([
            foreground::Options::augment_args(
                Command::new("run").about("Run the daemon process in the foreground"),
            ),
            service::Install::augment_args(
                Command::new("install")
                    .about("Install and enable the Construct daemon as a system service"),
            ),
            service::Uninstall::augment_args(
                Command::new("uninstall").about("Uninstall the Construct daemon from the system"),
            ),
            service::Scope::augment_args(
                Command::new("stop").about("Stop the running daemon service"),
            ),
            token::command(),
        ])
}

pub async fn run(app: &mut App, matches: &ArgMatches) -> Result<()> {
    let Some(subcommand) = matches.subcommand() else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    match subcommand {
        ("run", args) => foreground::run(app, foreground::Options::from_arg_matches(args)?).await,
        ("install", args) => service::install(app, service::Install::from_arg_matches(args)?).await,
        ("uninstall", args) => {
            service::uninstall(app, service::Uninstall::from_arg_matches(args)?).await
        }
        ("stop", args) => service::stop(app, service::Scope::from_arg_matches(args)?).await,
        ("token", args) => token::run(app, args).await,
        _ => unreachable!(),
    }
}
