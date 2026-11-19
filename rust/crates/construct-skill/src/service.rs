//! `construct.v1.SkillService` over [`SkillManager`] (Go `backend/api/skill.go`).
use crate::{parse_source, GitProvider, InstallOptions, InstalledSkill, SkillError, SkillManager};
use construct_connect::{async_trait, Code, ConnectError, Request, Response};
use construct_proto::construct::v1 as pb;
use construct_types::Timestamp;
use std::sync::Arc;

/// Connect handler for `SkillService`.
#[derive(Clone, Debug)]
pub struct SkillHandler {
    manager: Arc<SkillManager>,
}

impl SkillHandler {
    /// Handler backed by `manager`.
    pub fn new(manager: Arc<SkillManager>) -> Self {
        Self { manager }
    }

    async fn blocking<T: Send + 'static>(
        &self,
        f: impl FnOnce(&SkillManager) -> Result<T, SkillError> + Send + 'static,
    ) -> Result<T, ConnectError> {
        let manager = self.manager.clone();
        tokio::task::spawn_blocking(move || f(&manager))
            .await
            .map_err(|e| ConnectError::new(Code::Internal, e.to_string()))?
            .map_err(internal)
    }
}

/// Go `apiError`: anything not already a Connect error is internal.
fn internal(err: SkillError) -> ConnectError {
    ConnectError::new(Code::Internal, err.to_string())
}

#[async_trait]
impl pb::SkillService for SkillHandler {
    async fn install_skill(
        &self,
        req: Request<pb::InstallSkillRequest>,
    ) -> Result<Response<pb::InstallSkillResponse>, ConnectError> {
        let msg = req.message;
        let source = parse_source(&msg.source)
            .map_err(|e| ConnectError::new(Code::InvalidArgument, e.to_string()))?;
        let opts = InstallOptions {
            force: msg.force,
            skill_names: msg.skill_names,
        };
        let installed = self
            .manager
            .install(&source, &opts)
            .await
            .map_err(internal)?;
        Ok(Response::new(pb::InstallSkillResponse {
            installed_skills: installed.iter().map(to_proto).collect(),
        }))
    }

    async fn list_skills(
        &self,
        _req: Request<pb::ListSkillsRequest>,
    ) -> Result<Response<pb::ListSkillsResponse>, ConnectError> {
        let skills = self.blocking(|m| m.list()).await?;
        Ok(Response::new(pb::ListSkillsResponse {
            skills: skills.iter().map(to_proto).collect(),
        }))
    }

    async fn delete_skill(
        &self,
        req: Request<pb::DeleteSkillRequest>,
    ) -> Result<Response<pb::DeleteSkillResponse>, ConnectError> {
        let name = req.message.name;
        self.blocking(move |m| m.delete(&name)).await?;
        Ok(Response::new(pb::DeleteSkillResponse {}))
    }

    async fn update_skill(
        &self,
        req: Request<pb::UpdateSkillRequest>,
    ) -> Result<Response<pb::UpdateSkillResponse>, ConnectError> {
        let updated = self
            .manager
            .update(req.message.name.as_deref())
            .await
            .map_err(internal)?;
        Ok(Response::new(pb::UpdateSkillResponse {
            updated_skills: updated.iter().map(to_proto).collect(),
        }))
    }
}

/// Go `convertInstalledSkillToProto`.
pub fn to_proto(s: &InstalledSkill) -> pb::Skill {
    let timestamp = |t: &Timestamp| {
        (*t != Timestamp::default()).then(|| pbjson_types::Timestamp {
            seconds: t.0.timestamp(),
            nanos: t.0.timestamp_subsec_nanos() as i32,
        })
    };
    let source = match (&s.git, &s.url) {
        (Some(git), _) => Some(pb::skill::Source::Git(pb::GitSource {
            provider: provider_to_proto(&git.provider) as i32,
            clone_url: git.clone_url.clone(),
            path: git.path.clone(),
            r#ref: git.r#ref.clone(),
            tree_hash: git.tree_hash.clone(),
        })),
        (None, Some(url)) => Some(pb::skill::Source::Url(pb::UrlSource {
            url: url.url.clone(),
        })),
        (None, None) => None,
    };
    pb::Skill {
        name: s.skill.name.clone(),
        description: s.skill.description.clone(),
        installed_at: timestamp(&s.installed_at),
        updated_at: timestamp(&s.updated_at),
        source,
    }
}

fn provider_to_proto(provider: &GitProvider) -> pb::GitProvider {
    match provider.as_ref() {
        GitProvider::GIT_HUB => pb::GitProvider::Github,
        GitProvider::GIT_LAB => pb::GitProvider::Gitlab,
        _ => pb::GitProvider::Unspecified,
    }
}
