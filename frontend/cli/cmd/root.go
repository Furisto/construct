package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/common-nighthawk/go-figure"
	"github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
	"gopkg.in/natefinch/lumberjack.v2"
	_ "modernc.org/sqlite"

	api "github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/shared"
	"github.com/furisto/construct/shared/config"
	"github.com/furisto/construct/shared/keyring"
)

var (
	// Version is the version of the CLI
	Version = "unknown"

	// Git Commit is the commit that the CLI was built from
	GitCommit = "unknown"

	// BuildDate is the date the CLI was built
	BuildDate = "unknown"
)

type globalOptions struct {
	LogLevel LogLevel
	Context  string
}

func NewRootCmd() *cobra.Command {
	options := globalOptions{}
	cmd := &cobra.Command{
		Use:   "construct",
		Short: "Construct: Build intelligent agents.",
		Long:  figure.NewColorFigure("construct", "standard", "blue", true).String(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			userInfo := getUserInfo(cmd.Context())

			options.LogLevel = resolveLogLevel(cmd, &options)
			slog.SetDefault(slog.New(slog.NewJSONHandler(setupLogSink(cmd.Context(), userInfo, cmd.OutOrStdout()), &slog.HandlerOptions{
				Level: options.LogLevel.SlogLevel(),
			})))
			cmd.SetContext(setGlobalOptions(cmd.Context(), &options))

			configStore, err := config.NewStore(getFileSystem(cmd.Context()), userInfo)
			if err != nil {
				return err
			}
			cmd.SetContext(setConfigStore(cmd.Context(), configStore))

			if requiresContext(cmd) {
				err := setAPIClient(cmd.Context(), cmd, options.Context)
				if err != nil {
					slog.Error("failed to set API client", "error", err)
					return err
				}
			}

			return nil
		},
	}

	cmd.PersistentFlags().Var(&options.LogLevel, "log-level", "set the log level")
	cmd.PersistentFlags().StringVar(&options.Context, "context", "", "context to use (overrides current context)")

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
	cmd.AddCommand(NewExecCmd())

	cmd.AddCommand(NewAgentCmd())
	cmd.AddCommand(NewTaskCmd())
	cmd.AddCommand(NewMessageCmd())
	cmd.AddCommand(NewModelCmd())
	cmd.AddCommand(NewModelProviderCmd())
	cmd.AddCommand(NewSkillCmd())

	cmd.AddCommand(NewContextCmd())
	cmd.AddCommand(NewConfigCmd())
	cmd.AddCommand(NewDaemonCmd())
	cmd.AddCommand(NewInfoCmd())
	cmd.AddCommand(NewUpdateCmd())
	return cmd
}

func Execute() {
	defer func() {
		if r := recover(); r != nil {
			sentry.CurrentHub().Recover(r)
			sentry.Flush(2 * time.Second)
			fmt.Fprintf(os.Stderr, "Panic occurred: %v\n", r)
			fmt.Fprintf(os.Stderr, "Stack:\n%s\n", debug.Stack())
			os.Exit(1)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://03f4bdd9c27c4f234971bebd7318b4ff@o4509509926387712.ingest.de.sentry.io/4509509931434064",
	})
	if err != nil {
		fmt.Printf("failed to initialize sentry: %s\n", err)
	}

	rootCmd := NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		sentry.CaptureException(err)
		sentry.Flush(2 * time.Second)
		os.Exit(1)
	}

	sentry.Flush(2 * time.Second)
}

