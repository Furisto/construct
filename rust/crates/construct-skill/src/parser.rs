use crate::{Skill, MAX_NAME_LENGTH};
use std::collections::HashMap;
use yaml_rust2::parser::{Event, MarkedEventReceiver, Parser, Tag};
use yaml_rust2::scanner::{Marker, TScalarStyle};

/// SKILL.md parse and validation failures. Messages match Go.
#[derive(Clone, Debug, PartialEq, Eq, thiserror::Error)]
pub enum ParseError {
    /// No `---` delimited frontmatter, or it is empty.
    #[error("SKILL.md must contain YAML frontmatter delimited by ---")]
    NoFrontmatter,
    /// The frontmatter is not valid YAML or not a mapping of strings.
    #[error("failed to parse YAML frontmatter: {0}")]
    Yaml(String),
    /// `name` is missing or empty.
    #[error("skill name is required")]
    MissingName,
    /// `description` is missing or empty.
    #[error("skill description is required")]
    MissingDescription,
    /// `name` exceeds [`MAX_NAME_LENGTH`] bytes.
    #[error("skill name must not exceed {MAX_NAME_LENGTH} characters")]
    NameTooLong,
    /// `name` is not lowercase alphanumeric with single hyphens.
    #[error("skill name must be lowercase alphanumeric with hyphens only")]
    InvalidNameFormat,
}

/// Parse SKILL.md frontmatter. Does not validate; see [`validate`].
pub fn parse(content: &[u8], location: &str) -> Result<Skill, ParseError> {
    let frontmatter = extract_frontmatter(content)?;
    let text = std::str::from_utf8(frontmatter)
        .map_err(|_| ParseError::Yaml("yaml: invalid leading UTF-8 octet".into()))?;
    let (name, description) = decode_frontmatter(text)?;
    Ok(Skill {
        name,
        description,
        location: location.to_owned(),
        scope: None,
    })
}

/// Validate a parsed skill. Like Go, the directory name is not checked.
pub fn validate(skill: &Skill) -> Result<(), ParseError> {
    if skill.name.is_empty() {
        return Err(ParseError::MissingName);
    }
    if skill.name.len() > MAX_NAME_LENGTH {
        return Err(ParseError::NameTooLong);
    }
    if !is_valid_name(&skill.name) {
        return Err(ParseError::InvalidNameFormat);
    }
    if skill.description.is_empty() {
        return Err(ParseError::MissingDescription);
    }
    Ok(())
}

/// `^[a-z0-9]+(-[a-z0-9]+)*$`
pub(crate) fn is_valid_name(name: &str) -> bool {
    !name.is_empty()
        && name.split('-').all(|part| {
            !part.is_empty()
                && part
                    .bytes()
                    .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit())
        })
}

fn extract_frontmatter(content: &[u8]) -> Result<&[u8], ParseError> {
    let content = trim_space(content);
    let content = content
        .strip_prefix(b"---")
        .ok_or(ParseError::NoFrontmatter)?;
    let end = find(content, b"\n---")
        .or_else(|| find(content, b"\r\n---"))
        .ok_or(ParseError::NoFrontmatter)?;
    let frontmatter = trim_space(&content[..end]);
    if frontmatter.is_empty() {
        return Err(ParseError::NoFrontmatter);
    }
    Ok(frontmatter)
}

fn find(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    haystack.windows(needle.len()).position(|w| w == needle)
}

/// Go `bytes.TrimSpace`: trims Unicode white space, leaving invalid UTF-8 intact.
fn trim_space(mut s: &[u8]) -> &[u8] {
    while let Some((c, len)) = first_char(s) {
        if !c.is_whitespace() {
            break;
        }
        s = &s[len..];
    }
    while let Some((c, len)) = last_char(s) {
        if !c.is_whitespace() {
            break;
        }
        s = &s[..s.len() - len];
    }
    s
}

fn first_char(s: &[u8]) -> Option<(char, usize)> {
    (1..=s.len().min(4)).find_map(|n| {
        std::str::from_utf8(&s[..n])
            .ok()
            .and_then(|t| t.chars().next().map(|c| (c, n)))
    })
}

fn last_char(s: &[u8]) -> Option<(char, usize)> {
    (1..=s.len().min(4)).find_map(|n| {
        std::str::from_utf8(&s[s.len() - n..])
            .ok()
            .and_then(|t| t.chars().next_back().map(|c| (c, n)))
    })
}

#[derive(Default)]
struct Events(Vec<Event>);

impl MarkedEventReceiver for Events {
    fn on_event(&mut self, ev: Event, _mark: Marker) {
        self.0.push(ev);
    }
}

#[derive(Clone)]
enum Node {
    Scalar(String, TScalarStyle, Option<Tag>),
    Sequence,
    Mapping,
}

