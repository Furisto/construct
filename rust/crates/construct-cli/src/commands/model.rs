use super::{
    agent::model_id,
    modelprovider::{provider_id, rpc_error},
};
use crate::{
    app::App,
    render::{DisplayResource, Field, RenderOptions},
    Result,
};
use clap::{Args, CommandFactory, FromArgMatches, Parser, Subcommand};
use construct_connect::Request;
use construct_proto::construct::v1::*;
use serde::{ser::SerializeStruct, Serialize};
use std::io::Write;

#[derive(Parser)]
#[command(
    name = "model",
    about = "Manage the AI models available to agents",
    visible_alias = "models",
    display_order = 6
)]
struct Options {
    #[command(subcommand)]
    action: Option<Action>,
}
#[derive(Subcommand)]
enum Action {
    Create {
        name: String,
        #[arg(
            short = 'p',
            long,
            help = "The name or ID of the model provider (required)"
        )]
        provider: String,
        #[arg(
            short = 'w',
            long,
            allow_hyphen_values = true,
            help = "The maximum context window size for the model (required)"
        )]
        context_window: i64,
    },
    Get {
        name: String,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "ls")]
    List(List),
    #[command(visible_alias = "rm")]
    Delete {
        #[arg(required=true,num_args=1..)]
        names: Vec<String>,
        #[arg(short = 'f', long, help = "force deletion without confirmation")]
        force: bool,
    },
}
#[derive(Args)]
struct List {
    #[arg(
        short = 'p',
        long,
        default_value = "",
        help = "Filter models by their provider"
    )]
    provider: String,
    #[arg(short = 'a', long, help = "Show all models including disabled ones")]
    all: bool,
    #[command(flatten)]
    render: RenderOptions,
}
pub fn command() -> clap::Command {
    let mut command = Options::command()
        .mut_subcommand("get", |c| c.mut_arg("format", |a| a.default_value("card")));
    command = command.mut_subcommand("create", |c| {
        c.about("Register a new large language model for use by agents")
            .long_about(r#"Register a new large language model for use by agents.

Makes a specific model from a provider (like gpt-4o from OpenAI) available to
construct. You must configure a provider before you can create a model."#)
            .after_help(r#"Examples:
  # Register GPT-4o from the 'openai-prod' provider
  construct model create "gpt-4o" --provider "openai-prod" --context-window 128000

  # Register Claude Sonnet 3.5 from the 'anthropic' provider
  construct model create "claude-3-5-sonnet-20240620" --provider "anthropic" --context-window 200000"#)
    });
    command = command.mut_subcommand("get", |c| {
        c.about("Get a model by ID or name")
            .long_about(r#"Get detailed information about a model by specifying its ID or name."#)
            .after_help(
                r#"Examples:
  # Get model by name
  construct model get "gpt-4"

  # Get model by ID
  construct model get "123e4567-e89b-12d3-a456-426614174000""#,
            )
    });
    command = command.mut_subcommand("list", |c| {
        c.about("List all registered models").after_help(
            r#"Examples:
  # List all available models
  construct model list

  # List all models available from the 'anthropic' provider
  construct model ls --provider "anthropic""#,
        )
    });
    command = command.mut_subcommand("delete", |c| {
        c.about("Delete one or more models by ID or name")
            .long_about(r#"Delete models by specifying their IDs or names."#)
            .after_help(
                r#"Examples:
  # Delete model by name
  construct model delete "gpt-4"

  # Delete multiple models
  construct model delete "claude-3-5-sonnet" "llama-3.1-8b" "gpt-4""#,
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
#[derive(Serialize)]
struct Display {
    id: String,
    name: String,
    model_provider: String,
    context_window: i64,
    enabled: bool,
    capabilities: Vec<String>,
}
impl DisplayResource for Display {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("Id", &self.id),
            Field::new("Name", &self.name),
            Field::new("ModelProvider", &self.model_provider),
            Field::new("ContextWindow", self.context_window),
            Field::new("Enabled", self.enabled).full(),
            Field::new("Capabilities", format!("[{}]", self.capabilities.join(" "))).full(),
        ]
    }
}
struct YamlDisplay<'a>(&'a Display);
impl Serialize for YamlDisplay<'_> {
    fn serialize<S: serde::Serializer>(
        &self,
        serializer: S,
    ) -> std::result::Result<S::Ok, S::Error> {
        let mut record = serializer.serialize_struct("ModelDisplay", 6)?;
        record.serialize_field("id", &self.0.id)?;
        record.serialize_field("name", &self.0.name)?;
        record.serialize_field("modelprovider", &self.0.model_provider)?;
        record.serialize_field("contextwindow", &self.0.context_window)?;
        record.serialize_field("enabled", &self.0.enabled)?;
        record.serialize_field("capabilities", &self.0.capabilities)?;
        record.end()
    }
}
impl DisplayResource for YamlDisplay<'_> {
    fn fields(&self) -> Vec<Field<'_>> {
        self.0.fields()
    }
}
fn display(model: Model) -> Result<Display> {
    let metadata = model.metadata.ok_or("model metadata missing")?;
    let spec = model.spec.ok_or("model spec missing")?;
    Ok(Display {
        id: metadata.id,
        name: spec.name,
        model_provider: metadata.model_provider_id,
        context_window: spec.context_window,
        enabled: spec.enabled,
        capabilities: spec
            .capabilities
            .into_iter()
            .map(|cap| {
                ModelCapability::try_from(cap)
                    .map(|c| c.as_str_name().into())
                    .unwrap_or_else(|_| cap.to_string())
            })
            .collect(),
    })
}
pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let Some(action) = Options::from_arg_matches(matches)?.action else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    match action {
        Action::Create {
            name,
            provider,
            context_window,
        } => {
            let client = app.client()?;
            let id = provider_id(client, &provider)
                .await
                .map_err(|e| format!("failed to resolve model provider {provider}: {e}"))?;
            let model = client
                .model
                .create_model(Request::new(CreateModelRequest {
                    name,
                    model_provider_id: id,
                    context_window,
                    ..Default::default()
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to create model: {e}"))?
                .message
                .model
                .ok_or("model missing")?;
            writeln!(
                app.stderr,
                "{}",
                model.metadata.ok_or("model metadata missing")?.id
            )?;
        }
        Action::Get { name, render } => {
            let client = app.client()?;
            let id = model_id(client, &name)
                .await
                .map_err(|e| format!("failed to resolve model {name}: {e}"))?;
            let model = client
                .model
                .get_model(Request::new(GetModelRequest { id }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to get model {name}: {e}"))?
                .message
                .model
                .ok_or("model missing")?;
            let item = display(model)?;
            if render.format == crate::render::OutputFormat::Yaml {
                app.renderer
                    .render_one(app.stdout.as_mut(), &YamlDisplay(&item), &render)?;
            } else {
                app.renderer
                    .render_one(app.stdout.as_mut(), &item, &render)?;
            }
        }
        Action::List(options) => {
            let client = app.client()?;
            let provider = if options.provider.is_empty() {
                None
            } else {
                Some(provider_id(client, &options.provider).await.map_err(|e| {
                    format!("failed to resolve model provider {}: {e}", options.provider)
                })?)
            };
            let models = client
                .model
                .list_models(Request::new(ListModelsRequest {
                    filter: Some(list_models_request::Filter {
                        model_provider_id: provider,
                        enabled: (!options.all).then_some(true),
                        ..Default::default()
                    }),
                    ..Default::default()
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to list models: {e}"))?
                .message
                .models;
            let items = models
                .into_iter()
                .map(display)
                .collect::<Result<Vec<_>>>()?;
            if options.render.format == crate::render::OutputFormat::Yaml {
                let items: Vec<_> = items.iter().map(YamlDisplay).collect();
                app.renderer
                    .render(app.stdout.as_mut(), &items, &options.render)?;
            } else {
                app.renderer
                    .render(app.stdout.as_mut(), &items, &options.render)?;
            }
        }
        Action::Delete { names, force } => {
            let mut ids = Vec::new();
            for name in &names {
                ids.push(
                    model_id(app.client()?, name)
                        .await
                        .map_err(|e| format!("failed to resolve model {name}: {e}"))?,
                );
            }
            if !force
                && !crate::helpers::confirm_deletion(
                    app.stdin.as_mut(),
                    app.stdout.as_mut(),
                    "model",
                    &names,
                )
            {
                return Ok(());
            }
            for (name, id) in names.into_iter().zip(ids) {
                app.client()?
                    .model
                    .delete_model(Request::new(DeleteModelRequest { id }))
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to delete model {name}: {e}"))?;
            }
        }
    }
    Ok(())
}
