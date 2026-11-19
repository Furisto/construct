pub fn command() -> clap::Command {
    clap::Command::new("task")
        .about("Manage tasks, which are the records of conversations")
        .display_order(4)
        .visible_aliases(["tasks"])
        .arg(
            clap::Arg::new("arguments")
                .num_args(0..)
                .allow_hyphen_values(true)
                .trailing_var_arg(true),
        )
}
pub async fn run(_app: &mut crate::app::App, _matches: &clap::ArgMatches) -> crate::Result<()> {
    super::stub("task")
}
