package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/furisto/construct/shared"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/gofrs/flock"
	"github.com/spf13/afero"
)

const (
	lockFileVersion = 1
	lockFileName    = "skills.lock.json"
	skillsDirName   = "skills"
)

// GitProvider identifies the git hosting service.
type GitProvider string

const (
	GitProviderGitHub GitProvider = "github"
	GitProviderGitLab GitProvider = "gitlab"
)

// Source is the parsed user input (used during install).
type Source struct {
	Provider GitProvider
	CloneURL string
	Path     string // path within repo
	Ref      string // branch/tag, default "main"

	DirectURL string // direct URL to SKILL.md
}

// IsGit returns true if the source is a git repository.
func (s *Source) IsGit() bool { return s.CloneURL != "" }

// IsURL returns true if the source is a direct URL.
func (s *Source) IsURL() bool { return s.DirectURL != "" }

// LockFile represents the skills.lock.json file.
type LockFile struct {
	Version int                   `json:"version"`
	Skills  map[string]*LockEntry `json:"skills"`
}

// LockEntry represents a single skill entry in the lock file.
type LockEntry struct {
	InstalledAt time.Time  `json:"installed_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Git         *GitSource `json:"git,omitempty"`
	URL         *URLSource `json:"url,omitempty"`
}

// IsLocal returns true if the skill was manually installed (no remote source).
func (e *LockEntry) IsLocal() bool { return e.Git == nil && e.URL == nil }

// GitSource represents a skill installed from a git repository.
type GitSource struct {
	Provider GitProvider `json:"provider"`
	CloneURL string      `json:"clone_url"`
	Path     string      `json:"path"`
	Ref      string      `json:"ref"`
	TreeHash string      `json:"tree_hash"`
}

// URLSource represents a skill installed from a direct URL.
type URLSource struct {
	URL string `json:"url"`
}

// InstallOptions configures the installation behavior.
type InstallOptions struct {
	Force      bool     // Overwrite existing skills
	SkillNames []string // Install only specific skill(s), empty means all
}

// InstalledSkill represents a skill with its installation metadata.
type InstalledSkill struct {
	*Skill
	InstalledAt time.Time
	UpdatedAt   time.Time
	Git         *GitSource
	URL         *URLSource
}

// SkillManager handles skill installation, deletion, and updates.
type SkillManager struct {
	fs       afero.Fs
	userInfo shared.UserInfo
	parser   *Parser
}

func NewSkillManager(fs afero.Fs, userInfo shared.UserInfo) *SkillManager {
	return &SkillManager{
		fs:       fs,
		userInfo: userInfo,
		parser:   NewParser(),
	}
}

var (
	// owner/repo or owner/repo/path
	shorthandPattern = regexp.MustCompile(`^([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)(?:/(.+))?$`)

	// github.com/owner/repo/tree/branch/path
	githubURLPattern = regexp.MustCompile(`^(?:https?://)?github\.com/([^/]+)/([^/]+)(?:/tree/([^/]+)(?:/(.+))?)?$`)

	// gitlab.com/owner/repo/-/tree/branch/path
	gitlabURLPattern = regexp.MustCompile(`^(?:https?://)?gitlab\.com/([^/]+)/([^/]+)(?:/-/tree/([^/]+)(?:/(.+))?)?$`)
)

// ParseSource parses a source string into a SourceInfo.
// Supported formats:
// - owner/repo (GitHub shorthand)
// - owner/repo/path (GitHub shorthand with path)
// - github.com/owner/repo/tree/branch/path
// - gitlab.com/owner/repo/-/tree/branch/path
// - Direct URL ending in /SKILL.md
func ParseSource(source string) (*Source, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, errors.New("source cannot be empty")
	}

	// Check for direct URL to SKILL.md
	if strings.HasSuffix(source, "/"+SkillFileName) || strings.HasSuffix(source, "/"+strings.ToLower(SkillFileName)) {
		url := source
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "https://" + url
		}
		return &Source{DirectURL: url}, nil
	}

	// Check for GitHub URL pattern
	if matches := githubURLPattern.FindStringSubmatch(source); matches != nil {
		owner, repo := matches[1], matches[2]
		ref := "main"
		path := ""
		if matches[3] != "" {
			ref = matches[3]
		}
		if matches[4] != "" {
			path = matches[4]
		}
		return &Source{
			Provider: GitProviderGitHub,
			CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
			Path:     path,
			Ref:      ref,
		}, nil
	}

	// Check for GitLab URL pattern
	if matches := gitlabURLPattern.FindStringSubmatch(source); matches != nil {
		owner, repo := matches[1], matches[2]
		ref := "main"
		path := ""
		if matches[3] != "" {
			ref = matches[3]
		}
		if matches[4] != "" {
			path = matches[4]
		}
		return &Source{
			Provider: GitProviderGitLab,
			CloneURL: fmt.Sprintf("https://gitlab.com/%s/%s.git", owner, repo),
			Path:     path,
			Ref:      ref,
		}, nil
	}

	// Check for shorthand pattern (GitHub default)
	if matches := shorthandPattern.FindStringSubmatch(source); matches != nil {
		// Make sure it's not a URL
		if strings.Contains(source, "://") {
			return nil, fmt.Errorf("unrecognized source format: %s", source)
		}
		owner, repo := matches[1], matches[2]
		path := ""
		if matches[3] != "" {
			path = matches[3]
		}
		return &Source{
			Provider: GitProviderGitHub,
			CloneURL: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
			Path:     path,
			Ref:      "main",
		}, nil
	}

	return nil, fmt.Errorf("unrecognized source format: %s", source)
}

