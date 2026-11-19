pub fn command() -> clap::Command {
    clap::Command::new("new")
        .about("Launch a new interactive session with an agent")
        .display_order(0)
        .arg(
            clap::Arg::new("arguments")
                .num_args(0..)
                .allow_hyphen_values(true)
                .trailing_var_arg(true),
        )
}
pub async fn run(_app: &mut crate::app::App, _matches: &clap::ArgMatches) -> crate::Result<()> {
    super::stub("new")
}
