use crate::{
    app::App,
    config::{is_section_key, keys_under_section, parse_value, validate_key},
    Result,
};
use clap::{CommandFactory, FromArgMatches, Parser, Subcommand};
use serde_yaml::Value;
use std::io::Write;
#[derive(Parser)]
#[command(name = "config", about = "Manage configuration", display_order = 10)]
struct Options {
    #[command(subcommand)]
    action: Option<Action>,
}
#[derive(Subcommand)]
enum Action {
    Set {
        key: String,
        value: String,
    },
    Get {
        key: String,
    },
    Unset {
        key: String,
        #[arg(
            short = 'f',
            long,
            help = "Force the removal of the configuration value"
        )]
        force: bool,
    },
    Explain {
        key: String,
    },
    #[command(visible_alias = "ls")]
    List,
}
pub fn command() -> clap::Command {
    let mut command = Options::command();
    command = command.mut_subcommand("set", |c| {
        c.about("Set a configuration value")
            .long_about(
                r#"Set a configuration value.

Sets a persistent configuration key-value pair. Use dot notation for nested keys."#,
            )
            .after_help(
                r#"Examples:
  # Set the default agent for the 'new' command
  construct config set cmd.new.agent "coder"

  # Set the default output format to JSON
  construct config set output.format "json""#,
            )
    });
    command = command.mut_subcommand("get", |c| {
        c.about("Get a configuration value").after_help(
            r#"Examples:
  # Get the default agent for the 'new' command
  construct config get cmd.new.agent"#,
        )
    });
    command = command.mut_subcommand("unset", |c| {
        c.about("Unset a configuration value")
            .long_about(r#"The "unset" command allows you to unset a configuration value"#)
    });
    command = command.mut_subcommand("explain", |c| {
        c.about("Explain a configuration value")
            .long_about(r#"The "explain" command allows you to explain a configuration value"#)
    });
    command = command.mut_subcommand("list", |c| c.about("List all current configuration values"));
    command
}
fn available(keys: &[&str]) -> String {
    keys.iter()
        .map(|k| format!(" - {}", k.rsplit('.').next().unwrap_or(k)))
        .collect::<Vec<_>>()
        .join("\n")
}
fn scalar(value: &Value) -> String {
    match value {
        Value::Null => "<nil>".into(),
        Value::Bool(v) => v.to_string(),
        Value::Number(v) => v.to_string(),
        Value::String(v) => v.clone(),
        Value::Sequence(v) => format!("[{}]", v.iter().map(scalar).collect::<Vec<_>>().join(" ")),
        _ => String::new(),
    }
}
fn print_value(app: &mut App, value: &Value, prefix: &str) -> Result<()> {
    if let Some(map) = value.as_mapping() {
        let mut keys: Vec<_> = map.keys().filter_map(Value::as_str).collect();
        keys.sort();
        for key in keys {
            let value = &map[Value::String(key.into())];
            let path = format!("{prefix}.{key}");
            if value.is_mapping() {
                print_value(app, value, &path)?;
            } else {
                writeln!(app.stdout, "{path}: {}", scalar(value))?;
            }
        }
    } else {
        writeln!(app.stdout, "{}", scalar(value))?;
    }
    Ok(())
}
pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let Some(action) = Options::from_arg_matches(matches)?.action else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    match action {
        Action::Set { key, value } => {
            validate_key(&key)?;
            if is_section_key(&key) {
                let keys = keys_under_section(&key);
                if !keys.is_empty() {
                    return Err(format!("'{key}' is a configuration section, not a single value.\nYou can only set a specific key within a section.\n\nAvailable keys under '{key}' are:\n{}\n\nExample: construct config set {} value",available(&keys),keys[0]).into());
                }
            }
            app.config.set(&key, parse_value(&value))?;
            app.config.flush()?;
        }
        Action::Get { key } => {
            validate_key(&key)?;
            if let Some(value) = app.config.get(&key).cloned() {
                print_value(app, &value, &key)?;
            }
        }
        Action::Unset { key, force } => {
            validate_key(&key)?;
            let Some(value) = app.config.get(&key) else {
                return Ok(());
            };
            if !force && value.is_mapping() {
                let keys = keys_under_section(&key);
                if !keys.is_empty() {
                    writeln!(app.stdout,"You are about to remove the entire '{key}' section and all its children:\n{}\n",available(&keys))?;
                    if !crate::helpers::confirm(
                        app.stdin.as_mut(),
                        app.stdout.as_mut(),
                        "Are you sure?",
                    ) {
                        return Ok(());
                    }
                }
            }
            app.config.delete(&key);
            app.config.flush()?;
        }
        Action::Explain { key } => {
            if key != "defaults.agent" {
                return Err(format!("unknown configuration key: {key}").into());
            }
            write!(app.stdout,"{key}\n\n  Specifies the default agent to use when running `construct new` without the\n  --agent flag. This allows you to set a preferred agent for new conversations.\n\n  Type                         Default\n  {:<28} (none)\n\n  Example: construct config set defaults.agent \"my-favorite-agent\"\n","String (Agent Name or ID)")?;
        }
        Action::List => {}
    }
    Ok(())
}
