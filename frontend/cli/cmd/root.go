package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"entgo.io/ent/dialect"
	"github.com/common-nighthawk/go-figure"
	"github.com/furisto/construct/backend/agent"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/tool"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/tink-crypto/tink-go/keyset"

	api "github.com/furisto/construct/api/go/client"
)

var globalOptions struct {
	Verbose bool
}

var rootCmd = &cobra.Command{
	Use:   "construct",
	Short: "Construct: Build intelligent agents.",
	Long:  figure.NewColorFigure("construct", "standard", "blue", true).String(),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	},
}

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "construct",
		Short: "Construct: Build intelligent agents.",
		Long:  figure.NewColorFigure("construct", "standard", "blue", true).String(),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})))
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

	cmd.AddCommand(NewAgentCmd())
	cmd.AddCommand(NewTaskCmd())
	cmd.AddCommand(NewMessageCmd())
	cmd.AddCommand(NewModelCmd())
	cmd.AddCommand(NewModelProviderCmd())

	cmd.AddCommand(NewNewCmd())
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

func RunAgent(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	client, err := memory.Open(dialect.SQLite, "file:"+homeDir+"/.construct/construct.db?_fk=1&_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Schema.Create(ctx); err != nil {
		return err
	}

	encryption, err := getEncryptionClient()
	if err != nil {
		return err
	}

	runtime, err := agent.NewRuntime(
		client,
		encryption,
		agent.WithServerPort(29333),
		agent.WithCodeActTools(
			tool.NewCreateFileTool(),
			tool.NewReadFileTool(),
			tool.NewEditFileTool(),
			tool.NewListFilesTool(),
			tool.NewGrepTool(),
			tool.NewExecuteCommandTool(),
			tool.NewPrintTool(),
		),
	)

	if err != nil {
		return err
	}

	return runtime.Run(ctx)
}

type ContextKey string

const (
	ContextKeyAPI        ContextKey = "api"
	ContextKeyFileSystem ContextKey = "filesystem"
	ContextKeyFormatter  ContextKey = "formatter"
)

func getAPIClient(ctx context.Context) *api.Client {
	apiTestClient := ctx.Value(ContextKeyAPI)
	if apiTestClient != nil {
		return apiTestClient.(*api.Client)
	}

	return api.NewClient("http://localhost:29333/api")
}

func getFileSystem(ctx context.Context) *afero.Afero {
	fs := ctx.Value(ContextKeyFileSystem)
	if fs != nil {
		return fs.(*afero.Afero)
	}

	return &afero.Afero{Fs: afero.NewOsFs()}
}

func getEncryptionClient() (*secret.Client, error) {
	var keyHandle *keyset.Handle
	keyHandleJson, err := secret.GetSecret[string](secret.ModelProviderEncryptionKey())
	if err != nil {
		if !errors.Is(err, &secret.ErrSecretNotFound{}) {
			return nil, err
		}

		slog.Debug("generating new encryption key")
		keyHandle, err = secret.GenerateKeyset()
		if err != nil {
			return nil, err
		}
		keysetJson, err := secret.KeysetToJSON(keyHandle)
		if err != nil {
			return nil, err
		}

		err = secret.SetSecret(secret.ModelProviderEncryptionKey(), &keysetJson)
		if err != nil {
			return nil, err
		}
	} else {
		slog.Debug("loading encryption key")
		keyHandle, err = secret.KeysetFromJSON(*keyHandleJson)
		if err != nil {
			return nil, err
		}
	}

	return secret.NewClient(keyHandle)
}

func getFormatter(ctx context.Context) ResourceFormatter {
	printer := ctx.Value(ContextKeyFormatter)
	if printer != nil {
		return printer.(ResourceFormatter)
	}

	return &DefaultResourceFormatter{}
}

func confirmDeletion(stdin io.Reader, stdout io.Writer, kind string, idOrNames []string) bool {
	if len(idOrNames) > 1 {
		kind = kind + "s"
	}
	fmt.Fprintf(stdout, "Are you sure you want to delete %s %s? (y/n): ", kind, strings.Join(idOrNames, " "))
	var confirm string
	_, err := fmt.Fscan(stdin, &confirm)
	if err != nil {
		return false
	}
	return confirm == "y"
}
