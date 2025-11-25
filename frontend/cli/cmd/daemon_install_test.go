package cmd

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/shared/mocks"
	"github.com/furisto/construct/shared/conv"
	"github.com/spf13/afero"
	"go.uber.org/mock/gomock"
)

func TestDaemonInstall(t *testing.T) {
	setup := &TestSetup{}

	setup.RunTests(t, []TestScenario{
		{
			Name:     "success - basic unix socket install on Linux",
			Command:  []string{"daemon", "install"},
			Platform: "linux",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupConnectionCheckMock(mockClient, true)
			},
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "daemon-reload").Return("", nil)
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "enable", "construct.socket").Return("", nil)
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				// Simulate executable path
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: conv.Ptr("✔ Socket file written to /etc/systemd/system/construct.socket\n✔ Service file written to /etc/systemd/system/construct.service\n✔ Systemd daemon reloaded\n✔ Socket enabled\n ✔ Context 'default' created\n\r\x1b[K✔ Daemon is responding to requests\n✔ Daemon installed successfully\n➡️ Next: Create a model provider with 'construct modelprovider create'\n"),
			},
		},
		{
			Name:     "success - basic unix socket install on macOS",
			Command:  []string{"daemon", "install"},
			Platform: "darwin",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupConnectionCheckMock(mockClient, true)
			},
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "launchctl", "bootstrap", "gui/501", gomock.Any()).Return("", nil)
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().HomeDir().Return("/Users/testuser", nil)
				userInfo.EXPECT().UserID().Return("501", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				// Simulate executable path
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: conv.Ptr(" ✔ Service file written to /Users/testuser/Library/LaunchAgents/construct-default.plist\n ✔ Launchd service loaded\n ✔ Context 'default' created\n\r\x1b[K✔ Daemon is responding to requests\n✔ Daemon installed successfully\n➡️ Next: Create a model provider with 'construct modelprovider create'\n"),
			},
		},
		{
			Name:     "success - HTTP socket install",
			Command:  []string{"daemon", "install", "--listen-http", "http://127.0.0.1:8080"},
			Platform: "linux",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupConnectionCheckMock(mockClient, true)
			},
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "daemon-reload").Return("", nil)
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "enable", "construct.socket").Return("", nil)
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: conv.Ptr("✔ Socket file written to /etc/systemd/system/construct.socket\n✔ Service file written to /etc/systemd/system/construct.service\n✔ Systemd daemon reloaded\n✔ Socket enabled\n ✔ Context 'default' created\n\r\x1b[K✔ Daemon is responding to requests\n✔ Daemon installed successfully\n➡️ Next: Create a model provider with 'construct modelprovider create'\n"),
			},
		},
		{
			Name:     "success - custom name install",
			Command:  []string{"daemon", "install", "--name", "production"},
			Platform: "linux",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupConnectionCheckMock(mockClient, true)
			},
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "daemon-reload").Return("", nil)
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "enable", "construct.socket").Return("", nil)
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: conv.Ptr("✔ Socket file written to /etc/systemd/system/construct.socket\n✔ Service file written to /etc/systemd/system/construct.service\n✔ Systemd daemon reloaded\n✔ Socket enabled\n ✔ Context 'production' created\n\r\x1b[K✔ Daemon is responding to requests\n✔ Daemon installed successfully\n➡️ Next: Create a model provider with 'construct modelprovider create'\n"),
			},
		},
		{
			Name:     "success - force reinstall",
			Command:  []string{"daemon", "install", "--force"},
			Platform: "linux",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupConnectionCheckMock(mockClient, true)
			},
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "daemon-reload").Return("", nil)
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "enable", "construct.socket").Return("", nil)
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
				// Simulate existing installation
				fs.WriteFile("/etc/systemd/system/construct.socket", []byte("existing"), 0644)
				fs.WriteFile("/etc/systemd/system/construct.service", []byte("existing"), 0644)
			},
			Expected: TestExpectation{
				Stdout: conv.Ptr("✔ Socket file written to /etc/systemd/system/construct.socket\n✔ Service file written to /etc/systemd/system/construct.service\n✔ Systemd daemon reloaded\n✔ Socket enabled\n ✔ Context 'default' created\n\r\x1b[K✔ Daemon is responding to requests\n✔ Daemon installed successfully\n➡️ Next: Create a model provider with 'construct modelprovider create'\n"),
			},
		},
		{
			Name:     "success - quiet mode",
			Command:  []string{"daemon", "install", "--quiet"},
			Platform: "linux",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupConnectionCheckMock(mockClient, true)
			},
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "daemon-reload").Return("", nil)
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "enable", "construct.socket").Return("", nil)
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: conv.Ptr(""),
			},
		},
		{
			Name:     "error - already installed without force",
			Command:  []string{"daemon", "install"},
			Platform: "linux",
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
				// Simulate existing installation
				fs.WriteFile("/etc/systemd/system/construct.socket", []byte("existing"), 0644)
			},
			Expected: TestExpectation{
				Error: "Construct daemon is already installed on this system\n\nTroubleshooting steps:\n  1. Use '--force' flag to overwrite: construct daemon install --force\n  2. Uninstall first: construct daemon uninstall && construct daemon install\n  3. Use '--name' flag to create a separate daemon instance (advanced)\n\nTechnical details:\nService file exists at: /etc/systemd/system/construct.socket\nIf the problem persists:\n→ https://docs.construct.sh/daemon/troubleshooting#already-installed\n→ https://github.com/furisto/construct/issues/new\n",
			},
		},
		{
			Name:     "error - command failure",
			Command:  []string{"daemon", "install"},
			Platform: "linux",
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "daemon-reload").Return("Failed to reload", fmt.Errorf("systemctl error"))
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Error: "Command failed: systemctl daemon-reload\n\nTroubleshooting steps:\n  1. Check if the required system service is running\n  2. Verify you have permission to manage system services\n  3. Check system logs for more details\n  4. Try running the command manually to diagnose the issue\n\nTechnical details:\nCommand 'systemctl daemon-reload ' failed: systemctl error\nOutput: Failed to reload\nIf the problem persists:\n→ https://docs.construct.sh/daemon/troubleshooting#command-failed\n→ https://github.com/furisto/construct/issues/new\n",
			},
		},
		{
			Name:     "error - connection failure",
			Command:  []string{"daemon", "install"},
			Platform: "linux",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupConnectionCheckMock(mockClient, false)
			},
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "daemon-reload").Return("", nil)
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "enable", "construct.socket").Return("", nil)
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Error: "Connection to daemon failed: failed to check connection: connection failed\n\nTroubleshooting steps:\n  1. Check if the daemon socket is active:\n        systemctl --user status construct.socket\n        systemctl --user status construct.service\n\n  2. Check service logs:\n        journalctl --user -u construct.service --no-pager -n 20\n        journalctl --user -u construct.socket --no-pager -n 20\n\n  3. Try manually starting the socket:\n        systemctl --user start construct.socket\n        systemctl --user start construct.service\n\n  4. Verify the daemon endpoint:\n        Address: /tmp/construct-default.sock\n        Type: unix\n        Check if socket file exists and has correct permissions:\n        ls -la /tmp/construct.sock\n\n  5. Check for permission issues:\n        # Check if systemd files exist:\n        ls -la /etc/systemd/system/construct.*\n\n  6. Try reinstalling the daemon:\n        construct daemon uninstall\n        construct daemon install\n\n  7. For additional help:\n        - Check if the construct binary is accessible and executable\n        - Verify system resources (disk space, memory)\n        - Run 'construct daemon run' manually to see direct error output\n\n\nIf the problem persists:\n→ https://docs.construct.sh/daemon/troubleshooting\n",
			},
		},
		{
			Name:     "error - unsupported OS",
			Command:  []string{"daemon", "install"},
			Platform: "windows",
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().ConstructConfigDir().Return("/home/user/.construct", nil).AnyTimes()
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Error: "unsupported operating system: windows",
			},
		},
	})
}

func setupConnectionCheckMock(mockClient *api_client.MockClient, success bool) {
	if success {
		mockClient.ModelProvider.EXPECT().ListModelProviders(
			gomock.Any(),
			&connect.Request[v1.ListModelProvidersRequest]{
				Msg: &v1.ListModelProvidersRequest{},
			},
		).Return(&connect.Response[v1.ListModelProvidersResponse]{
			Msg: &v1.ListModelProvidersResponse{
				ModelProviders: []*v1.ModelProvider{},
			},
		}, nil)
	} else {
		mockClient.ModelProvider.EXPECT().ListModelProviders(
			gomock.Any(),
			&connect.Request[v1.ListModelProvidersRequest]{
				Msg: &v1.ListModelProvidersRequest{},
			},
		).Return(nil, fmt.Errorf("connection failed"))
	}
}
