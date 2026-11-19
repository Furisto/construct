use super::*;

pub(super) fn edit(input: &EditFileInput) -> Result<EditFileResult, ToolError> {
    absolute(&input.path)?;
    let info = fs::metadata(&input.path).map_err(|e| {
        if e.kind() == std::io::ErrorKind::NotFound {
            standard(ToolErrorCode::FileNotFound, &input.path)
        } else {
            file_error(
                "error accessing file",
                "Verify that you have the permission to access the file",
                &input.path,
                e,
            )
        }
    })?;
    if info.is_dir() {
        return Err(standard(ToolErrorCode::PathIsDirectory, &input.path));
    }
    let original = fs::read(&input.path).map_err(|e| {
        file_error(
            "error reading file",
            "Verify that you have the permission to read the file",
            &input.path,
            e,
        )
    })?;
    let diffs = input.diffs.as_deref().unwrap_or_default();
    let mut content = original.clone();
    let mut replacements = 0;
    let mut errors = Vec::new();
    for (i, diff) in diffs.iter().enumerate() {
        if diff.old.is_empty() && diff.new.is_empty() {
            continue;
        }
        let failure = if diff.old == diff.new {
            Some((
                "no_op",
                "old and new text are identical",
                "Remove this diff or provide different new text",
            ))
        } else if diff.old.is_empty() {
            content.extend_from_slice(diff.new.as_bytes());
            replacements += 1;
            None
        } else if let Some(start) = content
            .windows(diff.old.len())
            .position(|part| part == diff.old.as_bytes())
        {
            content.splice(start..start + diff.old.len(), diff.new.bytes());
            replacements += 1;
            None
        } else if let Some((start, end)) = trimmed_match(&content, diff.old.as_bytes()) {
            content.splice(start..end, diff.new.bytes());
            replacements += 1;
            None
        } else {
            Some(("not_found", "old text not found in file", "Check that the old text exactly matches the file content, including whitespace and indentation"))
        };
        if let Some((kind, message, fix)) = failure {
            errors.push(DiffValidationError {
                diff_index: i as i64,
                error_type: kind.into(),
                error_message: message.into(),
                suggested_fix: fix.into(),
            });
        }
    }
    if !errors.is_empty() {
        return Err(custom(
            "validation failed",
            &["Please fix the validation errors and try again"],
            &[
                ("path", input.path.clone().into()),
                ("validation_errors", json!(errors)),
            ],
        ));
    }
    let mut patch_info = PatchInfo::default();
    if content != original {
        let name = Path::new(&input.path)
            .file_name()
            .unwrap_or_default()
            .to_string_lossy();
        patch_info.patch = similar::TextDiff::from_lines(
            &crate::go_string(&original),
            &crate::go_string(&content),
        )
        .unified_diff()
        .context_radius(3)
        .header(&format!("a/{name}"), &format!("b/{name}"))
        .to_string();
        patch_info.patch = patch_info
            .patch
            .split_inclusive('\n')
            .map(|line| {
                if !line.starts_with("@@ ") {
                    return line.to_owned();
                }
                let parts: Vec<_> = line.split_whitespace().collect();
                let range = |s: &str| {
                    if s.contains(',') {
                        s.to_owned()
                    } else {
                        format!("{s},1")
                    }
                };
                format!("@@ {} {} @@\n", range(parts[1]), range(parts[2]))
            })
            .collect();
        for line in patch_info.patch.lines() {
            if line.starts_with('+') && !line.starts_with("+++") {
                patch_info.lines_added += 1;
            }
            if line.starts_with('-') && !line.starts_with("---") {
                patch_info.lines_removed += 1;
            }
        }
        fs::write(&input.path, &content).map_err(|e| {
            file_error(
                "error writing file",
                "Verify that you have the permission to write to the file",
                &input.path,
                e,
            )
        })?;
    }
    Ok(EditFileResult {
        success: replacements == diffs.len() as i64,
        path: input.path.clone(),
        replacements_made: replacements,
        expected_replacements: diffs.len() as i64,
        conflict_warnings: Some(conflicts(diffs)),
        patch_info,
        ..Default::default()
    })
}

fn trimmed_match(original: &[u8], search: &[u8]) -> Option<(usize, usize)> {
    let lines: Vec<_> = original.split(|b| *b == b'\n').collect();
    let mut target: Vec<_> = search.split(|b| *b == b'\n').collect();
    if target.last().is_some_and(|line| line.is_empty()) {
        target.pop();
    }
    if target.is_empty() || target.len() > lines.len() {
        return None;
    }
    let mut start = 0;
    for i in 0..=lines.len() - target.len() {
        if lines[i..i + target.len()]
            .iter()
            .zip(&target)
            .all(|(a, b)| trim_bytes(a) == trim_bytes(b))
        {
            let end = start
                + lines[i..i + target.len()]
                    .iter()
                    .map(|l| l.len() + 1)
                    .sum::<usize>();
            return Some((start, end.min(original.len())));
        }
        start += lines[i].len() + 1;
    }
    None
}

