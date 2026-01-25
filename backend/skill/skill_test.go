package skill

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

type mockUserInfo struct {
	homeDir string
}

func (m *mockUserInfo) UserID() (string, error)  { return "1000", nil }
func (m *mockUserInfo) HomeDir() (string, error) { return m.homeDir, nil }
func (m *mockUserInfo) ConstructConfigDir() (string, error) {
	return m.homeDir + "/.config/construct", nil
}
func (m *mockUserInfo) ConstructDataDir() (string, error) {
	return m.homeDir + "/.local/share/construct", nil
}
func (m *mockUserInfo) ConstructLogDir() (string, error) {
	return m.homeDir + "/.local/state/construct", nil
}
func (m *mockUserInfo) ConstructRuntimeDir() (string, error) { return "/tmp/construct", nil }
func (m *mockUserInfo) Cwd() (string, error)                 { return "/", nil }
func (m *mockUserInfo) IsRoot() (bool, error)                { return false, nil }

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantName    string
		wantDesc    string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid skill",
			content: `---
name: test-skill
description: A test skill for testing
---
# Test Skill
Instructions here`,
			wantName: "test-skill",
			wantDesc: "A test skill for testing",
		},
		{
			name:        "missing frontmatter",
			content:     "# Just markdown",
			wantErr:     true,
			errContains: "frontmatter",
		},
		{
			name: "empty frontmatter",
			content: `---
---
# Content`,
			wantErr:     true,
			errContains: "frontmatter",
		},
	}

	parser := NewParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill, err := parser.Parse([]byte(tt.content), "/path/to/SKILL.md")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if skill.Name != tt.wantName {
				t.Errorf("name = %q, want %q", skill.Name, tt.wantName)
			}
			if skill.Description != tt.wantDesc {
				t.Errorf("description = %q, want %q", skill.Description, tt.wantDesc)
			}
		})
	}
}

func TestParser_Validate(t *testing.T) {
	tests := []struct {
		name    string
		skill   *Skill
		dirPath string
		wantErr error
	}{
		{
			name:    "valid skill",
			skill:   &Skill{Name: "test-skill", Description: "A valid description"},
			dirPath: "/path/to/test-skill",
			wantErr: nil,
		},
		{
			name:    "missing name",
			skill:   &Skill{Description: "A description"},
			dirPath: "/path/to/skill",
			wantErr: ErrMissingName,
		},
		{
			name:    "missing description",
			skill:   &Skill{Name: "test-skill"},
			dirPath: "/path/to/test-skill",
			wantErr: ErrMissingDescription,
		},
		{
			name:    "invalid name format - uppercase",
			skill:   &Skill{Name: "Test-Skill", Description: "Desc"},
			dirPath: "/path/to/Test-Skill",
			wantErr: ErrInvalidNameFormat,
		},
		{
			name:    "invalid name format - spaces",
			skill:   &Skill{Name: "test skill", Description: "Desc"},
			dirPath: "/path/to/test skill",
			wantErr: ErrInvalidNameFormat,
		},
		{
			name:    "name mismatch",
			skill:   &Skill{Name: "test-skill", Description: "Desc"},
			dirPath: "/path/to/other-skill",
			wantErr: ErrNameMismatch,
		},
	}

	parser := NewParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.skill, tt.dirPath)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !errorIs(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func errorIs(err, target error) bool {
	if errors.Is(err, target) {
		return true
	}
	if err != nil && target != nil && strings.Contains(err.Error(), target.Error()) {
		return true
	}
	return false
}

func TestDiscoverer_Discover(t *testing.T) {
	fs := afero.NewMemMapFs()

	fs.MkdirAll("/repo/.git", 0755)
	fs.MkdirAll("/repo/.construct/skills/my-skill", 0755)
	afero.WriteFile(fs, "/repo/.construct/skills/my-skill/SKILL.md", []byte(`---
name: my-skill
description: My test skill description
---
# My Skill`), 0644)

	fs.MkdirAll("/home/user/.config/construct/skills/user-skill", 0755)
	afero.WriteFile(fs, "/home/user/.config/construct/skills/user-skill/SKILL.md", []byte(`---
name: user-skill
description: User scope skill
---
# User Skill`), 0644)

	discoverer := NewDiscoverer(fs, &mockUserInfo{homeDir: "/home/user"})
	skills, err := discoverer.Discover("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 2 {
		t.Errorf("got %d skills, want 2", len(skills))
	}

	skillMap := make(map[string]*Skill)
	for _, s := range skills {
		skillMap[s.Name] = s
	}

	if s, ok := skillMap["my-skill"]; !ok {
		t.Error("my-skill not found")
	} else if s.Scope != SkillScopeRepo {
		t.Errorf("my-skill scope = %v, want %v", s.Scope, SkillScopeRepo)
	}

	if s, ok := skillMap["user-skill"]; !ok {
		t.Error("user-skill not found")
	} else if s.Scope != SkillScopeUser {
		t.Errorf("user-skill scope = %v, want %v", s.Scope, SkillScopeUser)
	}
}

func TestDiscoverer_Precedence(t *testing.T) {
	fs := afero.NewMemMapFs()

	fs.MkdirAll("/repo/.git", 0755)
	fs.MkdirAll("/repo/.construct/skills/shared-skill", 0755)
	afero.WriteFile(fs, "/repo/.construct/skills/shared-skill/SKILL.md", []byte(`---
name: shared-skill
description: Repo version
---`), 0644)

	fs.MkdirAll("/home/user/.config/construct/skills/shared-skill", 0755)
	afero.WriteFile(fs, "/home/user/.config/construct/skills/shared-skill/SKILL.md", []byte(`---
name: shared-skill
description: User version
---`), 0644)

	discoverer := NewDiscoverer(fs, &mockUserInfo{homeDir: "/home/user"})
	skills, err := discoverer.Discover("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sharedSkill *Skill
	for _, s := range skills {
		if s.Name == "shared-skill" {
			sharedSkill = s
			break
		}
	}

	if sharedSkill == nil {
		t.Fatal("shared-skill not found")
	}

	if sharedSkill.Description != "Repo version" {
		t.Errorf("description = %q, want %q (repo should win)", sharedSkill.Description, "Repo version")
	}

	if sharedSkill.Scope != SkillScopeRepo {
		t.Errorf("scope = %v, want %v", sharedSkill.Scope, SkillScopeRepo)
	}
}