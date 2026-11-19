use std::process::Command;

#[test]
fn parser_failures_use_go_exit_status_and_stderr() {
    for args in [
        vec!["agent", "get"],
        vec!["agent", "list", "--unknown"],
        vec!["model", "create", "missing-flags"],
        vec!["agent", "list", "-o", "invalid"],
    ] {
        let output = Command::new(env!("CARGO_BIN_EXE_construct"))
            .args(args)
            .output()
            .unwrap();
        assert_eq!(output.status.code(), Some(1));
        assert!(output.stdout.is_empty());
        assert!(!output.stderr.is_empty());
    }
    let output = Command::new(env!("CARGO_BIN_EXE_construct"))
        .arg("--help")
        .output()
        .unwrap();
    assert!(output.status.success());
    assert!(output.stderr.is_empty());
    assert!(!output.stdout.is_empty());
}
