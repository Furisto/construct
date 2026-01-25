package skill

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func TestParseSource(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		want      *Source
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "github shorthand owner/repo",
			source: "anthropics/skills",
			want: &Source{
				Provider: GitProviderGitHub,
				CloneURL: "https://github.com/anthropics/skills.git",
				Path:     "",
				Ref:      "main",
			},
		},
		{
			name:   "github shorthand with path",
			source: "anthropics/skills/coding/commit",
			want: &Source{
				Provider: GitProviderGitHub,
				CloneURL: "https://github.com/anthropics/skills.git",
				Path:     "coding/commit",
				Ref:      "main",
			},
		},
		{
			name:   "github full URL",
			source: "github.com/anthropics/skills/tree/main/coding",
			want: &Source{
				Provider: GitProviderGitHub,
				CloneURL: "https://github.com/anthropics/skills.git",
				Path:     "coding",
				Ref:      "main",
			},
		},
		{
			name:   "github full URL with branch",
			source: "github.com/anthropics/skills/tree/develop/coding",
			want: &Source{
				Provider: GitProviderGitHub,
				CloneURL: "https://github.com/anthropics/skills.git",
				Path:     "coding",
				Ref:      "develop",
			},
		},
		{
			name:   "github full URL with https",
			source: "https://github.com/anthropics/skills/tree/v1.0/coding",
			want: &Source{
				Provider: GitProviderGitHub,
				CloneURL: "https://github.com/anthropics/skills.git",
				Path:     "coding",
				Ref:      "v1.0",
			},
		},
		{
			name:   "gitlab full URL",
			source: "gitlab.com/group/project/-/tree/main/skills",
			want: &Source{
				Provider: GitProviderGitLab,
				CloneURL: "https://gitlab.com/group/project.git",
				Path:     "skills",
				Ref:      "main",
			},
		},
		{
			name:   "direct URL to SKILL.md",
			source: "https://example.com/path/to/SKILL.md",
			want: &Source{
				DirectURL: "https://example.com/path/to/SKILL.md",
			},
		},
		{
			name:   "direct URL without https",
			source: "example.com/path/to/SKILL.md",
			want: &Source{
				DirectURL: "https://example.com/path/to/SKILL.md",
			},
		},
		{
			name:      "empty source",
			source:    "",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "unrecognized format",
			source:    "not-a-valid-source",
			wantErr:   true,
			errSubstr: "unrecognized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSource(tt.source)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Provider != tt.want.Provider {
				t.Errorf("Provider = %q, want %q", got.Provider, tt.want.Provider)
			}
			if got.CloneURL != tt.want.CloneURL {
				t.Errorf("CloneURL = %q, want %q", got.CloneURL, tt.want.CloneURL)
			}
			if got.Path != tt.want.Path {
				t.Errorf("Path = %q, want %q", got.Path, tt.want.Path)
			}
			if got.Ref != tt.want.Ref {
				t.Errorf("Ref = %q, want %q", got.Ref, tt.want.Ref)
			}
			if got.DirectURL != tt.want.DirectURL {
				t.Errorf("DirectURL = %q, want %q", got.DirectURL, tt.want.DirectURL)
			}
		})
	}
}

func TestInstaller_LockFile(t *testing.T) {
	// Use real filesystem for lock file tests (flock doesn't work with afero memfs)
	tempDir := t.TempDir()
	fs := afero.NewOsFs()
	userInfo := &mockUserInfo{homeDir: tempDir}

	// Create config directory
	fs.MkdirAll(tempDir+"/.config/construct", 0755)

	installer := NewSkillManager(fs, userInfo)

	// Test updating lock file
	entry := &LockEntry{
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Git: &GitSource{
			Provider: GitProviderGitHub,
			CloneURL: "https://github.com/owner/repo.git",
			Path:     "skills/test",
			Ref:      "main",
			TreeHash: "abc123",
		},
	}

	err := installer.updateLockFile("test-skill", entry)
	if err != nil {
		t.Fatalf("failed to update lock file: %v", err)
	}

	// Verify lock file was created
	lockPath := tempDir + "/.config/construct/skills.lock.json"
	exists, err := afero.Exists(fs, lockPath)
	if err != nil {
		t.Fatalf("failed to check lock file: %v", err)
	}
	if !exists {
		t.Fatal("lock file was not created")
	}

	// Load and verify content
	lockFile, err := installer.loadLockFile()
	if err != nil {
		t.Fatalf("failed to load lock file: %v", err)
	}

	if lockFile.Version != lockFileVersion {
		t.Errorf("Version = %d, want %d", lockFile.Version, lockFileVersion)
	}

	if len(lockFile.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(lockFile.Skills))
	}

	loadedEntry, ok := lockFile.Skills["test-skill"]
	if !ok {
		t.Fatal("test-skill not found in lock file")
	}

	if loadedEntry.Git == nil {
		t.Fatal("Git source is nil")
	}

	if loadedEntry.Git.CloneURL != entry.Git.CloneURL {
		t.Errorf("CloneURL = %q, want %q", loadedEntry.Git.CloneURL, entry.Git.CloneURL)
	}

	// Test removing entry
	err = installer.removeLockEntry("test-skill")
	if err != nil {
		t.Fatalf("failed to remove lock entry: %v", err)
	}

	lockFile, err = installer.loadLockFile()
	if err != nil {
		t.Fatalf("failed to load lock file after removal: %v", err)
	}

	if len(lockFile.Skills) != 0 {
		t.Errorf("expected 0 skills after removal, got %d", len(lockFile.Skills))
	}
}

