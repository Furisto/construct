package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/spf13/cobra"
)

type skillDeleteOptions struct {
	Force bool
}

func NewSkillDeleteCmd() *cobra.Command {
	options := &skillDeleteOptions{}

	cmd := &cobra.Command{
		Use:     "delete <name> [flags]",
		Short:   "Delete an installed skill",
		Aliases: []string{"rm"},
		Example: `  # Delete a skill
  construct skill delete commit

  # Force delete without confirmation
  construct skill rm review-pr --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillName := args[0]

			if !options.Force && !confirmDeletion(cmd.InOrStdin(), cmd.OutOrStdout(), "skill", []string{skillName}) {
				return nil
			}

			client := getAPIClient(cmd.Context())

			_, err := client.Skill().DeleteSkill(cmd.Context(), &connect.Request[v1.DeleteSkillRequest]{
				Msg: &v1.DeleteSkillRequest{
					Name: skillName,
				},
			})
			if err != nil {
				return fail.HandleError(cmd, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Skill %q deleted\n", skillName)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Skip the confirmation prompt")

	return cmd
}
