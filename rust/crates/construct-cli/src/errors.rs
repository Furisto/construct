use std::fmt::{Display, Formatter};

#[derive(Debug, Default)]
pub struct UserFacingError {
    pub cause: Option<crate::Error>,
    pub user_message: String,
    pub solutions: Vec<String>,
    pub multiline: bool,
    pub tech_details: String,
    pub help_urls: Vec<String>,
}
impl Display for UserFacingError {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        write!(f, "\x1b[1m{}\x1b[0m\n\n", self.user_message)?;
        if !self.solutions.is_empty() {
            writeln!(f, "Troubleshooting steps:")?;
            for (i, solution) in self.solutions.iter().enumerate() {
                if self.multiline {
                    let mut lines = solution.split('\n');
                    writeln!(f, "  {}. {}", i + 1, lines.next().unwrap_or_default())?;
                    for line in lines {
                        if !line.is_empty() {
                            write!(f, "     {line}")?;
                        }
                        writeln!(f)?;
                    }
                    writeln!(f)?;
                } else {
                    writeln!(f, "  {}. {solution}", i + 1)?;
                }
            }
            writeln!(f)?;
        }
        if !self.tech_details.is_empty() {
            writeln!(f, "Technical details:\n{}", self.tech_details)?;
        }
        if !self.help_urls.is_empty() {
            writeln!(f, "If the problem persists:")?;
            for url in &self.help_urls {
                writeln!(f, "\x1b[38;5;75m→\x1b[0m {url}")?;
            }
        }
        Ok(())
    }
}
impl std::error::Error for UserFacingError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        self.cause.as_deref().map(|e| e as _)
    }
}
impl UserFacingError {
    pub fn new(
        message: impl Into<String>,
        cause: Option<crate::Error>,
        solutions: Vec<String>,
        tech_details: String,
        help_urls: Vec<String>,
    ) -> Self {
        Self {
            cause,
            user_message: message.into(),
            solutions,
            multiline: false,
            tech_details,
            help_urls,
        }
    }
    fn with_help(mut self, section: &str) -> Self {
        self.help_urls = vec![
            format!("https://docs.construct.sh/daemon/troubleshooting#{section}"),
            "https://github.com/furisto/construct/issues/new".into(),
        ];
        self
    }
    pub fn permission(path: &str, error: crate::Error) -> Self {
        let details = format!("Failed to access {path}: {error}");
        Self::new(
            format!("Permission denied accessing {path}"),
            Some(error),
            strings(&[
                "Check file permissions and ownership",
                "Ensure you have write access to the directory",
                "Try running with appropriate privileges if needed",
                "Verify the path exists and is accessible",
            ]),
            details,
            vec![],
        )
        .with_help("permission-errors")
    }
    pub fn already_installed(path: &str) -> Self {
        Self::new(
            "Construct daemon is already installed on this system",
            None,
            strings(&[
                "Use '--force' flag to overwrite: construct daemon install --force",
                "Uninstall first: construct daemon uninstall && construct daemon install",
                "Use '--name' flag to create a separate daemon instance (advanced)",
            ]),
            format!("Service file exists at: {path}"),
            vec![],
        )
        .with_help("already-installed")
    }
    pub fn command(command: &str, error: crate::Error, output: &str, args: &[String]) -> Self {
        let details = format!(
            "Command '{command} {}' failed: {error}\nOutput: {output}",
            args.join(" ")
        );
        Self::new(
            format!("Command failed: {command}"),
            Some(error),
            strings(&[
                "Check if the required system service is running",
                "Verify you have permission to manage system services",
                "Check system logs for more details",
                "Try running the command manually to diagnose the issue",
            ]),
            details,
            vec![],
        )
        .with_help("command-failed")
    }
    pub fn connection(address: &str, error: crate::Error) -> Self {
        let message = error.to_string();
        let details = format!("Connection failed to {address}: {error}");
        let solutions = if message.contains("connection refused") {
            strings(&[
                "Wait a few seconds for the daemon to start, then try again",
                "Check if the daemon process is running",
                "Restart the daemon service",
                "Verify the address is correct and accessible",
            ])
        } else if message.contains("no such file") {
            strings(&[
                "Check if the socket file exists and has correct permissions",
                "Restart the daemon to recreate the socket",
                "Verify the socket path is correct",
            ])
        } else {
            strings(&[
                "Check daemon logs for startup errors",
                "Verify the daemon binary is working",
                "Try reinstalling the daemon",
            ])
        };
        Self::new(
            "Installation completed but cannot connect to the daemon",
            Some(error),
            solutions,
            details,
            vec![],
        )
        .with_help("connection-failed")
    }
}
fn strings(values: &[&str]) -> Vec<String> {
    values.iter().map(|s| (*s).into()).collect()
}
pub fn transform_error(error: crate::Error) -> Option<crate::Error> {
    if error.is::<UserFacingError>() {
        return Some(error);
    }
    let message = error.to_string();
    let connect = error.downcast_ref::<construct_connect::ConnectError>();
    if connect.is_some_and(|e| e.code == construct_connect::Code::Canceled)
        || error
            .downcast_ref::<std::io::Error>()
            .is_some_and(|e| e.kind() == std::io::ErrorKind::Interrupted)
    {
        return None;
    }
    let (user, solutions, section) = if message.starts_with("unavailable")
        || connect.is_some_and(|e| e.code == construct_connect::Code::Unavailable)
    {
        (
            "Agent runtime unavailable. Retrying...",
            strings(&[
                "Wait a few seconds for the agent runtime to become available",
                "Try again later",
            ]),
            "",
        )
    } else if message.contains("no such file or directory") {
        (
            "Required file or directory not found",
            strings(&[
                "Verify the path exists and is accessible",
                "Check if the parent directory exists",
                "Ensure the construct binary is properly installed",
            ]),
            "file-not-found",
        )
    } else if message.contains("address already in use") {
        (
            "The network address is already in use by another process",
            strings(&[
                "Choose a different port number",
                "Stop the process using this port",
                "Use Unix socket instead: construct daemon install",
            ]),
            "address-in-use",
        )
    } else if message.contains("operation not permitted") {
        (
            "Operation not permitted - insufficient privileges",
            strings(&[
                "Check if you have the necessary permissions",
                "Try running with appropriate privileges if needed",
                "Verify you can manage system services",
            ]),
            "operation-not-permitted",
        )
    } else {
        return Some(error);
    };
    let mut transformed = UserFacingError::new(user, Some(error), solutions, String::new(), vec![]);
    if !section.is_empty() {
        transformed.tech_details = message;
        transformed = transformed.with_help(section);
    }
    Some(Box::new(transformed))
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn formatting() {
        let e = UserFacingError::new("Oops", None, vec!["Retry".into()], String::new(), vec![]);
        assert_eq!(
            e.to_string(),
            "\x1b[1mOops\x1b[0m\n\nTroubleshooting steps:\n  1. Retry\n\n"
        );
    }
    #[test]
    fn transformation() {
        assert!(
            transform_error(std::io::Error::from(std::io::ErrorKind::Interrupted).into()).is_none()
        );
        assert!(transform_error("plain".into()).unwrap().to_string() == "plain");
        assert!(transform_error("unavailable: failed".into())
            .unwrap()
            .is::<UserFacingError>());
    }
}
