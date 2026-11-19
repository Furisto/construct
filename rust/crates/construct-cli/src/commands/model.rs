pub fn command() -> clap::Command {
    clap::Command::new("model")
        .about("Manage the AI models available to agents")
        .display_order(6)
        .visible_aliases(["models"])
        .arg(
            clap::Arg::new("arguments")
                .num_args(0..)
                .allow_hyphen_values(true)
                .trailing_var_arg(true),
        )
}
pub async fn run(_app: &mut crate::app::App, _matches: &clap::ArgMatches) -> crate::Result<()> {
    super::stub("model")
}
