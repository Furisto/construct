use crate::{
    app::App,
    helpers::{
        confirm, format_relative_time, parse_duration, validate_setup_code_expiry,
        validate_token_expiry,
    },
    render::{DisplayResource, Field, OutputFormat, RenderOptions},
    Result,
};
use chrono::{DateTime, SecondsFormat, Utc};
use clap::{Arg, ArgAction, ArgMatches, Args, Command, FromArgMatches};
use construct_connect::Request;
use construct_proto::construct::v1::{
    CreateSetupCodeRequest, CreateTokenRequest, ListTokensRequest, RevokeTokenRequest,
};
use construct_server::auth::{issue_api_token, TokenProvider};
use construct_store::SqliteStore;
use pbjson_types::{Duration, Timestamp};
use serde::Serialize;
use uuid::Uuid;

pub(super) fn command() -> Command {
    let named = |name| Arg::new(name).required(true);
    let expiry = |name, default| Arg::new(name).long(name).default_value(default);
    Command::new("token")
        .about("Manage authentication tokens for remote daemon access")
        .subcommands([
            RenderOptions::augment_args(Command::new("create").about("Generate a new API token"))
                .mut_arg("format", |arg| arg.default_value("card"))
                .arg(named("name"))
                .arg(Arg::new("description").long("description"))
                .arg(
                    Arg::new("local")
                        .long("local")
                        .help("Create the token in this user's local daemon database (useful with a TCP-only daemon)")
                        .action(ArgAction::SetTrue),
                )
                .arg(expiry("expires", "90d")),
            RenderOptions::augment_args(
                Command::new("create-setup").about("Generate a one-time setup code"),
            )
            .mut_arg("format", |arg| arg.default_value("card"))
            .arg(named("token-name"))
            .arg(expiry("code-expires", "5m"))
            .arg(expiry("token-expires", "90d")),
            RenderOptions::augment_args(Command::new("list").about("List token metadata"))
                .visible_alias("ls")
                .arg(Arg::new("name-prefix").long("name-prefix"))
                .arg(
                    Arg::new("include-expired")
                        .long("include-expired")
                        .action(ArgAction::SetTrue),
                ),
            Command::new("revoke")
                .about("Permanently revoke a token by ID")
                .arg(named("id"))
                .arg(
                    Arg::new("force")
                        .short('f')
                        .long("force")
                        .action(ArgAction::SetTrue),
                ),
        ])
}

