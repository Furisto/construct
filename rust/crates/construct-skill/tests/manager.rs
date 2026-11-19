//! Manager and SkillService behavior against local git repositories and an HTTP server.
use construct_connect::{Channel, ClientConfig, Request};
use construct_proto::construct::v1 as pb;
use construct_skill::service::SkillHandler;
use construct_skill::{parse_source, InstallOptions, SkillError, SkillManager, Source, UserDirs};
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::Arc;

fn git(repo: &Path, args: &[&str]) {
    let out = Command::new("git")
        .arg("-C")
        .arg(repo)
        .args([
            "-c",
            "user.name=t",
            "-c",
            "user.email=t@t",
            "-c",
            "commit.gpgsign=false",
            "-c",
            "tag.gpgsign=false",
        ])
        .args(args)
        .output()
        .unwrap();
    assert!(
        out.status.success(),
        "git {args:?}: {}",
        String::from_utf8_lossy(&out.stderr)
    );
}

fn write(path: PathBuf, content: &str) {
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    std::fs::write(path, content).unwrap();
}

fn skill_md(name: &str, description: &str) -> String {
    format!("---\nname: {name}\ndescription: {description}\n---\n# {name}\n")
}

fn commit_all(repo: &Path, message: &str) {
    git(repo, &["add", "-A", "--", "."]);
    git(repo, &["commit", "-q", "-m", message]);
}

/// A repo on `main` with `skills/{alpha,beta}`, a README, and a `v1` tag on a `release` branch.
fn fixture_repo(root: &Path) -> (PathBuf, String) {
    let repo = root.join("origin");
    std::fs::create_dir_all(&repo).unwrap();
    git(&repo, &["init", "-q", "-b", "main"]);
    write(
        repo.join("skills/alpha/SKILL.md"),
        &skill_md("alpha", "Alpha v1"),
    );
    write(repo.join("skills/alpha/scripts/run.sh"), "echo hi");
    write(repo.join("skills/alpha/README.md"), "excluded");
    write(repo.join("skills/beta/SKILL.md"), &skill_md("beta", "Beta"));
    commit_all(&repo, "init");
    git(&repo, &["checkout", "-q", "-b", "release"]);
    write(
        repo.join("skills/gamma/SKILL.md"),
        &skill_md("gamma", "Tagged"),
    );
    commit_all(&repo, "gamma");
    git(&repo, &["tag", "v1"]);
    git(&repo, &["checkout", "-q", "main"]);
    git(&repo, &["branch", "-q", "-D", "release"]);
    let url = format!("file://{}", repo.display());
    (repo, url)
}

fn git_source(url: &str, path: &str, r#ref: &str) -> Source {
    Source {
        provider: "github".into(),
        clone_url: url.into(),
        path: path.into(),
        r#ref: r#ref.into(),
        direct_url: String::new(),
    }
}

fn manager(root: &Path) -> (SkillManager, PathBuf) {
    let config = root.join("config/construct");
    (
        SkillManager::new(UserDirs {
            home: Some(root.join("home")),
            config_dir: config.clone(),
        }),
        config,
    )
}

fn tree_hash(repo: &Path) -> String {
    let out = Command::new("git")
        .arg("-C")
        .arg(repo)
        .args(["rev-parse", "HEAD^{tree}"])
        .output()
        .unwrap();
    String::from_utf8(out.stdout).unwrap().trim().to_owned()
}

fn lock_json(config: &Path) -> serde_json::Value {
    serde_json::from_slice(&std::fs::read(config.join("skills.lock.json")).unwrap()).unwrap()
}

