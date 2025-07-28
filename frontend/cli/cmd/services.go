package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"

	api "github.com/furisto/construct/api/go/client"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

type ContextManager struct {
	fs       *afero.Afero
	userInfo UserInfo
}

func NewContextManager(fs *afero.Afero, userInfo UserInfo) *ContextManager {
	return &ContextManager{fs: fs, userInfo: userInfo}
}

func (m *ContextManager) LoadContext() (*api.EndpointContexts, error) {
	constructDir, err := m.userInfo.ConstructDir()
	if err != nil {
		return nil, err
	}

	endpointContextsFile := filepath.Join(constructDir, "context.yaml")
	exists, err := m.fs.Exists(endpointContextsFile)
	if err != nil {
		return nil, err
	}

	var endpointContexts api.EndpointContexts
	if exists {
		content, err := m.fs.ReadFile(endpointContextsFile)
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(content, &endpointContexts)
		if err != nil {
			return nil, err
		}
	} else {
		endpointContexts = api.EndpointContexts{
			Contexts: make(map[string]api.EndpointContext),
		}
	}

	return &endpointContexts, nil
}

func (m *ContextManager) UpsertContext(contextName string, kind string, address string, setCurrent bool) (bool, error) {
	endpointContexts, err := m.LoadContext()
	if err != nil {
		return false, err
	}

	context := api.EndpointContext{
		Address: address,
		Kind:    kind,
	}

	if err := context.Validate(); err != nil {
		return false, err
	}

	_, exists := endpointContexts.Contexts[contextName]
	endpointContexts.Contexts[contextName] = context

	if setCurrent {
		err = endpointContexts.SetCurrent(contextName)
		if err != nil {
			return false, err
		}
	}

	return exists, m.saveContext(endpointContexts)
}

func (m *ContextManager) SetCurrentContext(contextName string) error {
	endpointContexts, err := m.LoadContext()
	if err != nil {
		return err
	}

	err = endpointContexts.SetCurrent(contextName)
	if err != nil {
		return err
	}

	return m.saveContext(endpointContexts)
}

func (m *ContextManager) saveContext(endpointContexts *api.EndpointContexts) error {
	constructDir, err := m.userInfo.ConstructDir()
	if err != nil {
		return err
	}

	content, err := yaml.Marshal(endpointContexts)
	if err != nil {
		return err
	}

	endpointContextsFile := filepath.Join(constructDir, "context.yaml")
	return m.fs.WriteFile(endpointContextsFile, content, 0644)
}

type ContextKey string

const (
	ContextKeyAPIClient       ContextKey = "api_client"
	ContextKeyFileSystem      ContextKey = "filesystem"
	ContextKeyOutputRenderer  ContextKey = "output_renderer"
	ContextKeyCommandRunner   ContextKey = "command_runner"
	ContextKeyEndpointContext ContextKey = "endpoint_context"
	ContextKeyRuntimeInfo     ContextKey = "runtime_info"
	ContextKeyUserInfo        ContextKey = "user_info"
	ContextKeyGlobalOptions   ContextKey = "global_options"
)

func getAPIClient(ctx context.Context) *api.Client {
	apiClient := ctx.Value(ContextKeyAPIClient)
	if apiClient != nil {
		return apiClient.(*api.Client)
	}

	return nil
}

func getFileSystem(ctx context.Context) *afero.Afero {
	fs := ctx.Value(ContextKeyFileSystem)
	if fs != nil {
		return fs.(*afero.Afero)
	}

	return &afero.Afero{Fs: afero.NewOsFs()}
}

//go:generate mockgen -destination=mocks/command_runner_mock.go -package=mocks . CommandRunner
type CommandRunner interface {
	Run(ctx context.Context, command string, args ...string) (string, error)
}

type RuntimeInfo interface {
	GOOS() string
}

//go:generate mockgen -destination=mocks/user_info_mock.go -package=mocks . UserInfo
type UserInfo interface {
	UserID() string
	HomeDir() (string, error)
	ConstructDir() (string, error)
}

type DefaultCommandRunner struct{}

func (r *DefaultCommandRunner) Run(ctx context.Context, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

type DefaultRuntimeInfo struct{}

func (r *DefaultRuntimeInfo) GOOS() string {
	return runtime.GOOS
}

type DefaultUserInfo struct {
	fs *afero.Afero
}

func NewDefaultUserInfo(fs *afero.Afero) *DefaultUserInfo {
	return &DefaultUserInfo{fs: fs}
}

func (u *DefaultUserInfo) UserID() string {
	user, err := user.Current()
	if err != nil {
		return ""
	}
	return user.Uid
}

func (u *DefaultUserInfo) HomeDir() (string, error) {
	return os.UserHomeDir()
}

func (u *DefaultUserInfo) ConstructDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	constructDir := filepath.Join(homeDir, ".construct")
	if err := u.fs.MkdirAll(constructDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create construct directory: %w", err)
	}

	return constructDir, nil
}

func getCommandRunner(ctx context.Context) CommandRunner {
	runner := ctx.Value(ContextKeyCommandRunner)
	if runner != nil {
		return runner.(CommandRunner)
	}

	return &DefaultCommandRunner{}
}

func getRuntimeInfo(ctx context.Context) RuntimeInfo {
	runtimeInfo := ctx.Value(ContextKeyRuntimeInfo)
	if runtimeInfo != nil {
		return runtimeInfo.(RuntimeInfo)
	}

	return &DefaultRuntimeInfo{}
}

func getUserInfo(ctx context.Context) UserInfo {
	userInfo := ctx.Value(ContextKeyUserInfo)
	if userInfo != nil {
		return userInfo.(UserInfo)
	}

	return NewDefaultUserInfo(getFileSystem(ctx))
}

func getRenderer(ctx context.Context) OutputRenderer {
	printer := ctx.Value(ContextKeyOutputRenderer)
	if printer != nil {
		return printer.(OutputRenderer)
	}

	return &DefaultRenderer{}
}

func setGlobalOptions(ctx context.Context, options *globalOptions) context.Context {
	return context.WithValue(ctx, ContextKeyGlobalOptions, options)
}

func getGlobalOptions(ctx context.Context) *globalOptions {
	if opts, ok := ctx.Value(ContextKeyGlobalOptions).(*globalOptions); ok {
		return opts
	}
	return &globalOptions{}
}
