package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/afero"
)

func TestDaemonInstall(t *testing.T) {
	setup := &TestSetup{}

	// Skip these tests on unsupported platforms in CI
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("daemon install tests only run on darwin and linux")
	}

	setup.RunTests(t, []TestScenario{
		{
			Name:    "success install http socket",
			Command: []string{"daemon", "install", "http"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupMockFileSystem(fs)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:    "success install unix socket",
			Command: []string{"daemon", "install", "unix"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupMockFileSystem(fs)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:    "success install with force flag",
			Command: []string{"daemon", "install", "http", "--force"},
			SetupFileSystem: func(fs *afero.Afero) {
				setupMockFileSystemWithExisting(fs)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:    "error - invalid socket type",
			Command: []string{"daemon", "install", "invalid"},
			Expected: TestExpectation{
				Error: "invalid argument \"invalid\" for \"install\"",
			},
		},
		{
			Name:    "error - no socket type provided",
			Command: []string{"daemon", "install"},
			Expected: TestExpectation{
				Error: "accepts 1 arg(s), received 0",
			},
		},
		{
			Name:    "error - too many arguments",
			Command: []string{"daemon", "install", "http", "extra"},
			Expected: TestExpectation{
				Error: "accepts 1 arg(s), received 2",
			},
		},
	})
}

func TestGetExecutableInfo(t *testing.T) {
	execPath, execDir, err := getExecutableInfo()
	if err != nil {
		t.Fatalf("GetExecutableInfo() failed: %v", err)
	}

	if execPath == "" {
		t.Error("execPath should not be empty")
	}

	if execDir == "" {
		t.Error("execDir should not be empty")
	}

	if !filepath.IsAbs(execPath) {
		t.Error("execPath should be absolute")
	}

	if !filepath.IsAbs(execDir) {
		t.Error("execDir should be absolute")
	}

	expectedDir := filepath.Dir(execPath)
	if execDir != expectedDir {
		t.Errorf("execDir = %q, want %q", execDir, expectedDir)
	}
}

func TestDaemonInstallTemplates(t *testing.T) {
	tests := []struct {
		name         string
		template     string
		expectedKeys []string
	}{
		{
			name:     "macOS HTTP template",
			template: macosHTTPTemplate,
			expectedKeys: []string{
				"sh.construct",
				"/usr/local/bin/construct",
				"daemon",
				"run",
				"HTTPSocket",
				"8080",
			},
		},
		{
			name:     "macOS Unix template",
			template: macosUnixTemplate,
			expectedKeys: []string{
				"sh.construct",
				"/usr/local/bin/construct",
				"daemon",
				"run",
				"ConstructSocket",
				"/tmp/construct.sock",
			},
		},
		{
			name:     "Linux HTTP socket template",
			template: linuxHTTPSocketTemplate,
			expectedKeys: []string{
				"construct.socket",
				"Construct HTTP Socket",
				"ListenStream=8080",
				"sockets.target",
			},
		},
		{
			name:     "Linux Unix socket template",
			template: linuxUnixSocketTemplate,
			expectedKeys: []string{
				"construct.socket",
				"Construct Unix Socket",
				"ListenStream=/tmp/construct.sock",
				"sockets.target",
			},
		},
		{
			name:     "Linux service template",
			template: linuxServiceTemplate,
			expectedKeys: []string{
				"construct.service",
				"Construct Service",
				"construct.socket",
				"/usr/local/bin/construct daemon run",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range tt.expectedKeys {
				if !contains(tt.template, key) {
					t.Errorf("template %s missing expected key: %s", tt.name, key)
				}
			}
		})
	}
}

func TestDaemonInstallValidArgs(t *testing.T) {
	cmd := NewDaemonInstallCmd()

	validArgs := cmd.ValidArgs
	expectedArgs := []string{"http", "unix"}

	if len(validArgs) != len(expectedArgs) {
		t.Errorf("ValidArgs length = %d, want %d", len(validArgs), len(expectedArgs))
	}

	for i, arg := range expectedArgs {
		if i >= len(validArgs) || validArgs[i] != arg {
			t.Errorf("ValidArgs[%d] = %q, want %q", i, validArgs[i], arg)
		}
	}
}

func TestDaemonInstallFlags(t *testing.T) {
	cmd := NewDaemonInstallCmd()

	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("force flag should exist")
	}

	if forceFlag.Shorthand != "f" {
		t.Errorf("force flag shorthand = %q, want %q", forceFlag.Shorthand, "f")
	}

	if forceFlag.DefValue != "false" {
		t.Errorf("force flag default = %q, want %q", forceFlag.DefValue, "false")
	}
}

// Helper functions

func setupMockFileSystem(fs *afero.Afero) {
	// Create directories that would be needed
	if runtime.GOOS == "darwin" {
		homeDir := "/home/testuser"
		os.Setenv("HOME", homeDir)
		launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
		fs.MkdirAll(launchAgentsDir, 0755)
	} else if runtime.GOOS == "linux" {
		fs.MkdirAll("/etc/systemd/system", 0755)
	}
}

func setupMockFileSystemWithExisting(fs *afero.Afero) {
	setupMockFileSystem(fs)

	// Create existing files to test force behavior
	if runtime.GOOS == "darwin" {
		homeDir := "/home/testuser"
		launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
		fs.WriteFile(filepath.Join(launchAgentsDir, "construct-http.plist"), []byte("existing"), 0644)
		fs.WriteFile(filepath.Join(launchAgentsDir, "construct-unix.plist"), []byte("existing"), 0644)
	} else if runtime.GOOS == "linux" {
		fs.WriteFile("/etc/systemd/system/construct.socket", []byte("existing"), 0644)
		fs.WriteFile("/etc/systemd/system/construct.service", []byte("existing"), 0644)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
