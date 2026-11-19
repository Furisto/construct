use crate::{
    app::{ApiClient, App},
    render::{DisplayResource, Field, RenderOptions},
    Result,
};
use clap::{CommandFactory, FromArgMatches, Parser, Subcommand};
use construct_connect::Request;
use construct_proto::construct::v1::*;
use serde::{ser::SerializeStruct, Serialize};
use std::io::{IsTerminal, Write};

#[derive(Parser)]
#[command(name="modelprovider",about="Configure providers like OpenAI and Anthropic",visible_aliases=["modelproviders","mp"],display_order=7)]
struct Options {
    #[command(subcommand)]
    action: Option<Action>,
}
#[derive(Subcommand)]
enum Action {
    Create {
        #[arg(num_args=0..)]
        names: Vec<String>,
        #[arg(
            short = 'k',
            long,
            default_value = "",
            help = "The API key. If omitted, the corresponding environment variable will be used"
        )]
        api_key: String,
        #[arg(
            short = 't',
            long = "type",
            help = "The type of the model provider (required)"
        )]
        provider_type: Option<String>,
    },
    Get {
        name: String,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "ls")]
    List {
        #[arg(short='t',long="provider-type",action=clap::ArgAction::Append,help="Filter providers by type")]
        types: Vec<String>,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "rm")]
    Delete {
        #[arg(required=true,num_args=1..)]
        names: Vec<String>,
        #[arg(short = 'f', long, help = "Skip the confirmation prompt")]
        force: bool,
    },
}
pub fn command() -> clap::Command {
    let mut command = Options::command()
        .mut_subcommand("get", |c| c.mut_arg("format", |a| a.default_value("card")));
    command = command.long_about(r#"Manage integrations to AI model providers to access their language models for your agents.

Providers require API credentials and offer different model capabilities. At least one provider must be configured before creating agents.

Supported providers:
- OpenAI: Access to GPT models (gpt-4, gpt-3.5-turbo, etc.)
- Anthropic: Access to Claude models (claude-3-5-sonnet, claude-3-haiku, etc.)"#);
    command = command.mut_subcommand("create", |c| {
        c.about("Configure a new model provider integration")
            .long_about(
                r#"Configure a new model provider integration.

Connects construct to an external AI model provider. This step is required to
gain access to models. API credentials can be provided interactively, via flags
or environment variables (e.g., $OPENAI_API_KEY, $ANTHROPIC_API_KEY)."#,
            )
            .after_help(
                r#"Examples:
  # Create an OpenAI provider, using the API key from the environment
  export OPENAI_API_KEY="sk-..."
  construct provider create "openai-prod" --type openai

  # Create an Anthropic provider, passing the API key directly
  construct provider create "anthropic-dev" --type anthropic --api-key "sk-ant-...""#,
            )
    });
    command = command.mut_subcommand("get", |c| {
        c.about("Get a model provider by ID or name").after_help(
            r#"Examples:
  # Get model provider by name
  construct modelprovider get "anthropic-dev"

  # Get model provider with JSON output
  construct modelprovider get "openai-prod" --output json"#,
        )
    });
    command = command.mut_subcommand("list", |c| {
        c.about("List all configured model providers").after_help(
            r#"Examples:
  # List all configured providers
  construct provider list"#,
        )
    });
    command = command.mut_subcommand("delete", |c| {
        c.about("Permanently delete one or more model providers")
            .long_about(
                r#"Permanently delete one or more model providers.

Deletes a provider configuration. **Warning**: This action will also delete all
models that depend on this provider."#,
            )
            .after_help(
                r#"Examples:
  # Delete the 'anthropic-dev' provider
  construct provider delete anthropic-dev"#,
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
pub(super) fn rpc_error(error: impl std::fmt::Display) -> String {
    error.to_string().trim_end_matches(": ").to_string()
}
pub async fn provider_id(client: &ApiClient, name: &str) -> Result<String> {
    if uuid::Uuid::parse_str(name).is_ok() {
        return Ok(name.into());
    }
    let providers = client
        .model_provider
        .list_model_providers(Request::new(ListModelProvidersRequest::default()))
        .await
        .map_err(rpc_error)
        .map_err(|e| format!("failed to list model providers: {e}"))?
        .message
        .model_providers;
    let matches: Vec<_> = providers
        .into_iter()
        .filter(|p| p.spec.as_ref().is_some_and(|s| s.name == name))
        .collect();
    match matches.as_slice() {
        [] => Err(format!("model provider {name} not found").into()),
        [p] => Ok(p
            .metadata
            .as_ref()
            .ok_or("model provider metadata missing")?
            .id
            .clone()),
        _ => Err(format!("multiple model providers found for {name}").into()),
    }
}
fn provider_type(value: &str, flag: &str) -> Result<ModelProviderType> {
    match value.trim().to_lowercase().as_str() {
        "openai"=>Ok(ModelProviderType::Openai),"anthropic"=>Ok(ModelProviderType::Anthropic),"gemini"=>Ok(ModelProviderType::Gemini),"xai"=>Ok(ModelProviderType::Xai),
        _=>Err(format!("invalid argument {value:?} for {flag:?} flag: must be one of \"openai\",\"anthropic\",\"gemini\",\"xai\"").into()),
    }
}
fn type_name(value: i32) -> &'static str {
    match ModelProviderType::try_from(value) {
        Ok(ModelProviderType::Openai) => "openai",
        Ok(ModelProviderType::Anthropic) => "anthropic",
        Ok(ModelProviderType::Gemini) => "gemini",
        Ok(ModelProviderType::Xai) => "xai",
        _ => "unknown",
    }
}
#[derive(Serialize)]
struct Display {
    id: String,
    name: String,
    provider_type: String,
    enabled: bool,
}
impl DisplayResource for Display {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("Id", &self.id),
            Field::new("Name", &self.name),
            Field::new("ProviderType", &self.provider_type),
            Field::new("Enabled", self.enabled).full(),
        ]
    }
}
struct YamlDisplay<'a>(&'a Display);
impl Serialize for YamlDisplay<'_> {
    fn serialize<S: serde::Serializer>(
        &self,
        serializer: S,
    ) -> std::result::Result<S::Ok, S::Error> {
        let mut record = serializer.serialize_struct("ModelProviderDisplay", 4)?;
        record.serialize_field("id", &self.0.id)?;
        record.serialize_field("name", &self.0.name)?;
        record.serialize_field("providertype", &self.0.provider_type)?;
        record.serialize_field("enabled", &self.0.enabled)?;
        record.end()
    }
}
impl DisplayResource for YamlDisplay<'_> {
    fn fields(&self) -> Vec<Field<'_>> {
        self.0.fields()
    }
}
fn display(provider: ModelProvider) -> Result<Display> {
    let metadata = provider.metadata.ok_or("model provider metadata missing")?;
    let spec = provider.spec.ok_or("model provider spec missing")?;
    Ok(Display {
        id: metadata.id,
        name: spec.name,
        provider_type: type_name(metadata.provider_type).into(),
        enabled: spec.enabled,
    })
}
fn api_key(app: &mut App, key: String, kind: ModelProviderType, name: &str) -> Result<String> {
    if !key.is_empty() {
        return Ok(key);
    }
    let (environment, display) = match kind {
        ModelProviderType::Openai => ("OPENAI_API_KEY", "OpenAI"),
        ModelProviderType::Anthropic => ("ANTHROPIC_API_KEY", "Anthropic"),
        ModelProviderType::Gemini => ("GEMINI_API_KEY", "Gemini"),
        ModelProviderType::Xai => ("XAI_API_KEY", "xAI"),
        _ => return Err("invalid model provider type".into()),
    };
    if let Ok(key) = std::env::var(environment) {
        if !key.is_empty() {
            return Ok(key);
        }
    }
    write!(app.stdout, "Enter {display} API key for {name}: ")?;
    app.stdout.flush()?;
    if !std::io::stdin().is_terminal() {
        return Err("failed to read API key: cannot prompt for API key in non-interactive terminal\n\nPlease use --api-key flag or set environment variable".into());
    }
    let key = rpassword::read_password().map_err(|e| format!("failed to read API key: {e}"))?;
    writeln!(app.stdout)?;
    if key.trim().is_empty() {
        return Err(format!("API key cannot be empty\n\nTip: You can also set the {environment} environment variable or use the --api-key flag").into());
    }
    Ok(key)
}
pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let Some(action) = Options::from_arg_matches(matches)?.action else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    match action {
        Action::Create {
            names,
            api_key: key,
            provider_type: kind,
        } => {
            let kind = kind.map(|v| provider_type(&v, "-t, --type")).transpose()?;
            if names.len() != 1 {
                return Err(format!("accepts 1 arg(s), received {}", names.len()).into());
            }
            let kind = kind.ok_or("required flag(s) \"type\" not set")?;
            let name = &names[0];
            let key = api_key(app, key, kind, name)?;
            let provider = app
                .client()?
                .model_provider
                .create_model_provider(Request::new(CreateModelProviderRequest {
                    name: name.clone(),
                    provider_type: kind as i32,
                    authentication: Some(create_model_provider_request::Authentication::ApiKey(
                        key,
                    )),
                    ..Default::default()
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to create model provider: {e}"))?
                .message
                .model_provider
                .ok_or("model provider missing")?;
            writeln!(
                app.stdout,
                "{}",
                provider
                    .metadata
                    .ok_or("model provider metadata missing")?
                    .id
            )?;
        }
        Action::Get { name, render } => {
            let client = app.client()?;
            let id = provider_id(client, &name)
                .await
                .map_err(|e| format!("failed to resolve model provider {name}: {e}"))?;
            let provider = client
                .model_provider
                .get_model_provider(Request::new(GetModelProviderRequest { id }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to get model provider {name}: {e}"))?
                .message
                .model_provider
                .ok_or("model provider missing")?;
            let item = display(provider)?;
            if render.format == crate::render::OutputFormat::Yaml {
                app.renderer
                    .render_one(app.stdout.as_mut(), &YamlDisplay(&item), &render)?;
            } else {
                app.renderer
                    .render_one(app.stdout.as_mut(), &item, &render)?;
            }
        }
        Action::List { types, render } => {
            let mut provider_types = Vec::new();
            for value in types {
                for part in value.split(',') {
                    provider_types.push(provider_type(part,"-t, --provider-type").map_err(|_| format!("invalid argument {value:?} for \"-t, --provider-type\" flag: must be one of \"openai\",\"anthropic\",\"gemini\",\"xai\""))? as i32);
                }
            }
            let providers = app
                .client()?
                .model_provider
                .list_model_providers(Request::new(ListModelProvidersRequest {
                    filter: Some(list_model_providers_request::Filter {
                        provider_types,
                        ..Default::default()
                    }),
                    ..Default::default()
                }))
                .await
                .map_err(rpc_error)
                .map_err(|e| format!("failed to list model providers: {e}"))?
                .message
                .model_providers;
            let items = providers
                .into_iter()
                .map(display)
                .collect::<Result<Vec<_>>>()?;
            if render.format == crate::render::OutputFormat::Yaml {
                let items: Vec<_> = items.iter().map(YamlDisplay).collect();
                app.renderer.render(app.stdout.as_mut(), &items, &render)?;
            } else {
                app.renderer.render(app.stdout.as_mut(), &items, &render)?;
            }
        }
        Action::Delete { names, force } => {
            let mut ids = Vec::new();
            for name in &names {
                ids.push(
                    provider_id(app.client()?, name)
                        .await
                        .map_err(|e| format!("failed to resolve model provider {name}: {e}"))?,
                );
            }
            if !force
                && !crate::helpers::confirm_deletion(
                    app.stdin.as_mut(),
                    app.stdout.as_mut(),
                    "model-provider",
                    &names,
                )
            {
                return Ok(());
            }
            for (name, id) in names.into_iter().zip(ids) {
                let client = app.client()?;
                let models = client
                    .model
                    .list_models(Request::new(ListModelsRequest {
                        filter: Some(list_models_request::Filter {
                            model_provider_id: Some(id.clone()),
                            ..Default::default()
                        }),
                        ..Default::default()
                    }))
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to list models for model provider {name}: {e}"))?
                    .message
                    .models;
                for model in models {
                    let model_name = model.spec.ok_or("model spec missing")?.name;
                    client.model.delete_model(Request::new(DeleteModelRequest { id:model.metadata.ok_or("model metadata missing")?.id })).await.map_err(rpc_error).map_err(|e| format!("failed to delete model {model_name} for model provider {name}: {e}"))?;
                }
                client
                    .model_provider
                    .delete_model_provider(Request::new(DeleteModelProviderRequest { id }))
                    .await
                    .map_err(rpc_error)
                    .map_err(|e| format!("failed to delete model provider {name}: {e}"))?;
            }
        }
    }
    Ok(())
}
