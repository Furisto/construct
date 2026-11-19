use rquickjs::{object::Property, Ctx, Object, Value};

const STRUCT_KIND: &str = "__construct_go_struct_kind";

// Sobek Export retains Go structs (and their declaration order), unlike a JS
// object literal. Non-enumerable metadata survives aliases but not JS copies.
pub(crate) fn tag(value: &Value<'_>, kind: &str) -> rquickjs::Result<()> {
    let Some(object) = value.as_object() else {
        return Ok(());
    };
    object.prop(STRUCT_KIND, Property::from(kind))?;
    if let Some(element_kind) = kind.strip_prefix("[]") {
        if !fields(element_kind).is_empty() {
            if let Some(array) = value.as_array() {
                for item in array.iter::<Value>() {
                    tag(&item?, &format!("&{element_kind}"))?;
                }
            }
        }
        return Ok(());
    }
    for &(field, field_kind) in fields(kind.trim_start_matches('&')) {
        let value: Value = object.get(field)?;
        if field_kind.starts_with("[]") {
            tag(&value, field_kind)?;
        } else if !fields(field_kind).is_empty() {
            tag(&value, &format!("&{field_kind}"))?;
        }
    }
    Ok(())
}

pub(crate) fn format<'js>(
    ctx: &Ctx<'js>,
    object: &Object<'js>,
) -> rquickjs::Result<Option<String>> {
    let kind: Option<String> = object.get(STRUCT_KIND)?;
    let Some(kind) = kind else {
        return Ok(None);
    };
    if kind.starts_with("[]") {
        return format_as(ctx, object.as_value(), &kind).map(Some);
    }
    if fields(kind.trim_start_matches('&')).is_empty() {
        return Ok(None);
    }
    format_struct(ctx, object, &kind).map(Some)
}

fn format_struct<'js>(
    ctx: &Ctx<'js>,
    object: &Object<'js>,
    kind: &str,
) -> rquickjs::Result<String> {
    let mut values = Vec::new();
    for &(field, field_kind) in fields(kind.trim_start_matches('&')) {
        let value: Value = object.get(field)?;
        let text = if value.is_undefined() || value.is_null() {
            match field_kind {
                "string" => String::new(),
                "int" => "0".into(),
                "bool" => "false".into(),
                kind if kind.starts_with("[]") => "[]".into(),
                _ => crate::bindings::format_value(ctx, &value)?,
            }
        } else {
            format_as(ctx, &value, field_kind)?
        };
        values.push(text);
    }
    Ok(format!(
        "{}{{{}}}",
        if kind.starts_with('&') { "&" } else { "" },
        values.join(" ")
    ))
}

fn format_as<'js>(ctx: &Ctx<'js>, value: &Value<'js>, kind: &str) -> rquickjs::Result<String> {
    if let Some(element_kind) = kind.strip_prefix("[]") {
        let mut elements = Vec::new();
        if let Some(array) = value.as_array() {
            for item in array.iter::<Value>() {
                elements.push(format_as(ctx, &item?, element_kind)?);
            }
        }
        return Ok(format!("[{}]", elements.join(" ")));
    }
    if !fields(kind).is_empty() {
        if let Some(object) = value.as_object() {
            return format_struct(ctx, object, kind);
        }
    }
    crate::bindings::format_value(ctx, value)
}

fn fields(kind: &str) -> &'static [(&'static str, &'static str)] {
    match kind {
        "create_file" => &[("overwritten", "bool")],
        "read_file" => &[("path", "string"), ("content", "string")],
        "edit_file" => &[
            ("success", "bool"),
            ("path", "string"),
            ("replacements_made", "int"),
            ("expected_replacements", "int"),
            ("failure_reason", "string"),
            ("validation_errors", "[]validation_error"),
            ("conflict_warnings", "[]conflict_warning"),
            ("patch_info", "patch_info"),
        ],
        "validation_error" => &[
            ("diff_index", "int"),
            ("error_type", "string"),
            ("error_message", "string"),
            ("suggested_fix", "string"),
        ],
        "conflict_warning" => &[
            ("edit1_index", "int"),
            ("edit2_index", "int"),
            ("conflict_type", "string"),
            ("message", "string"),
        ],
        "patch_info" => &[
            ("patch", "string"),
            ("lines_added", "int"),
            ("lines_removed", "int"),
        ],
        "list_files" => &[("path", "string"), ("entries", "[]directory_entry")],
        "directory_entry" => &[("n", "string"), ("t", "string"), ("s", "int")],
        "find_file" => &[
            ("files", "[]string"),
            ("total_files", "int"),
            ("truncated_count", "int"),
        ],
        "grep" => &[
            ("matches", "[]grep_match"),
            ("total_matches", "int"),
            ("truncated_matches", "int"),
            ("searched_files", "int"),
        ],
        "grep_match" => &[("file_path", "string"), ("value", "string")],
        "execute_command" => &[
            ("stdout", "string"),
            ("stderr", "string"),
            ("exitCode", "int"),
            ("command", "string"),
        ],
        "fetch" => &[
            ("url", "string"),
            ("title", "string"),
            ("content", "string"),
            ("content_type", "string"),
            ("byte_size", "int"),
            ("truncated", "bool"),
        ],
        "submit_report" => &[
            ("summary", "string"),
            ("completed", "bool"),
            ("deliverables", "[]string"),
            ("next_steps", "string"),
        ],
        _ => &[],
    }
}
