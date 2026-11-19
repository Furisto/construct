use super::*;
use std::collections::{BTreeMap, BTreeSet};
use tokio::process::Command;

/// Go find-file tool; uses ripgrep when available, glob matching otherwise.
#[derive(Clone, Copy, Debug, Default)]
pub struct FindFile;
/// Go grep tool; uses ripgrep when available, system grep otherwise.
#[derive(Clone, Copy, Debug, Default)]
pub struct Grep;

fn ripgrep_available() -> bool {
    std::env::var_os("PATH").is_some_and(|p| {
        std::env::split_paths(&p).any(|d| {
            let path = d.join("rg");
            if !path.is_file() {
                return false;
            }
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                path.metadata()
                    .is_ok_and(|m| m.permissions().mode() & 0o111 != 0)
            }
            #[cfg(not(unix))]
            {
                true
            }
        })
    })
}

#[construct_core::async_trait]
impl Tool for FindFile {
    fn name(&self) -> &str {
        "find_file"
    }
    fn description(&self) -> &str {
        description(self.name())
    }
    fn input_schema(&self) -> Value {
        schema(self.name())
    }
    async fn execute(&self, ctx: &ToolContext, value: Value) -> Result<ToolOutput, ToolError> {
        let input: FindFileInput = decode(value)?;
        if input.path.is_empty() || input.pattern.is_empty() {
            return Err(custom(
                "pattern and path are required",
                &["Please provide a valid glob pattern and absolute path"],
                &[],
            ));
        }
        absolute(&input.path)?;
        let files = if ripgrep_available() {
            let mut cmd = Command::new("rg");
            cmd.args(["--files", "--null", "--glob", &input.pattern]);
            if !input.exclude_pattern.is_empty() {
                cmd.args(["--glob", &format!("!{}", input.exclude_pattern)]);
            }
            cmd.arg(&input.path);
            let output = run(ctx, cmd, "ripgrep error", true).await?;
            crate::go_string(&output)
                .trim_end_matches('\0')
                .split('\0')
                .filter(|s| !s.is_empty())
                .map(str::to_owned)
                .collect::<Vec<_>>()
        } else {
            let input = input.clone();
            tokio::task::spawn_blocking(move || fallback_find(&input))
                .await
                .map_err(|e| custom(e.to_string(), &[], &[]))??
        };
        let total = files.len();
        let limit = usize::try_from(input.max_results).unwrap_or(0).min(total);
        let files = files.into_iter().take(limit).collect();
        Ok(ToolOutput {
            find_file: Some(Box::new(FindFileResult {
                files: Some(files),
                total_files: limit as i64,
                truncated_count: (total - limit) as i64,
            })),
            ..Default::default()
        })
    }
}

fn fallback_find(input: &FindFileInput) -> Result<Vec<String>, ToolError> {
    let matcher = globset::GlobBuilder::new(&input.pattern)
        .literal_separator(true)
        .build()
        .map_err(|_e| {
            custom(
                "glob pattern error",
                &["Check that your glob pattern is valid"],
                &[
                    ("pattern", input.pattern.clone().into()),
                    ("error", "syntax error in pattern".into()),
                ],
            )
        })?
        .compile_matcher();
    let exclude = globset::GlobBuilder::new(&input.exclude_pattern)
        .literal_separator(true)
        .build()
        .ok()
        .map(|g| g.compile_matcher());
    let mut files = Vec::new();
    fn visit(
        root: &Path,
        path: &Path,
        matcher: &globset::GlobMatcher,
        exclude: &Option<globset::GlobMatcher>,
        files: &mut Vec<String>,
    ) {
        let Ok(entries) = fs::read_dir(path) else {
            return;
        };
        let mut entries: Vec<_> = entries.flatten().collect();
        entries.sort_by_key(|e| e.file_name());
        for e in entries {
            let path = e.path();
            if path.is_dir() {
                visit(root, &path, matcher, exclude, files);
            } else if matcher.is_match(path.strip_prefix(root).unwrap_or(&path))
                && !exclude.as_ref().is_some_and(|g| g.is_match(&path))
            {
                files.push(path.to_string_lossy().into_owned());
            }
        }
    }
    visit(
        Path::new(&input.path),
        Path::new(&input.path),
        &matcher,
        &exclude,
        &mut files,
    );
    Ok(files)
}

