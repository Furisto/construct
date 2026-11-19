use crate::{
    app::App,
    render::{DisplayResource, Field, OutputFormat, RenderOptions},
    Result,
};
use clap::{ArgMatches, Args, Command, FromArgMatches};
use serde::Serialize;

pub(super) const VERSION: &str = metadata(option_env!("CONSTRUCT_VERSION"));
const COMMIT: &str = metadata(option_env!("CONSTRUCT_GIT_COMMIT"));
const BUILD_DATE: &str = metadata(option_env!("CONSTRUCT_BUILD_DATE"));

const fn metadata(value: Option<&'static str>) -> &'static str {
    match value {
        Some(value) if !value.is_empty() => value,
        _ => "unknown",
    }
}

#[derive(Serialize)]
#[serde(rename_all = "PascalCase")]
struct VersionInfo {
    version: &'static str,
    commit: &'static str,
    build_date: &'static str,
}

impl DisplayResource for VersionInfo {
    fn fields(&self) -> Vec<Field<'_>> {
        vec![
            Field::new("Version", self.version),
            Field::new("Commit", self.commit),
            Field::new("BuildDate", self.build_date),
        ]
    }
}

pub fn command() -> Command {
    RenderOptions::augment_args(
        Command::new("info")
            .alias("version")
            .about("Print information about the Construct CLI")
            .display_order(12),
    )
    .mut_arg("format", |arg| arg.default_value("card"))
}

pub async fn run(app: &mut App, matches: &ArgMatches) -> Result<()> {
    let options = RenderOptions::from_arg_matches(matches)?;
    let info = VersionInfo {
        version: VERSION,
        commit: COMMIT,
        build_date: BUILD_DATE,
    };
    if options.format == OutputFormat::Yaml {
        #[derive(Serialize)]
        struct YamlVersionInfo {
            version: &'static str,
            commit: &'static str,
            builddate: &'static str,
        }
        writeln!(
            app.stdout,
            "{}",
            crate::yaml::to_string(&YamlVersionInfo {
                version: info.version,
                commit: info.commit,
                builddate: info.build_date,
            })?
        )?;
        return Ok(());
    }
    app.renderer.render_one(&mut app.stdout, &info, &options)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn build_metadata_defaults() {
        assert_eq!(metadata(None), "unknown");
        assert_eq!(metadata(Some("")), "unknown");
        assert_eq!(metadata(Some("v1.2.3")), "v1.2.3");
    }

    #[test]
    fn version_alias() {
        let matches = crate::cli::command()
            .try_get_matches_from(["construct", "version"])
            .unwrap();
        assert_eq!(matches.subcommand_name(), Some("info"));
        assert!(crate::cli::command()
            .try_get_matches_from(["construct", "version", "--invalid"])
            .is_err());
    }
}
