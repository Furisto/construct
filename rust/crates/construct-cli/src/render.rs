use crate::Result;
use clap::{Args, ValueEnum};
use serde::Serialize;
use std::io::{IsTerminal, Write};

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, ValueEnum)]
pub enum OutputFormat {
    Json,
    Yaml,
    #[default]
    Table,
    Card,
    #[value(alias = "md")]
    Markdown,
}
#[derive(Clone, Debug, Default, Args)]
pub struct RenderOptions {
    #[arg(short = 'o', long = "output", value_enum, default_value = "table")]
    pub format: OutputFormat,
    #[arg(short = 'w', long)]
    pub wide: bool,
    #[arg(long)]
    pub no_headers: bool,
}
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Detail {
    #[default]
    Default,
    Full,
}
pub struct Field<'a> {
    pub name: &'a str,
    pub column: Option<&'a str>,
    pub value: String,
    pub detail: Detail,
}
impl<'a> Field<'a> {
    pub fn new(name: &'a str, value: impl ToString) -> Self {
        Self {
            name,
            column: None,
            value: value.to_string(),
            detail: Detail::Default,
        }
    }
    pub fn column(mut self, name: &'a str) -> Self {
        self.column = Some(name);
        self
    }
    pub fn full(mut self) -> Self {
        self.detail = Detail::Full;
        self
    }
}
pub trait DisplayResource: Serialize {
    fn fields(&self) -> Vec<Field<'_>>;
}
#[derive(Default)]
pub struct Renderer;
impl Renderer {
    pub fn render_one<T: DisplayResource>(
        &self,
        writer: &mut dyn Write,
        item: &T,
        options: &RenderOptions,
    ) -> Result<()> {
        match options.format {
            OutputFormat::Json => writeln!(writer, "{}", pretty_go_json(item)?)?,
            OutputFormat::Yaml => writeln!(writer, "{}", crate::yaml::to_string(item)?)?,
            _ => return self.render(writer, std::slice::from_ref(item), options),
        }
        Ok(())
    }
    pub fn render<T: DisplayResource>(
        &self,
        writer: &mut dyn Write,
        items: &[T],
        options: &RenderOptions,
    ) -> Result<()> {
        match options.format {
            OutputFormat::Json => {
                writeln!(writer, "{}", pretty_go_json(items)?)?;
            }
            OutputFormat::Yaml => writeln!(writer, "{}", crate::yaml::to_string(items)?)?,
            _ => {
                let rows: Vec<Vec<_>> = items
                    .iter()
                    .map(|i| {
                        i.fields()
                            .into_iter()
                            .filter(|f| options.wide || f.detail == Detail::Default)
                            .collect()
                    })
                    .collect();
                let Some(first) = rows.first().filter(|r| !r.is_empty()) else {
                    return Ok(());
                };
                match options.format {
                    OutputFormat::Table => {
                        let headers: Vec<_> =
                            first.iter().map(|f| f.column.unwrap_or(f.name)).collect();
                        let mut widths: Vec<_> = headers.iter().map(|h| h.len()).collect();
                        for row in &rows {
                            for (i, f) in row.iter().enumerate() {
                                widths[i] = widths[i].max(strip_ansi(&f.value).len());
                            }
                        }
                        if !options.no_headers {
                            let bold = std::io::stdout().is_terminal();
                            for (i, h) in headers.iter().enumerate() {
                                if i > 0 {
                                    write!(writer, "  ")?;
                                }
                                // Go's lipgloss drops the bold style when stdout is not a terminal.
                                if bold {
                                    write!(writer, "\x1b[1m{h}\x1b[0m")?;
                                } else {
                                    write!(writer, "{h}")?;
                                }
                                write!(writer, "{}", " ".repeat(widths[i] - h.len()))?;
                            }
                            writeln!(writer)?;
                        }
                        for row in rows {
                            for (i, f) in row.iter().enumerate() {
                                if i > 0 {
                                    write!(writer, "  ")?;
                                }
                                write!(
                                    writer,
                                    "{}{}",
                                    f.value,
                                    " ".repeat(widths[i] - strip_ansi(&f.value).len())
                                )?;
                            }
                            writeln!(writer)?;
                        }
                    }
                    OutputFormat::Card => {
                        let width = first.iter().map(|f| f.name.len() + 1).max().unwrap_or(0);
                        for (i, row) in rows.iter().enumerate() {
                            if i > 0 {
                                writeln!(writer)?;
                            }
                            for f in row {
                                writeln!(writer, "{:<width$} {}", format!("{}:", f.name), f.value)?;
                            }
                        }
                    }
                    OutputFormat::Markdown => {
                        for (i, row) in rows.iter().enumerate() {
                            if i > 0 {
                                write!(writer, "\n---\n\n")?;
                            }
                            for f in row {
                                writeln!(writer, "**{}:** {}", f.name, f.value)?;
                            }
                        }
                    }
                    _ => unreachable!(),
                }
            }
        }
        Ok(())
    }
}
pub fn pretty_go_json<T: ?Sized + Serialize>(value: &T) -> Result<String> {
    let compact = String::from_utf8(construct_types::to_go_json(value)?)?;
    let chars: Vec<_> = compact.chars().collect();
    let (mut quoted, mut escaped, mut depth) = (false, false, 0usize);
    let mut output = String::new();
    for (i, c) in chars.iter().copied().enumerate() {
        if quoted {
            output.push(c);
            if escaped {
                escaped = false;
            } else if c == '\\' {
                escaped = true;
            } else if c == '"' {
                quoted = false;
            }
            continue;
        }
        match c {
            '"' => {
                quoted = true;
                output.push(c);
            }
            '[' | '{' => {
                output.push(c);
                depth += 1;
                if !matches!(chars.get(i + 1), Some(']' | '}')) {
                    output.push('\n');
                    output.push_str(&"  ".repeat(depth));
                }
            }
            ']' | '}' => {
                depth -= 1;
                if !matches!(i.checked_sub(1).and_then(|i| chars.get(i)), Some('[' | '{')) {
                    output.push('\n');
                    output.push_str(&"  ".repeat(depth));
                }
                output.push(c);
            }
            ',' => {
                output.push_str(",\n");
                output.push_str(&"  ".repeat(depth));
            }
            ':' => output.push_str(": "),
            _ => output.push(c),
        }
    }
    Ok(output)
}
pub fn strip_ansi(value: &str) -> String {
    static RE: std::sync::OnceLock<regex::Regex> = std::sync::OnceLock::new();
    RE.get_or_init(|| regex::Regex::new("\x1b\\[[0-9;]*m").unwrap())
        .replace_all(value, "")
        .into_owned()
}
#[cfg(test)]
mod tests {
    use super::*;
    #[derive(Serialize)]
    struct Item {
        name: String,
    }
    impl DisplayResource for Item {
        fn fields(&self) -> Vec<Field<'_>> {
            vec![Field::new("Name", &self.name)]
        }
    }
    #[test]
    fn formats() {
        let item = Item { name: "<a>".into() };
        let mut out = Vec::new();
        let mut options = RenderOptions {
            format: OutputFormat::Card,
            ..Default::default()
        };
        Renderer.render(&mut out, &[item], &options).unwrap();
        assert_eq!(out, b"Name: <a>\n");
        out.clear();
        Renderer.render::<Item>(&mut out, &[], &options).unwrap();
        assert!(out.is_empty());
        options.format = OutputFormat::Json;
        Renderer
            .render(&mut out, &[Item { name: "<a>".into() }], &options)
            .unwrap();
        assert!(String::from_utf8(out).unwrap().contains("\\u003ca\\u003e"));
    }
}
