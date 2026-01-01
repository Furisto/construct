package cmd

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

func NewDaemonTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage authentication tokens",
		Long: `Manage authentication tokens for remote daemon access.

Tokens provide secure authentication for connecting to remote Construct daemons
over HTTPS. Each token has a name, optional description, and expiration time.

Token management requires admin privileges (local Unix socket connection).`,
	}

	cmd.AddCommand(NewDaemonTokenCreateCmd())
	cmd.AddCommand(NewDaemonTokenCreateSetupCmd())
	cmd.AddCommand(NewDaemonTokenListCmd())
	cmd.AddCommand(NewDaemonTokenRevokeCmd())

	return cmd
}

type TokenDisplay struct {
	ID      string `json:"id" detail:"default"`
	Name    string `json:"name" detail:"default"`
	Created string `json:"created" detail:"default"`
	Expires string `json:"expires" detail:"default"`
	Status  string `json:"status" detail:"default"`
}

func ConvertTokenInfoToDisplay(token *v1.TokenInfo) *TokenDisplay {
	status := "Active"
	if !token.IsActive {
		status = "Expired"
	}

	return &TokenDisplay{
		ID:      token.Id,
		Name:    token.Name,
		Created: FormatRelativeTime(token.CreatedAt.AsTime()),
		Expires: FormatRelativeTime(token.ExpiresAt.AsTime()),
		Status:  status,
	}
}

type TokenCreateDisplay struct {
	Name      string `json:"name" yaml:"name"`
	Token     string `json:"token" yaml:"token"`
	ExpiresAt string `json:"expires_at" yaml:"expires_at"`
}

type SetupCodeDisplay struct {
	TokenName string `json:"token_name" yaml:"token_name"`
	SetupCode string `json:"setup_code" yaml:"setup_code"`
	ExpiresAt string `json:"expires_at" yaml:"expires_at"`
}