// Install installs skills from the given source.
func (i *SkillManager) Install(ctx context.Context, source *Source, opts InstallOptions) ([]*InstalledSkill, error) {
	if source.IsURL() {
		return i.installFromURL(ctx, source, opts)
	}
	return i.installFromGit(ctx, source, opts)
}

func (i *SkillManager) installFromURL(ctx context.Context, source *Source, opts InstallOptions) ([]*InstalledSkill, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.DirectURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch SKILL.md: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch SKILL.md: HTTP %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md: %w", err)
	}

	skill, err := i.parser.Parse(content, source.DirectURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SKILL.md: %w", err)
	}

	// Check skill name filter
	if len(opts.SkillNames) > 0 && !contains(opts.SkillNames, skill.Name) {
		return nil, nil // Skip this skill
	}

	skillsDir, err := i.skillsDir()
	if err != nil {
		return nil, err
	}

	destDir := filepath.Join(skillsDir, skill.Name)
	exists, err := afero.Exists(i.fs, destDir)
	if err != nil {
		return nil, fmt.Errorf("failed to check skill directory: %w", err)
	}
	if exists && !opts.Force {
		return nil, fmt.Errorf("skill %q already exists, use --force to overwrite", skill.Name)
	}

	if err := i.fs.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create skill directory: %w", err)
	}

	// Write SKILL.md
	skillFile := filepath.Join(destDir, SkillFileName)
	if err := afero.WriteFile(i.fs, skillFile, content, 0644); err != nil {
		return nil, fmt.Errorf("failed to write SKILL.md: %w", err)
	}

	now := time.Now()
	entry := &LockEntry{
		InstalledAt: now,
		UpdatedAt:   now,
		URL:         &URLSource{URL: source.DirectURL},
	}

	if err := i.updateLockFile(skill.Name, entry); err != nil {
		return nil, fmt.Errorf("failed to update lock file: %w", err)
	}

	return []*InstalledSkill{{
		Skill:       skill,
		InstalledAt: now,
		UpdatedAt:   now,
		URL:         entry.URL,
	}}, nil
}

