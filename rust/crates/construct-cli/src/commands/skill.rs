pub fn command() -> clap::Command {
    clap::Command::new("skill")
        .about("Manage skills that extend agent capabilities")
        .display_order(8)
        .visible_aliases(["skills"])
        .arg(
            clap::Arg::new("arguments")
                .num_args(0..)
                .allow_hyphen_values(true)
                .trailing_var_arg(true),
        )
}
pub async fn run(_app: &mut crate::app::App, _matches: &clap::ArgMatches) -> crate::Result<()> {
    super::stub("skill")
}
