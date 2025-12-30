package shared

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/adrg/xdg"
	api "github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/shared/keyring"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

type ContextManager struct {
	fs              *afero.Afero
	userInfo        UserInfo
	keyringProvider keyring.Provider
}

func NewContextManager(fs *afero.Afero, userInfo UserInfo) *ContextManager {
	return &ContextManager{
		fs:              fs,
		userInfo:        userInfo,
		keyringProvider: keyring.NewKeyringProvider(),
	}
}

func NewContextManagerWithKeyring(fs *afero.Afero, userInfo UserInfo, keyringProvider keyring.Provider) *ContextManager {
	return &ContextManager{
		fs:              fs,
		userInfo:        userInfo,
		keyringProvider: keyringProvider,
	}
}

func (m *ContextManager) LoadContext() (*api.EndpointContexts, error) {
	constructDir, err := m.userInfo.ConstructConfigDir()
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

func (m *ContextManager) GetContext(contextName string) (*api.EndpointContext, error) {
	endpointContexts, err := m.LoadContext()
	if err != nil {
		return nil, err
	}

	ctx, ok := endpointContexts.Contexts[contextName]
	if !ok {
		return nil, fmt.Errorf("context %q not found", contextName)
	}

	return &ctx, nil
}

func (m *ContextManager) ListContexts() ([]string, error) {
	endpointContexts, err := m.LoadContext()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(endpointContexts.Contexts))
	for name := range endpointContexts.Contexts {
		names = append(names, name)
	}

	return names, nil
}

func (m *ContextManager) UpsertContext(contextName string, kind string, address string, setCurrent bool, auth *api.AuthConfig) (bool, error) {
	endpointContexts, err := m.LoadContext()
	if err != nil {
		return false, err
	}

	endpointContext := api.EndpointContext{
		Address: address,
		Kind:    kind,
		Auth:    auth,
	}

	if err := endpointContext.Validate(); err != nil {
		return false, err
	}

	_, exists := endpointContexts.Contexts[contextName]
	endpointContexts.Contexts[contextName] = endpointContext

	if setCurrent {
		err = endpointContexts.SetCurrent(contextName)
		if err != nil {
			return false, err
		}
	}

	return exists, m.saveContext(endpointContexts)
}

func (m *ContextManager) DeleteContext(contextName string) error {
	endpointContexts, err := m.LoadContext()
	if err != nil {
		return err
	}

	ctx, ok := endpointContexts.Contexts[contextName]
	if !ok {
		return fmt.Errorf("context %q not found", contextName)
	}

	if ctx.Auth != nil && ctx.Auth.KeyringKey() != "" {
		if err := m.keyringProvider.Delete(ctx.Auth.KeyringKey()); err != nil {
			if _, ok := err.(*keyring.ErrSecretNotFound); !ok {
				return fmt.Errorf("failed to delete token from keyring: %w", err)
			}
		}
	}

	delete(endpointContexts.Contexts, contextName)

	if endpointContexts.CurrentContext == contextName {
		endpointContexts.CurrentContext = ""
	}

	return m.saveContext(endpointContexts)
}

func (m *ContextManager) SetCurrentContext(contextName string) error {
	endpointContexts, err := m.LoadContext()
	if err != nil {
		return err
	}

	previousContext := endpointContexts.CurrentContext

	err = endpointContexts.SetCurrent(contextName)
	if err != nil {
		return err
	}

	if previousContext != "" {
		endpointContexts.SetPrevious(previousContext)
	}

	return m.saveContext(endpointContexts)
}

func (m *ContextManager) StoreToken(contextName string, token string) error {
	return m.keyringProvider.Set(contextName, token)
}

func (m *ContextManager) RetrieveToken(contextName string) (string, error) {
	return m.keyringProvider.Get(contextName)
}

func (m *ContextManager) DeleteToken(contextName string) error {
	return m.keyringProvider.Delete(contextName)
}

func (m *ContextManager) saveContext(endpointContexts *api.EndpointContexts) error {
	constructDir, err := m.userInfo.ConstructConfigDir()
	if err != nil {
		return err
	}

	content, err := yaml.Marshal(endpointContexts)
	if err != nil {
		return err
	}

	endpointContextsFile := filepath.Join(constructDir, "context.yaml")
	return m.fs.WriteFile(endpointContextsFile, content, 0600)
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
	UserID() (string, error)
	HomeDir() (string, error)
	ConstructConfigDir() (string, error)
	ConstructDataDir() (string, error)
	ConstructLogDir() (string, error)
	ConstructRuntimeDir() (string, error)
	Cwd() (string, error)
	IsRoot() (bool, error)
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

func (u *DefaultUserInfo) UserID() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	return user.Uid, nil
}

func (u *DefaultUserInfo) HomeDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return homeDir, nil
}

func (u *DefaultUserInfo) ConstructConfigDir() (string, error) {
	configDir := filepath.Join(xdg.ConfigHome, "construct")
	if err := u.fs.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}
	return configDir, nil
}

func (u *DefaultUserInfo) ConstructDataDir() (string, error) {
	dataDir := filepath.Join(xdg.DataHome, "construct")
	if err := u.fs.MkdirAll(dataDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create data directory: %w", err)
	}
	return dataDir, nil
}

func (u *DefaultUserInfo) ConstructLogDir() (string, error) {
	var logDir string
	switch runtime.GOOS {
	case "darwin":
		homeDir, err := u.HomeDir()
		if err != nil {
			return "", err
		}
		logDir = filepath.Join(homeDir, "Library", "Logs", "construct")
	default:
		logDir = filepath.Join(xdg.StateHome, "construct")
	}

	if err := u.fs.MkdirAll(logDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create log directory: %w", err)
	}
	return logDir, nil
}

func (u *DefaultUserInfo) ConstructRuntimeDir() (string, error) {
	var runtimeDir string
	if runtime.GOOS == "darwin" {
		runtimeDir = filepath.Join("/tmp", "construct")
	} else {
		runtimeDir = filepath.Join(xdg.RuntimeDir, "construct")
	}

	if err := u.fs.MkdirAll(runtimeDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create runtime directory: %w", err)
	}
	return runtimeDir, nil
}

func (u *DefaultUserInfo) Cwd() (string, error) {
	return os.Getwd()
}

func (u *DefaultUserInfo) IsRoot() (bool, error) {
	userID, err := u.UserID()
	if err != nil {
		return false, err
	}
	return userID == "0", nil
}

var _ UserInfo = (*DefaultUserInfo)(nil)