impl Node {
    fn kind(&self) -> &'static str {
        match self {
            Node::Scalar(..) => "!!str",
            Node::Sequence => "!!seq",
            Node::Mapping => "!!map",
        }
    }

    /// Go yaml.v3 decoding into a `string`: nulls are empty, other scalars keep their text.
    fn into_string(self) -> Result<String, ParseError> {
        match self {
            Node::Scalar(value, style, tag) => {
                if is_null(&value, style, tag.as_ref()) {
                    Ok(String::new())
                } else {
                    Ok(value)
                }
            }
            other => Err(ParseError::Yaml(format!(
                "yaml: unmarshal errors:\n  cannot unmarshal {} into Go struct field Skill of type string",
                other.kind()
            ))),
        }
    }
}

fn is_null(value: &str, style: TScalarStyle, tag: Option<&Tag>) -> bool {
    match tag {
        Some(tag) => {
            tag.suffix == "null" && (tag.handle == "tag:yaml.org,2002:" || tag.handle == "!!")
        }
        None => {
            style == TScalarStyle::Plain && matches!(value, "" | "~" | "null" | "Null" | "NULL")
        }
    }
}

struct Decoder {
    events: Vec<Event>,
    pos: usize,
    anchors: HashMap<usize, Node>,
}

impl Decoder {
    fn next_node(&mut self) -> Result<Node, ParseError> {
        let event = self
            .events
            .get(self.pos)
            .cloned()
            .ok_or_else(|| ParseError::Yaml("yaml: unexpected end of document".into()))?;
        self.pos += 1;
        let (node, anchor) = match event {
            Event::Scalar(value, style, anchor, tag) => (Node::Scalar(value, style, tag), anchor),
            Event::Alias(id) => {
                return self
                    .anchors
                    .get(&id)
                    .cloned()
                    .ok_or_else(|| ParseError::Yaml("yaml: unknown anchor referenced".into()))
            }
            Event::SequenceStart(anchor, _) => {
                self.skip_until(|e| matches!(e, Event::SequenceEnd))?;
                (Node::Sequence, anchor)
            }
            Event::MappingStart(anchor, _) => {
                self.skip_until(|e| matches!(e, Event::MappingEnd))?;
                (Node::Mapping, anchor)
            }
            _ => {
                return Err(ParseError::Yaml(
                    "yaml: did not find expected node content".into(),
                ))
            }
        };
        if anchor != 0 {
            self.anchors.insert(anchor, node.clone());
        }
        Ok(node)
    }

    fn skip_until(&mut self, end: impl Fn(&Event) -> bool) -> Result<(), ParseError> {
        loop {
            match self.events.get(self.pos) {
                Some(e) if end(e) => {
                    self.pos += 1;
                    return Ok(());
                }
                Some(_) => {
                    self.next_node()?;
                }
                None => return Err(ParseError::Yaml("yaml: unexpected end of document".into())),
            }
        }
    }
}

