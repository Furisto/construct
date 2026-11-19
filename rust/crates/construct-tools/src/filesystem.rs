//! Filesystem tools. Absolute paths are not confined to the workspace.
use crate::{custom, decode, description, schema, standard};
use construct_core::tool::{Tool, ToolContext, ToolError, ToolErrorCode};
use construct_types::tools::*;
use serde_json::{json, Value};
use std::{fs, path::Path};

mod edit;
mod search;

macro_rules! tool {
    ($ty:ident, $name:literal, $input:ty, $field:ident, $run:path) => {
        #[doc = concat!("Go's `", $name, "` tool using the host filesystem.")]
        #[derive(Clone, Copy, Debug, Default)]
        pub struct $ty;
        #[construct_core::async_trait]
        impl Tool for $ty {
            fn name(&self) -> &str {
                $name
            }
            fn description(&self) -> &str {
                description($name)
            }
            fn input_schema(&self) -> Value {
                schema($name)
            }
            async fn execute(
                &self,
                _ctx: &ToolContext,
                input: Value,
            ) -> Result<ToolOutput, ToolError> {
                let input: $input = decode(input)?;
                let result = tokio::task::spawn_blocking(move || $run(&input))
                    .await
                    .map_err(|e| custom(e.to_string(), &[], &[]))??;
                Ok(ToolOutput {
                    $field: Some(Box::new(result)),
                    ..Default::default()
                })
            }
        }
    };
}
tool!(
    CreateFile,
    "create_file",
    CreateFileInput,
    create_file,
    create
);
tool!(ReadFile, "read_file", ReadFileInput, read_file, read);
tool!(ListFiles, "list_files", ListFilesInput, list_files, list);
tool!(EditFile, "edit_file", EditFileInput, edit_file, edit::edit);
pub use search::{FindFile, Grep};

fn absolute(path: &str) -> Result<(), ToolError> {
    if Path::new(path).is_absolute() {
        Ok(())
    } else {
        Err(standard(ToolErrorCode::PathIsNotAbsolute, path))
    }
}

fn file_error(
    message: &str,
    suggestion: &str,
    path: &str,
    error: impl std::fmt::Display,
) -> ToolError {
    custom(
        message,
        &[suggestion],
        &[("path", path.into()), ("error", error.to_string().into())],
    )
}

fn stat(path: &str, directory: bool) -> Result<fs::Metadata, ToolError> {
    fs::metadata(path).map_err(|e| {
        standard(
            match e.kind() {
                std::io::ErrorKind::NotFound => {
                    if directory {
                        ToolErrorCode::DirectoryNotFound
                    } else {
                        ToolErrorCode::FileNotFound
                    }
                }
                std::io::ErrorKind::PermissionDenied => ToolErrorCode::PermissionDenied,
                _ => ToolErrorCode::CannotStatFile,
            },
            path,
        )
    })
}

fn create(input: &CreateFileInput) -> Result<CreateFileResult, ToolError> {
    absolute(&input.path)?;
    let metadata = fs::metadata(&input.path).ok();
    if metadata.as_ref().is_some_and(fs::Metadata::is_dir) {
        return Err(standard(ToolErrorCode::PathIsDirectory, &input.path));
    }
    if let Some(parent) = Path::new(&input.path).parent() {
        fs::create_dir_all(parent).map_err(|e| {
            custom(
                "could not create the parent directory",
                &[
                    "Verify that you have the permissions to create the parent directories",
                    "Create the missing parent directories manually",
                ],
                &[
                    ("path", input.path.clone().into()),
                    ("error", e.to_string().into()),
                ],
            )
        })?;
    }
    write_file(&input.path, input.content.as_bytes()).map_err(|e| {
        file_error(
            "error writing file",
            "Ensure that you have the permission to write to the file",
            &input.path,
            e,
        )
    })?;
    Ok(CreateFileResult {
        overwritten: metadata.is_some(),
    })
}

fn write_file(path: &str, bytes: &[u8]) -> std::io::Result<()> {
    use std::io::Write;
    let mut options = fs::OpenOptions::new();
    options.write(true).create(true).truncate(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o644);
    }
    options.open(path)?.write_all(bytes)
}

