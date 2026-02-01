package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/spf13/cobra"
)

type skillInstallOptions struct {
	Force         bool
	SkillNames    []string
	RenderOptions RenderOptions
}

func NewSkillInstallCmd() *cobra.Command {
	options := &skillInstallOptions{}

	cmd := &cobra.Command{
		Use:   "install <source> [flags]",
		Short: "Install skills from a remote source",
		Long: `Install skills from GitHub, GitLab, or a direct URL.

Supported source formats:
  owner/repo                              GitHub shorthand (installs from main branch)
  owner/repo/path                         GitHub shorthand with path
  github.com/owner/repo/tree/branch/path  Full GitHub URL
  gitlab.com/owner/repo/-/tree/branch/path  Full GitLab URL
  https://example.com/path/to/SKILL.md    Direct URL to SKILL.md file`,
		Example: `  # Install all skills from a GitHub repository
  construct skill install anthropics/skills

  # Install skills from a specific path in a repository
  construct skill install anthropics/skills/coding

  # Install only specific skills by name
  construct skill install anthropics/skills --skill commit --skill review-pr

  # Overwrite existing skills
  construct skill install anthropics/skills --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			resp, err := client.Skill().InstallSkill(cmd.Context(), &connect.Request[v1.InstallSkillRequest]{
				Msg: &v1.InstallSkillRequest{
					Source:     args[0],
					Force:      options.Force,
					SkillNames: options.SkillNames,
				},
			})
			if err != nil {
				return fail.HandleError(cmd, err)
			}

			if len(resp.Msg.InstalledSkills) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No skills installed")
				return nil
			}

			displays := make([]*SkillDisplay, len(resp.Msg.InstalledSkills))
			for i, skill := range resp.Msg.InstalledSkills {
				displays[i] = ConvertSkillToDisplay(skill)
			}

			return getRenderer(cmd.Context()).Render(displays, &options.RenderOptions)
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Overwrite existing skills")
	cmd.Flags().StringArrayVarP(&options.SkillNames, "skill", "s", []string{}, "Install only specific skill(s) by name")
	addRenderOptions(cmd, &options.RenderOptions)

	return cmd
}
