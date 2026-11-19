use crate::discovery::{read_valid_skill, sorted_subdirs};
use crate::lockfile::{self, LOCK_FILE_NAME};
use crate::{
    git, parser, GitSource, LockEntry, Skill, SkillError, Source, URLSource, UserDirs,
    SKILL_FILE_NAME,
};
use construct_types::Timestamp;
use std::path::{Component, Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

const SKILLS_DIR_NAME: &str = "skills";

/// Install behavior (Go `InstallOptions`).
#[derive(Clone, Debug, Default)]
pub struct InstallOptions {
    /// Overwrite skills that are already installed.
    pub force: bool,
    /// Install only these skills; empty installs all.
    pub skill_names: Vec<String>,
}

/// A skill with its lock file metadata (Go `InstalledSkill`).
#[derive(Clone, Debug, Default)]
pub struct InstalledSkill {
    /// The parsed skill; `location` is the installed SKILL.md (the URL for URL installs, like Go).
    pub skill: Skill,
    /// First install time; Go zero time when unknown.
    pub installed_at: Timestamp,
    /// Last install or update time; Go zero time when unknown.
    pub updated_at: Timestamp,
    /// Git origin, if installed from a repository.
    pub git: Option<GitSource>,
    /// URL origin, if installed from a direct URL.
    pub url: Option<URLSource>,
}

/// Installs, updates, deletes, and lists skills in `<config>/skills` (Go `SkillManager`).
#[derive(Clone, Debug)]
pub struct SkillManager {
    dirs: UserDirs,
    http: reqwest::Client,
}

impl SkillManager {
    /// Manager rooted at `dirs.config_dir`.
    pub fn new(dirs: UserDirs) -> Self {
        // reqwest and gix share rustls without a bundled provider; use ring like construct-connect.
        let _ = rustls::crypto::ring::default_provider().install_default();
        Self {
            dirs,
            http: reqwest::Client::new(),
        }
    }

    /// Install from a git repository or a direct SKILL.md URL.
    pub async fn install(
        &self,
        source: &Source,
        opts: &InstallOptions,
    ) -> Result<Vec<InstalledSkill>, SkillError> {
        if source.is_url() {
            return self.install_from_url(source, opts).await;
        }
        let interrupt = Arc::new(AtomicBool::new(false));
        let _cancel_on_drop = CancelOnDrop(interrupt.clone());
        let (this, source, opts) = (self.clone(), source.clone(), opts.clone());
        tokio::task::spawn_blocking(move || this.install_from_git(&source, &opts, &interrupt))
            .await
            .map_err(|e| SkillError::other("install task failed", e))?
    }

    async fn install_from_url(
        &self,
        source: &Source,
        opts: &InstallOptions,
    ) -> Result<Vec<InstalledSkill>, SkillError> {
        let response = self
            .http
            .get(&source.direct_url)
            .send()
            .await
            .map_err(|e| SkillError::other("failed to fetch SKILL.md", e))?;
        if response.status() != reqwest::StatusCode::OK {
            return Err(SkillError::Other(format!(
                "failed to fetch SKILL.md: HTTP {}",
                response.status().as_u16()
            )));
        }
        let content = response
            .bytes()
            .await
            .map_err(|e| SkillError::other("failed to read SKILL.md", e))?;
        let skill = parser::parse(&content, &source.direct_url)?;
        // Go only parses here; validating keeps a hostile name from escaping the skills directory.
        parser::validate(&skill)?;
        if !opts.skill_names.is_empty() && !opts.skill_names.contains(&skill.name) {
            return Ok(Vec::new());
        }
        let dest = self.skills_dir()?.join(&skill.name);
        if dest.exists() && !opts.force {
            return Err(SkillError::AlreadyExists(skill.name));
        }
        create_dir(&dest).map_err(|e| SkillError::other("failed to create skill directory", e))?;
        write_file(&dest.join(SKILL_FILE_NAME), &content)
            .map_err(|e| SkillError::other("failed to write SKILL.md", e))?;
        let now = Timestamp(chrono::Utc::now());
        let url = URLSource {
            url: source.direct_url.clone(),
        };
        self.upsert_lock(
            &skill.name,
            LockEntry {
                installed_at: now.clone(),
                updated_at: now.clone(),
                git: None,
                url: Some(Box::new(url.clone())),
            },
        )?;
        Ok(vec![InstalledSkill {
            skill,
            installed_at: now.clone(),
            updated_at: now,
            git: None,
            url: Some(url),
        }])
    }

    fn install_from_git(
        &self,
        source: &Source,
        opts: &InstallOptions,
        interrupt: &AtomicBool,
    ) -> Result<Vec<InstalledSkill>, SkillError> {
        let temp = tempfile::Builder::new()
            .prefix("skill-install-")
            .tempdir()
            .map_err(|e| SkillError::other("failed to create temp directory", e))?;
        let root = temp.path();
        let tree_hash = git::clone(&source.clone_url, &source.r#ref, root, interrupt)?;

        let search_root = if source.path.is_empty() {
            root.to_path_buf()
        } else {
            root.join(&source.path)
        };
        let escapes = Path::new(&source.path)
            .components()
            .any(|c| !matches!(c, Component::Normal(_) | Component::CurDir));
        if escapes || !search_root.exists() {
            return Err(SkillError::Other(format!(
                "path {:?} not found in repository",
                source.path
            )));
        }
        let skills = discover_skills_in_dir(&search_root);
        if skills.is_empty() {
            return Err(SkillError::Other("no skills found in repository".into()));
        }

        let skills_dir = self.skills_dir()?;
        let now = Timestamp(chrono::Utc::now());
        let mut installed = Vec::new();
        for mut skill in skills {
            if !opts.skill_names.is_empty() && !opts.skill_names.contains(&skill.name) {
                continue;
            }
            let dest = skills_dir.join(&skill.name);
            if dest.exists() {
                if !opts.force {
                    return Err(SkillError::AlreadyExists(skill.name));
                }
                std::fs::remove_dir_all(&dest)
                    .map_err(|e| SkillError::other("failed to remove existing skill", e))?;
            }
            let src_dir = Path::new(&skill.location)
                .parent()
                .unwrap_or(root)
                .to_path_buf();
            copy_skill_dir(&src_dir, &dest).map_err(|e| {
                SkillError::Other(format!("failed to copy skill {:?}: {e}", skill.name))
            })?;

            let rel = src_dir
                .strip_prefix(root)
                .unwrap_or(&src_dir)
                .to_string_lossy()
                .into_owned();
            #[cfg(windows)]
            let rel = rel.replace('\\', "/");
            let git = GitSource {
                provider: source.provider.clone(),
                clone_url: source.clone_url.clone(),
                path: if rel.is_empty() { ".".into() } else { rel },
                r#ref: source.r#ref.clone(),
                tree_hash: tree_hash.clone(),
            };
            self.upsert_lock(
                &skill.name,
                LockEntry {
                    installed_at: now.clone(),
                    updated_at: now.clone(),
                    git: Some(Box::new(git.clone())),
                    url: None,
                },
            )?;
            skill.location = dest.join(SKILL_FILE_NAME).to_string_lossy().into_owned();
            installed.push(InstalledSkill {
                skill,
                installed_at: now.clone(),
                updated_at: now.clone(),
                git: Some(git),
                url: None,
            });
        }
        Ok(installed)
    }

    /// Remove an installed skill and its lock entry.
    pub fn delete(&self, name: &str) -> Result<(), SkillError> {
        check_dir_name(name)?;
        let dir = self.skills_dir()?.join(name);
        if !dir.exists() {
            return Err(SkillError::NotFound(name.to_owned()));
        }
        std::fs::remove_dir_all(&dir)
            .map_err(|e| SkillError::other("failed to delete skill", e))?;
        lockfile::remove(&self.lock_path()?, name)
    }

    /// Reinstall one skill, or with `None`/empty every skill with a remote source,
    /// keeping each original `installed_at`.
    pub async fn update(&self, name: Option<&str>) -> Result<Vec<InstalledSkill>, SkillError> {
        let skills = self.load_lock()?.skills.unwrap_or_default();
        let remote = |entry: &Option<Box<LockEntry>>| {
            entry
                .as_ref()
                .is_some_and(|e| e.git.is_some() || e.url.is_some())
        };
        let to_update: Vec<String> = match name.filter(|n| !n.is_empty()) {
            Some(name) => {
                let entry = skills
                    .get(name)
                    .ok_or_else(|| SkillError::NotFound(name.to_owned()))?;
                if !remote(entry) {
                    return Err(SkillError::NotUpdatable(name.to_owned()));
                }
                vec![name.to_owned()]
            }
            None => skills
                .iter()
                .filter(|(_, e)| remote(e))
                .map(|(n, _)| n.clone())
                .collect(),
        };

        let mut updated = Vec::new();
        for skill_name in to_update {
            let Some(Some(entry)) = skills.get(&skill_name) else {
                continue;
            };
            let source = match (&entry.git, &entry.url) {
                (Some(git), _) => Source {
                    provider: git.provider.clone(),
                    clone_url: git.clone_url.clone(),
                    path: git.path.clone(),
                    r#ref: git.r#ref.clone(),
                    direct_url: String::new(),
                },
                (None, Some(url)) => Source {
                    direct_url: url.url.clone(),
                    ..Source::default()
                },
                (None, None) => continue,
            };
            let opts = InstallOptions {
                force: true,
                skill_names: vec![skill_name.clone()],
            };
            let installed = self.install(&source, &opts).await.map_err(|e| {
                SkillError::Other(format!("failed to update skill {skill_name:?}: {e}"))
            })?;
            for mut skill in installed {
                skill.installed_at = entry.installed_at.clone();
                self.upsert_lock(
                    &skill_name,
                    LockEntry {
                        installed_at: entry.installed_at.clone(),
                        updated_at: skill.updated_at.clone(),
                        git: skill.git.clone().map(Box::new),
                        url: skill.url.clone().map(Box::new),
                    },
                )?;
                updated.push(skill);
            }
        }
        Ok(updated)
    }

    /// Installed skills by directory name, with lock metadata when present.
    /// Like Go, SKILL.md is parsed but not validated, and lock file errors are ignored.
    pub fn list(&self) -> Result<Vec<InstalledSkill>, SkillError> {
        let skills_dir = self.skills_dir()?;
        let lock = self
            .load_lock()
            .ok()
            .and_then(|l| l.skills)
            .unwrap_or_default();
        let mut result = Vec::new();
        for dir in sorted_subdirs(&skills_dir) {
            let file = dir.join(SKILL_FILE_NAME);
            let Ok(content) = std::fs::read(&file) else {
                continue;
            };
            let Ok(skill) = parser::parse(&content, &file.to_string_lossy()) else {
                continue;
            };
            let mut installed = InstalledSkill {
                skill,
                ..InstalledSkill::default()
            };
            if let Some(Some(entry)) = lock.get(&installed.skill.name) {
                installed.installed_at = entry.installed_at.clone();
                installed.updated_at = entry.updated_at.clone();
                installed.git = entry.git.as_deref().cloned();
                installed.url = entry.url.as_deref().cloned();
            }
            result.push(installed);
        }
        Ok(result)
    }

    fn skills_dir(&self) -> Result<PathBuf, SkillError> {
        let config = self
            .dirs
            .ensure_config_dir()
            .map_err(|e| SkillError::other("failed to get config directory", e))?;
        let dir = config.join(SKILLS_DIR_NAME);
        create_dir(&dir).map_err(|e| SkillError::other("failed to create skills directory", e))?;
        Ok(dir)
    }

    fn lock_path(&self) -> Result<PathBuf, SkillError> {
        let config = self
            .dirs
            .ensure_config_dir()
            .map_err(|e| SkillError::Other(e.to_string()))?;
        Ok(config.join(LOCK_FILE_NAME))
    }

    fn load_lock(&self) -> Result<crate::LockFile, SkillError> {
        lockfile::load(&self.lock_path()?)
    }

    fn upsert_lock(&self, name: &str, entry: LockEntry) -> Result<(), SkillError> {
        lockfile::upsert(&self.lock_path()?, name, entry)
            .map_err(|e| SkillError::other("failed to update lock file", e))
    }
}

struct CancelOnDrop(Arc<AtomicBool>);

impl Drop for CancelOnDrop {
    fn drop(&mut self) {
        self.0.store(true, Ordering::Relaxed);
    }
}

/// Rejects names that are not a single directory component.
fn check_dir_name(name: &str) -> Result<(), SkillError> {
    let mut components = Path::new(name).components();
    match (components.next(), components.next()) {
        (Some(Component::Normal(c)), None) if c == name => Ok(()),
        _ => Err(SkillError::InvalidName(name.to_owned())),
    }
}

/// Go `discoverSkillsInDir`: `skills/`, `skills/.curated/`, then `./` children,
/// stopping at the first with valid skills; otherwise a recursive walk.
fn discover_skills_in_dir(root: &Path) -> Vec<Skill> {
    for path in ["skills", "skills/.curated", "."] {
        let skills: Vec<Skill> = sorted_subdirs(&root.join(path))
            .iter()
            .filter(|dir| {
                !dir.file_name()
                    .is_some_and(|n| n.to_string_lossy().starts_with('_'))
            })
            .filter_map(|dir| read_valid_skill(dir))
            .collect();
        if !skills.is_empty() {
            return skills;
        }
    }
    let mut skills = Vec::new();
    walk(root, &mut skills);
    skills
}

/// Go `afero.Walk` in lexical order, skipping `.`/`_` directories (including the root).
fn walk(path: &Path, skills: &mut Vec<Skill>) {
    let Ok(meta) = std::fs::symlink_metadata(path) else {
        return;
    };
    let name = path
        .file_name()
        .map(|n| n.to_string_lossy().into_owned())
        .unwrap_or_default();
    if meta.is_dir() {
        if name.starts_with('.') || name.starts_with('_') {
            return;
        }
        let Ok(entries) = std::fs::read_dir(path) else {
            return;
        };
        let mut children: Vec<PathBuf> = entries.flatten().map(|e| e.path()).collect();
        children.sort();
        for child in children {
            walk(&child, skills);
        }
    } else if name == SKILL_FILE_NAME {
        if let Some(skill) = path.parent().and_then(read_valid_skill) {
            skills.push(skill);
        }
    }
}

/// Go `copySkillDir`: skips README.md, metadata.json, and `_`-prefixed entries.
fn copy_skill_dir(src: &Path, dst: &Path) -> std::io::Result<()> {
    create_dir(dst)?;
    let mut entries: Vec<_> = std::fs::read_dir(src)?.collect::<Result<_, _>>()?;
    entries.sort_by_key(|e| e.file_name());
    for entry in entries {
        let name = entry.file_name();
        let name_str = name.to_string_lossy();
        if name_str == "README.md" || name_str == "metadata.json" || name_str.starts_with('_') {
            continue;
        }
        let (from, to) = (entry.path(), dst.join(&name));
        if entry.file_type()?.is_dir() {
            copy_skill_dir(&from, &to)?;
        } else {
            write_file(&to, &std::fs::read(&from)?)?;
        }
    }
    Ok(())
}

fn create_dir(path: &Path) -> std::io::Result<()> {
    let mut builder = std::fs::DirBuilder::new();
    builder.recursive(true);
    #[cfg(unix)]
    std::os::unix::fs::DirBuilderExt::mode(&mut builder, 0o755);
    builder.create(path)
}

fn write_file(path: &Path, content: &[u8]) -> std::io::Result<()> {
    use std::io::Write;
    let mut options = std::fs::OpenOptions::new();
    options.write(true).create(true).truncate(true);
    #[cfg(unix)]
    std::os::unix::fs::OpenOptionsExt::mode(&mut options, 0o644);
    options.open(path)?.write_all(content)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn write_skill(dir: &Path, name: &str) {
        std::fs::create_dir_all(dir).unwrap();
        std::fs::write(
            dir.join("SKILL.md"),
            format!("---\nname: {name}\ndescription: {name} skill\n---\n"),
        )
        .unwrap();
    }

    fn names(skills: &[Skill]) -> Vec<&str> {
        skills.iter().map(|s| s.name.as_str()).collect()
    }

    #[test]
    fn priority_paths_win_over_recursion() {
        let tmp = tempfile::tempdir().unwrap();
        let root = tmp.path().join("repo");
        write_skill(&root.join("skills/b"), "b");
        write_skill(&root.join("skills/a"), "a");
        write_skill(&root.join("skills/_draft"), "draft");
        write_skill(&root.join("skills/.curated/c"), "c");
        write_skill(&root.join("other/deep/d"), "d");
        assert_eq!(names(&discover_skills_in_dir(&root)), ["a", "b"]);
        std::fs::remove_dir_all(root.join("skills/a")).unwrap();
        std::fs::remove_dir_all(root.join("skills/b")).unwrap();
        assert_eq!(names(&discover_skills_in_dir(&root)), ["c"]);
    }

    #[test]
    fn recursion_skips_hidden_and_underscore_dirs() {
        let tmp = tempfile::tempdir().unwrap();
        let root = tmp.path().join("repo");
        write_skill(&root.join("x/deep/one"), "one");
        write_skill(&root.join(".hidden/two"), "two");
        write_skill(&root.join("_private/three"), "three");
        write_skill(&root.join("x/Bad"), "Bad");
        assert_eq!(names(&discover_skills_in_dir(&root)), ["one"]);
        // A skill at the search root is found by the walk.
        let single = tmp.path().join("single");
        write_skill(&single, "single");
        assert_eq!(names(&discover_skills_in_dir(&single)), ["single"]);
    }

    #[test]
    fn copy_excludes_go_files() {
        let tmp = tempfile::tempdir().unwrap();
        let src = tmp.path().join("src");
        write_skill(&src, "s");
        for f in [
            "README.md",
            "metadata.json",
            "_notes.md",
            "scripts/run.sh",
            "scripts/_skip",
            "references/doc.md",
        ] {
            let p = src.join(f);
            std::fs::create_dir_all(p.parent().unwrap()).unwrap();
            std::fs::write(p, f).unwrap();
        }
        let dst = tmp.path().join("dst");
        copy_skill_dir(&src, &dst).unwrap();
        let mut copied: Vec<_> = walkdir(&dst);
        copied.sort();
        assert_eq!(copied, ["SKILL.md", "references/doc.md", "scripts/run.sh"]);
    }

    fn walkdir(root: &Path) -> Vec<String> {
        let mut out = Vec::new();
        for entry in std::fs::read_dir(root).unwrap().flatten() {
            if entry.file_type().unwrap().is_dir() {
                let name = entry.file_name().to_string_lossy().into_owned();
                out.extend(
                    walkdir(&entry.path())
                        .into_iter()
                        .map(|p| format!("{name}/{p}")),
                );
            } else {
                out.push(entry.file_name().to_string_lossy().into_owned());
            }
        }
        out
    }

    #[test]
    fn rejects_path_names() {
        for bad in ["", ".", "..", "a/b", "../x", "/abs"] {
            assert!(
                matches!(check_dir_name(bad), Err(SkillError::InvalidName(_))),
                "{bad}"
            );
        }
        check_dir_name("ok-name").unwrap();
    }
}
