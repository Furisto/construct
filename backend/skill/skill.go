package skill

import (
	"path/filepath"

	"github.com/furisto/construct/shared"
)

type Skill struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Location    string     `yaml:"-"`
	Scope       SkillScope `yaml:"-"`
}

type SkillScope string

const (
	SkillScopeRepo   SkillScope = "repo"
	SkillScopeUser   SkillScope = "user"
	SkillScopeSystem SkillScope = "system"
)

type DiscoveryPath struct {
	Pattern    string
	Scope      SkillScope
	IsRelative bool
}

func DefaultDiscoveryPaths(userInfo shared.UserInfo) []DiscoveryPath {
	configDir, err := userInfo.ConstructConfigDir()
	if err != nil {
		return []DiscoveryPath{}
	}

	return []DiscoveryPath{
		{Pattern: ".construct/skills", Scope: SkillScopeRepo, IsRelative: true},
		{Pattern: ".codex/skills", Scope: SkillScopeRepo, IsRelative: true},
		{Pattern: ".claude/skills", Scope: SkillScopeRepo, IsRelative: true},
		{Pattern: ".github/skills", Scope: SkillScopeRepo, IsRelative: true},

		{Pattern: filepath.Join(configDir, "skills"), Scope: SkillScopeUser, IsRelative: false},
		{Pattern: "~/.config/construct/skills", Scope: SkillScopeUser, IsRelative: false},
		{Pattern: "~/.codex/skills", Scope: SkillScopeUser, IsRelative: false},
		{Pattern: "~/.claude/skills", Scope: SkillScopeUser, IsRelative: false},

		{Pattern: "/etc/construct/skills", Scope: SkillScopeSystem, IsRelative: false},
	}
}

const (
	SkillFileName        = "SKILL.md"
	MaxNameLength        = 64
	MaxDescriptionLength = 1024
	MinDescriptionLength = 1
)
