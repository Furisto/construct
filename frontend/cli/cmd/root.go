package cmd

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/common-nighthawk/go-figure"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/furisto/construct/api/go/client"
	api "github.com/furisto/construct/api/go/client"
)

var globalOptions struct {
	Verbose bool
}

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "construct",
		Short: "Construct: Build intelligent agents.",
		Long:  figure.NewColorFigure("construct", "standard", "blue", true).String(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})))

			if requiresContext(cmd) {
				err := setAPIClient(cmd)
				if err != nil {
					slog.Error("failed to set API client", "error", err)
					return err
				}
			}

			return nil
		},
	}

	cmd.PersistentFlags().BoolVarP(&globalOptions.Verbose, "verbose", "v", false, "verbose output")

	cmd.AddGroup(
		&cobra.Group{
			ID:    "core",
			Title: "Core Commands",
		},
	)

	cmd.AddGroup(
		&cobra.Group{
			ID:    "resource",
			Title: "Resource Management",
		},
	)

	cmd.AddGroup(
		&cobra.Group{
			ID:    "system",
			Title: "System Commands",
		},
	)

	cmd.AddCommand(NewNewCmd())
	cmd.AddCommand(NewResumeCmd())

	cmd.AddCommand(NewAgentCmd())
	cmd.AddCommand(NewTaskCmd())
	cmd.AddCommand(NewMessageCmd())
	cmd.AddCommand(NewModelCmd())
	cmd.AddCommand(NewModelProviderCmd())

	cmd.AddCommand(NewConfigCmd())
	cmd.AddCommand(NewDaemonCmd())
	cmd.AddCommand(NewVersionCmd())
	cmd.AddCommand(NewUpdateCmd())
	return cmd
}

func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rootCmd := NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

type Config struct {
	EndpointContexts client.EndpointContexts `json:"endpoint_contexts"`
}

func setAPIClient(cmd *cobra.Command) error {
	endpointContext, err := loadContext(cmd)
	if err != nil {
		return err
	}

	if endpointContext.Current == "" {
		return fmt.Errorf("no current context found. please run `construct context set` to set a current context")
	}

	apiClient := api.NewClient(endpointContext.Contexts[endpointContext.Current])
	cmd.SetContext(context.WithValue(cmd.Context(), ContextKeyAPIClient, apiClient))

	return nil
}

func requiresContext(cmd *cobra.Command) bool {
	skipCommands := []string{"version", "help", "update", "daemon.", "config."}
	for _, skipCmd := range skipCommands {
		cmdName := cmd.Name()
		parentCmd := cmd.Parent()
		if parentCmd != nil {
			cmdName = parentCmd.Name() + "." + cmdName
		}

		if strings.HasPrefix(cmdName, skipCmd) {
			return false
		}
	}

	return true
}

func loadContext(cmd *cobra.Command) (*client.EndpointContexts, error) {
	fs := getFileSystem(cmd.Context())

	constructDir, err := ConstructUserDir()
	if err != nil {
		return nil, err
	}

	endpointContextsFile := filepath.Join(constructDir, "context.yaml")
	exists, err := fs.Exists(endpointContextsFile)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, fmt.Errorf("construct context not found. please run `construct daemon install` to create a new context")
	}

	content, err := fs.ReadFile(endpointContextsFile)
	if err != nil {
		return nil, err
	}

	var endpointContexts client.EndpointContexts
	err = yaml.Unmarshal(content, &endpointContexts)
	if err != nil {
		return nil, err
	}

	return &endpointContexts, nil
}

type ContextKey string

const (
	ContextKeyAPIClient       ContextKey = "api_client"
	ContextKeyFileSystem      ContextKey = "filesystem"
	ContextKeyOutputRenderer  ContextKey = "output_renderer"
	ContextKeyCommandRunner   ContextKey = "command_runner"
	ContextKeyEndpointContext ContextKey = "endpoint_context"
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

type CommandRunner interface {
	Run(ctx context.Context, command string, args ...string) (string, error)
}

type DefaultCommandRunner struct{}

func (r *DefaultCommandRunner) Run(ctx context.Context, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func getCommandRunner(ctx context.Context) CommandRunner {
	runner := ctx.Value(ContextKeyCommandRunner)
	if runner != nil {
		return runner.(CommandRunner)
	}

	return &DefaultCommandRunner{}
}

func getRenderer(ctx context.Context) OutputRenderer {
	printer := ctx.Value(ContextKeyOutputRenderer)
	if printer != nil {
		return printer.(OutputRenderer)
	}

	return &DefaultRenderer{}
}

func confirmDeletion(stdin io.Reader, stdout io.Writer, kind string, idOrNames []string) bool {
	if len(idOrNames) == 0 {
		return false
	}

	if len(idOrNames) > 1 {
		kind = kind + "s"
	}

	message := fmt.Sprintf("Are you sure you want to delete %s %s?", kind, strings.Join(idOrNames, " "))
	return confirm(stdin, stdout, message)
}

func confirm(stdin io.Reader, stdout io.Writer, message string) bool {
	fmt.Fprintf(stdout, "%s (y/n): ", message)
	var confirm string
	_, err := fmt.Fscan(stdin, &confirm)
	if err != nil {
		return false
	}

	confirm = strings.TrimSpace(strings.ToLower(confirm))
	return confirm == "y" || confirm == "yes"
}

func ConstructUserDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".construct"), nil
}
