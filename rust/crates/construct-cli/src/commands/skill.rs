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
#[command(
    name = "skill",
    about = "Manage skills that extend agent capabilities",
    visible_alias = "skills",
    display_order = 8
)]
struct Options {
    #[command(subcommand)]
    action: Option<Action>,
}
#[derive(Subcommand)]
enum Action {
    Install {
        source: String,
        #[arg(short = 'f', long, help = "Overwrite existing skills")]
        force: bool,
        #[arg(short='s',long="skill",action=clap::ArgAction::Append,value_delimiter=',',help="Install only specific skill(s) by name")]
        names: Vec<String>,
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "ls")]
    List {
        #[command(flatten)]
        render: RenderOptions,
    },
    #[command(visible_alias = "rm")]
    Delete {
        name: String,
        #[arg(short = 'f', long, help = "Skip the confirmation prompt")]
        force: bool,
    },
    Update {
        name: Option<String>,
        #[command(flatten)]
        render: RenderOptions,
    },
}
pub fn command() -> clap::Command {
    let mut command = Options::command();
    command = command.mut_subcommand("install", |c| {
        c.about("Install skills from a remote source")
            .long_about(
                r#"Install skills from GitHub, GitLab, or a direct URL.

Supported source formats:
  owner/repo                              GitHub shorthand (installs from main branch)
  owner/repo/path                         GitHub shorthand with path
  github.com/owner/repo/tree/branch/path  Full GitHub URL
  gitlab.com/owner/repo/-/tree/branch/path  Full GitLab URL
  https://example.com/path/to/SKILL.md    Direct URL to SKILL.md file"#,
            )
            .after_help(
                r#"Examples:
  # Install all skills from a GitHub repository
  construct skill install anthropics/skills

  # Install skills from a specific path in a repository
  construct skill install anthropics/skills/coding

  # Install only specific skills by name
  construct skill install anthropics/skills --skill commit --skill review-pr

  # Overwrite existing skills
  construct skill install anthropics/skills --force"#,
            )
    });
    command = command.mut_subcommand("list", |c| {
        c.about("List all installed skills").after_help(
            r#"Examples:
  # List all installed skills
  construct skill list

  # List skills in JSON format
  construct skill ls -o json"#,
        )
    });
    command = command.mut_subcommand("delete", |c| {
        c.about("Delete an installed skill").after_help(
            r#"Examples:
  # Delete a skill
  construct skill delete commit

  # Force delete without confirmation
  construct skill rm review-pr --force"#,
        )
    });
    command = command.mut_subcommand("update", |c| {
        c.about("Update installed skills to their latest versions")
            .long_about(
                r#"Update skills that were installed from remote sources (GitHub, GitLab, or URLs).
Locally placed skills cannot be updated.

If no name is specified, all remotely-installed skills are updated."#,
            )
            .after_help(
                r#"Examples:
  # Update all remotely-installed skills
  construct skill update

  # Update a specific skill
  construct skill update commit"#,
            )
    });
    for name in ["install", "list", "update"] {
        command = command.mut_subcommand(name, |c| {
            c.mut_arg("format", |a| {
                a.help("output format (json, yaml, table, card, markdown) (default: table)")
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
    name: String,
    description: String,
    source: String,
    installed_at: String,
    updated_at: String,
}
impl DisplayResource for Display {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("Name", &self.name),
            Field::new("Description", &self.description).full(),
            Field::new("Source", &self.source),
            Field::new("InstalledAt", &self.installed_at).column("Installed"),
            Field::new("UpdatedAt", &self.updated_at)
                .column("Updated")
                .full(),
        ]
    }
}
fn relative<T: Serialize>(value: &Option<T>) -> Result<String> {
    if value.is_none() {
        return Ok(String::new());
    }
    let time = chrono::DateTime::parse_from_rfc3339(&super::task::timestamp(value)?)?;
    let delta = chrono::Utc::now().signed_duration_since(time);
    let seconds = delta.num_milliseconds().unsigned_abs() as f64 / 1000.;
    let suffix = if delta < chrono::Duration::zero() {
        "from now"
    } else {
        "ago"
    };
    let day = 86400.;
    let week = 7. * day;
    let month = 30. * day;
    let year = 12. * month;
    let (count, unit) = if seconds < 1. {
        return Ok("now".into());
    } else if seconds < 2. {
        (1, "second")
    } else if seconds < 60. {
        (seconds as u64, "seconds")
    } else if seconds < 120. {
        (1, "minute")
    } else if seconds < 3600. {
        ((seconds / 60.) as u64, "minutes")
    } else if seconds < 7200. {
        (1, "hour")
    } else if seconds < day {
        ((seconds / 3600.) as u64, "hours")
    } else if seconds < 2. * day {
        (1, "day")
    } else if seconds < week {
        ((seconds / day) as u64, "days")
    } else if seconds < 2. * week {
        (1, "week")
    } else if seconds < month {
        ((seconds / week) as u64, "weeks")
    } else if seconds < 2. * month {
        (1, "month")
    } else if seconds < year {
        ((seconds / month) as u64, "months")
    } else if seconds < 18. * month {
        (1, "year")
    } else if seconds < 2. * year {
        (2, "years")
    } else if seconds < 37. * year {
        ((seconds / year) as u64, "years")
    } else {
        return Ok(format!("a long while {suffix}"));
    };
    Ok(format!("{count} {unit} {suffix}"))
}
fn display(skill: Skill) -> Result<Display> {
    let source = match skill.source {
        None => "local",
        Some(skill::Source::Url(_)) => "url",
        Some(skill::Source::Git(git)) => match GitProvider::try_from(git.provider) {
            Ok(GitProvider::Github) => "github",
            Ok(GitProvider::Gitlab) => "gitlab",
            _ => "git",
        },
    };
    Ok(Display {
        name: skill.name,
        description: skill.description,
        source: source.into(),
        installed_at: relative(&skill.installed_at)?,
        updated_at: relative(&skill.updated_at)?,
    })
}
pub async fn run(app: &mut App, matches: &clap::ArgMatches) -> Result<()> {
    let Some(action) = Options::from_arg_matches(matches)?.action else {
        command().write_help(&mut app.stdout)?;
        return Ok(());
    };
    let result: Result<()> = async {
        match action {
            Action::Install {
                source,
                force,
                names,
                render,
            } => {
                let skills = app
                    .client()?
                    .skill
                    .install_skill(Request::new(InstallSkillRequest {
                        source,
                        force,
                        skill_names: names,
                    }))
                    .await?
                    .message
                    .installed_skills;
                if skills.is_empty() {
                    writeln!(app.stdout, "No skills installed")?;
                } else {
                    let items = skills
                        .into_iter()
                        .map(display)
                        .collect::<Result<Vec<_>>>()?;
                    app.renderer.render(app.stdout.as_mut(), &items, &render)?;
                }
            }
            Action::List { render } => {
                let skills = app
                    .client()?
                    .skill
                    .list_skills(Request::new(ListSkillsRequest {}))
                    .await?
                    .message
                    .skills;
                let items = skills
                    .into_iter()
                    .map(display)
                    .collect::<Result<Vec<_>>>()?;
                app.renderer.render(app.stdout.as_mut(), &items, &render)?;
            }
            Action::Update { name, render } => {
                let skills = app
                    .client()?
                    .skill
                    .update_skill(Request::new(UpdateSkillRequest { name }))
                    .await?
                    .message
                    .updated_skills;
                if skills.is_empty() {
                    writeln!(app.stdout, "No skills updated")?;
                } else {
                    let items = skills
                        .into_iter()
                        .map(display)
                        .collect::<Result<Vec<_>>>()?;
                    app.renderer.render(app.stdout.as_mut(), &items, &render)?;
                }
            }
            Action::Delete { name, force } => {
                if !force
                    && !crate::helpers::confirm_deletion(
                        app.stdin.as_mut(),
                        app.stdout.as_mut(),
                        "skill",
                        std::slice::from_ref(&name),
                    )
                {
                    return Ok(());
                }
                app.client()?
                    .skill
                    .delete_skill(Request::new(DeleteSkillRequest { name: name.clone() }))
                    .await?;
                writeln!(app.stdout, "Skill {name:?} deleted")?;
            }
        }
        Ok(())
    }
    .await;
    match result {
        Ok(()) => Ok(()),
        Err(error) => match crate::errors::transform_error(error) {
            None => Ok(()),
            Some(error) if error.is::<crate::errors::UserFacingError>() => Err(error),
            Some(error) => Err(super::modelprovider::rpc_error(error).into()),
        },
    }
}
