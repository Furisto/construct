package cmd

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"

	"entgo.io/ent/dialect"
	"github.com/furisto/construct/backend/agent"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/migrate"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/furisto/construct/shared"
	"github.com/furisto/construct/shared/config"
	"github.com/furisto/construct/shared/listener"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/tink-crypto/tink-go/keyset"
)

type daemonRunOptions struct {
	HTTPAddress string
	UnixSocket  string
}

func NewDaemonRunCmd() *cobra.Command {
	options := daemonRunOptions{}
	cmd := &cobra.Command{
		Use:   "run [flags]",
		Short: "Run the daemon process in the foreground",
		Long: `Run the daemon process in the foreground.

Starts the daemon process directly in the current terminal. This is useful for 
debugging and development. For normal use, 'construct daemon install' is recommended.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			userInfo := getUserInfo(cmd.Context())
			config := getConfigStore(cmd.Context())
			fs := getFileSystem(cmd.Context())

			dataDir, err := userInfo.ConstructDataDir()
			if err != nil {
				return fmt.Errorf("failed to get construct data directory: %w", err)
			}

			db, err := memory.Open(dialect.SQLite, "file:"+filepath.Join(dataDir, "construct.db")+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			err = setupMemory(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("failed to setup memory/database schema: %w", err)
			}

			secretProvider, err := getSecretProvider(config, userInfo, fs)
			if err != nil {
				return fmt.Errorf("failed to get secret provider: %w", err)
			}

			encryption, err := getEncryptionClient(secretProvider)
			if err != nil {
				return fmt.Errorf("failed to get encryption client: %w", err)
			}

			provider, err := listener.DetectProvider(options.HTTPAddress, options.UnixSocket)
			if err != nil {
				return fmt.Errorf("failed to detect listener provider: %w", err)
			}

			listener, err := provider.Create()
			if err != nil {
				return fmt.Errorf("failed to create listener: %w", err)
			}

			if explicitLaunch(provider.ActivationType()) {
				contextManager := shared.NewContextManager(getFileSystem(cmd.Context()), getUserInfo(cmd.Context()))
				contextName := generateContextName(provider.ActivationType(), listener)
				_, err = contextManager.UpsertContext(contextName, provider.ActivationType(), listener.Addr().String(), true)
				if err != nil {
					return fmt.Errorf("failed to upsert context: %w", err)
				}
			}

			var analyticsClient analytics.Client
			analyticsClient, err = analytics.NewPostHogClient()
			if err != nil {
				slog.Error("failed to create analytics client", "error", err)
				analyticsClient = analytics.NewNoopClient()
			}

			runtime, err := agent.NewRuntime(
				db,
				encryption,
				listener,
				agent.WithCodeActTools(
					codeact.NewCreateFileTool(),
					codeact.NewReadFileTool(),
					codeact.NewEditFileTool(),
					codeact.NewListFilesTool(),
					codeact.NewGrepTool(),
					codeact.NewFindFileTool(),
					codeact.NewExecuteCommandTool(),
					// codeact.NewSubmitReportTool(),
					codeact.NewPrintTool(),
				),
				agent.WithAnalytics(analyticsClient),
			)

			if err != nil {
				return fmt.Errorf("failed to create agent runtime: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "🤖 Starting Agent Runtime...\n")
			err = runtime.Run(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to run agent runtime: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&options.HTTPAddress, "listen-http", "", "The address and port to listen on (e.g., 127.0.0.1:8080)")
	cmd.Flags().StringVar(&options.UnixSocket, "listen-unix", "", "The path to listen on for Unix socket requests")

	return cmd
}

func explicitLaunch(kind string) bool {
	return kind == "unix" || kind == "tcp"
}

func generateContextName(kind string, listener net.Listener) string {
	hash := sha256.Sum256([]byte(listener.Addr().String()))
	return fmt.Sprintf("%s-%x", kind, hash[:3])
}

func getEncryptionClient(secretProvider secret.Provider) (*secret.Encryption, error) {
	var keyHandle *keyset.Handle
	keyHandleJson, err := secretProvider.Get(secret.EncryptionKeySecret())
	if err != nil {
		if !errors.Is(err, &secret.ErrSecretNotFound{}) {
			return nil, fmt.Errorf("failed to get encryption key secret: %w", err)
		}

		slog.Debug("generating new encryption key")
		keyHandle, err = secret.GenerateKeyset()
		if err != nil {
			return nil, fmt.Errorf("failed to generate encryption keyset: %w", err)
		}
		keysetJson, err := secret.KeysetToJSON(keyHandle)
		if err != nil {
			return nil, fmt.Errorf("failed to convert keyset to JSON: %w", err)
		}

		err = secretProvider.Set(secret.EncryptionKeySecret(), keysetJson)
		if err != nil {
			return nil, fmt.Errorf("failed to store encryption key secret: %w", err)
		}
	} else {
		slog.Debug("loading encryption key")
		keyHandle, err = secret.KeysetFromJSON(keyHandleJson)
		if err != nil {
			return nil, fmt.Errorf("failed to load encryption keyset from JSON: %w", err)
		}
	}

	return secret.NewClient(keyHandle)
}

func getSecretProvider(cfg *config.Store, userInfo shared.UserInfo, fs afero.Fs) (secret.Provider, error) {
	provider, _ := cfg.Get("secret.provider")
	value, _ := provider.String()

	if value == "file" {
		dataDir, err := userInfo.ConstructDataDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get construct data directory: %w", err)
		}
		return secret.NewFileProvider(filepath.Join(dataDir, "secrets"), fs)
	}

	return secret.NewKeyringProvider(), nil
}

func setupMemory(ctx context.Context, db *memory.Client) error {
	return db.Schema.Create(ctx,
		migrate.WithDropColumn(true),
		migrate.WithDropIndex(true),
	)
}
