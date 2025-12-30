package cmd

import (
	"context"

	"github.com/furisto/construct/shared"
	"github.com/spf13/cobra"
)

func NewContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage connection contexts for local and remote daemons",
		Long: `Manage contexts to connect to different Construct daemon instances.

A context defines an endpoint (Unix socket or HTTP URL) and optional authentication.
Use contexts to switch between local development, staging, and production environments.

Examples:
  • Local daemon (Unix socket): unix:///home/user/.construct/construct.sock
  • Remote daemon (HTTP): https://construct.dev.internal:8443`,
		Aliases: []string{"ctx"},
		GroupID: "system",
	}

	cmd.AddCommand(NewContextListCmd())
	cmd.AddCommand(NewContextCurrentCmd())
	cmd.AddCommand(NewContextUseCmd())
	cmd.AddCommand(NewContextAddCmd())
	cmd.AddCommand(NewContextRemoveCmd())

	return cmd
}

type ContextDisplay struct {
	Name     string `json:"name" detail:"default"`
	Endpoint string `json:"endpoint" detail:"default"`
	Kind     string `json:"kind" detail:"default"`
	Auth     string `json:"auth" detail:"default"`
	Current  bool   `json:"current" detail:"default"`
}

func getContextManager(ctx context.Context) *shared.ContextManager {
	return shared.NewContextManager(getFileSystem(ctx), getUserInfo(ctx))
}
