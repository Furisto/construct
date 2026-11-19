use construct_connect::Request;
use construct_proto::construct::v1::*;

pub fn command() -> clap::Command {
    clap::Command::new("new")
        .about("Launch a new interactive session with an agent")
        .display_order(0)
        .arg(clap::Arg::new("arguments").num_args(0..))
        .arg(
            clap::Arg::new("agent")
                .long("agent")
                .default_value("")
                .help("Start the session with a specific agent. Defaults to the last used agent"),
        )
        .arg(
            clap::Arg::new("workspace")
                .long("workspace")
                .default_value("")
                .help("Set the agent's working directory. Defaults to the current directory"),
        )
}

pub async fn run(app: &mut crate::app::App, matches: &clap::ArgMatches) -> crate::Result<()> {
    let workspace = matches.get_one::<String>("workspace").unwrap();
    let workspace = if workspace.is_empty() {
        std::env::current_dir()?.to_string_lossy().into_owned()
    } else {
        workspace.clone()
    };
    let client = app.client()?;
    let id =
        crate::terminal::resolve_agent(client, matches.get_one::<String>("agent").unwrap()).await?;
    let agent = client
        .agent
        .get_agent(Request::new(GetAgentRequest { id }))
        .await?
        .message
        .agent
        .ok_or("missing agent")?;
    let task = client
        .task
        .create_task(Request::new(CreateTaskRequest {
            agent_id: agent
                .metadata
                .as_ref()
                .ok_or("missing agent metadata")?
                .id
                .clone(),
            project_directory: workspace,
            ..Default::default()
        }))
        .await?
        .message
        .task
        .ok_or("missing task")?;
    crate::terminal::ui::run(client, task, agent, app.cancellation.clone()).await
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn flags_and_defaults() {
        let args = command().try_get_matches_from(["new"]).unwrap();
        assert_eq!(args.get_one::<String>("agent").unwrap(), "");
        assert_eq!(args.get_one::<String>("workspace").unwrap(), "");
        let args = command()
            .try_get_matches_from([
                "new",
                "ignored",
                "--agent",
                "coder",
                "--workspace",
                "/path with spaces",
            ])
            .unwrap();
        assert_eq!(
            args.get_one::<String>("workspace").unwrap(),
            "/path with spaces"
        );
        assert_eq!(args.get_one::<String>("agent").unwrap(), "coder");
        assert!(command().try_get_matches_from(["new", "--agent"]).is_err());
    }
}
