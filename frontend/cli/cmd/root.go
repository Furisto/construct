package cmd

import (
	"context"

	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"entgo.io/ent/dialect"
	"github.com/furisto/construct/backend/agent"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"

	api "github.com/furisto/construct/api/go/client"
)

var globalOptions struct {
	Verbose bool
}

var rootCmd = &cobra.Command{
	Use: "construct",
	PreRun: func(cmd *cobra.Command, args []string) {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	},
}

func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		slog.Error("failed to execute command", "error", err)
		os.Exit(1)
	}
}

func RunAgent(ctx context.Context) error {
	client, err := memory.Open(dialect.SQLite, "file:./construct.db?_fk=1&_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Schema.Create(ctx); err != nil {
		return err
	}

	handle, err := secret.GenerateKeyset()
	if err != nil {
		return err
	}

	encryption, err := secret.NewClient(handle)
	if err != nil {
		return err
	}

	runtime := agent.NewRuntime(
		client,
		encryption,
		agent.WithServerPort(29333),
	)

	return runtime.Run(ctx)
}

func getClient() *api.Client {
	return api.NewClient("http://localhost:29333/api")
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&globalOptions.Verbose, "verbose", "v", false, "verbose output")
}
