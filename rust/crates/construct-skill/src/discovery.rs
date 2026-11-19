use crate::{parser, Skill, SkillScope, UserDirs, SKILL_FILE_NAME};
use std::collections::HashSet;
use std::path::{Path, PathBuf};

/// One place skills are searched for, in precedence order.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct DiscoveryPath {
    /// Directory; relative to the repo root if `is_relative`, `~/` expands to home.
    pub pattern: String,
    /// Scope assigned to skills found here.
    pub scope: SkillScope,
    /// Whether `pattern` is relative to the repository root.
    pub is_relative: bool,
}

/// Go `DefaultDiscoveryPaths`: repo, then user, then system directories.
pub fn default_discovery_paths(dirs: &UserDirs) -> Vec<DiscoveryPath> {
    let path = |pattern: &str, scope, is_relative| DiscoveryPath {
        pattern: pattern.to_owned(),
        scope,
        is_relative,
    };
    let config_skills = dirs.config_dir.join("skills");
    vec![
        path(".construct/skills", SkillScope::Repo, true),
        path(".codex/skills", SkillScope::Repo, true),
        path(".claude/skills", SkillScope::Repo, true),
        path(".github/skills", SkillScope::Repo, true),
        path(&config_skills.to_string_lossy(), SkillScope::User, false),
        path("~/.config/construct/skills", SkillScope::User, false),
        path("~/.codex/skills", SkillScope::User, false),
        path("~/.claude/skills", SkillScope::User, false),
        path("/etc/construct/skills", SkillScope::System, false),
    ]
}

/// Finds the skills available to an agent working in a directory.
#[derive(Clone, Debug)]
pub struct Discoverer {
    dirs: UserDirs,
    paths: Vec<DiscoveryPath>,
}

impl Discoverer {
    /// Discoverer over [`default_discovery_paths`].
    pub fn new(dirs: UserDirs) -> Self {
        let paths = default_discovery_paths(&dirs);
        Self { dirs, paths }
    }

    /// Discoverer over custom paths (for tests and embedders).
    pub fn with_paths(dirs: UserDirs, paths: Vec<DiscoveryPath>) -> Self {
        Self { dirs, paths }
    }

    /// Valid skills visible from `cwd`. The first path defining a name wins and
    /// unreadable or invalid skills are skipped. Unlike Go's map iteration, the
    /// result is ordered by precedence, then directory name.
    pub fn discover(&self, cwd: &Path) -> Vec<Skill> {
        let repo_root = find_git_root(cwd).unwrap_or(cwd);
        let mut seen = HashSet::new();
        let mut result = Vec::new();
        for dp in &self.paths {
            let Some(search) = self.resolve(dp, repo_root) else {
                continue;
            };
            for skill in discover_in_path(&search, dp.scope) {
                if seen.insert(skill.name.clone()) {
                    result.push(skill);
                }
            }
        }
        result
    }

    fn resolve(&self, dp: &DiscoveryPath, repo_root: &Path) -> Option<PathBuf> {
        if dp.is_relative {
            return Some(repo_root.join(&dp.pattern));
        }
        match dp.pattern.strip_prefix("~/") {
            Some(rest) => self.dirs.home.as_ref().map(|h| h.join(rest)),
            None => Some(PathBuf::from(&dp.pattern)),
        }
    }
}

fn discover_in_path(base: &Path, scope: SkillScope) -> Vec<Skill> {
    sorted_subdirs(base)
        .into_iter()
        .filter_map(|dir| read_valid_skill(&dir))
        .map(|mut skill| {
            skill.scope = Some(scope);
            skill
        })
        .collect()
}

/// Subdirectories sorted by name; symlinks are not followed (Go `afero.ReadDir`).
pub(crate) fn sorted_subdirs(base: &Path) -> Vec<PathBuf> {
    let Ok(entries) = std::fs::read_dir(base) else {
        return Vec::new();
    };
    let mut dirs: Vec<PathBuf> = entries
        .flatten()
        .filter(|e| e.file_type().is_ok_and(|t| t.is_dir()))
        .map(|e| e.path())
        .collect();
    dirs.sort();
    dirs
}

/// Parse and validate `dir/SKILL.md`, or `None` if unreadable or invalid.
pub(crate) fn read_valid_skill(dir: &Path) -> Option<Skill> {
    let file = dir.join(SKILL_FILE_NAME);
    let content = std::fs::read(&file).ok()?;
    let skill = parser::parse(&content, &file.to_string_lossy()).ok()?;
    parser::validate(&skill).ok()?;
    Some(skill)
}

