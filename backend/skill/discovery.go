package skill

import (
	"path/filepath"
	"strings"

	"github.com/furisto/construct/shared"
	"github.com/spf13/afero"
)

type Discoverer struct {
	fs      afero.Fs
	userInfo shared.UserInfo
	parser  *Parser
}

func NewDiscoverer(fs afero.Fs, userInfo shared.UserInfo) *Discoverer {
	return &Discoverer{
		fs:      fs,
		userInfo: userInfo,
		parser:  NewParser(),
	}
}

func (d *Discoverer) Discover(cwd string) ([]*Skill, error) {
	repoRoot := findGitRoot(d.fs, cwd)
	if repoRoot == "" {
		repoRoot = cwd
	}

	skillsByName := make(map[string]*Skill)

	for _, dp := range DefaultDiscoveryPaths() {
		searchPath := d.resolvePath(dp, repoRoot)
		if searchPath == "" {
			continue
		}

		skills, err := d.discoverInPath(searchPath, dp.Scope)
		if err != nil {
			continue
		}

		for _, skill := range skills {
			if _, exists := skillsByName[skill.Name]; !exists {
				skillsByName[skill.Name] = skill
			}
		}
	}

	result := make([]*Skill, 0, len(skillsByName))
	for _, skill := range skillsByName {
		result = append(result, skill)
	}

	return result, nil
}

func (d *Discoverer) resolvePath(dp DiscoveryPath, repoRoot string) string {
	if dp.IsRelative {
		return filepath.Join(repoRoot, dp.Pattern)
	}

	if strings.HasPrefix(dp.Pattern, "~/") {
		homeDir, err := d.userInfo.HomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(homeDir, dp.Pattern[2:])
	}

	return dp.Pattern
}

func (d *Discoverer) discoverInPath(basePath string, scope SkillScope) ([]*Skill, error) {
	exists, err := afero.Exists(d.fs, basePath)
	if err != nil || !exists {
		return nil, err
	}

	entries, err := afero.ReadDir(d.fs, basePath)
	if err != nil {
		return nil, err
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(basePath, entry.Name())
		skillFile := filepath.Join(skillDir, SkillFileName)

		content, err := afero.ReadFile(d.fs, skillFile)
		if err != nil {
			continue
		}

		skill, err := d.parser.Parse(content, skillFile)
		if err != nil {
			continue
		}

		if err := d.parser.Validate(skill, skillDir); err != nil {
			continue
		}

		skill.Scope = scope
		skills = append(skills, skill)
	}

	return skills, nil
}

func findGitRoot(fs afero.Fs, startPath string) string {
	current := startPath
	for {
		gitDir := filepath.Join(current, ".git")
		exists, err := afero.Exists(fs, gitDir)
		if err == nil && exists {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}
