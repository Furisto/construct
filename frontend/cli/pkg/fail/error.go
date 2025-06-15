package fail

import (
	"fmt"
	"os"
	"strings"

	"github.com/furisto/construct/frontend/cli/pkg/terminal"
)

type UserFacingSolutionFormat string

const (
	UserFacingSolutionFormatMultiline  UserFacingSolutionFormat = "multiline"
	UserFacingSolutionFormatSingleline UserFacingSolutionFormat = "singleline"
)

type Troubleshooting struct {
	Format    UserFacingSolutionFormat
	Solutions []string
}

type UserFacingError struct {
	Cause           error
	UserMessage     string
	Troubleshooting Troubleshooting
	TechDetails     string
	HelpURLs        []string
}

func (e *UserFacingError) Error() string {
	var msg strings.Builder

	msg.WriteString(fmt.Sprintf("%s\n\n", terminal.Bold(e.UserMessage)))

	if len(e.Troubleshooting.Solutions) > 0 {
		msg.WriteString("Troubleshooting steps:\n")
		for i, solution := range e.Troubleshooting.Solutions {
			if e.Troubleshooting.Format == UserFacingSolutionFormatMultiline {
				// Split multi-line solutions and indent continuation lines properly
				lines := strings.Split(solution, "\n")
				msg.WriteString(fmt.Sprintf("  %d. %s\n", i+1, lines[0]))
				for j := 1; j < len(lines); j++ {
					if lines[j] != "" {
						msg.WriteString(fmt.Sprintf("     %s\n", lines[j]))
					} else {
						msg.WriteString("\n")
					}
				}
				msg.WriteString("\n")
			} else {
				msg.WriteString(fmt.Sprintf("  %d. %s\n", i+1, solution))
			}
		}
		msg.WriteString("\n")
	}

	if e.TechDetails != "" {
		msg.WriteString("Technical details:\n")
		msg.WriteString(e.TechDetails)
		msg.WriteString("\n")
	}

	if len(e.HelpURLs) > 0 {
		msg.WriteString("If the problem persists:\n")
		for _, url := range e.HelpURLs {
			msg.WriteString(fmt.Sprintf("%s %s\n", terminal.LinkSymbol, url))
		}
	}

	return msg.String()
}

func (e *UserFacingError) Unwrap() error {
	return e.Cause
}

func NewUserFacingError(userMessage string, cause error, troubleshooting Troubleshooting, techDetails string, helpURLs []string) *UserFacingError {
	return &UserFacingError{
		Cause:           cause,
		UserMessage:     userMessage,
		Troubleshooting: troubleshooting,
		TechDetails:     techDetails,
		HelpURLs:        helpURLs,
	}
}

func NewPermissionError(path string, err error) *UserFacingError {
	return &UserFacingError{
		Cause:       err,
		UserMessage: fmt.Sprintf("Permission denied accessing %s", path),
		Troubleshooting: Troubleshooting{
			Format: UserFacingSolutionFormatSingleline,
			Solutions: []string{
				"Check file permissions and ownership",
				"Ensure you have write access to the directory",
				"Try running with appropriate privileges if needed",
				"Verify the path exists and is accessible",
			},
		},
		TechDetails: fmt.Sprintf("Failed to access %s: %v", path, err),
		HelpURLs: []string{
			"https://docs.construct.sh/daemon/troubleshooting#permission-errors",
			"https://github.com/furisto/construct/issues/new",
		},
	}
}

func NewAlreadyInstalledError(path string) *UserFacingError {
	return &UserFacingError{
		Cause:       nil,
		UserMessage: "Construct daemon is already installed on this system",
		Troubleshooting: Troubleshooting{
			Format: UserFacingSolutionFormatSingleline,
			Solutions: []string{
				"Use '--force' flag to overwrite: construct daemon install --force",
				"Uninstall first: construct daemon uninstall && construct daemon install",
				"Use '--name' flag to create a separate daemon instance (advanced)",
			},
		},
		TechDetails: fmt.Sprintf("Service file exists at: %s", path),
		HelpURLs: []string{
			"https://docs.construct.sh/daemon/troubleshooting#already-installed",
			"https://github.com/furisto/construct/issues/new",
		},
	}
}

