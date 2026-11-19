#[tokio::main]
async fn main() {
    let matches = construct::cli::command().get_matches();
    if let Err(error) = construct::cli::run(matches).await {
        if let Some(error) = construct::errors::transform_error(error) {
            eprintln!("Error: {error}");
            std::process::exit(1);
        }
    }
}