fn read(input: &ReadFileInput) -> Result<ReadFileResult, ToolError> {
    if input.path.is_empty() {
        return Err(custom(
            "path is required",
            &["Please provide a valid path to the file you want to read"],
            &[],
        ));
    }
    if !Path::new(&input.path).is_absolute() {
        return Err(custom(
            "path must be absolute",
            &["Please provide a valid absolute path to the file you want to read"],
            &[],
        ));
    }
    let start = input.start_line.as_deref().copied().unwrap_or(1);
    let end = input.end_line.as_deref().copied();
    for (name, value) in [("start_line", Some(start)), ("end_line", end)] {
        if value.is_some_and(|n| n < 1) {
            return Err(custom(
                format!("{name} must be positive"),
                &[&format!(
                    "Please provide {} {name} value of 1 or greater",
                    if name == "end_line" { "an" } else { "a" }
                )],
                &[],
            ));
        }
    }
    if end.is_some_and(|n| start > n) {
        return Err(custom(
            "start_line must be less than or equal to end_line",
            &["Please ensure start_line <= end_line"],
            &[],
        ));
    }
    if stat(&input.path, false)?.is_dir() {
        return Err(standard(ToolErrorCode::PathIsDirectory, &input.path));
    }
    let bytes = fs::read(&input.path).map_err(|e| {
        file_error(
            "error reading file",
            "Verify that you have the permission to read the file",
            &input.path,
            e,
        )
    })?;
    let mut lines: Vec<&[u8]> = bytes.split(|b| *b == b'\n').collect();
    if bytes.is_empty() || bytes.ends_with(b"\n") {
        lines.pop();
    }
    if lines.iter().any(|l| l.len() >= 65536) {
        return Err(file_error(
            "error reading file",
            "Verify that you have the permission to read the file",
            &input.path,
            "bufio.Scanner: token too long",
        ));
    }
    let total = lines.len() as i64;
    let selected: Vec<String> = lines
        .iter()
        .enumerate()
        .filter(|(i, _)| (*i as i64 + 1) >= start && end.is_none_or(|e| (*i as i64 + 1) <= e))
        .map(|(_, l)| crate::go_string(l.strip_suffix(b"\r").unwrap_or(l)))
        .collect();
    let mut content = String::new();
    if !selected.is_empty() {
        if start > 1 {
            content.push_str(&format!("// skipped {} lines\n", start - 1));
        }
        content.push_str(&selected.join("\n"));
        if let Some(end) = end {
            content.push_str(&format!("\n// {} lines remaining", (total - end).max(0)));
        }
    }
    Ok(ReadFileResult {
        path: input.path.clone(),
        content,
    })
}

fn list(input: &ListFilesInput) -> Result<ListFilesResult, ToolError> {
    absolute(&input.path)?;
    if !stat(&input.path, true)?.is_dir() {
        return Err(standard(ToolErrorCode::PathIsNotDirectory, &input.path));
    }
    fn walk(
        path: &Path,
        recursive: bool,
        entries: &mut Vec<DirectoryEntry>,
    ) -> std::io::Result<()> {
        let mut children = fs::read_dir(path)?.collect::<Result<Vec<_>, _>>()?;
        children.sort_by_key(|e| e.file_name());
        for child in children {
            let info = fs::symlink_metadata(child.path())?;
            entries.push(DirectoryEntry {
                name: child.path().to_string_lossy().into_owned(),
                r#type: if info.is_dir() { "d" } else { "f" }.into(),
                size: if info.is_dir() {
                    0
                } else {
                    info.len().div_ceil(1024) as i64
                },
            });
            if recursive && info.is_dir() {
                walk(&child.path(), true, entries)?;
            }
        }
        Ok(())
    }
    let mut entries = Vec::new();
    walk(Path::new(&input.path), input.recursive, &mut entries).map_err(|e| {
        if e.kind() == std::io::ErrorKind::PermissionDenied {
            standard(ToolErrorCode::PermissionDenied, &input.path)
        } else {
            let mut error = standard(ToolErrorCode::GenericFileError, &input.path);
            error.details.insert("error".into(), json!(e.to_string()));
            error
        }
    })?;
    Ok(ListFilesResult {
        path: input.path.clone(),
        entries: Some(entries),
    })
}