async fn run(
    ctx: &ToolContext,
    mut cmd: Command,
    message: &str,
    plain_error: bool,
) -> Result<Vec<u8>, ToolError> {
    cmd.kill_on_drop(true);
    let fail = |error: String| {
        if plain_error {
            custom(format!("{message}: {error}"), &[], &[])
        } else {
            custom(
                message,
                &[
                    "Check that your regex pattern is valid",
                    "Verify that the search path exists and is accessible",
                ],
                &[("error", error.into())],
            )
        }
    };
    let output = tokio::select! {
        result = cmd.output() => result.map_err(|e|fail(e.to_string()))?,
        () = ctx.cancellation.cancelled() => return Err(fail("context canceled".into())),
    };
    if output.status.success() {
        return Ok(output.stdout);
    }
    if output.status.code() == Some(1) {
        return Ok(Vec::new());
    }
    Err(fail(crate::process_error(output.status)))
}

#[construct_core::async_trait]
impl Tool for Grep {
    fn name(&self) -> &str {
        "grep"
    }
    fn description(&self) -> &str {
        description(self.name())
    }
    fn input_schema(&self) -> Value {
        schema(self.name())
    }
    async fn execute(&self, ctx: &ToolContext, value: Value) -> Result<ToolOutput, ToolError> {
        let mut input: GrepInput = decode(value)?;
        if input.query.is_empty() || input.path.is_empty() {
            return Err(custom(
                "query and path are required",
                &["Provide both a search query and a path to search in"],
                &[],
            ));
        }
        if input.max_results == 0 {
            input.max_results = 50;
        }
        if input.context == 0 {
            input.context = 2;
        }
        let rg = ripgrep_available();
        let mut cmd = Command::new(if rg { "rg" } else { "grep" });
        if rg {
            cmd.args([
                "--json",
                "--line-number",
                "--with-filename",
                "--context",
                &input.context.to_string(),
            ]);
            if !input.case_sensitive {
                cmd.arg("--ignore-case");
            }
            if !input.include_pattern.is_empty() {
                cmd.args(["--glob", &input.include_pattern]);
            }
            if !input.exclude_pattern.is_empty() {
                cmd.args(["--glob", &format!("!{}", input.exclude_pattern)]);
            }
        } else {
            cmd.args(["-r", "-n", "-H", "-C", &input.context.to_string()]);
            if !input.case_sensitive {
                cmd.arg("-i");
            }
            if !input.include_pattern.is_empty() {
                cmd.arg(format!("--include={}", input.include_pattern));
            }
            if !input.exclude_pattern.is_empty() {
                cmd.arg(format!("--exclude={}", input.exclude_pattern));
            }
        }
        cmd.args([&input.query, &input.path]);
        let output = run(
            ctx,
            cmd,
            if rg { "ripgrep error" } else { "grep error" },
            false,
        )
        .await?;
        let output = crate::go_string(&output);
        let result = if rg {
            parse_ripgrep(&output, input.max_results, input.context)
        } else {
            parse_grep(&output, input.max_results, input.context)
        };
        Ok(ToolOutput {
            grep: Some(Box::new(result)),
            ..Default::default()
        })
    }
}

#[derive(Clone)]
struct Entry {
    line: i64,
    text: String,
    matched: bool,
}

fn groups(path: &str, entries: &[Entry], context: i64) -> Vec<GrepMatch> {
    let mut result = Vec::new();
    let mut start = 0;
    for end in 1..=entries.len() {
        if end == entries.len() || entries[end].line - entries[end - 1].line > context * 2 {
            let group = &entries[start..end];
            if group.iter().any(|e| e.matched) {
                let value = group
                    .iter()
                    .map(|e| {
                        if e.matched {
                            format!(":{}:{}", e.line, e.text)
                        } else {
                            format!("-{}-{}", e.line, e.text)
                        }
                    })
                    .collect::<String>();
                result.push(GrepMatch {
                    file_path: path.into(),
                    value,
                });
            }
            start = end;
        }
    }
    result
}

fn limited(mut matches: Vec<GrepMatch>, searched: usize, max: i64) -> GrepResult {
    let total = matches.len();
    matches.truncate(usize::try_from(max).unwrap_or(0));
    GrepResult {
        total_matches: total as i64,
        truncated_matches: (total - matches.len()) as i64,
        matches: Some(matches),
        searched_files: searched as i64,
    }
}

