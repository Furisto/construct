package cmd

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

func NewSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "skill",
		Short:   "Manage skills that extend agent capabilities",
		Aliases: []string{"skills"},
		GroupID: "resource",
	}

	cmd.AddCommand(NewSkillInstallCmd())
	cmd.AddCommand(NewSkillListCmd())
	cmd.AddCommand(NewSkillDeleteCmd())
	cmd.AddCommand(NewSkillUpdateCmd())

	return cmd
}

type SkillDisplay struct {
	Name        string `json:"name" yaml:"name" detail:"default"`
	Description string `json:"description" yaml:"description" detail:"default"`
	Source      string `json:"source" yaml:"source" detail:"default"`
	InstalledAt string `json:"installed_at" yaml:"installed_at" detail:"full"`
	UpdatedAt   string `json:"updated_at" yaml:"updated_at" detail:"full"`
}

func ConvertSkillToDisplay(skill *v1.Skill) *SkillDisplay {
	if skill == nil {
		return nil
	}

	source := "local"
	if skill.GetGit() != nil {
		switch skill.GetGit().Provider {
		case v1.GitProvider_GIT_PROVIDER_GITHUB:
			source = "github"
		case v1.GitProvider_GIT_PROVIDER_GITLAB:
			source = "gitlab"
		default:
			source = "git"
		}
	} else if skill.GetUrl() != nil {
		source = "url"
	}

	display := &SkillDisplay{
		Name:        skill.Name,
		Description: skill.Description,
		Source:      source,
	}

	if skill.InstalledAt != nil {
		display.InstalledAt = skill.InstalledAt.AsTime().Format("2006-01-02 15:04:05")
	}
	if skill.UpdatedAt != nil {
		display.UpdatedAt = skill.UpdatedAt.AsTime().Format("2006-01-02 15:04:05")
	}

	return display
}
