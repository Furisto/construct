package cmd

import (
	"github.com/spf13/cobra"
)

func NewDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "daemon",
		Short:   "Run the daemon",
		GroupID: "system",
	}

	cmd.AddCommand(NewDaemonRunCmd())
	cmd.AddCommand(NewDaemonInstallCmd())
	return cmd
}
