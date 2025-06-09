package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update the CLI to the latest version",
		GroupID: "system",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Updating CLI...")
		},
	}
}
