package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"log/slog"

	"github.com/common-nighthawk/go-figure"
	"github.com/getsentry/sentry-go"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"

	api "github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/shared"
	"github.com/furisto/construct/shared/config"
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
}

func NewRootCmd() *cobra.Command {
	options := globalOptions{}
	cmd := &cobra.Command{
		Use:   "construct",
		Short: "Construct: Build intelligent agents.",
		Long:  figure.NewColorFigure("construct", "standard", "blue", true).String(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			options.LogLevel = resolveLogLevel(cmd, &options)
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: options.LogLevel.SlogLevel(),
			})))

			cmd.SetContext(setGlobalOptions(cmd.Context(), &options))

			configStore, err := config.NewStore(getFileSystem(cmd.Context()), getUserInfo(cmd.Context()))
			if err != nil {
				return err
			}
			cmd.SetContext(setConfigStore(cmd.Context(), configStore))

			if requiresContext(cmd) {
				err := setAPIClient(cmd.Context(), cmd)
				if err != nil {
					slog.Error("failed to set API client", "error", err)
					return err
				}
			}

			return nil
		},
	}

	cmd.PersistentFlags().Var(&options.LogLevel, "log-level", "set the log level")

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

func setAPIClient(ctx context.Context, cmd *cobra.Command) error {
	if getAPIClient(ctx) != nil {
		return nil
	}

	endpointContexts, err := shared.NewContextManager(getFileSystem(cmd.Context()), getUserInfo(cmd.Context())).LoadContext()
	if err != nil {
		return err
	}

	if err := endpointContexts.Validate(); err != nil {
		return err
	}

	endpointContext, ok := endpointContexts.Current()
	if !ok {
		return fmt.Errorf("no current context found. please run `construct config context set` to set a current context")
	}

	apiClient := api.NewClient(endpointContext)
	cmd.SetContext(context.WithValue(cmd.Context(), ContextKeyAPIClient, apiClient))
	cmd.SetContext(context.WithValue(cmd.Context(), ContextKeyEndpointContext, endpointContext))

	return nil
}

func requiresContext(cmd *cobra.Command) bool {
	skipCommands := []string{"info", "help", "update", "daemon.", "config."}
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