pub(super) async fn run(app: &mut App, matches: &ArgMatches) -> Result<()> {
    let Some((name, args)) = matches.subcommand() else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    let optional = |key| {
        args.get_one::<String>(key)
            .cloned()
            .filter(|s| !s.is_empty())
    };
    let required = |key| {
        args.get_one::<String>(key)
            .expect("argument is required or has default")
    };
    match name {
        "create" => {
            let options = RenderOptions::from_arg_matches(args)?;
            let name = required("name").clone();
            let expires_in = expiry(required("expires"), "expiry", validate_token_expiry)?;
            let request = CreateTokenRequest {
                name: name.clone(),
                expires_in: Some(expires_in),
                description: optional("description"),
            };
            let response = if args.get_flag("local") {
                let path = app.user.data.join("construct.db");
                if !path.is_file() {
                    return Err("local daemon database not found; start the daemon first".into());
                }
                let store = SqliteStore::open(path).await?;
                issue_api_token(&store, &TokenProvider::new(), request).await
            } else {
                app.client()?
                    .auth
                    .create_token(Request::new(request))
                    .await
                    .map(|response| response.message)
            }
            .map_err(|err| format!("failed to create token: {err}"))?;
            let display = CreatedToken {
                name,
                token: response.token,
                expires_at: rfc3339(response.expires_at),
            };
            app.renderer
                .render_one(&mut app.stdout, &display, &options)?;
            if options.format == OutputFormat::Card {
                eprintln!("Save this token securely; it cannot be retrieved again.");
            }
        }
        "create-setup" => {
            let options = RenderOptions::from_arg_matches(args)?;
            let token_name = required("token-name").clone();
            let expires_in = expiry(
                required("code-expires"),
                "code expiry",
                validate_setup_code_expiry,
            )?;
            let token_expires_in = expiry(
                required("token-expires"),
                "token expiry",
                validate_token_expiry,
            )?;
            let request = CreateSetupCodeRequest {
                token_name: token_name.clone(),
                expires_in: Some(expires_in),
                token_expires_in: Some(token_expires_in),
            };
            let response = app
                .client()?
                .auth
                .create_setup_code(Request::new(request))
                .await
                .map_err(|err| format!("failed to create setup code: {err}"))?
                .message;
            let display = SetupCode {
                token_name,
                setup_code: response.setup_code,
                expires_at: rfc3339(response.expires_at),
            };
            app.renderer
                .render_one(&mut app.stdout, &display, &options)?;
            if options.format == OutputFormat::Card {
                eprintln!("Share this one-time code securely. Exchange it with 'construct context add <name> --endpoint <daemon-url> --setup-code <code>'.");
            }
        }
        "list" => {
            let options = RenderOptions::from_arg_matches(args)?;
            let request = ListTokensRequest {
                name_prefix: optional("name-prefix").unwrap_or_default(),
                include_expired: args.get_flag("include-expired"),
            };
            let response = app
                .client()?
                .auth
                .list_tokens(Request::new(request))
                .await
                .map_err(|err| format!("failed to list tokens: {err}"))?;
            let tokens: Vec<_> = response
                .message
                .tokens
                .into_iter()
                .map(|token| Token {
                    id: token.id,
                    name: token.name,
                    created: format_relative_time(timestamp(token.created_at)),
                    expires: format_relative_time(timestamp(token.expires_at)),
                    status: if token.is_active { "Active" } else { "Expired" },
                })
                .collect();
            app.renderer.render(&mut app.stdout, &tokens, &options)?;
        }
        "revoke" => {
            let id = Uuid::parse_str(required("id"))
                .map_err(|_| "invalid token ID format: must be UUID")?;
            if !args.get_flag("force") && !confirm(
                &mut app.stdin,
                &mut app.stdout,
                "This will permanently revoke the token. Any systems using it will lose access immediately.\n\nAre you sure you want to continue?",
            ) {
                return Ok(());
            }
            let request = RevokeTokenRequest { id: id.to_string() };
            app.client()?
                .auth
                .revoke_token(Request::new(request))
                .await
                .map_err(|err| format!("failed to revoke token: {err}"))?;
            writeln!(app.stdout, "Token has been revoked")?;
        }
        _ => unreachable!(),
    }
    Ok(())
}

fn expiry(value: &str, label: &str, validate: fn(i64) -> Result<()>) -> Result<Duration> {
    let nanos = parse_duration(value).map_err(|err| format!("invalid {label} duration: {err}"))?;
    validate(nanos)?;
    Ok(Duration {
        seconds: nanos / 1_000_000_000,
        nanos: (nanos % 1_000_000_000) as i32,
    })
}

fn timestamp(value: Option<Timestamp>) -> DateTime<Utc> {
    value
        .and_then(|stamp| DateTime::from_timestamp(stamp.seconds, stamp.nanos as u32))
        .unwrap_or(DateTime::UNIX_EPOCH)
}

fn rfc3339(stamp: Option<Timestamp>) -> String {
    timestamp(stamp).to_rfc3339_opts(SecondsFormat::Secs, true)
}

#[derive(Serialize)]
struct Token {
    id: String,
    name: String,
    created: String,
    expires: String,
    status: &'static str,
}

impl DisplayResource for Token {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("ID", &self.id),
            Field::new("Name", &self.name),
            Field::new("Created", &self.created),
            Field::new("Expires", &self.expires),
            Field::new("Status", self.status),
        ]
    }
}

#[derive(Serialize)]
struct CreatedToken {
    name: String,
    token: String,
    expires_at: String,
}

impl DisplayResource for CreatedToken {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("Name", &self.name),
            Field::new("Token", &self.token),
            Field::new("ExpiresAt", &self.expires_at),
        ]
    }
}

#[derive(Serialize)]
struct SetupCode {
    token_name: String,
    setup_code: String,
    expires_at: String,
}

impl DisplayResource for SetupCode {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("TokenName", &self.token_name),
            Field::new("SetupCode", &self.setup_code),
            Field::new("ExpiresAt", &self.expires_at),
        ]
    }
}
