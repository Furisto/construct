package skill

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/spf13/afero"
)

type expectation struct {
	skills []*Skill
	err    error
}

type scenario struct {
	cwd         string
	homeDir     string
	setup       func(fs afero.Fs)
	expectation expectation
}

func skillByName(a, b *Skill) bool {
	return a.Name < b.Name
}

func TestDiscoverer_Discover(t *testing.T) {
	validSkillContent := func(name, description string) []byte {
		return []byte("---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name)
	}

	tests := []struct {
		name     string
		scenario scenario
	}{
		{
			name: "no git root uses cwd",
			scenario: scenario{
				cwd:     "/project",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/project/.construct/skills/my-skill", 0755)
					afero.WriteFile(fs, "/project/.construct/skills/my-skill/SKILL.md", validSkillContent("my-skill", "A skill"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "my-skill", Description: "A skill", Location: "/project/.construct/skills/my-skill/SKILL.md", Scope: SkillScopeRepo},
					},
				},
			},
		},
		{
			name: "repo scope only",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
					fs.MkdirAll("/repo/.construct/skills/repo-skill", 0755)
					afero.WriteFile(fs, "/repo/.construct/skills/repo-skill/SKILL.md", validSkillContent("repo-skill", "Repo skill"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "repo-skill", Description: "Repo skill", Location: "/repo/.construct/skills/repo-skill/SKILL.md", Scope: SkillScopeRepo},
					},
				},
			},
		},
		{
			name: "user scope only",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
					fs.MkdirAll("/home/user/.config/construct/skills/user-skill", 0755)
					afero.WriteFile(fs, "/home/user/.config/construct/skills/user-skill/SKILL.md", validSkillContent("user-skill", "User skill"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "user-skill", Description: "User skill", Location: "/home/user/.config/construct/skills/user-skill/SKILL.md", Scope: SkillScopeUser},
					},
				},
			},
		},
		{
			name: "system scope only",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
					fs.MkdirAll("/etc/construct/skills/system-skill", 0755)
					afero.WriteFile(fs, "/etc/construct/skills/system-skill/SKILL.md", validSkillContent("system-skill", "System skill"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "system-skill", Description: "System skill", Location: "/etc/construct/skills/system-skill/SKILL.md", Scope: SkillScopeSystem},
					},
				},
			},
		},
		{
			name: "multiple scopes",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
					fs.MkdirAll("/repo/.construct/skills/repo-skill", 0755)
					afero.WriteFile(fs, "/repo/.construct/skills/repo-skill/SKILL.md", validSkillContent("repo-skill", "Repo skill"), 0644)
					fs.MkdirAll("/home/user/.config/construct/skills/user-skill", 0755)
					afero.WriteFile(fs, "/home/user/.config/construct/skills/user-skill/SKILL.md", validSkillContent("user-skill", "User skill"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "repo-skill", Description: "Repo skill", Location: "/repo/.construct/skills/repo-skill/SKILL.md", Scope: SkillScopeRepo},
						{Name: "user-skill", Description: "User skill", Location: "/home/user/.config/construct/skills/user-skill/SKILL.md", Scope: SkillScopeUser},
					},
				},
			},
		},
		{
			name: "precedence repo wins over user",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
					fs.MkdirAll("/repo/.construct/skills/shared-skill", 0755)
					afero.WriteFile(fs, "/repo/.construct/skills/shared-skill/SKILL.md", validSkillContent("shared-skill", "Repo version"), 0644)
					fs.MkdirAll("/home/user/.config/construct/skills/shared-skill", 0755)
					afero.WriteFile(fs, "/home/user/.config/construct/skills/shared-skill/SKILL.md", validSkillContent("shared-skill", "User version"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "shared-skill", Description: "Repo version", Location: "/repo/.construct/skills/shared-skill/SKILL.md", Scope: SkillScopeRepo},
					},
				},
			},
		},
		{
			name: "precedence first repo path wins",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
					fs.MkdirAll("/repo/.construct/skills/dup-skill", 0755)
					afero.WriteFile(fs, "/repo/.construct/skills/dup-skill/SKILL.md", validSkillContent("dup-skill", "From construct"), 0644)
					fs.MkdirAll("/repo/.claude/skills/dup-skill", 0755)
					afero.WriteFile(fs, "/repo/.claude/skills/dup-skill/SKILL.md", validSkillContent("dup-skill", "From claude"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "dup-skill", Description: "From construct", Location: "/repo/.construct/skills/dup-skill/SKILL.md", Scope: SkillScopeRepo},
					},
				},
			},
		},
		{
			name: "empty no skills found",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
				},
				expectation: expectation{
					skills: []*Skill{},
				},
			},
		},
		{
			name: "invalid skills skipped",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
					fs.MkdirAll("/repo/.construct/skills/valid-skill", 0755)
					afero.WriteFile(fs, "/repo/.construct/skills/valid-skill/SKILL.md", validSkillContent("valid-skill", "Valid"), 0644)
					fs.MkdirAll("/repo/.construct/skills/invalid-skill", 0755)
					afero.WriteFile(fs, "/repo/.construct/skills/invalid-skill/SKILL.md", []byte("no frontmatter"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "valid-skill", Description: "Valid", Location: "/repo/.construct/skills/valid-skill/SKILL.md", Scope: SkillScopeRepo},
					},
				},
			},
		},
		{
			name: "non-directory entries ignored",
			scenario: scenario{
				cwd:     "/repo",
				homeDir: "/home/user",
				setup: func(fs afero.Fs) {
					fs.MkdirAll("/repo/.git", 0755)
					fs.MkdirAll("/repo/.construct/skills", 0755)
					afero.WriteFile(fs, "/repo/.construct/skills/not-a-dir.md", []byte("just a file"), 0644)
					fs.MkdirAll("/repo/.construct/skills/real-skill", 0755)
					afero.WriteFile(fs, "/repo/.construct/skills/real-skill/SKILL.md", validSkillContent("real-skill", "Real skill"), 0644)
				},
				expectation: expectation{
					skills: []*Skill{
						{Name: "real-skill", Description: "Real skill", Location: "/repo/.construct/skills/real-skill/SKILL.md", Scope: SkillScopeRepo},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tt.scenario.setup(fs)

			discoverer := NewDiscoverer(fs, &mockUserInfo{homeDir: tt.scenario.homeDir})
			skills, err := discoverer.Discover(tt.scenario.cwd)

			actual := expectation{
				skills: skills,
				err:    err,
			}

			if diff := cmp.Diff(tt.scenario.expectation, actual, cmp.AllowUnexported(expectation{}), cmpopts.SortSlices(skillByName), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
