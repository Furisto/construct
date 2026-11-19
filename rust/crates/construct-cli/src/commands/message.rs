pub fn command() -> clap::Command {
    clap::Command::new("message")
        .about("Manage messages")
        .display_order(5)
        .visible_aliases(["messages", "msg"])
        .arg(
            clap::Arg::new("arguments")
                .num_args(0..)
                .allow_hyphen_values(true)
                .trailing_var_arg(true),
        )
}
pub async fn run(_app: &mut crate::app::App, _matches: &clap::ArgMatches) -> crate::Result<()> {
    super::stub("message")
}
