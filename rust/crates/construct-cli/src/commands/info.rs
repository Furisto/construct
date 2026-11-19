pub fn command() -> clap::Command {
    clap::Command::new("info")
        .about("Print information about the Construct CLI")
        .display_order(12)
        .arg(
            clap::Arg::new("arguments")
                .num_args(0..)
                .allow_hyphen_values(true)
                .trailing_var_arg(true),
        )
}
pub async fn run(_app: &mut crate::app::App, _matches: &clap::ArgMatches) -> crate::Result<()> {
    super::stub("info")
}
