package cmd

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/cmd/mocks"
	"github.com/google/uuid"
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
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				// Simulate executable path
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: " ✓ Socket file written to /etc/systemd/system/construct.socket\n ✓ Service file written to /etc/systemd/system/construct.service\n ✓ Systemd daemon reloaded\n ✓ Socket enabled\n ✓ Context 'default' created\n✓ Daemon installed successfully\n→ Next: Create a model provider with 'construct modelprovider create'\n",
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
				userInfo.EXPECT().UserID().Return("501")
			},
			SetupFileSystem: func(fs *afero.Afero) {
				// Simulate executable path
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: " ✓ Service file written to /Users/testuser/Library/LaunchAgents/construct-default.plist\n ✓ Launchd service loaded\n ✓ Context 'default' created\n✓ Daemon installed successfully\n→ Next: Create a model provider with 'construct modelprovider create'\n",
			},
		},
		{
			Name:     "success - HTTP socket install",
			Command:  []string{"daemon", "install", "--listen-http", "127.0.0.1:8080"},
			Platform: "linux",
			SetupMocks: func(mockClient *api_client.MockClient) {
				setupConnectionCheckMock(mockClient, true)
			},
			SetupCommandRunner: func(commandRunner *mocks.MockCommandRunner) {
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "daemon-reload").Return("", nil)
				commandRunner.EXPECT().Run(gomock.Any(), "systemctl", "enable", "construct.socket").Return("", nil)
			},
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: " ✓ Socket file written to /etc/systemd/system/construct.socket\n ✓ Service file written to /etc/systemd/system/construct.service\n ✓ Systemd daemon reloaded\n ✓ Socket enabled\n ✓ Context 'default' created\n✓ Daemon installed successfully\n→ Next: Create a model provider with 'construct modelprovider create'\n",
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
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: " ✓ Socket file written to /etc/systemd/system/construct.socket\n ✓ Service file written to /etc/systemd/system/construct.service\n ✓ Systemd daemon reloaded\n ✓ Socket enabled\n ✓ Context 'production' created\n✓ Daemon installed successfully\n→ Next: Create a model provider with 'construct modelprovider create'\n",
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
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
				// Simulate existing installation
				fs.WriteFile("/etc/systemd/system/construct.socket", []byte("existing"), 0644)
				fs.WriteFile("/etc/systemd/system/construct.service", []byte("existing"), 0644)
			},
			Expected: TestExpectation{
				Stdout: " ✓ Socket file written to /etc/systemd/system/construct.socket\n ✓ Service file written to /etc/systemd/system/construct.service\n ✓ Systemd daemon reloaded\n ✓ Socket enabled\n ✓ Context 'default' created\n✓ Daemon installed successfully\n→ Next: Create a model provider with 'construct modelprovider create'\n",
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
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Stdout: "",
			},
		},
		{
			Name:     "error - already installed without force",
			Command:  []string{"daemon", "install"},
			Platform: "linux",
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
				// Simulate existing installation
				fs.WriteFile("/etc/systemd/system/construct.socket", []byte("existing"), 0644)
			},
			Expected: TestExpectation{
				Error: "Construct daemon is already installed on this system",
			},
		},
		{
			Name:     "error - permission denied",
			Command:  []string{"daemon", "install"},
			Platform: "linux",
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
				// Make /etc read-only to simulate permission error
				fs.Chmod("/etc", 0444)
			},
			Expected: TestExpectation{
				Error: "Permission denied accessing /etc/systemd/system/construct.socket",
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
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Error: "Command failed: systemctl daemon-reload",
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
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
			},
			SetupFileSystem: func(fs *afero.Afero) {
				fs.WriteFile("/usr/local/bin/construct", []byte("binary"), 0755)
			},
			Expected: TestExpectation{
				Error: "Connection to daemon failed: failed to check connection: connection failed",
			},
		},
		{
			Name:     "error - unsupported OS",
			Command:  []string{"daemon", "install"},
			Platform: "windows",
			SetupUserInfo: func(userInfo *mocks.MockUserInfo) {
				userInfo.EXPECT().HomeDir().Return("/home/user", nil)
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
				ModelProviders: []*v1.ModelProvider{
					{
						Id:           uuid.New().String(),
						Name:         "openai",
						ProviderType: v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI,
						Enabled:      true,
					},
				},
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
