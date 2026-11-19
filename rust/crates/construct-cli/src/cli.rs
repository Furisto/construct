use crate::{
    app::{requires_context, App},
    commands, Result,
};
use clap::{Command, CommandFactory, FromArgMatches, Parser};

#[derive(Debug, Parser)]
#[command(name = "construct", about = "Construct: Build intelligent agents.")]
pub struct Cli {
    #[arg(long,global=true,value_parser=crate::logging::parse_level,help="set the log level")]
    pub log_level: Option<String>,
    #[arg(
        long,
        global = true,
        help = "context to use (overrides current context)"
    )]
    pub context: Option<String>,
}
pub fn command() -> Command {
    Cli::command()
        .subcommands([
            commands::new_session::command(),
            commands::resume::command(),
            commands::exec::command(),
            commands::agent::command(),
            commands::task::command(),
            commands::message::command(),
            commands::model::command(),
            commands::modelprovider::command(),
            commands::skill::command(),
            commands::context::command(),
            commands::config::command(),
            commands::daemon::command(),
            commands::info::command(),
            commands::update::command(),
        ])
        .subcommand_help_heading("Commands")
        .override_help(ROOT_HELP)
}
const ROOT_HELP:&str="Construct: Build intelligent agents.\n\nUsage: construct [OPTIONS] [COMMAND]\n\nCore Commands:\n  new            Launch a new interactive session with an agent\n  resume         Continue a previous task\n  exec           Execute a non-interactive task with an agent\n\nResource Management:\n  agent          Manage the AI agents that perform tasks\n  task           Manage tasks, which are the records of conversations\n  message        Manage messages\n  model          Manage the AI models available to agents\n  modelprovider  Configure providers like OpenAI and Anthropic\n  skill          Manage skills that extend agent capabilities\n\nSystem Commands:\n  context        Manage connection contexts for local and remote daemons\n  config         Manage configuration\n  daemon         Run the daemon\n  info           Print information about the Construct CLI\n  update         Update Construct to the latest version\n\nOptions:\n      --log-level <LOG_LEVEL>  set the log level\n      --context <CONTEXT>      context to use (overrides current context)\n  -h, --help                   Print help\n";
pub async fn run(matches: clap::ArgMatches) -> Result<()> {
    let options = Cli::from_arg_matches(&matches)?;
    let Some((name, args)) = matches.subcommand() else {
        command().print_help()?;
        return Ok(());
    };
    let user = crate::paths::UserInfo::from_env()?;
    let level = crate::logging::resolve_level(
        options.log_level.as_deref(),
        std::env::var("CONSTRUCT_LOG_LEVEL").ok().as_deref(),
    );
    crate::logging::init(&user, &level, false)?;
    let mut app = App::new(user, options.context.unwrap_or_default())?;
    if requires_context(name) {
        app.client()?;
    }
    let cancellation = app.cancellation.clone();
    let future = async {
        match name {
            "new" => commands::new_session::run(&mut app, args).await,
            "resume" => commands::resume::run(&mut app, args).await,
            "exec" => commands::exec::run(&mut app, args).await,
            "agent" => commands::agent::run(&mut app, args).await,
            "task" => commands::task::run(&mut app, args).await,
            "message" => commands::message::run(&mut app, args).await,
            "model" => commands::model::run(&mut app, args).await,
            "modelprovider" => commands::modelprovider::run(&mut app, args).await,
            "skill" => commands::skill::run(&mut app, args).await,
            "context" => commands::context::run(&mut app, args).await,
            "config" => commands::config::run(&mut app, args).await,
            "daemon" => commands::daemon::run(&mut app, args).await,
            "info" => commands::info::run(&mut app, args).await,
            "update" => commands::update::run(&mut app, args).await,
            _ => unreachable!(),
        }
    };
    if name == "daemon" && args.subcommand_name() == Some("run") {
        future.await
    } else {
        tokio::select! {result=future=>result,_=tokio::signal::ctrl_c()=>{cancellation.cancel();Ok(())}}
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn tree() {
        command().debug_assert();
        for name in [
            "new", "resume", "exec", "agents", "tasks", "msg", "models", "mp", "skills", "ctx",
            "config", "daemon", "info", "update",
        ] {
            assert!(command()
                .try_get_matches_from(["construct", name, "--context", "local"])
                .is_ok());
        }
        assert!(command()
            .try_get_matches_from(["construct", "--log-level", "trace"])
            .unwrap_err()
            .to_string()
            .contains("must be one of \"debug\", \"info\", \"warn\", or \"error\""));
    }
}