func setAPIClient(ctx context.Context, cmd *cobra.Command, contextOverride string) error {
	if getAPIClient(ctx) != nil {
		return nil
	}

	contextManager := shared.NewContextManager(getFileSystem(cmd.Context()), getUserInfo(cmd.Context()))
	endpointContexts, err := contextManager.LoadContext()
	if err != nil {
		return err
	}

	if err := endpointContexts.Validate(); err != nil {
		return err
	}

	contextName := resolveContextName(contextOverride, endpointContexts.CurrentContext)
	if contextName == "" {
		return fmt.Errorf("no context configured\n\nTo get started:\n  • Run 'construct daemon run' to start the daemon (first-time setup)\n  • Or run 'construct context add' to configure a remote context")
	}

	endpointContext, ok := endpointContexts.Contexts[contextName]
	if !ok {
		return fmt.Errorf("context %q not found", contextName)
	}

	clientOptions, err := buildClientOptions(endpointContext, contextManager)
	if err != nil {
		return fmt.Errorf("failed to configure client: %w", err)
	}

	apiClient, err := api.NewClient(endpointContext, clientOptions...)
	if err != nil {
		return fmt.Errorf("failed to create api client: %w", err)
	}
	cmd.SetContext(context.WithValue(cmd.Context(), ContextKeyAPIClient, apiClient))
	cmd.SetContext(context.WithValue(cmd.Context(), ContextKeyEndpointContext, endpointContext))

	return nil
}

func resolveContextName(flagValue, configValue string) string {
	if flagValue != "" {
		return flagValue
	}

	if envValue := os.Getenv("CONSTRUCT_CONTEXT"); envValue != "" {
		return envValue
	}

	return configValue
}

func buildClientOptions(endpointContext api.EndpointContext, contextManager *shared.ContextManager) ([]api.ClientOption, error) {
	var options []api.ClientOption

	if endpointContext.Auth != nil && endpointContext.Auth.IsConfigured() {
		token, err := resolveToken(endpointContext.Auth, contextManager)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve auth token: %w", err)
		}
		options = append(options, api.WithAuthToken(token))
	}

	return options, nil
}

func resolveToken(auth *api.AuthConfig, contextManager *shared.ContextManager) (string, error) {
	if auth.Token != "" {
		return auth.Token, nil
	}

	if auth.TokenRef != "" {
		keyringKey := auth.KeyringKey()
		if keyringKey == "" {
			return "", fmt.Errorf("invalid token-ref format")
		}

		token, err := contextManager.RetrieveToken(keyringKey)
		if err != nil {
			if errors.Is(err, &keyring.ErrSecretNotFound{}) {
				return "", fmt.Errorf("token not found in keyring for context %q - try re-authenticating with 'construct context add'", keyringKey)
			}
			return "", err
		}
		return token, nil
	}

	return "", fmt.Errorf("no token configured")
}

func requiresContext(cmd *cobra.Command) bool {
	skipCommands := []string{"info", "help", "update", "daemon.", "config.", "context."}
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

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

func (e *LogLevel) String() string {
	if e == nil {
		return ""
	}
	return string(*e)
}

func (e *LogLevel) Set(v string) error {
	for _, level := range []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError} {
		if v == string(level) {
			*e = level
			return nil
		}
	}
	return errors.New(`must be one of "debug", "info", "warn", or "error"`)
}

func (e *LogLevel) Type() string {
	return "log-level"
}

func (e *LogLevel) SlogLevel() slog.Level {
	switch *e {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelInfo:
		return slog.LevelInfo
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	}

	return slog.LevelInfo
}

func resolveLogLevel(cmd *cobra.Command, options *globalOptions) LogLevel {
	if cmd.Flags().Changed("log-level") {
		return options.LogLevel
	}

	logLevel := os.Getenv("CONSTRUCT_LOG_LEVEL")
	if logLevel != "" {
		switch logLevel {
		case "debug":
			return LogLevelDebug
		case "info":
			return LogLevelInfo
		case "warn":
			return LogLevelWarn
		case "error":
			return LogLevelError
		}
	}
	return LogLevelInfo
}

func setupLogSink(ctx context.Context, userInfo shared.UserInfo, stdout io.Writer) io.Writer {
	if disable, ok := ctx.Value(ContextKeyDisableFileLogs).(bool); ok && disable {
		return stdout
	}

	dataDir, err := userInfo.ConstructLogDir()
	if err != nil {
		return stdout
	}

	fileLogger := &lumberjack.Logger{
		Filename:   filepath.Join(dataDir, "construct.json"),
		MaxSize:    50,
		MaxAge:     7,
		MaxBackups: 3,
		Compress:   true,
	}
	return io.MultiWriter(stdout, fileLogger)
}