fn decode_frontmatter(text: &str) -> Result<(String, String), ParseError> {
    let mut events = Events::default();
    Parser::new_from_str(text)
        .load(&mut events, false)
        .map_err(|e| ParseError::Yaml(format!("yaml: {e}")))?;
    let mut decoder = Decoder {
        events: events.0,
        pos: 0,
        anchors: HashMap::new(),
    };
    while matches!(
        decoder.events.get(decoder.pos),
        Some(Event::StreamStart | Event::DocumentStart)
    ) {
        decoder.pos += 1;
    }
    let (mut name, mut description) = (String::new(), String::new());
    match decoder.events.get(decoder.pos) {
        None | Some(Event::StreamEnd | Event::DocumentEnd) => return Ok((name, description)),
        Some(Event::MappingStart(anchor, _)) => {
            if *anchor != 0 {
                decoder.anchors.insert(*anchor, Node::Mapping);
            }
            decoder.pos += 1;
        }
        Some(_) => {
            let node = decoder.next_node()?;
            if let Node::Scalar(value, style, tag) = &node {
                if is_null(value, *style, tag.as_ref()) {
                    return Ok((name, description));
                }
            }
            return Err(ParseError::Yaml(format!(
                "yaml: unmarshal errors:\n  cannot unmarshal {} into Go value of type skill.Skill",
                node.kind()
            )));
        }
    }
    let mut seen: Vec<(String, bool)> = Vec::new();
    while !matches!(
        decoder.events.get(decoder.pos),
        Some(Event::MappingEnd) | None
    ) {
        let key = decoder.next_node()?;
        let value = decoder.next_node()?;
        let Node::Scalar(key, style, _) = key else {
            continue;
        };
        let identity = (key.clone(), style == TScalarStyle::Plain);
        if seen.contains(&identity) {
            return Err(ParseError::Yaml(format!(
                "yaml: mapping key {key:?} already defined"
            )));
        }
        seen.push(identity);
        match key.as_str() {
            "name" => name = value.into_string()?,
            "description" => description = value.into_string()?,
            _ => {}
        }
    }
    Ok((name, description))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parsed(content: &str) -> Result<Skill, ParseError> {
        parse(content.as_bytes(), "/path/to/SKILL.md")
    }

    #[test]
    fn parses_valid_skill() {
        let skill = parsed("---\nname: test-skill\ndescription: A test skill for testing\n---\n# Test Skill\nInstructions here").unwrap();
        assert_eq!(skill.name, "test-skill");
        assert_eq!(skill.description, "A test skill for testing");
        assert_eq!(skill.location, "/path/to/SKILL.md");
    }

    #[test]
    fn rejects_missing_or_empty_frontmatter() {
        assert_eq!(parsed("# Just markdown"), Err(ParseError::NoFrontmatter));
        assert_eq!(
            parsed("---\n---\n# Content"),
            Err(ParseError::NoFrontmatter)
        );
        assert_eq!(parsed("---\nname: a\n"), Err(ParseError::NoFrontmatter));
    }

    #[test]
    fn matches_go_frontmatter_edges() {
        let skill = parsed("\r\n  ---\r\nname: crlf\r\ndescription: d\r\n---\r\nbody").unwrap();
        assert_eq!(
            (skill.name.as_str(), skill.description.as_str()),
            ("crlf", "d")
        );
        // Go matches "\n---" anywhere, so "----" also closes the frontmatter.
        assert_eq!(
            parsed("---\nname: a\ndescription: b\n-----x").unwrap().name,
            "a"
        );
        // Only comments: valid YAML, empty skill.
        assert_eq!(parsed("---\n# nothing\n---").unwrap().name, "");
    }

    #[test]
    fn decodes_scalars_like_yaml_v3() {
        let skill = parsed("---\nname: 0x10\ndescription: true\nother: [1, 2]\n---").unwrap();
        assert_eq!(
            (skill.name.as_str(), skill.description.as_str()),
            ("0x10", "true")
        );
        assert_eq!(
            parsed("---\nname: ~\ndescription: null\n---").unwrap(),
            Skill {
                location: "/path/to/SKILL.md".into(),
                ..Skill::default()
            }
        );
        let skill = parsed("---\nname: &n x\ndescription: |\n  multi\n  line\n---").unwrap();
        assert_eq!(skill.description, "multi\nline\n");
        let skill = parsed("---\nbase: &d shared\nname: n\ndescription: *d\n---").unwrap();
        assert_eq!(skill.description, "shared");
        assert_eq!(
            parsed("---\nname: \"null\"\ndescription: d\n---")
                .unwrap()
                .name,
            "null"
        );
    }

    #[test]
    fn rejects_non_string_fields_and_duplicates() {
        assert!(matches!(
            parsed("---\nname: [a]\ndescription: d\n---"),
            Err(ParseError::Yaml(_))
        ));
        assert!(matches!(
            parsed("---\nname: a\nname: b\n---"),
            Err(ParseError::Yaml(_))
        ));
        assert!(matches!(
            parsed("---\njust a string\n---"),
            Err(ParseError::Yaml(_))
        ));
        assert!(matches!(parsed("---\n- a\n---"), Err(ParseError::Yaml(_))));
        assert!(matches!(
            parsed("---\nname: [unclosed\n---"),
            Err(ParseError::Yaml(_))
        ));
    }

    #[test]
    fn validates_like_go() {
        let skill = |name: &str, description: &str| Skill {
            name: name.into(),
            description: description.into(),
            ..Skill::default()
        };
        assert_eq!(
            validate(&skill("test-skill", "A valid description")),
            Ok(())
        );
        assert_eq!(validate(&skill("", "d")), Err(ParseError::MissingName));
        assert_eq!(
            validate(&skill("test-skill", "")),
            Err(ParseError::MissingDescription)
        );
        assert_eq!(
            validate(&skill("Test-Skill", "d")),
            Err(ParseError::InvalidNameFormat)
        );
        assert_eq!(
            validate(&skill("test skill", "d")),
            Err(ParseError::InvalidNameFormat)
        );
        assert_eq!(
            validate(&skill("a--b", "d")),
            Err(ParseError::InvalidNameFormat)
        );
        assert_eq!(
            validate(&skill("-a", "d")),
            Err(ParseError::InvalidNameFormat)
        );
        assert_eq!(
            validate(&skill(&"a".repeat(65), "d")),
            Err(ParseError::NameTooLong)
        );
        assert_eq!(validate(&skill(&"a".repeat(64), "d")), Ok(()));
    }
}
