use super::{agent::agent_id, modelprovider::rpc_error};
use crate::{
    app::App,
    render::{DisplayResource, Field, OutputFormat, RenderOptions},
    Result,
};
use clap::{CommandFactory, FromArgMatches, Parser, Subcommand};
use construct_connect::Request;
use construct_proto::construct::v1::*;
use serde::Serialize;
use std::{collections::BTreeMap, io::Write};

#[derive(Parser)]
#[command(
    name = "task",
    about = "Manage tasks, which are the records of conversations",
    visible_alias = "tasks",
    display_order = 4
)]
struct Options {
    #[command(subcommand)]
    action: Option<Action>,
}
#[derive(Subcommand)]
enum Action {
    Create {
        #[arg(short = 'a', long, help = "The agent to assign to the task (required)")]
        agent: Option<String>,
        #[arg(
            short = 'w',
            long,
            default_value = "",
            help = "The workspace directory for the task"
        )]
        workspace: String,
    },
    Get {
        id: String,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "ls")]
    List {
        #[arg(
            short = 'a',
            long,
            default_value = "",
            help = "Filter tasks by the agent assigned to them"
        )]
        agent: String,
        #[arg(
            short = 'l',
            long,
            default_value_t = 0,
            allow_hyphen_values = true,
            help = "Limit the number of results returned"
        )]
        limit: i32,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "rm")]
    Delete {
        #[arg(required=true,num_args=1..)]
        ids: Vec<String>,
        #[arg(short = 'f', long, help = "Skip the confirmation prompt")]
        force: bool,
    },
}
pub fn command() -> clap::Command {
    let mut command = Options::command()
        .mut_subcommand("get", |c| c.mut_arg("format", |a| a.default_value("card")));
    command = command.mut_subcommand("create", |c| {
        c.about("Create a new task without starting an interactive session")
            .long_about(
                r#"Create a new task without starting an interactive session.

Programmatically creates a new task. This is useful for setting up tasks that
will be used later or by automated systems. To start an interactive task,
use construct new."#,
            )
            .after_help(
                r#"Examples:
  # Create a new task assigned to the 'coder' agent
  construct task create --agent coder

  # Create a task with a specific workspace
  construct task create --agent sql-expert --workspace /path/to/db/repo"#,
            )
    });
    command = command.mut_subcommand("get", |c| {
        c.about("Inspect the details of a specific task")
            .after_help(
                r#"Examples:
  # Get details for a specific task
  construct task get 01974c1d-0be8-70e1-88b4-ad9462fff25e

  # Get task details and format as YAML
  construct task get 01974c1d-0be8-70e1-88b4-ad9462fff25e --output yaml"#,
            )
    });
    command = command.mut_subcommand("list", |c| {
        c.about("List all tasks").after_help(
            r#"Examples:
  # List all recent tasks
  construct task list

  # List tasks assigned to the 'coder' agent, in JSON format
  construct task ls --agent "coder" --output json"#,
        )
    });
    command = command.mut_subcommand("delete", |c| {
        c.about("Permanently delete one or more tasks").after_help(
            r#"Examples:
  # Delete a single task
  construct task delete 01974c1d-0be8-70e1-88b4-ad9462fff25e

  # Delete multiple tasks at once
  construct task rm 01974c1d-0be8-70e1-88b4-ad9462fff25e 01974c1d-0be8-70e1-88b4-ad9462fff26f"#,
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
#[derive(Default, Serialize)]
struct Usage {
    input_tokens: i64,
    output_tokens: i64,
    cache_write_tokens: i64,
    cache_read_tokens: i64,
    cost: f64,
    tool_uses: Option<BTreeMap<String, i64>>,
}
#[derive(Serialize)]
struct Display {
    id: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    description: String,
    agent_id: String,
    workspace: String,
    created_at: String,
    updated_at: String,
    usage: Usage,
}
impl DisplayResource for Display {
    fn yaml_value(&self) -> Result<serde_yaml::Value> {
        crate::yaml::with_timestamps(self, &["created_at", "updated_at"])
    }

    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("Id", &self.id),
            Field::new("Description", &self.description),
            Field::new("AgentId", &self.agent_id),
            Field::new("Workspace", &self.workspace),
        ]
    }
}
pub(super) fn timestamp<T: Serialize>(value: &Option<T>) -> Result<String> {
    Ok(match value {
        Some(value) => {
            let value = serde_json::to_value(value)?;
            let value = value.as_str().ok_or("invalid timestamp")?;
            let value = value
                .strip_suffix("+00:00")
                .or_else(|| value.strip_suffix('Z'))
                .ok_or("invalid timestamp")?;
            match value.split_once('.') {
                Some((seconds, fraction)) => {
                    format!("{seconds}.{}Z", fraction.trim_end_matches('0'))
                }
                None => format!("{value}Z"),
            }
        }
        None => "1970-01-01T00:00:00Z".into(),
    })
}
fn display(task: Task, format: OutputFormat) -> Result<Display> {
    let metadata = task.metadata.ok_or("task metadata missing")?;
    let spec = task.spec.ok_or("task spec missing")?;
    let usage = task
        .status
        .and_then(|s| s.usage)
        .map(|u| Usage {
            input_tokens: u.input_tokens,
            output_tokens: u.output_tokens,
            cache_write_tokens: u.cache_write_tokens,
            cache_read_tokens: u.cache_read_tokens,
            cost: u.cost,
            tool_uses: (!u.tool_uses.is_empty()).then(|| u.tool_uses.into_iter().collect()),
        })
        .unwrap_or_default();
    let mut item = Display {
        id: metadata.id,
        description: spec.description,
        agent_id: spec.agent_id.unwrap_or_default(),
        workspace: spec.workspace,
        created_at: timestamp(&metadata.created_at)?,
        updated_at: timestamp(&metadata.updated_at)?,
        usage,
    };
    if format == OutputFormat::Yaml && item.usage.tool_uses.is_none() {
        item.usage.tool_uses = Some(BTreeMap::new());
    }
    Ok(item)
}
pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let Some(action) = Options::from_arg_matches(matches)?.action else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    match action {
        Action::Create { agent, workspace } => {
            let agent = agent.ok_or("required flag(s) \"agent\" not set")?;
            let workspace = if workspace.is_empty() {
                workspace
            } else {
                match std::fs::metadata(&workspace) {
                    Ok(_) => {}
                    Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
                        return Err(format!("workspace directory {workspace} does not exist").into())
                    }
                    Err(e) => {
                        return Err(format!(
                            "failed to check if workspace directory {workspace} exists: {e}"
                        )
                        .into())
                    }
                }
                let absolute = std::path::absolute(&workspace).map_err(|e| {
                    format!("failed to get absolute path of workspace directory {workspace}: {e}")
                })?;
                let mut clean = std::path::PathBuf::new();
                for component in absolute.components() {
                    if component == std::path::Component::ParentDir {
                        clean.pop();
                    } else {
                        clean.push(component);
                    }
                }
                clean.to_string_lossy().into_owned()
            };
            let id = if agent.is_empty() {
                agent
            } else {
                agent_id(app.client()?, &agent)
                    .await
                    .map_err(|e| format!("failed to resolve agent {agent}: {e}"))?
            };
            let task = app
                .client()?
                .task
                .create_task(Request::new(CreateTaskRequest {
                    agent_id: id,
                    project_directory: workspace,
                    ..Default::default()
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to create task: {e}"))?
                .message
                .task
                .ok_or("task missing")?;
            writeln!(
                app.stderr,
                "{}",
                task.metadata.ok_or("task metadata missing")?.id
            )?;
        }
        Action::Get { id, render } => {
            let task = app
                .client()?
                .task
                .get_task(Request::new(GetTaskRequest { id: id.clone() }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to get task {id}: {e}"))?
                .message
                .task
                .ok_or("task missing")?;
            app.renderer.render_one(
                app.stdout.as_mut(),
                &display(task, render.format)?,
                &render,
            )?;
        }
        Action::List {
            agent,
            limit,
            render,
        } => {
            let agent_id = if agent.is_empty() {
                None
            } else {
                Some(
                    agent_id(app.client()?, &agent)
                        .await
                        .map_err(|e| format!("failed to resolve agent {agent}: {e}"))?,
                )
            };
            let tasks = app
                .client()?
                .task
                .list_tasks(Request::new(ListTasksRequest {
                    filter: Some(list_tasks_request::Filter {
                        agent_id,
                        ..Default::default()
                    }),
                    page_size: (limit != 0).then_some(limit),
                    ..Default::default()
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to list tasks: {e}"))?
                .message
                .tasks;
            let items = tasks
                .into_iter()
                .map(|t| display(t, render.format))
                .collect::<Result<Vec<_>>>()?;
            app.renderer.render(app.stdout.as_mut(), &items, &render)?;
        }
        Action::Delete { ids, force } => {
            if !force
                && !crate::helpers::confirm_deletion(
                    app.stdin.as_mut(),
                    app.stdout.as_mut(),
                    "task",
                    &ids,
                )
            {
                return Ok(());
            }
            for id in ids {
                app.client()?
                    .task
                    .delete_task(Request::new(DeleteTaskRequest { id: id.clone() }))
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to delete task {id}: {e}"))?;
            }
        }
    }
    Ok(())
}
