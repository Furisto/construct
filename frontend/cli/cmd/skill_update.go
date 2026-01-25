package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/spf13/cobra"
)

type skillUpdateOptions struct {
	RenderOptions RenderOptions
}

func NewSkillUpdateCmd() *cobra.Command {
	options := &skillUpdateOptions{}

	cmd := &cobra.Command{
		Use:   "update [name] [flags]",
		Short: "Update installed skills to their latest versions",
		Long: `Update skills that were installed from remote sources (GitHub, GitLab, or URLs).
Locally placed skills cannot be updated.

If no name is specified, all remotely-installed skills are updated.`,
		Example: `  # Update all remotely-installed skills
  construct skill update

  # Update a specific skill
  construct skill update commit`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			var name *string
			if len(args) > 0 {
				name = api.Ptr(args[0])
			}

			resp, err := client.Skill().UpdateSkill(cmd.Context(), &connect.Request[v1.UpdateSkillRequest]{
				Msg: &v1.UpdateSkillRequest{
					Name: name,
				},
			})
			if err != nil {
				return fail.HandleError(cmd, err)
			}

			if len(resp.Msg.UpdatedSkills) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No skills updated")
				return nil
			}

			displays := make([]*SkillDisplay, len(resp.Msg.UpdatedSkills))
			for i, skill := range resp.Msg.UpdatedSkills {
				displays[i] = ConvertSkillToDisplay(skill)
			}

			return getRenderer(cmd.Context()).Render(displays, &options.RenderOptions)
		},
	}

	addRenderOptions(cmd, &options.RenderOptions)

	return cmd
}
