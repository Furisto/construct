use crate::Skill;
use std::fmt::Write;

const HEADER: &str = "<available_skills>
Skills are specialized instructions that can be loaded on-demand when working on specific tasks like frontend development, building CPython modules, database migrations, etc.

To use a skill:
1. Read the SKILL.md file at the location path to get detailed instructions
2. Use the list_files tool on the skill directory to discover additional content

Skill directories may contain:
- SKILL.md: Main instructions (required)
- scripts/: Executable helper scripts
- references/: Additional documentation and examples
- assets/: Templates, configs, and other resources

";

/// The system-prompt block listing skills (Go agent `formatSkills`); empty for no skills.
pub fn format_available_skills(skills: &[Skill]) -> String {
    if skills.is_empty() {
        return String::new();
    }
    let mut out = String::from(HEADER);
    for skill in skills {
        let _ = writeln!(
            out,
            "  <skill name={} location={}>",
            go_quote(&skill.name),
            go_quote(&skill.location)
        );
        let _ = writeln!(out, "    {}", skill.description.trim());
        out.push_str("  </skill>\n");
    }
    out.push_str("</available_skills>");
    out
}

/// Go `strconv.Quote` (`%q`), treating non-control characters as printable.
fn go_quote(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\u{7}' => out.push_str("\\a"),
            '\u{8}' => out.push_str("\\b"),
            '\u{c}' => out.push_str("\\f"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            '\u{b}' => out.push_str("\\v"),
            c if (c as u32) < 0x20 || c == '\u{7f}' => {
                let _ = write!(out, "\\x{:02x}", c as u32);
            }
            c if c.is_control() || c == '\u{feff}' => {
                let _ = write!(out, "\\u{:04x}", c as u32);
            }
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_is_empty() {
        assert_eq!(format_available_skills(&[]), "");
    }

    #[test]
    fn formats_like_go() {
        let skills = [Skill {
            name: "pdf".into(),
            description: "  Work with PDFs\n".into(),
            location: "/s/pdf/SKILL.md".into(),
            scope: None,
        }];
        let out = format_available_skills(&skills);
        assert!(out.starts_with("<available_skills>\nSkills are specialized"));
        assert!(out.ends_with("\n\n  <skill name=\"pdf\" location=\"/s/pdf/SKILL.md\">\n    Work with PDFs\n  </skill>\n</available_skills>"));
    }

    #[test]
    fn quotes_like_strconv() {
        assert_eq!(
            go_quote("a\"b\\c\n\u{1}\u{7f}é\u{85}"),
            "\"a\\\"b\\\\c\\n\\x01\\x7fé\\u0085\""
        );
    }
}
