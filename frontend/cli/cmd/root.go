package cmd

import (
	"context"
	"errors"

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
	"github.com/tink-crypto/tink-go/keyset"

	api "github.com/furisto/construct/api/go/client"
)

var globalOptions struct {
	Verbose bool
}

var rootCmd = &cobra.Command{
	Use: "construct",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
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
	)

	if err != nil {
		return err
	}

	return runtime.Run(ctx)
}

func getAPIClient() *api.Client {
	return api.NewClient("http://localhost:29333/api")
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

func init() {
	rootCmd.PersistentFlags().BoolVarP(&globalOptions.Verbose, "verbose", "v", false, "verbose output")
}
