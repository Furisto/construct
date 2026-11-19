use crate::SkillError;
use std::num::NonZeroU32;
use std::path::Path;
use std::sync::atomic::AtomicBool;

/// Shallow-clone `ref` (branch, falling back to tag) into `dest`; returns the HEAD commit's tree hash.
pub(crate) fn clone(
    url: &str,
    r#ref: &str,
    dest: &Path,
    interrupt: &AtomicBool,
) -> Result<String, SkillError> {
    let branch = clone_ref(url, &format!("refs/heads/{ref}"), dest, interrupt);
    let result = match branch {
        Ok(tree) => Ok(tree),
        Err(_) => {
            clear_dir(dest);
            clone_ref(url, &format!("refs/tags/{ref}"), dest, interrupt)
        }
    };
    result.map_err(|e| SkillError::other("failed to clone repository", e))
}

fn clone_ref(
    url: &str,
    ref_name: &str,
    dest: &Path,
    interrupt: &AtomicBool,
) -> Result<String, Box<dyn std::error::Error + Send + Sync>> {
    let mut prepare = gix::prepare_clone(url, dest)?
        .with_ref_name(Some(ref_name))?
        .with_shallow(gix::remote::fetch::Shallow::DepthAtRemote(NonZeroU32::MIN));
    let (mut checkout, _) = prepare.fetch_then_checkout(gix::progress::Discard, interrupt)?;
    let (repo, _) = checkout.main_worktree(gix::progress::Discard, interrupt)?;
    let commit = repo.head_commit()?;
    Ok(commit.tree_id()?.to_string())
}

fn clear_dir(dir: &Path) {
    if let Ok(entries) = std::fs::read_dir(dir) {
        for entry in entries.flatten() {
            let path = entry.path();
            let _ = if entry.file_type().is_ok_and(|t| t.is_dir()) {
                std::fs::remove_dir_all(path)
            } else {
                std::fs::remove_file(path)
            };
        }
    }
}
