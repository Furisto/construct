pub fn command() -> clap::Command {
    clap::Command::new("update")
        .about("Update Construct to the latest version")
        .display_order(13)
        .arg(
            clap::Arg::new("arguments")
                .num_args(0..)
                .allow_hyphen_values(true)
                .trailing_var_arg(true),
        )
}
pub async fn run(_app: &mut crate::app::App, _matches: &clap::ArgMatches) -> crate::Result<()> {
    super::stub("update")
}
