package agent

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

func ProjectStructure(root string) (string, error) {
	files, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, file := range files {
		fmt.Fprintf(&sb, "|_%s\n", file.Name())
	}
	return sb.String(), nil
}

type Shell struct {
	Path string
	Name string
}

func DefaultShell() (*Shell, error) {
	var shellPath string
	var err error

	shellPath, err = shellFromEnv()
	if err == nil && shellPath != "" {
		return newShell(shellPath)
	}

	shellPath, err = shellFromPasswd()
	if err == nil && shellPath != "" {
		return newShell(shellPath)
	}

	shellPath, err = shellFromCommon()
	if err == nil && shellPath != "" {
		return newShell(shellPath)
	}

	return nil, fmt.Errorf("unable to determine user shell using any available method")
}

func shellFromEnv() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "", fmt.Errorf("SHELL environment variable not set")
	}

	if _, err := os.Stat(shell); err != nil {
		return "", fmt.Errorf("shell from environment variable does not exist: %w", err)
	}

	return shell, nil
}

func shellFromPasswd() (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("passwd file not available on Windows")
	}

	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	passwdFile, err := os.Open("/etc/passwd")
	if err != nil {
		return "", fmt.Errorf("failed to open passwd file: %w", err)
	}
	defer passwdFile.Close()

	scanner := bufio.NewScanner(passwdFile)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) >= 7 && fields[0] == currentUser.Username {
			shell := fields[6]
			if shell != "" && shell != "/sbin/nologin" && shell != "/bin/false" {
				if _, err := os.Stat(shell); err != nil {
					return "", fmt.Errorf("shell from passwd file does not exist: %w", err)
				}
				return shell, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading passwd file: %w", err)
	}

	return "", fmt.Errorf("user shell not found in passwd file")
}

func shellFromCommon() (string, error) {
	switch runtime.GOOS {
	case "windows":
		psPath := filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, err := os.Stat(psPath); err == nil {
			return psPath, nil
		}

		output, err := exec.Command("where", "pwsh.exe").Output()
		if err == nil && len(output) > 0 {
			path := strings.TrimSpace(string(output))
			return path, nil
		}

		cmdPath := filepath.Join(os.Getenv("SYSTEMROOT"), "System32", "cmd.exe")
		if _, err := os.Stat(cmdPath); err == nil {
			return cmdPath, nil
		}

		return "", fmt.Errorf("could not find a valid shell on Windows")

	case "darwin":
		shells := []string{"/bin/zsh", "/bin/bash", "/bin/sh"}
		for _, shell := range shells {
			if _, err := os.Stat(shell); err == nil {
				return shell, nil
			}
		}

	default:
		shells := []string{"/bin/bash", "/bin/sh", "/usr/bin/bash", "/usr/bin/sh"}
		for _, shell := range shells {
			if _, err := os.Stat(shell); err == nil {
				return shell, nil
			}
		}
	}

	return "", fmt.Errorf("no default shell found for %s", runtime.GOOS)
}

func newShell(path string) (*Shell, error) {
	if path == "" {
		return nil, fmt.Errorf("empty shell path")
	}
	name := filepath.Base(path)

	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}

	return &Shell{
		Path: path,
		Name: name,
	}, nil
}
