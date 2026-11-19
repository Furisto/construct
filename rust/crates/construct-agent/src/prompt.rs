//! Embedded Go instructions and the restricted text/template syntax they use.
use crate::{environment::DevTools, Result};
use std::collections::BTreeMap;

pub const EDIT: &str = include_str!("../../../../backend/prompt/edit.md");
pub const PLAN: &str = include_str!("../../../../backend/prompt/plan.md");
pub const SUMMARY: &str = include_str!("../../../../backend/prompt/summary.md");
pub const TOOL_INSTRUCTIONS: &str = include_str!("../../../../backend/prompt/tools.md");

#[derive(Default)]
pub struct PromptContext {
    pub working_directory: String,
    pub operating_system: String,
    pub default_shell: String,
    pub project_structure: String,
    pub tools: Vec<(String, String)>,
    pub dev_tools: DevTools,
    pub skills: Vec<construct_skill::Skill>,
}

#[derive(Debug)]
enum Node {
    Text(String),
    Value(String),
    If(String, Vec<Node>),
    Range(String, Vec<Node>),
}

fn parse(input: &str) -> Result<Vec<Node>> {
    let mut tokens = Vec::new();
    let mut rest = input;
    while let Some(start) = rest.find("{{") {
        let mut text = rest[..start].to_owned();
        rest = &rest[start + 2..];
        if let Some(stripped) = rest.strip_prefix('-') {
            text = text.trim_end_matches([' ', '\n', '\r', '\t']).to_owned();
            rest = stripped;
        }
        tokens.push((false, text));
        let end = rest.find("}}").ok_or("unclosed template action")?;
        let action = rest[..end].trim();
        if action.ends_with('-') {
            return Err("unsupported template syntax: right trim".into());
        }
        tokens.push((true, action.into()));
        rest = &rest[end + 2..];
    }
    tokens.push((false, rest.into()));
    fn nodes(tokens: &[(bool, String)], index: &mut usize, nested: bool) -> Result<Vec<Node>> {
        let mut result = Vec::new();
        while let Some((action, value)) = tokens.get(*index) {
            *index += 1;
            if !action {
                result.push(Node::Text(value.clone()));
                continue;
            }
            if value == "end" {
                return if nested {
                    Ok(result)
                } else {
                    Err("unexpected template end".into())
                };
            }
            if let Some(condition) = value.strip_prefix("if ") {
                result.push(Node::If(condition.into(), nodes(tokens, index, true)?));
            } else if let Some(field) = value.strip_prefix("range $i, $tool := ") {
                if !field.starts_with(".DevTools.") {
                    return Err(format!("unsupported template range: {field}").into());
                }
                result.push(Node::Range(field.into(), nodes(tokens, index, true)?));
            } else if value.starts_with('.') || value == "$tool" {
                result.push(Node::Value(value.clone()));
            } else {
                return Err(format!("unsupported template syntax: {value}").into());
            }
        }
        if nested {
            Err("unclosed template block".into())
        } else {
            Ok(result)
        }
    }
    nodes(&tokens, &mut 0, false)
}

pub fn assemble_system_prompt(instructions: &str, context: &PromptContext) -> Result<String> {
    let tools = context
        .tools
        .iter()
        .map(|(name, description)| format!("# {name}\n{description}\n\n"))
        .collect::<String>();
    let values = BTreeMap::from([
        (".CurrentTime", String::new()),
        (".WorkingDirectory", context.working_directory.clone()),
        (".OperatingSystem", context.operating_system.clone()),
        (".DefaultShell", context.default_shell.clone()),
        (".ProjectStructure", context.project_structure.clone()),
        (
            ".ToolInstructions",
            if tools.is_empty() {
                String::new()
            } else {
                TOOL_INSTRUCTIONS.into()
            },
        ),
        (".Tools", tools),
        (
            ".AvailableSkills",
            construct_skill::format_available_skills(&context.skills),
        ),
    ]);
    fn render(
        nodes: &[Node],
        values: &BTreeMap<&str, String>,
        tools: &DevTools,
        item: Option<(usize, &str)>,
        out: &mut String,
    ) -> Result<()> {
        for node in nodes {
            match node {
                Node::Text(text) => out.push_str(text),
                Node::Value(field) => {
                    if field == "$tool" {
                        out.push_str(item.ok_or("$tool outside range")?.1);
                    } else {
                        out.push_str(
                            values
                                .get(field.as_str())
                                .ok_or_else(|| format!("unsupported template field: {field}"))?,
                        );
                    }
                }
                Node::If(field, body) => {
                    let enabled = if field == "$i" {
                        item.ok_or("$i outside range")?.0 != 0
                    } else if let Some(category) = field.strip_prefix(".DevTools.") {
                        if !crate::environment::TOOL_CATEGORIES
                            .iter()
                            .any(|(name, _)| *name == category)
                        {
                            return Err(format!("unknown DevTools field: {category}").into());
                        }
                        tools.get(category).is_some_and(|v| !v.is_empty())
                    } else {
                        return Err(format!("unsupported template condition: {field}").into());
                    };
                    // Validate unsupported syntax even in a false branch.
                    let mut text = String::new();
                    render(body, values, tools, item, &mut text)?;
                    if enabled {
                        out.push_str(&text);
                    }
                }
                Node::Range(field, body) => {
                    let category = field
                        .strip_prefix(".DevTools.")
                        .ok_or("unsupported range")?;
                    if !crate::environment::TOOL_CATEGORIES
                        .iter()
                        .any(|(name, _)| *name == category)
                    {
                        return Err(format!("unknown DevTools field: {category}").into());
                    }
                    let mut validation = String::new();
                    render(body, values, tools, Some((0, "")), &mut validation)?;
                    for (index, tool) in tools.get(category).into_iter().flatten().enumerate() {
                        render(body, values, tools, Some((index, tool)), out)?;
                    }
                }
            }
        }
        Ok(())
    }
    let mut out = String::new();
    render(
        &parse(instructions)?,
        &values,
        &context.dev_tools,
        None,
        &mut out,
    )?;
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn empty_and_unsupported() {
        let ctx = PromptContext::default();
        assert_eq!(assemble_system_prompt("", &ctx).unwrap(), "");
        for template in [
            "{{ bad }}",
            "{{ .Unknown }}",
            "{{if .Bad}}x{{end}}",
            "{{if .DevTools.Testing}}{{call foo}}{{end}}",
            "{{if $i}}x{{end}}",
            "{{",
            "{{end}}",
        ] {
            assert!(
                assemble_system_prompt(template, &ctx).is_err(),
                "{template}"
            );
        }
        assert!(!assemble_system_prompt(EDIT, &ctx)
            .unwrap()
            .contains("Version Control:"));
    }
}
