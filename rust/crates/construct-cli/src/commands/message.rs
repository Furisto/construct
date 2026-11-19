use super::{agent::agent_id, modelprovider::rpc_error, task::timestamp};
use crate::{
    app::App,
    render::{DisplayResource, Field, RenderOptions},
    Result,
};
use clap::{CommandFactory, FromArgMatches, Parser, Subcommand};
use construct_connect::Request;
use construct_proto::construct::v1::*;
use serde::Serialize;
use std::io::Write;

#[derive(Parser)]
#[command(name="message",about="Manage messages",visible_aliases=["messages","msg"],display_order=5)]
struct Options {
    #[command(subcommand)]
    action: Option<Action>,
}
#[derive(Subcommand)]
enum Action {
    Create {
        task_id: String,
        content: String,
        #[command(flatten)]
        render: RenderOptions,
    },
    Get {
        id: String,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "ls")]
    List {
        #[arg(
            short = 't',
            long,
            default_value = "",
            help = "(Recommended) Filter messages by task ID"
        )]
        task: String,
        #[arg(
            short = 'a',
            long,
            default_value = "",
            help = "Filter by the agent that participated in the conversation"
        )]
        agent: String,
        #[arg(
            short = 'r',
            long,
            default_value = "",
            help = "Filter messages by the role of the author"
        )]
        role: String,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "rm")]
    Delete {
        #[arg(required=true,num_args=1..)]
        ids: Vec<String>,
        #[arg(short = 'f', long, help = "force deletion without confirmation")]
        force: bool,
    },
}
pub fn command() -> clap::Command {
    let mut command = Options::command()
        .mut_subcommand("get", |c| c.mut_arg("format", |a| a.default_value("card")));
    command = command.mut_subcommand("create", |c| {
        c.about("Add a message to a task programmatically")
            .long_about(
                r#"Add a message to a task programmatically.

Appends a new message to a task's history. This is an advanced command, typically
used for scripting or integrating external tools with Construct tasks."#,
            )
            .after_help(
                r#"Examples:
  # Add a user message to an existing task
  construct message create "01974c1d-0be8-70e1-88b4-ad9462fff25e" "Please check the file again.""#,
            )
    });
    command = command.mut_subcommand("get", |c| {
        c.about("Get a message by ID")
            .long_about(r#"Get detailed information about a message by specifying its ID."#)
            .after_help(
                r#"Examples:
  # Get a message by ID
  construct message get "123e4567-e89b-12d3-a456-426614174000""#,
            )
    });
    command = command.mut_subcommand("list", |c| {
        c.about("List messages")
            .long_about(
                r#"List messages.

Lists messages, typically filtered by a specific task. Useful for reviewing or
exporting a conversation history."#,
            )
            .after_help(
                r#"Examples:
  # List all messages for a specific task
  construct message list --task "01974c1d-0be8-70e1-88b4-ad9462fff25e"

  # List only the assistant's responses in that task
  construct message list --task "01974c1d-0be8-70e1-88b4-ad9462fff25e" --role assistant"#,
            )
    });
    command = command.mut_subcommand("delete", |c| {
        c.about("Delete one or more messages by ID")
            .long_about(r#"Delete messages by specifying their IDs."#)
            .after_help(
                r#"Examples:
  # Delete a single message
  construct message delete "123e4567-e89b-12d3-a456-426614174000""#,
            )
    });
    for name in ["create", "get", "list"] {
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
}
#[derive(Serialize)]
struct Display {
    id: String,
    task_id: String,
    agent: String,
    model: String,
    role: String,
    content: String,
    created_at: String,
    updated_at: String,
    usage: Usage,
}
impl DisplayResource for Display {
    fn fields(&self) -> Vec<Field<'_>> {
        let mut fields = vec![
            Field::new("ID", &self.id),
            Field::new("TaskID", &self.task_id),
            Field::new("Agent", &self.agent),
            Field::new("Model", &self.model),
            Field::new("Role", &self.role),
        ];
        fields.extend([
            Field::new("CreatedAt", go_time(&self.created_at)).full(),
            Field::new("UpdatedAt", go_time(&self.updated_at)).full(),
        ]);
        fields
    }
}
fn go_time(value: &str) -> String {
    format!(
        "{} +0000 UTC",
        value.trim_end_matches('Z').replace('T', " ")
    )
}
fn display(message: Message) -> Result<Display> {
    let metadata = message.metadata.ok_or("message metadata missing")?;
    let spec = message.spec.ok_or("message spec missing")?;
    let mut content = String::new();
    for part in spec.content {
        if let Some(message_part::Data::Text(text)) = part.data {
            content.push_str(&text.content);
        }
    }
    let usage = message
        .status
        .and_then(|s| s.usage)
        .map(|u| Usage {
            input_tokens: u.input_tokens,
            output_tokens: u.output_tokens,
            cache_write_tokens: u.cache_write_tokens,
            cache_read_tokens: u.cache_read_tokens,
            cost: u.cost,
        })
        .unwrap_or_default();
    Ok(Display {
        id: metadata.id,
        task_id: metadata.task_id,
        agent: metadata.agent_id.unwrap_or_default(),
        model: metadata.model_id.unwrap_or_default(),
        role: match MessageRole::try_from(metadata.role) {
            Ok(MessageRole::User) => "user",
            Ok(MessageRole::Assistant) => "assistant",
            _ => "unknown",
        }
        .into(),
        content,
        created_at: timestamp(&metadata.created_at)?,
        updated_at: timestamp(&metadata.updated_at)?,
        usage,
    })
}
pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let Some(action) = Options::from_arg_matches(matches)?.action else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    match action {
        Action::Create {
            task_id, content, ..
        } => {
            let message = app
                .client()?
                .message
                .create_message(Request::new(CreateMessageRequest {
                    task_id,
                    content: vec![MessagePart {
                        data: Some(message_part::Data::Text(message_part::Text { content })),
                    }],
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to create message: {e}"))?
                .message
                .message
                .ok_or("message missing")?;
            writeln!(
                app.stderr,
                "{}",
                message.metadata.ok_or("message metadata missing")?.id
            )?;
        }
        Action::Get { id, render } => {
            let message = app
                .client()?
                .message
                .get_message(Request::new(GetMessageRequest { id: id.clone() }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to get message {id}: {e}"))?
                .message
                .message
                .ok_or("message missing")?;
            app.renderer
                .render_one(app.stdout.as_mut(), &display(message)?, &render)?;
        }
        Action::List {
            task,
            agent,
            role,
            render,
        } => {
            let agent_ids = if agent.is_empty() {
                None
            } else {
                Some(
                    agent_id(app.client()?, &agent)
                        .await
                        .map_err(|e| format!("failed to resolve agent {agent}: {e}"))?,
                )
            };
            let roles = match role.as_str() {
                "" => None,
                "user" => Some(MessageRole::User as i32),
                "assistant" => Some(MessageRole::Assistant as i32),
                _ => {
                    return Err(
                        format!("invalid role {role}: must be 'user' or 'assistant'").into(),
                    )
                }
            };
            let messages = app
                .client()?
                .message
                .list_messages(Request::new(ListMessagesRequest {
                    filter: Some(list_messages_request::Filter {
                        task_ids: (!task.is_empty()).then_some(task),
                        agent_ids,
                        roles,
                    }),
                    ..Default::default()
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to list messages: {e}"))?
                .message
                .messages;
            let items = messages
                .into_iter()
                .map(display)
                .collect::<Result<Vec<_>>>()?;
            app.renderer.render(app.stdout.as_mut(), &items, &render)?;
        }
        Action::Delete { ids, force } => {
            if !force
                && !crate::helpers::confirm_deletion(
                    app.stdin.as_mut(),
                    app.stdout.as_mut(),
                    "message",
                    &ids,
                )
            {
                return Ok(());
            }
            for id in ids {
                app.client()?
                    .message
                    .delete_message(Request::new(DeleteMessageRequest { id: id.clone() }))
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to delete message {id}: {e}"))?;
            }
        }
    }
    Ok(())
}
