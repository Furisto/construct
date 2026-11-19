#[tokio::main]
async fn main() {
    let matches = construct::cli::command()
        .try_get_matches()
        .unwrap_or_else(|error| {
            let code = i32::from(error.use_stderr());
            let _ = error.print();
            std::process::exit(code);
        });
    if let Err(error) = construct::cli::run(matches).await {
        if let Some(error) = construct::errors::transform_error(error) {
            eprintln!("Error: {error}");
            std::process::exit(1);
        }
    }
}