func (i *SkillManager) installFromGit(ctx context.Context, source *Source, opts InstallOptions) ([]*InstalledSkill, error) {
	tempDir, err := afero.TempDir(i.fs, "", "skill-install-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer i.fs.RemoveAll(tempDir)

	// Clone the repository (shallow clone)
	cloneOpts := &git.CloneOptions{
		URL:           source.CloneURL,
		ReferenceName: plumbing.NewBranchReferenceName(source.Ref),
		SingleBranch:  true,
		Depth:         1,
	}

	repo, err := git.PlainCloneContext(ctx, tempDir, false, cloneOpts)
	if err != nil {
		// Try with tag reference if branch fails
		cloneOpts.ReferenceName = plumbing.NewTagReferenceName(source.Ref)
		repo, err = git.PlainCloneContext(ctx, tempDir, false, cloneOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	// Get the tree hash for update detection
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}
	treeHash := commit.TreeHash.String()

	// Determine search root
	searchRoot := tempDir
	if source.Path != "" {
		searchRoot = filepath.Join(tempDir, source.Path)
		exists, err := afero.Exists(i.fs, searchRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to check path: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("path %q not found in repository", source.Path)
		}
	}

	// Discover skills
	skills, err := i.discoverSkillsInDir(searchRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to discover skills: %w", err)
	}

	if len(skills) == 0 {
		return nil, errors.New("no skills found in repository")
	}

	// Get skills directory
	skillsDir, err := i.skillsDir()
	if err != nil {
		return nil, err
	}

	var installed []*InstalledSkill
	now := time.Now()

	for _, skill := range skills {
		// Check skill name filter
		if len(opts.SkillNames) > 0 && !contains(opts.SkillNames, skill.Name) {
			continue
		}

		destDir := filepath.Join(skillsDir, skill.Name)

		// Check if skill already exists
		exists, err := afero.Exists(i.fs, destDir)
		if err != nil {
			return nil, fmt.Errorf("failed to check skill directory: %w", err)
		}
		if exists && !opts.Force {
			return nil, fmt.Errorf("skill %q already exists, use --force to overwrite", skill.Name)
		}

		if exists {
			if err := i.fs.RemoveAll(destDir); err != nil {
				return nil, fmt.Errorf("failed to remove existing skill: %w", err)
			}
		}

		// Copy skill directory
		srcDir := filepath.Dir(skill.Location)
		if err := i.copySkillDir(srcDir, destDir); err != nil {
			return nil, fmt.Errorf("failed to copy skill %q: %w", skill.Name, err)
		}

		// Calculate path within repo for this skill
		relPath, _ := filepath.Rel(tempDir, srcDir)
		gitPath := relPath
		if source.Path != "" && !strings.HasPrefix(relPath, source.Path) {
			gitPath = relPath
		}

		entry := &LockEntry{
			InstalledAt: now,
			UpdatedAt:   now,
			Git: &GitSource{
				Provider: source.Provider,
				CloneURL: source.CloneURL,
				Path:     gitPath,
				Ref:      source.Ref,
				TreeHash: treeHash,
			},
		}

		if err := i.updateLockFile(skill.Name, entry); err != nil {
			return nil, fmt.Errorf("failed to update lock file: %w", err)
		}

		// Update skill location to installed path
		skill.Location = filepath.Join(destDir, SkillFileName)

		installed = append(installed, &InstalledSkill{
			Skill:       skill,
			InstalledAt: now,
			UpdatedAt:   now,
			Git:         entry.Git,
		})
	}

	return installed, nil
}

func (i *SkillManager) discoverSkillsInDir(root string) ([]*Skill, error) {
	var skills []*Skill

	// Priority paths to search
	priorityPaths := []string{
		"skills",
		"skills/.curated",
		".",
	}

	for _, path := range priorityPaths {
		searchDir := filepath.Join(root, path)
		exists, err := afero.Exists(i.fs, searchDir)
		if err != nil || !exists {
			continue
		}

		entries, err := afero.ReadDir(i.fs, searchDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
				continue
			}

			skillDir := filepath.Join(searchDir, entry.Name())
			skillFile := filepath.Join(skillDir, SkillFileName)

			content, err := afero.ReadFile(i.fs, skillFile)
			if err != nil {
				continue
			}

			skill, err := i.parser.Parse(content, skillFile)
			if err != nil {
				continue
			}

			if err := i.parser.Validate(skill, skillDir); err != nil {
				continue
			}

			skills = append(skills, skill)
		}

		// If we found skills in a priority path, don't search further
		if len(skills) > 0 {
			return skills, nil
		}
	}

	// Recursive search if no skills found in priority paths
	return i.discoverSkillsRecursive(root)
}

func (i *SkillManager) discoverSkillsRecursive(root string) ([]*Skill, error) {
	var skills []*Skill

	err := afero.Walk(i.fs, root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and _ prefixed directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check for SKILL.md
		if info.Name() != SkillFileName {
			return nil
		}

		skillDir := filepath.Dir(path)
		content, err := afero.ReadFile(i.fs, path)
		if err != nil {
			return nil
		}

		skill, err := i.parser.Parse(content, path)
		if err != nil {
			return nil
		}

		if err := i.parser.Validate(skill, skillDir); err != nil {
			return nil
		}

		skills = append(skills, skill)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return skills, nil
}

func (i *SkillManager) copySkillDir(src, dst string) error {
	if err := i.fs.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := afero.ReadDir(i.fs, src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()

		// Skip excluded files
		if name == "README.md" || name == "metadata.json" || strings.HasPrefix(name, "_") {
			continue
		}

		srcPath := filepath.Join(src, name)
		dstPath := filepath.Join(dst, name)

		if entry.IsDir() {
			if err := i.copySkillDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			content, err := afero.ReadFile(i.fs, srcPath)
			if err != nil {
				return err
			}
			if err := afero.WriteFile(i.fs, dstPath, content, 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

// Delete removes an installed skill.
func (i *SkillManager) Delete(name string) error {
	skillsDir, err := i.skillsDir()
	if err != nil {
		return err
	}

	skillDir := filepath.Join(skillsDir, name)
	exists, err := afero.Exists(i.fs, skillDir)
	if err != nil {
		return fmt.Errorf("failed to check skill directory: %w", err)
	}
	if !exists {
		return fmt.Errorf("skill %q not found", name)
	}

	if err := i.fs.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("failed to delete skill: %w", err)
	}

	return i.removeLockEntry(name)
}

// Update updates installed skills to their latest versions.
func (i *SkillManager) Update(ctx context.Context, name string) ([]*InstalledSkill, error) {
	lockFile, err := i.loadLockFile()
	if err != nil {
		return nil, err
	}

	var toUpdate []string
	if name != "" {
		entry, exists := lockFile.Skills[name]
		if !exists {
			return nil, fmt.Errorf("skill %q not found", name)
		}
		if entry.IsLocal() {
			return nil, fmt.Errorf("skill %q was manually installed and cannot be updated", name)
		}
		toUpdate = append(toUpdate, name)
	} else {
		for skillName, entry := range lockFile.Skills {
			if !entry.IsLocal() {
				toUpdate = append(toUpdate, skillName)
			}
		}
	}

	if len(toUpdate) == 0 {
		return nil, nil
	}

	var updated []*InstalledSkill

	for _, skillName := range toUpdate {
		entry := lockFile.Skills[skillName]

		var source *Source
		if entry.Git != nil {
			source = &Source{
				Provider: entry.Git.Provider,
				CloneURL: entry.Git.CloneURL,
				Path:     entry.Git.Path,
				Ref:      entry.Git.Ref,
			}
		} else if entry.URL != nil {
			source = &Source{
				DirectURL: entry.URL.URL,
			}
		}

		skills, err := i.Install(ctx, source, InstallOptions{
			Force:      true,
			SkillNames: []string{skillName},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update skill %q: %w", skillName, err)
		}

		// Preserve original installed_at
		for _, skill := range skills {
			skill.InstalledAt = entry.InstalledAt
			if err := i.updateLockFile(skillName, &LockEntry{
				InstalledAt: entry.InstalledAt,
				UpdatedAt:   skill.UpdatedAt,
				Git:         skill.Git,
				URL:         skill.URL,
			}); err != nil {
				return nil, err
			}
		}

		updated = append(updated, skills...)
	}

	return updated, nil
}

// List returns all installed skills.
func (i *SkillManager) List() ([]*InstalledSkill, error) {
	skillsDir, err := i.skillsDir()
	if err != nil {
		return nil, err
	}

	lockFile, err := i.loadLockFile()
	if err != nil {
		// If lock file doesn't exist, still list skills but without metadata
		lockFile = &LockFile{
			Version: lockFileVersion,
			Skills:  make(map[string]*LockEntry),
		}
	}

	exists, err := afero.Exists(i.fs, skillsDir)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	entries, err := afero.ReadDir(i.fs, skillsDir)
	if err != nil {
		return nil, err
	}

	var skills []*InstalledSkill

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(skillsDir, entry.Name())
		skillFile := filepath.Join(skillDir, SkillFileName)

		content, err := afero.ReadFile(i.fs, skillFile)
		if err != nil {
			continue
		}

		skill, err := i.parser.Parse(content, skillFile)
		if err != nil {
			continue
		}

		installed := &InstalledSkill{Skill: skill}

		if lockEntry, exists := lockFile.Skills[skill.Name]; exists {
			installed.InstalledAt = lockEntry.InstalledAt
			installed.UpdatedAt = lockEntry.UpdatedAt
			installed.Git = lockEntry.Git
			installed.URL = lockEntry.URL
		}

		skills = append(skills, installed)
	}

	return skills, nil
}

// skillsDir returns the path to the skills directory.
func (i *SkillManager) skillsDir() (string, error) {
	configDir, err := i.userInfo.ConstructConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}

	skillsDir := filepath.Join(configDir, skillsDirName)
	if err := i.fs.MkdirAll(skillsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create skills directory: %w", err)
	}

	return skillsDir, nil
}

// lockFilePath returns the path to the lock file.
func (i *SkillManager) lockFilePath() (string, error) {
	configDir, err := i.userInfo.ConstructConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, lockFileName), nil
}

// loadLockFile loads the lock file.
func (i *SkillManager) loadLockFile() (*LockFile, error) {
	lockPath, err := i.lockFilePath()
	if err != nil {
		return nil, err
	}

	// Use flock for shared read lock
	fileLock := flock.New(lockPath + ".lock")
	if err := fileLock.RLock(); err != nil {
		return nil, fmt.Errorf("failed to acquire read lock: %w", err)
	}
	defer fileLock.Unlock()

	content, err := afero.ReadFile(i.fs, lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &LockFile{
				Version: lockFileVersion,
				Skills:  make(map[string]*LockEntry),
			}, nil
		}
		return nil, fmt.Errorf("failed to read lock file: %w", err)
	}

	var lockFile LockFile
	if err := json.Unmarshal(content, &lockFile); err != nil {
		return nil, fmt.Errorf("failed to parse lock file: %w", err)
	}

	if lockFile.Skills == nil {
		lockFile.Skills = make(map[string]*LockEntry)
	}

	return &lockFile, nil
}

// updateLockFile updates or adds an entry in the lock file.
func (i *SkillManager) updateLockFile(name string, entry *LockEntry) error {
	lockPath, err := i.lockFilePath()
	if err != nil {
		return err
	}

	// Use flock for exclusive write lock
	fileLock := flock.New(lockPath + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to acquire write lock: %w", err)
	}
	defer fileLock.Unlock()

	// Read current lock file
	var lockFile LockFile
	content, err := afero.ReadFile(i.fs, lockPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read lock file: %w", err)
		}
		lockFile = LockFile{
			Version: lockFileVersion,
			Skills:  make(map[string]*LockEntry),
		}
	} else {
		if err := json.Unmarshal(content, &lockFile); err != nil {
			return fmt.Errorf("failed to parse lock file: %w", err)
		}
		if lockFile.Skills == nil {
			lockFile.Skills = make(map[string]*LockEntry)
		}
	}

	lockFile.Skills[name] = entry
	return i.writeLockFileAtomic(lockPath, &lockFile)
}

// removeLockEntry removes an entry from the lock file.
func (i *SkillManager) removeLockEntry(name string) error {
	lockPath, err := i.lockFilePath()
	if err != nil {
		return err
	}

	// Use flock for exclusive write lock
	fileLock := flock.New(lockPath + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to acquire write lock: %w", err)
	}
	defer fileLock.Unlock()

	// Read current lock file
	content, err := afero.ReadFile(i.fs, lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to remove
		}
		return fmt.Errorf("failed to read lock file: %w", err)
	}

	var lockFile LockFile
	if err := json.Unmarshal(content, &lockFile); err != nil {
		return fmt.Errorf("failed to parse lock file: %w", err)
	}

	delete(lockFile.Skills, name)

	// Write atomically
	return i.writeLockFileAtomic(lockPath, &lockFile)
}

// writeLockFileAtomic writes the lock file atomically.
func (i *SkillManager) writeLockFileAtomic(lockPath string, lockFile *LockFile) error {
	content, err := json.MarshalIndent(lockFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal lock file: %w", err)
	}

	// Write to temp file first
	dir := filepath.Dir(lockPath)
	tempFile, err := afero.TempFile(i.fs, dir, "skills.lock.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		i.fs.Remove(tempPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		i.fs.Remove(tempPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := i.fs.Rename(tempPath, lockPath); err != nil {
		i.fs.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