#[tokio::test]
async fn git_install_update_delete_roundtrip() {
    let tmp = tempfile::tempdir().unwrap();
    let (repo, url) = fixture_repo(tmp.path());
    let (manager, config) = manager(tmp.path());

    let installed = manager
        .install(&git_source(&url, "", "main"), &InstallOptions::default())
        .await
        .unwrap();
    let names: Vec<_> = installed.iter().map(|s| s.skill.name.as_str()).collect();
    assert_eq!(names, ["alpha", "beta"]);
    let alpha_dir = config.join("skills/alpha");
    assert!(alpha_dir.join("scripts/run.sh").exists());
    assert!(!alpha_dir.join("README.md").exists());
    assert_eq!(
        installed[0].skill.location,
        alpha_dir.join("SKILL.md").to_string_lossy()
    );

    let lock = lock_json(&config);
    assert_eq!(lock["version"], 1);
    let git_meta = &lock["skills"]["alpha"]["git"];
    assert_eq!(git_meta["path"], "skills/alpha");
    assert_eq!(git_meta["ref"], "main");
    assert_eq!(git_meta["provider"], "github");
    assert_eq!(git_meta["clone_url"], url.as_str());
    let first_tree = tree_hash(&repo);
    assert_eq!(git_meta["tree_hash"], first_tree.as_str());
    let installed_at = lock["skills"]["alpha"]["installed_at"].clone();

    let err = manager
        .install(&git_source(&url, "", "main"), &InstallOptions::default())
        .await
        .unwrap_err();
    assert_eq!(
        err.to_string(),
        "skill \"alpha\" already exists, use --force to overwrite"
    );

    write(
        repo.join("skills/alpha/SKILL.md"),
        &skill_md("alpha", "Alpha v2"),
    );
    commit_all(&repo, "v2");
    let updated = manager.update(Some("alpha")).await.unwrap();
    assert_eq!(updated.len(), 1);
    assert_eq!(updated[0].skill.description, "Alpha v2");
    let lock = lock_json(&config);
    assert_eq!(lock["skills"]["alpha"]["installed_at"], installed_at);
    assert_ne!(lock["skills"]["alpha"]["updated_at"], installed_at);
    assert_eq!(
        lock["skills"]["alpha"]["git"]["tree_hash"],
        tree_hash(&repo).as_str()
    );
    assert_ne!(tree_hash(&repo), first_tree);

    let all = manager.update(None).await.unwrap();
    assert_eq!(
        all.iter()
            .map(|s| s.skill.name.as_str())
            .collect::<Vec<_>>(),
        ["alpha", "beta"]
    );

    let listed = manager.list().unwrap();
    assert_eq!(listed.len(), 2);
    assert_eq!(listed[0].installed_at, updated[0].installed_at);
    assert!(listed[0].git.is_some());

    manager.delete("alpha").unwrap();
    assert!(!alpha_dir.exists());
    assert!(lock_json(&config)["skills"].get("alpha").is_none());
    assert!(matches!(
        manager.delete("alpha"),
        Err(SkillError::NotFound(_))
    ));
    assert!(matches!(
        manager.delete("../config"),
        Err(SkillError::InvalidName(_))
    ));
}

#[tokio::test]
async fn git_install_filters_paths_and_falls_back_to_tags() {
    let tmp = tempfile::tempdir().unwrap();
    let (_repo, url) = fixture_repo(tmp.path());
    let (manager, config) = manager(tmp.path());

    let only_beta = InstallOptions {
        force: false,
        skill_names: vec!["beta".into()],
    };
    let installed = manager
        .install(&git_source(&url, "", "main"), &only_beta)
        .await
        .unwrap();
    assert_eq!(installed.len(), 1);
    assert!(!config.join("skills/alpha").exists());

    let tagged = manager
        .install(
            &git_source(&url, "skills/gamma", "v1"),
            &InstallOptions::default(),
        )
        .await
        .unwrap();
    assert_eq!(tagged[0].skill.name, "gamma");
    assert_eq!(tagged[0].git.as_ref().unwrap().path, "skills/gamma");

    let err = manager
        .install(
            &git_source(&url, "nope", "main"),
            &InstallOptions::default(),
        )
        .await
        .unwrap_err();
    assert_eq!(err.to_string(), "path \"nope\" not found in repository");
    let err = manager
        .install(
            &git_source(&url, "", "missing-ref"),
            &InstallOptions::default(),
        )
        .await
        .unwrap_err();
    assert!(
        err.to_string().starts_with("failed to clone repository"),
        "{err}"
    );
    let err = manager
        .install(
            &git_source(&url, "skills/alpha/scripts", "main"),
            &InstallOptions::default(),
        )
        .await
        .unwrap_err();
    assert_eq!(err.to_string(), "no skills found in repository");
}

