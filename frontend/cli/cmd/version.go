package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of Construct",
		GroupID: "system",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Construct version 0.1.0")
		},
	}

	return cmd
}
