package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/spf13/cobra"
)

type skillListOptions struct {
	RenderOptions RenderOptions
}

func NewSkillListCmd() *cobra.Command {
	options := &skillListOptions{}

	cmd := &cobra.Command{
		Use:     "list [flags]",
		Short:   "List all installed skills",
		Aliases: []string{"ls"},
		Example: `  # List all installed skills
  construct skill list

  # List skills in JSON format
  construct skill ls -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			resp, err := client.Skill().ListSkills(cmd.Context(), &connect.Request[v1.ListSkillsRequest]{
				Msg: &v1.ListSkillsRequest{},
			})
			if err != nil {
				return fail.HandleError(cmd, err)
			}

			displays := make([]*SkillDisplay, len(resp.Msg.Skills))
			for i, skill := range resp.Msg.Skills {
				displays[i] = ConvertSkillToDisplay(skill)
			}

			return getRenderer(cmd.Context()).Render(displays, &options.RenderOptions)
		},
	}

	addRenderOptions(cmd, &options.RenderOptions)

	return cmd
}