#[tokio::test]
async fn local_skills_are_listed_but_not_updatable() {
    let tmp = tempfile::tempdir().unwrap();
    let (manager, config) = manager(tmp.path());
    write(
        config.join("skills/manual/SKILL.md"),
        &skill_md("manual", "By hand"),
    );
    let listed = manager.list().unwrap();
    assert_eq!(listed[0].skill.name, "manual");
    assert!(listed[0].git.is_none() && listed[0].url.is_none());
    assert!(matches!(
        manager.update(Some("manual")).await,
        Err(SkillError::NotFound(_))
    ));
    std::fs::write(config.join("skills.lock.json"), r#"{"version":1,"skills":{"manual":{"installed_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}}}"#).unwrap();
    let err = manager.update(Some("manual")).await.unwrap_err();
    assert_eq!(
        err.to_string(),
        "skill \"manual\" was manually installed and cannot be updated"
    );
    assert!(manager.update(None).await.unwrap().is_empty());
}

async fn serve_skill_md(body: &'static str) -> String {
    let app = axum::Router::new()
        .route(
            "/skills/web/SKILL.md",
            axum::routing::get(move || async move { body }),
        )
        .route(
            "/missing/SKILL.md",
            axum::routing::get(|| async { (axum::http::StatusCode::NOT_FOUND, "") }),
        );
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    tokio::spawn(async move { axum::serve(listener, app).await.unwrap() });
    format!("http://{addr}")
}

#[tokio::test]
async fn url_install_and_update() {
    let tmp = tempfile::tempdir().unwrap();
    let base = serve_skill_md("---\nname: web\ndescription: From the web\n---\nBody").await;
    let (manager, config) = manager(tmp.path());
    let source = parse_source(&format!("{base}/skills/web/SKILL.md")).unwrap();
    let installed = manager
        .install(&source, &InstallOptions::default())
        .await
        .unwrap();
    assert_eq!(installed[0].skill.location, source.direct_url);
    assert_eq!(
        std::fs::read_to_string(config.join("skills/web/SKILL.md")).unwrap(),
        "---\nname: web\ndescription: From the web\n---\nBody"
    );
    assert_eq!(
        lock_json(&config)["skills"]["web"]["url"]["url"],
        source.direct_url.as_str()
    );
    assert!(lock_json(&config)["skills"]["web"].get("git").is_none());
    assert_eq!(manager.update(None).await.unwrap()[0].skill.name, "web");

    let missing = parse_source(&format!("{base}/missing/SKILL.md")).unwrap();
    let err = manager
        .install(&missing, &InstallOptions::default())
        .await
        .unwrap_err();
    assert_eq!(err.to_string(), "failed to fetch SKILL.md: HTTP 404");
    let filtered = InstallOptions {
        force: true,
        skill_names: vec!["other".into()],
    };
    assert!(manager
        .install(&source, &filtered)
        .await
        .unwrap()
        .is_empty());
}

#[tokio::test]
async fn connect_service_roundtrip() {
    let tmp = tempfile::tempdir().unwrap();
    let (_repo, url) = fixture_repo(tmp.path());
    let (manager, _config) = manager(tmp.path());
    let installer = manager.clone();
    let router = construct_connect::Router::new().nest(
        "/api",
        pb::SkillServiceServer::new(SkillHandler::new(Arc::new(manager))).router(),
    );
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    tokio::spawn(construct_connect::serve_tcp(
        listener,
        router,
        std::future::pending(),
    ));
    let client = pb::SkillServiceClient::new(
        Channel::new(ClientConfig::new(format!("http://{addr}/api"))).unwrap(),
    );

    let err = client
        .install_skill(Request::new(pb::InstallSkillRequest {
            source: "not a source".into(),
            ..Default::default()
        }))
        .await
        .unwrap_err();
    assert_eq!(err.code, construct_connect::Code::InvalidArgument);

    let installed = installer
        .install(&git_source(&url, "", "main"), &InstallOptions::default())
        .await
        .unwrap();
    let listed = client
        .list_skills(Request::new(pb::ListSkillsRequest {}))
        .await
        .unwrap()
        .message;
    assert_eq!(listed.skills.len(), 2);
    let alpha = &listed.skills[0];
    assert_eq!(
        (alpha.name.as_str(), alpha.description.as_str()),
        ("alpha", "Alpha v1")
    );
    let Some(pb::skill::Source::Git(git)) = &alpha.source else {
        panic!("missing git source")
    };
    assert_eq!(git.provider, pb::GitProvider::Github as i32);
    assert_eq!(git.path, "skills/alpha");
    assert_eq!(
        alpha.installed_at.as_ref().unwrap().seconds,
        installed[0].installed_at.0.timestamp()
    );
    let err = client
        .delete_skill(Request::new(pb::DeleteSkillRequest {
            name: "ghost".into(),
        }))
        .await
        .unwrap_err();
    assert_eq!(err.code, construct_connect::Code::Internal);
    assert_eq!(err.message, "skill \"ghost\" not found");
    let updated = client
        .update_skill(Request::new(pb::UpdateSkillRequest {
            name: Some("beta".into()),
        }))
        .await
        .unwrap()
        .message;
    assert_eq!(updated.updated_skills[0].name, "beta");
    client
        .delete_skill(Request::new(pb::DeleteSkillRequest {
            name: "beta".into(),
        }))
        .await
        .unwrap();
    assert_eq!(
        client
            .list_skills(Request::new(pb::ListSkillsRequest {}))
            .await
            .unwrap()
            .message
            .skills
            .len(),
        1
    );
}

#[tokio::test]
#[ignore = "network: clones github.com/anthropics/skills"]
async fn installs_from_github_over_https() {
    let tmp = tempfile::tempdir().unwrap();
    let (manager, config) = manager(tmp.path());
    let opts = InstallOptions {
        force: false,
        skill_names: vec!["pdf".into()],
    };
    let installed = manager
        .install(&parse_source("anthropics/skills").unwrap(), &opts)
        .await
        .unwrap();
    assert_eq!(installed[0].skill.name, "pdf");
    assert_eq!(installed[0].git.as_ref().unwrap().path, "skills/pdf");
    assert_eq!(installed[0].git.as_ref().unwrap().tree_hash.len(), 40);
    assert!(config.join("skills/pdf/SKILL.md").exists());
}