fn trim_bytes(mut bytes: &[u8]) -> &[u8] {
    loop {
        let first = (1..=4.min(bytes.len())).find_map(|n| {
            std::str::from_utf8(&bytes[..n])
                .ok()
                .and_then(|s| s.chars().next().filter(|ch| ch.is_whitespace()).map(|_| n))
        });
        if let Some(n) = first {
            bytes = &bytes[n..];
        } else {
            break;
        }
    }
    loop {
        let last = (1..=4.min(bytes.len())).find_map(|n| {
            std::str::from_utf8(&bytes[bytes.len() - n..])
                .ok()
                .and_then(|s| s.chars().next().filter(|ch| ch.is_whitespace()).map(|_| n))
        });
        if let Some(n) = last {
            bytes = &bytes[..bytes.len() - n];
        } else {
            break;
        }
    }
    bytes
}

fn conflicts(diffs: &[DiffPair]) -> Vec<ConflictWarning> {
    let mut warnings = Vec::new();
    for (i, a) in diffs.iter().enumerate() {
        for (j, b) in diffs.iter().enumerate().skip(i + 1) {
            if (a.old.is_empty() && a.new.is_empty()) || (b.old.is_empty() && b.new.is_empty()) {
                continue;
            }
            let mut add = |kind: &str, message: String| {
                warnings.push(ConflictWarning {
                    edit1_index: i as i64 + 1,
                    edit2_index: j as i64 + 1,
                    conflict_type: kind.into(),
                    message,
                })
            };
            if !b.old.is_empty() && !a.new.is_empty() && b.old.contains(&a.new) {
                add(
                    "dependency",
                    format!("Edit {} depends on result of edit {}", j + 1, i + 1),
                );
            }
            if !a.old.is_empty() && !b.old.is_empty() {
                if a.old.contains(&b.old) || b.old.contains(&a.old) {
                    add(
                        "overlap",
                        format!(
                            "Edits {} and {} may affect overlapping text regions",
                            i + 1,
                            j + 1
                        ),
                    );
                }
                if a.old == b.old {
                    add(
                        "duplicate_target",
                        format!("Edits {} and {} target the same text", i + 1, j + 1),
                    );
                }
                if a.old.contains('\n') && b.old.contains('\n') {
                    for line in a.old.split('\n') {
                        if !line.trim().is_empty()
                            && b.old.split('\n').any(|l| l.trim() == line.trim())
                        {
                            add(
                                "line_overlap",
                                format!("Edits {} and {} may affect the same line", i + 1, j + 1),
                            );
                        }
                    }
                }
            }
        }
    }
    warnings
}

#[cfg(test)]
mod tests {
    fn normalize(input: &str) -> String {
        let mut chars = input.chars();
        let mut delimiter = None;
        let mut result = String::new();
        while let Some(ch) = chars.next() {
            if ch == '\\' && delimiter.is_some() {
                result.push(ch);
                if let Some(next) = chars.next() {
                    result.push(next);
                }
                continue;
            }
            if matches!(ch, '"' | '\'' | '`') {
                if delimiter == Some(ch) {
                    delimiter = None;
                } else if delimiter.is_none() {
                    delimiter = Some(ch);
                }
                result.push(ch);
                continue;
            }
            match (delimiter.is_some(), ch) {
                (true, '\n') => result.push_str("\\n"),
                (true, '\t') => result.push_str("\\t"),
                (true, '\r') => result.push_str("\\r"),
                _ => result.push(ch),
            }
        }
        result
    }
    #[test]
    fn go_normalize_escapes_13_cases_unused_by_edit() {
        for (input, expected) in [
            ("\"hello\nworld\"", "\"hello\\nworld\""),
            ("\"hello\tworld\"", "\"hello\\tworld\""),
            ("\"hello\rworld\"", "\"hello\\rworld\""),
            ("\"line1\nline2\ttabbed\"", "\"line1\\nline2\\ttabbed\""),
            ("\"hello\\nworld\"", "\"hello\\nworld\""),
            (
                "func test() {\n\treturn \"hello\nworld\"\n}",
                "func test() {\n\treturn \"hello\\nworld\"\n}",
            ),
            ("'hello\nworld'", "'hello\\nworld'"),
            ("`hello\nworld`", "`hello\\nworld`"),
            ("fmt.Println(\"hello\")", "fmt.Println(\"hello\")"),
            (
                "\"first\nstring\" and 'second\tstring'",
                "\"first\\nstring\" and 'second\\tstring'",
            ),
            ("\"\"", "\"\""),
            ("\"already\\nescaped\"", "\"already\\nescaped\""),
            ("\"🤖 Agent\nRuntime\"", "\"🤖 Agent\\nRuntime\""),
        ] {
            assert_eq!(normalize(input), expected);
        }
    }
}
