use crate::{
    app::{ApiClient, App},
    render::{DisplayResource, Field, RenderOptions},
    Result,
};
use clap::{Args, CommandFactory, FromArgMatches, Parser, Subcommand};
use construct_connect::Request;
use construct_proto::construct::v1::*;
use serde::{Deserialize, Serialize};
use std::io::{Read, Write};

#[derive(Parser)]
#[command(
    name = "agent",
    about = "Manage the AI agents that perform tasks",
    visible_alias = "agents",
    display_order = 3
)]
struct Options {
    #[command(subcommand)]
    action: Option<Action>,
}
#[derive(Subcommand)]
enum Action {
    #[command(about = "Define a new, reusable AI agent")]
    Create(Create),
    #[command(about = "Inspect the details of a specific agent")]
    Get {
        name: String,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(about = "List all available agents", visible_alias = "ls")]
    List(List),
    #[command(about = "Delete one or more agents", visible_alias = "rm")]
    Delete {
        #[arg(required=true, num_args=1..)]
        names: Vec<String>,
        #[arg(short = 'f', long, help = "Skip the confirmation prompt")]
        force: bool,
    },
    #[command(about = "Apply agent configuration from a YAML file")]
    Apply {
        #[arg(short = 'f', long, help = "YAML file containing agent specification")]
        filename: String,
    },
    #[command(about = "Edit an agent's configuration in your default editor")]
    Edit { name: String },
}
#[derive(Args)]
struct Create {
    name: String,
    #[arg(
        short = 'd',
        long,
        default_value = "",
        help = "A brief description of what the agent does"
    )]
    description: String,
    #[arg(
        short = 'p',
        long,
        default_value = "",
        help = "The system prompt that defines the agent's behavior"
    )]
    prompt: String,
    #[arg(
        long,
        default_value = "",
        help = "Read the system prompt from a specified file"
    )]
    prompt_file: String,
    #[arg(long, help = "Read the system prompt from standard input (stdin)")]
    prompt_stdin: bool,
    #[arg(
        short = 'm',
        long,
        help = "The AI model the agent will use (e.g., gpt-4o) (required)"
    )]
    model: String,
}
#[derive(Args)]
struct List {
    #[arg(short='m',long="model",action=clap::ArgAction::Append,help="Filter agents by the model they use")]
    models: Vec<String>,
    #[arg(short='n',long="name",action=clap::ArgAction::Append,help="Filter agents by name (supports partial matching)")]
    names: Vec<String>,
    #[arg(
        short = 'l',
        long,
        default_value = "0",
        allow_hyphen_values = true,
        help = "Limit the number of results returned"
    )]
    limit: i32,
    #[command(flatten)]
    render: RenderOptions,
}
fn rpc_error(error: impl std::fmt::Display) -> String {
    error.to_string().trim_end_matches(": ").to_string()
}
pub fn command() -> clap::Command {
    let mut command = Options::command()
        .mut_subcommand("get", |c| c.mut_arg("format", |a| a.default_value("card")));
    command = command.mut_subcommand("create", |c| {
        c.about("Define a new, reusable AI agent")
            .long_about(
                r#"Define a new, reusable AI agent.

Creates a new agent by giving it a name, a system prompt, and assigning a model.
The system prompt defines the agent's personality, goals, and constraints.

You must specify the agent's system prompt using one of --prompt, --prompt-file,
or by piping it via --prompt-stdin."#,
            )
            .after_help(
                r#"Examples:
  # Create a simple coding assistant
  construct agent create "coder" \
    --model "gpt-4o" \
    --prompt "You are an expert Go developer. Your code is clean, efficient, and well-documented."

  # Create an agent with a prompt from a file
  construct agent create "sql-expert" \
    --model "claude-3-5-sonnet" \
    --prompt-file ./prompts/sql.txt

  # Create an agent by piping the prompt
  echo "You are a security expert reviewing code for vulnerabilities." | \
    construct agent create "reviewer" --model "gpt-4o" --prompt-stdin"#,
            )
    });
    command = command.mut_subcommand("get", |c| {
        c.about("Inspect the details of a specific agent")
            .after_help(
                r#"Examples:
  # Get details for the 'coder' agent
  construct agent get coder

  # Get details and format as JSON
  construct agent get 01974c1d-0be8-70e1-88b4-ad9462fff25e --output json"#,
            )
    });
    command = command.mut_subcommand("list", |c| {
        c.about("List all available agents").after_help(
            r#"Examples:
  # List all agents in a table
  construct agent list

  # Find all agents using a specific model
  construct agent ls --model "claude-3-5-sonnet""#,
        )
    });
    command = command.mut_subcommand("delete", |c| {
        c.about("Permanently delete one or more agents").after_help(
            r#"Examples:
  # Delete a single agent
  construct agent delete coder

  # Delete multiple agents at once
  construct agent rm architect security-reviewer

  # Force delete without a confirmation prompt
  construct agent delete old-agent --force"#,
        )
    });
    command = command.mut_subcommand("edit", |c| {
        c.about("Edit an agent's configuration in your default editor")
            .long_about(r#"Edit an agent's configuration in your default editor.

Opens the agent's configuration in your default text editor ($EDITOR). This provides
a fast and powerful way to modify an agent's prompt, model, or description. The changes are applied when you save and close the file."#)
            .after_help(r#"Examples:
  # Edit the 'coder' agent in your default editor
  construct agent edit coder

  # Use a specific editor for the session
  EDITOR=vim construct agent edit sql-expert"#)
    });
    command = command.mut_subcommand("apply", |c| {
        c.about("Apply agent configuration from a YAML file")
            .long_about(
                r#"Apply agent configuration from a YAML file.

This command reads an agent specification from a YAML file and either creates
a new agent or updates an existing one based on whether an ID is specified
in the file.

This is the declarative, automation-friendly way to manage agents, perfect
for CI/CD pipelines and scripted workflows."#,
            )
            .after_help(
                r#"Examples:
  # Apply agent configuration from file
  construct agent apply -f coder.yaml

  # Get current config, modify, and apply
  construct agent get "coder" -o yaml > coder.yaml
  # ... edit coder.yaml ...
  construct agent apply -f coder.yaml"#,
            )
    });
    for name in ["get", "list"] {
        command = command.mut_subcommand(name, |c| {
            c.mut_arg("format", |a| {
                a.help(if name == "get" {
                    "output format (json, yaml, table, card, markdown) (default: card)"
                } else {
                    "output format (json, yaml, table, card, markdown) (default: table)"
                })
            })
            .mut_arg("wide", |a| a.help("output verbosity (default: false)"))
            .mut_arg("no_headers", |a| {
                a.help("do not print headers (default: false)")
            })
        });
    }
    command
}
pub async fn agent_id(client: &ApiClient, name: &str) -> Result<String> {
    if uuid::Uuid::parse_str(name).is_ok() {
        return Ok(name.into());
    }
    let agents = client
        .agent
        .list_agents(Request::new(ListAgentsRequest {
            filter: Some(list_agents_request::Filter {
                names: vec![name.into()],
            }),
            ..Default::default()
        }))
        .await
        .map_err(rpc_error)
        .map_err(|e| format!("failed to list agents: {e}"))?
        .message
        .agents;
    match agents.as_slice() {
        [] => Err(format!("agent {name} not found").into()),
        [a] => Ok(a
            .metadata
            .as_ref()
            .ok_or("agent metadata missing")?
            .id
            .clone()),
        _ => Err(format!("multiple agents found for {name}").into()),
    }
}
pub async fn model_id(client: &ApiClient, name: &str) -> Result<String> {
    if uuid::Uuid::parse_str(name).is_ok() {
        return Ok(name.into());
    }
    let models = client
        .model
        .list_models(Request::new(ListModelsRequest {
            filter: Some(list_models_request::Filter {
                names: vec![name.into()],
                ..Default::default()
            }),
            ..Default::default()
        }))
        .await
        .map_err(rpc_error)
        .map_err(|e| format!("failed to list models: {e}"))?
        .message
        .models;
    match models.as_slice() {
        [] => Err(format!("model {name} not found").into()),
        [m] => Ok(m
            .metadata
            .as_ref()
            .ok_or("model metadata missing")?
            .id
            .clone()),
        _ => Err(format!("multiple models found for {name}").into()),
    }
}
#[derive(Debug, Serialize)]
struct Display {
    id: String,
    name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    description: String,
    instructions: String,
    model: String,
    created_at: String,
}
impl DisplayResource for Display {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("ID", &self.id),
            Field::new("Name", &self.name),
            Field::new("Description", &self.description),
            Field::new("Model", &self.model),
            Field::new("CreatedAt", &self.created_at).full(),
        ]
    }
}
fn display(agent: Agent, model: String) -> Result<Display> {
    let meta = agent.metadata.ok_or("agent metadata missing")?;
    let spec = agent.spec.ok_or("agent spec missing")?;
    let created_at = meta
        .created_at
        .and_then(|t| chrono::DateTime::from_timestamp(t.seconds, t.nanos as u32))
        .map(|t| t.format("%Y-%m-%d %H:%M:%S").to_string())
        .unwrap_or_else(|| "1970-01-01 00:00:00".into());
    Ok(Display {
        id: meta.id,
        name: spec.name,
        description: spec.description,
        instructions: spec.instructions,
        model: if model.is_empty() {
            "none".into()
        } else {
            model
        },
        created_at,
    })
}
async fn model_name(client: &ApiClient, id: &str) -> Result<String> {
    Ok(client
        .model
        .get_model(Request::new(GetModelRequest { id: id.into() }))
        .await
        .map_err(rpc_error)
        .map_err(|e| format!("failed to get model {id}: {e}"))?
        .message
        .model
        .ok_or("model missing")?
        .spec
        .ok_or("model spec missing")?
        .name)
}
fn read_file(path: &str) -> Result<String> {
    std::fs::read_to_string(path).map_err(|e| {
        if e.kind() == std::io::ErrorKind::NotFound {
            format!("open {path}: file does not exist").into()
        } else {
            e.into()
        }
    })
}
fn prompt(options: &Create, stdin: &mut dyn Read) -> Result<String> {
    let count = usize::from(!options.prompt.is_empty())
        + usize::from(!options.prompt_file.is_empty())
        + usize::from(options.prompt_stdin);
    if count == 0 {
        return Err(
            "system prompt is required (use --prompt, --prompt-file, or --prompt-stdin)".into(),
        );
    }
    if count > 1 {
        return Err(
            "only one prompt source can be specified (--prompt, --prompt-file, or --prompt-stdin)"
                .into(),
        );
    }
    if !options.prompt.is_empty() {
        return Ok(options.prompt.clone());
    }
    if !options.prompt_file.is_empty() {
        let content = read_file(&options.prompt_file)
            .map_err(|e| format!("failed to read prompt file {}: {e}", options.prompt_file))?;
        if content.is_empty() {
            return Err(format!("prompt file {} is empty", options.prompt_file).into());
        }
        return Ok(content);
    }
    let mut content = String::new();
    stdin
        .read_to_string(&mut content)
        .map_err(|e| format!("failed to read prompt from stdin: {e}"))?;
    if content.is_empty() {
        return Err("no prompt content received from stdin".into());
    }
    Ok(content)
}
#[derive(Clone, Default, Deserialize, Serialize, PartialEq)]
#[serde(default)]
struct Spec {
    #[serde(skip_serializing_if = "String::is_empty")]
    id: String,
    name: String,
    description: String,
    instructions: String,
    model: String,
}
fn parse_spec(filename: &str, applying: bool) -> Result<Spec> {
    let content = read_file(filename).map_err(|e| {
        if applying {
            format!("failed to read file: {e}").into()
        } else {
            e
        }
    })?;
    let spec = serde_yaml::from_str::<Option<Spec>>(&content)
        .map_err(|e| format!("invalid YAML format: {e}"))?
        .unwrap_or_default();
    for (key, value) in [
        ("name", &spec.name),
        ("instructions", &spec.instructions),
        ("model", &spec.model),
    ] {
        if value.is_empty() {
            if key == "instructions" {
                return Err("instructions are required".into());
            }
            return Err(format!("{key} is required").into());
        }
    }
    Ok(spec)
}
async fn update(client: &ApiClient, current: Agent, edited: Spec) -> Result<()> {
    let model = model_id(client, &edited.model)
        .await
        .map_err(rpc_error)
        .map_err(|e| format!("failed to resolve model {}: {e}", edited.model))?;
    let spec = current.spec.ok_or("agent spec missing")?;
    client
        .agent
        .update_agent(Request::new(UpdateAgentRequest {
            id: current.metadata.ok_or("agent metadata missing")?.id,
            name: (spec.name != edited.name).then_some(edited.name),
            description: (spec.description != edited.description).then_some(edited.description),
            instructions: (spec.instructions != edited.instructions).then_some(edited.instructions),
            model_id: (spec.model_id != model).then_some(model),
        }))
        .await
        .map_err(rpc_error)?;
    Ok(())
}
pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let Some(action) = Options::from_arg_matches(matches)?.action else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    match action {
        Action::Create(options) => {
            let instructions = prompt(&options, app.stdin.as_mut())?;
            let client = app.client()?;
            let model = model_id(client, &options.model).await?;
            let agent = client
                .agent
                .create_agent(Request::new(CreateAgentRequest {
                    name: options.name,
                    description: options.description,
                    instructions,
                    model_id: model,
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to create agent: {e}"))?
                .message
                .agent
                .ok_or("agent missing")?;
            writeln!(
                app.stdout,
                "{}",
                agent.metadata.ok_or("agent metadata missing")?.id
            )?;
        }
        Action::Get { name, render } => {
            let client = app.client()?;
            let id = agent_id(client, &name)
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to resolve agent {name}: {e}"))?;
            let agent = client
                .agent
                .get_agent(Request::new(GetAgentRequest { id }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to get agent {name}: {e}"))?
                .message
                .agent
                .ok_or("agent missing")?;
            let model = &agent.spec.as_ref().ok_or("agent spec missing")?.model_id;
            let model = if model.is_empty() {
                String::new()
            } else {
                model_name(client, model).await?
            };
            app.renderer
                .render_one(app.stdout.as_mut(), &display(agent, model)?, &render)?;
        }
        Action::List(options) => {
            let client = app.client()?;
            let mut models = Vec::new();
            for name in options.models {
                models.push(
                    model_id(client, &name)
                        .await
                        .map_err(rpc_error)
                        .map_err(|e| format!("failed to resolve model {name}: {e}"))?,
                );
            }
            let agents = client
                .agent
                .list_agents(Request::new(ListAgentsRequest {
                    filter: Some(list_agents_request::Filter {
                        names: options.names,
                    }),
                    ..Default::default()
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to list agents: {e}"))?
                .message
                .agents;
            let mut items = Vec::new();
            for agent in agents {
                let model = &agent.spec.as_ref().ok_or("agent spec missing")?.model_id;
                if !models.is_empty() && !models.contains(model) {
                    continue;
                }
                if options.limit > 0 && items.len() >= options.limit as usize {
                    break;
                }
                let name = model_name(client, model).await?;
                items.push(display(agent, name)?);
            }
            app.renderer
                .render(app.stdout.as_mut(), &items, &options.render)?;
        }
        Action::Delete { names, force } => {
            let mut ids = Vec::new();
            for name in &names {
                let id = agent_id(app.client()?, name).await?;
                if !ids.iter().any(|(n, _)| n == name) {
                    ids.push((name.clone(), id));
                }
            }
            if !force
                && !crate::helpers::confirm_deletion(
                    app.stdin.as_mut(),
                    app.stdout.as_mut(),
                    "agent",
                    &names,
                )
            {
                return Ok(());
            }
            for (name, id) in ids {
                app.client()?
                    .agent
                    .delete_agent(Request::new(DeleteAgentRequest { id }))
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to delete agent {name}: {e}"))?;
            }
        }
        Action::Apply { filename } => {
            if filename.is_empty() {
                return Err("filename is required. Use -f to specify the YAML file".into());
            }
            let spec = parse_spec(&filename, true)
                .map_err(|e| format!("failed to parse agent spec file: {e}"))?;
            let client = app.client()?;
            let name = spec.name.clone();
            if spec.id.is_empty() {
                let model = model_id(client, &spec.model)
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to resolve model {}: {e}", spec.model))?;
                let agent = client
                    .agent
                    .create_agent(Request::new(CreateAgentRequest {
                        name: spec.name,
                        description: spec.description,
                        instructions: spec.instructions,
                        model_id: model,
                    }))
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to create agent: {e}"))?
                    .message
                    .agent
                    .ok_or("agent missing")?;
                writeln!(
                    app.stdout,
                    "agent.construct.ai/{name} created\nID: {}",
                    agent.metadata.ok_or("agent metadata missing")?.id
                )?;
            } else {
                if uuid::Uuid::parse_str(&spec.id).is_err() {
                    return Err(format!("invalid agent ID format: {}", spec.id).into());
                }
                let current = client
                    .agent
                    .get_agent(Request::new(GetAgentRequest {
                        id: spec.id.clone(),
                    }))
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to get existing agent: {e}"))?
                    .message
                    .agent
                    .ok_or("agent missing")?;
                update(client, current, spec).await.map_err(|e| {
                    if e.to_string().starts_with("failed to resolve model") {
                        e
                    } else {
                        format!("failed to update agent: {e}").into()
                    }
                })?;
                writeln!(app.stdout, "agent.construct.ai/{name} configured")?;
            }
        }
        Action::Edit { name } => edit(app, &name).await?,
    }
    Ok(())
}
async fn edit(app: &mut App, name: &str) -> Result<()> {
    let client = app.client()?;
    let id = agent_id(client, name)
        .await
        .map_err(rpc_error)
        .map_err(|e| format!("failed to resolve agent {name}: {e}"))?;
    let current = client
        .agent
        .get_agent(Request::new(GetAgentRequest { id }))
        .await
        .map_err(rpc_error)
        .map_err(|e| format!("failed to get agent {name}: {e}"))?
        .message
        .agent
        .ok_or("agent missing")?;
    let spec = current.spec.as_ref().ok_or("agent spec missing")?;
    let original = Spec {
        name: spec.name.clone(),
        description: spec.description.clone(),
        instructions: spec.instructions.clone(),
        model: model_name(client, &spec.model_id).await?,
        ..Default::default()
    };
    let mut file = tempfile::Builder::new()
        .prefix("construct-agent-")
        .suffix(".yaml")
        .tempfile()
        .map_err(|e| format!("failed to create temporary file: {e}"))?;
    file.write_all(b"# Please edit the object below. Lines beginning with a '#' will be ignored,\n# and an empty file will abort the edit. If an error occurs while saving this file will be\n# reopened with the relevant failures.\n#\n").map_err(|e| format!("failed to create temporary file: {e}"))?;
    file.write_all(crate::yaml::to_string(&original)?.as_bytes())
        .map_err(|e| format!("failed to create temporary file: {e}"))?;
    open_editor(file.path()).map_err(|e| format!("failed to open editor: {e}"))?;
    let edited = parse_spec(file.path().to_str().ok_or("invalid temporary path")?, false)
        .map_err(|e| format!("failed to parse edited content: {e}"))?;
    if original == edited {
        writeln!(app.stdout, "No changes made.")?;
    } else {
        update(app.client()?, current, edited)
            .await
            .map_err(rpc_error)
            .map_err(|e| format!("failed to apply changes: {e}"))?;
    }
    Ok(())
}
fn open_editor(filename: &std::path::Path) -> Result<()> {
    let editor = std::env::var("EDITOR")
        .ok()
        .filter(|e| !e.is_empty())
        .or_else(|| {
            [
                "code",
                "code-insiders",
                "cursor",
                "subl",
                "atom",
                "vim",
                "nano",
                "emacs",
                "vi",
            ]
            .into_iter()
            .find(|name| {
                std::env::var_os("PATH").is_some_and(|p| {
                    std::env::split_paths(&p).any(|d| {
                        let path = d.join(name);
                        #[cfg(unix)]
                        {
                            use std::os::unix::fs::PermissionsExt;
                            path.metadata()
                                .is_ok_and(|m| m.is_file() && m.permissions().mode() & 0o111 != 0)
                        }
                        #[cfg(not(unix))]
                        {
                            path.is_file()
                        }
                    })
                })
            })
            .map(str::to_string)
        })
        .ok_or("no editor found. Please set the EDITOR environment variable")?;
    let mut command = std::process::Command::new(&editor);
    if matches!(
        std::path::Path::new(&editor)
            .file_name()
            .and_then(|s| s.to_str()),
        Some("code" | "code-insiders" | "cursor" | "subl" | "atom")
    ) {
        command.arg("--wait");
    }
    let status = command.arg(filename).status()?;
    if !status.success() {
        return Err(format!("exit status {}", status.code().unwrap_or(1)).into());
    }
    Ok(())
}
