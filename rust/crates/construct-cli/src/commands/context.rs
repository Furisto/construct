use crate::{
    app::App,
    context::{AuthConfig, EndpointContext, KEYRING_REF_PREFIX},
    render::{DisplayResource, Field, RenderOptions},
    Result,
};
use clap::{CommandFactory, FromArgMatches, Parser, Subcommand};
use serde::Serialize;
use std::io::{IsTerminal, Write};
#[derive(Parser)]
#[command(
    name = "context",
    about = "Manage connection contexts for local and remote daemons",
    visible_alias = "ctx",
    display_order = 9
)]
struct Options {
    #[command(subcommand)]
    action: Option<Action>,
}
#[derive(Subcommand)]
enum Action {
    Add {
        name: String,
        #[arg(long)]
        endpoint: Option<String>,
        #[arg(long, default_value = "")]
        kind: String,
        #[arg(long)]
        auth_token: bool,
        #[arg(long)]
        set_current: bool,
    },
    Current,
    Use {
        #[arg(num_args=0..)]
        names: Vec<String>,
    },
    #[command(visible_alias = "ls")]
    List {
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "rm")]
    Remove {
        #[arg(required=true,num_args=1..)]
        names: Vec<String>,
        #[arg(short = 'f', long, help = "Skip confirmation prompt")]
        force: bool,
    },
}
pub fn command() -> clap::Command {
    let mut command = Options::command();
    command = command.long_about(
        r#"Manage contexts to connect to different Construct daemon instances.

A context defines an endpoint (Unix socket or HTTP URL) and optional authentication.
Use contexts to switch between local development, staging, and production environments.

Examples:
  • Local daemon (Unix socket): unix:///home/user/.construct/construct.sock
  • Remote daemon (HTTP): https://construct.dev.internal:8443"#,
    );
    command = command.mut_subcommand("add", |c| {
        c.about("Add or update a context")
            .long_about(
                r#"Add a new context or update an existing one.

The endpoint can be a Unix socket path or HTTP(S) URL. The connection kind
is auto-detected from the endpoint format if not explicitly specified:
  • Paths starting with / are treated as Unix sockets (kind: unix)
  • URLs are treated as HTTP connections (kind: http)

Authentication tokens are securely stored in the system keyring (macOS Keychain,
Linux Secret Service, Windows Credential Manager)."#,
            )
            .after_help(
                r#"Examples:
  # Add local Unix socket context
  construct context add local --endpoint /home/user/.construct/construct.sock

  # Add remote HTTP context with authentication
  construct context add production \
    --endpoint https://construct.prod.example.com:8443 \
    --auth-token \
    --set-current

  # Update existing context endpoint
  construct context add staging --endpoint https://new-staging.example.com:8443"#,
            )
    });
    command = command.mut_subcommand("current", |c| {
        c.about("Display the current context name").after_help(
            r#"Examples:
  # Show current context
  construct context current"#,
        )
    });
    command = command.mut_subcommand("use", |c| {
        c.about("Switch to a different context").after_help(
            r#"Examples:
  # Switch to the 'ec2-dev' context
  construct context use ec2-dev"#,
        )
    });
    command = command.mut_subcommand("list", |c| {
        c.about("List all configured contexts").after_help(
            r#"Examples:
  # List all contexts
  construct context list

  # List contexts in JSON format
  construct context list --output json"#,
        )
    });
    command = command.mut_subcommand("remove", |c| {
        c.about("Remove one or more contexts")
            .long_about(
                r#"Remove one or more contexts from the configuration.

This will delete the context configuration and remove any associated
authentication tokens from the system keyring."#,
            )
            .after_help(
                r#"Examples:
  # Remove a single context
  construct context remove staging

  # Remove multiple contexts
  construct context remove dev staging test

  # Remove without confirmation
  construct context remove production --force"#,
            )
    });
    command = command.mut_subcommand("list", |c| {
        c.mut_arg("format", |a| {
            a.help("output format (json, yaml, table, card, markdown) (default: table)")
        })
        .mut_arg("wide", |a| a.help("output verbosity (default: false)"))
        .mut_arg("no_headers", |a| {
            a.help("do not print headers (default: false)")
        })
    });
    command = command.mut_subcommand("add", |c| {
        c.mut_arg("endpoint", |a| {
            a.help("Daemon endpoint (Unix socket path or HTTP URL)")
        })
        .mut_arg("kind", |a| {
            a.help("Connection type (unix or http, auto-detected if not specified)")
        })
        .mut_arg("auth_token", |a| a.help("Prompt for authentication token"))
        .mut_arg("set_current", |a| a.help("Set this context as current"))
    });
    command
}
#[derive(Serialize)]
struct Display {
    name: String,
    endpoint: String,
    kind: String,
    auth: String,
    current: bool,
}
impl DisplayResource for Display {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("Name", &self.name),
            Field::new("Endpoint", &self.endpoint),
            Field::new("Kind", &self.kind),
            Field::new("Auth", &self.auth),
            Field::new("Current", self.current),
        ]
    }
}
pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let Some(action) = Options::from_arg_matches(matches)?.action else {
        command().write_long_help(&mut app.stdout)?;
        return Ok(());
    };
    match action {
        Action::Add {
            name,
            endpoint,
            kind,
            auth_token,
            set_current,
        } => {
            let endpoint = endpoint.ok_or("required flag(s) \"endpoint\" not set")?;
            let kind = if kind.is_empty() {
                if endpoint.starts_with('/') {
                    "unix"
                } else {
                    "http"
                }
                .into()
            } else {
                kind
            };
            let auth = if auth_token {
                write!(app.stdout, "Enter authentication token: ")?;
                app.stdout.flush()?;
                let result = if std::io::stdin().is_terminal() {
                    rpassword::read_password().map_err(|e| e.to_string())
                } else {
                    Err("inappropriate ioctl for device".into())
                };
                writeln!(app.stdout)?;
                let token = result.map_err(|e| format!("failed to read token: {e}"))?;
                let token = token.trim();
                if token.is_empty() {
                    return Err("token cannot be empty".into());
                }
                app.contexts
                    .store_token(&name, token)
                    .map_err(|e| format!("failed to store token in keyring: {e}"))?;
                Some(AuthConfig {
                    auth_type: "token".into(),
                    token_ref: format!("{KEYRING_REF_PREFIX}{name}"),
                    ..Default::default()
                })
            } else {
                None
            };
            let existed = app.contexts.upsert(
                &name,
                EndpointContext {
                    address: endpoint,
                    kind,
                    auth,
                },
                set_current,
            )?;
            writeln!(
                app.stdout,
                "Context {name:?} {}{}",
                if existed { "updated" } else { "created" },
                if set_current {
                    " and set as current"
                } else {
                    ""
                }
            )?;
        }
        Action::Current => {
            let contexts = app
                .contexts
                .load()
                .map_err(|e| format!("failed to load contexts: {e}"))?;
            if contexts.current.is_empty() {
                return Err("no current context set".into());
            }
            writeln!(app.stdout, "{}", contexts.current)?;
        }
        Action::Use { names } => {
            if names.len() != 1 {
                return Err(format!("accepts 1 arg(s), received {}", names.len()).into());
            }
            let mut name = names[0].clone();
            if name == "-" {
                name = app
                    .contexts
                    .load()
                    .map_err(|e| format!("failed to load contexts: {e}"))?
                    .previous;
                if name.is_empty() {
                    return Err("no previous context found".into());
                }
            }
            app.contexts.set_current(&name)?;
            writeln!(app.stdout, "Switched to context {name:?}")?;
        }
        Action::List { render } => {
            let contexts = app
                .contexts
                .load()
                .map_err(|e| format!("failed to load contexts: {e}"))?;
            let items: Vec<_> = contexts
                .contexts
                .into_iter()
                .map(|(name, context)| {
                    let auth = context
                        .auth
                        .filter(|a| a.is_configured())
                        .map(|a| {
                            if !a.token.is_empty() {
                                "token (inline)".into()
                            } else {
                                a.token_ref
                            }
                        })
                        .unwrap_or_else(|| "none".into());
                    Display {
                        current: name == contexts.current,
                        name,
                        endpoint: context.address,
                        kind: context.kind,
                        auth,
                    }
                })
                .collect();
            app.renderer.render(app.stdout.as_mut(), &items, &render)?;
        }
        Action::Remove { names, force } => {
            if !force
                && !crate::helpers::confirm_deletion(
                    app.stdin.as_mut(),
                    app.stdout.as_mut(),
                    "context",
                    &names,
                )
            {
                return Ok(());
            }
            for name in names {
                app.contexts.delete(&name)?;
                writeln!(app.stdout, "Context {name:?} removed")?;
            }
        }
    }
    Ok(())
}