pub(super) fn parse_ripgrep(output: &str, max: i64, context: i64) -> GrepResult {
    let mut files: BTreeMap<String, Vec<Entry>> = BTreeMap::new();
    let mut current = String::new();
    let mut matches = Vec::new();
    for line in output.trim().lines() {
        let Ok(value) = serde_json::from_str::<Value>(line) else {
            continue;
        };
        match value["type"].as_str().unwrap_or("") {
            "begin" => {
                current = value["data"]["path"]["text"].as_str().unwrap_or("").into();
                files.entry(current.clone()).or_default();
            }
            "match" | "context" => {
                if !current.is_empty() {
                    if let Some(entries) = files.get_mut(&current) {
                        entries.push(Entry {
                            line: value["data"]["line_number"].as_i64().unwrap_or(0),
                            text: value["data"]["lines"]["text"].as_str().unwrap_or("").into(),
                            matched: value["type"] == "match",
                        });
                    }
                }
            }
            "end" => {
                if let Some(entries) = files.get(&current) {
                    matches.extend(groups(&current, entries, context));
                }
                current.clear();
            }
            _ => {}
        }
    }
    limited(matches, files.len(), max)
}

fn parse_grep(output: &str, max: i64, context: i64) -> GrepResult {
    let mut files: BTreeMap<String, Vec<Entry>> = BTreeMap::new();
    let mut searched = BTreeSet::new();
    for line in output.trim().lines() {
        if line.is_empty() || line.starts_with("--") {
            continue;
        }
        let matched = line.contains(':') && !line.contains(":-");
        let parts: Vec<_> = line.splitn(3, if matched { ':' } else { '-' }).collect();
        if parts.len() != 3 {
            continue;
        }
        let Ok(number) = parts[1].parse() else {
            continue;
        };
        if matched {
            searched.insert(parts[0]);
        }
        files.entry(parts[0].into()).or_default().push(Entry {
            line: number,
            text: parts[2].into(),
            matched,
        });
    }
    let mut matches = Vec::new();
    for (path, mut entries) in files {
        entries.sort_by_key(|e| e.line);
        matches.extend(groups(&path, &entries, context));
    }
    limited(matches, searched.len(), max)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn go_find_fallback_goldens() {
        let root = tempfile::tempdir().unwrap();
        fs::write(root.path().join("a.txt"), "hello").unwrap();
        let cases: Value =
            serde_json::from_str(include_str!("../../tests/fixtures/find_fallback.json")).unwrap();
        for case in cases.as_array().unwrap() {
            let mut input: FindFileInput = serde_json::from_value(case["input"].clone()).unwrap();
            input.path = root.path().to_string_lossy().into_owned();
            match fallback_find(&input) {
                Ok(files) => assert_eq!(
                    json!(files
                        .into_iter()
                        .map(|f| f.replace(root.path().to_str().unwrap(), "/workspace"))
                        .collect::<Vec<_>>()),
                    case["result"]["files"]
                ),
                Err(e) => assert_eq!(
                    json!({"Message":e.message,"Suggestions":e.suggestions,"Details":e.details}),
                    case["error"]
                ),
            }
        }
    }
    #[test]
    fn all_go_ripgrep_snapshots() {
        let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures");
        let mut count = 0;
        for entry in fs::read_dir(root).unwrap().flatten() {
            if !entry.file_name().to_string_lossy().starts_with("grep_") {
                continue;
            }
            let value: Value = serde_json::from_slice(&fs::read(entry.path()).unwrap()).unwrap();
            let result = parse_ripgrep(
                value["input"].as_str().unwrap(),
                value["max"].as_i64().unwrap(),
                value["context"].as_i64().unwrap(),
            );
            assert_eq!(json!(result), value["result"], "{:?}", entry.path());
            count += 1;
        }
        assert_eq!(count, 13);
    }
    #[test]
    fn fallback_grouping_and_truncation() {
        let output = "file-1-before\nfile:2:match\nfile-3-after\n--\nfile:20:other\n";
        let result = parse_grep(output, 1, 2);
        assert_eq!(result.total_matches, 2);
        assert_eq!(result.truncated_matches, 1);
        assert_eq!(result.searched_files, 1);
        assert_eq!(
            result.matches.unwrap()[0].value,
            "-1-before:2:match-3-after"
        );
        assert_eq!(parse_grep("", 50, 2).total_matches, 0);
    }
}