func NewCommandError(command string, err error, output string, args ...string) *UserFacingError {
	return &UserFacingError{
		Cause:       err,
		UserMessage: fmt.Sprintf("Command failed: %s", command),
		Troubleshooting: Troubleshooting{
			Format: UserFacingSolutionFormatSingleline,
			Solutions: []string{
				"Check if the required system service is running",
				"Verify you have permission to manage system services",
				"Check system logs for more details",
				"Try running the command manually to diagnose the issue",
			},
		},
		TechDetails: fmt.Sprintf("Command '%s %s' failed: %v\nOutput: %s", command, strings.Join(args, " "), err, output),
		HelpURLs: []string{
			"https://docs.construct.sh/daemon/troubleshooting#command-failed",
			"https://github.com/furisto/construct/issues/new",
		},
	}
}

func NewConnectionError(address string, err error) *UserFacingError {
	var solutions []string

	if strings.Contains(err.Error(), "connection refused") {
		solutions = []string{
			"Wait a few seconds for the daemon to start, then try again",
			"Check if the daemon process is running",
			"Restart the daemon service",
			"Verify the address is correct and accessible",
		}
	} else if strings.Contains(err.Error(), "no such file") {
		solutions = []string{
			"Check if the socket file exists and has correct permissions",
			"Restart the daemon to recreate the socket",
			"Verify the socket path is correct",
		}
	} else {
		solutions = []string{
			"Check daemon logs for startup errors",
			"Verify the daemon binary is working",
			"Try reinstalling the daemon",
		}
	}

	return &UserFacingError{
		Cause:       err,
		UserMessage: "Installation completed but cannot connect to the daemon",
		Troubleshooting: Troubleshooting{
			Format:    UserFacingSolutionFormatSingleline,
			Solutions: solutions,
		},
		TechDetails: fmt.Sprintf("Connection failed to %s: %v", address, err),
		HelpURLs: []string{
			"https://docs.construct.sh/daemon/troubleshooting#connection-failed",
			"https://github.com/furisto/construct/issues/new",
		},
	}
}

func EnhanceError(err error, context map[string]interface{}) error {
	if err == nil {
		return nil
	}

	if _, ok := err.(*UserFacingError); ok {
		return err
	}

	errStr := err.Error()

	if os.IsPermission(err) {
		if path, ok := context["path"].(string); ok {
			return NewPermissionError(path, err)
		}
	}

	if strings.Contains(errStr, "no such file or directory") {
		return &UserFacingError{
			Cause:       err,
			UserMessage: "Required file or directory not found",
			Troubleshooting: Troubleshooting{
				Format: UserFacingSolutionFormatSingleline,
				Solutions: []string{
					"Verify the path exists and is accessible",
					"Check if the parent directory exists",
					"Ensure the construct binary is properly installed",
				},
			},
			TechDetails: errStr,
			HelpURLs: []string{
				"https://docs.construct.sh/daemon/troubleshooting#file-not-found",
				"https://github.com/furisto/construct/issues/new",
			},
		}
	}

	if strings.Contains(errStr, "address already in use") {
		return &UserFacingError{
			Cause:       err,
			UserMessage: "The network address is already in use by another process",
			Troubleshooting: Troubleshooting{
				Format: UserFacingSolutionFormatSingleline,
				Solutions: []string{
					"Choose a different port number",
					"Stop the process using this port",
					"Use Unix socket instead: construct daemon install",
				},
			},
			TechDetails: errStr,
			HelpURLs: []string{
				"https://docs.construct.sh/daemon/troubleshooting#address-in-use",
				"https://github.com/furisto/construct/issues/new",
			},
		}
	}

	if strings.Contains(errStr, "operation not permitted") {
		return &UserFacingError{
			Cause:       err,
			UserMessage: "Operation not permitted - insufficient privileges",
			Troubleshooting: Troubleshooting{
				Format: UserFacingSolutionFormatSingleline,
				Solutions: []string{
					"Check if you have the necessary permissions",
					"Try running with appropriate privileges if needed",
					"Verify you can manage system services",
				},
			},
			TechDetails: errStr,
			HelpURLs: []string{
				"https://docs.construct.sh/daemon/troubleshooting#operation-not-permitted",
				"https://github.com/furisto/construct/issues/new",
			},
		}
	}

	return err
}