fn find_git_root(start: &Path) -> Option<&Path> {
    start.ancestors().find(|dir| dir.join(".git").exists())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn write_skill(dir: &Path, name: &str, description: &str) {
        std::fs::create_dir_all(dir).unwrap();
        std::fs::write(
            dir.join("SKILL.md"),
            format!("---\nname: {name}\ndescription: {description}\n---\n# {name}\n"),
        )
        .unwrap();
    }

    fn dirs(root: &Path) -> UserDirs {
        UserDirs {
            home: Some(root.join("home")),
            config_dir: root.join("config/construct"),
        }
    }

    /// Default paths minus `/etc`, which tests cannot control.
    fn discoverer(root: &Path) -> Discoverer {
        let dirs = dirs(root);
        let mut paths = default_discovery_paths(&dirs);
        paths.retain(|p| p.scope != SkillScope::System);
        Discoverer::with_paths(dirs, paths)
    }

    #[test]
    fn default_paths_match_go_order() {
        let paths = default_discovery_paths(&UserDirs {
            home: None,
            config_dir: "/cfg/construct".into(),
        });
        let user_skills = Path::new("/cfg/construct").join("skills");
        let patterns: Vec<_> = paths
            .iter()
            .map(|p| (p.pattern.as_str(), p.scope.as_str(), p.is_relative))
            .collect();
        assert_eq!(
            patterns,
            [
                (".construct/skills", "repo", true),
                (".codex/skills", "repo", true),
                (".claude/skills", "repo", true),
                (".github/skills", "repo", true),
                (user_skills.to_str().unwrap(), "user", false),
                ("~/.config/construct/skills", "user", false),
                ("~/.codex/skills", "user", false),
                ("~/.claude/skills", "user", false),
                ("/etc/construct/skills", "system", false),
            ]
        );
    }

    #[test]
    fn uses_cwd_without_git_root() {
        let tmp = tempfile::tempdir().unwrap();
        let cwd = tmp.path().join("work");
        write_skill(&cwd.join(".construct/skills/local"), "local", "Local skill");
        let skills = discoverer(tmp.path()).discover(&cwd);
        assert_eq!(skills.len(), 1);
        assert_eq!(skills[0].scope, Some(SkillScope::Repo));
        assert_eq!(
            Path::new(&skills[0].location),
            cwd.join(".construct/skills/local/SKILL.md")
        );
    }

    #[test]
    fn finds_repo_skills_from_git_root_and_user_skills() {
        let tmp = tempfile::tempdir().unwrap();
        let repo = tmp.path().join("repo");
        std::fs::create_dir_all(repo.join(".git")).unwrap();
        write_skill(
            &repo.join(".claude/skills/repo-skill"),
            "repo-skill",
            "Repo",
        );
        write_skill(
            &tmp.path().join("home/.codex/skills/user-skill"),
            "user-skill",
            "User",
        );
        let skills = discoverer(tmp.path()).discover(&repo.join("src/deep"));
        let found: Vec<_> = skills
            .iter()
            .map(|s| (s.name.as_str(), s.scope.unwrap()))
            .collect();
        assert_eq!(
            found,
            [
                ("repo-skill", SkillScope::Repo),
                ("user-skill", SkillScope::User)
            ]
        );
    }

    #[test]
    fn first_path_wins_and_invalid_skills_are_skipped() {
        let tmp = tempfile::tempdir().unwrap();
        let cwd = tmp.path().join("work");
        write_skill(&cwd.join(".codex/skills/shared"), "shared", "From repo");
        write_skill(
            &tmp.path().join("config/construct/skills/shared"),
            "shared",
            "From config",
        );
        write_skill(
            &tmp.path().join("config/construct/skills/bad"),
            "Bad_Name",
            "Invalid",
        );
        write_skill(
            &tmp.path().join("home/.claude/skills/no-desc"),
            "no-desc",
            "\"\"",
        );
        std::fs::create_dir_all(tmp.path().join("home/.claude/skills/empty")).unwrap();
        std::fs::write(tmp.path().join("home/.claude/skills/file.md"), "x").unwrap();
        let skills = discoverer(tmp.path()).discover(&cwd);
        assert_eq!(skills.len(), 1);
        assert_eq!(skills[0].description, "From repo");
    }

    #[test]
    fn config_dir_skills_are_user_scope() {
        let tmp = tempfile::tempdir().unwrap();
        write_skill(
            &tmp.path().join("config/construct/skills/cfg"),
            "cfg",
            "Config skill",
        );
        let skills = discoverer(tmp.path()).discover(tmp.path());
        assert_eq!(skills[0].scope, Some(SkillScope::User));
    }
}
