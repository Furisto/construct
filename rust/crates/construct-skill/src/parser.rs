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
struct Events(Vec<(Event, Marker)>);

impl MarkedEventReceiver for Events {
    fn on_event(&mut self, ev: Event, mark: Marker) {
        self.0.push((ev, mark));
    }
}

#[derive(Clone)]
enum Kind {
    Scalar(String, TScalarStyle, Option<Tag>),
    Sequence,
    Mapping(Vec<(Node, Node)>),
}

/// A yaml.v3 `Node`: kind, 1-based line within the frontmatter, and whether it was an alias.
#[derive(Clone)]
struct Node {
    kind: Kind,
    line: usize,
    alias: bool,
}

const YAML_TAG_PREFIX: &str = "tag:yaml.org,2002:";

impl Node {
    /// yaml.v3 `shortTag` of the node's explicit or resolved tag.
    fn tag(&self) -> String {
        match &self.kind {
            Kind::Sequence => "!!seq".into(),
            Kind::Mapping(_) => "!!map".into(),
            Kind::Scalar(value, style, tag) => match tag {
                Some(t) if t.handle == "!!" || t.handle == YAML_TAG_PREFIX => {
                    format!("!!{}", t.suffix)
                }
                Some(t) => format!("{}{}", t.handle, t.suffix),
                None if *style == TScalarStyle::Plain => resolve_plain(value).into(),
                None => "!!str".into(),
            },
        }
    }

    fn is_null(&self) -> bool {
        matches!(self.kind, Kind::Scalar(..)) && self.tag() == "!!null"
    }

    /// yaml.v3 duplicate-key identity: node kind and `Value` (empty for collections).
    fn key_identity(&self) -> (u8, &str) {
        match &self.kind {
            _ if self.alias => (3, ""),
            Kind::Scalar(value, ..) => (0, value.as_str()),
            Kind::Sequence => (1, ""),
            Kind::Mapping(_) => (2, ""),
        }
    }
}

/// yaml.v3 `resolve` for untagged plain scalars, as far as the tag name.
fn resolve_plain(value: &str) -> &'static str {
    static FLOAT: std::sync::OnceLock<regex::Regex> = std::sync::OnceLock::new();
    let float = FLOAT.get_or_init(|| {
        regex::Regex::new(r"^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$").unwrap()
    });
    match value {
        "" | "~" | "null" | "Null" | "NULL" => "!!null",
        "true" | "True" | "TRUE" | "false" | "False" | "FALSE" => "!!bool",
        ".inf" | ".Inf" | ".INF" | "+.inf" | "+.Inf" | "+.INF" | "-.inf" | "-.Inf" | "-.INF"
        | ".nan" | ".NaN" | ".NAN" => "!!float",
        "<<" => "!!merge",
        v if is_go_int(&v.replace('_', "")) => "!!int",
        v if float.is_match(&v.replace('_', "")) => "!!float",
        _ => "!!str",
    }
}

/// Go `strconv.ParseInt(s, 0, 64)` syntax (range not checked).
fn is_go_int(s: &str) -> bool {
    let digits = s.strip_prefix(['+', '-']).unwrap_or(s);
    let (radix, body) = match digits.get(..2) {
        Some("0x" | "0X") => (16, &digits[2..]),
        Some("0o" | "0O") => (8, &digits[2..]),
        Some("0b" | "0B") => (2, &digits[2..]),
        _ if digits.len() > 1 && digits.starts_with('0') => (8, &digits[1..]),
        _ => (10, digits),
    };
    !body.is_empty() && body.chars().all(|c| c.is_digit(radix))
}

/// Collects yaml.v3 `unmarshal errors` while decoding continues, like Go's `d.terrors`.
#[derive(Default)]
struct Unmarshal {
    errors: Vec<String>,
}

impl Unmarshal {
    /// yaml.v3 `terror`.
    fn type_error(&mut self, node: &Node, target: &str) {
        let mut value = String::new();
        if let Kind::Scalar(text, ..) = &node.kind {
            value = if text.len() > 10 {
                let mut cut = 7;
                while !text.is_char_boundary(cut) {
                    cut -= 1;
                }
                format!(" `{}...`", &text[..cut])
            } else {
                format!(" `{text}`")
            };
        }
        self.errors.push(format!(
            "line {}: cannot unmarshal {}{value} into {target}",
            node.line,
            node.tag()
        ));
    }

    /// Decode into a Go `string`: nulls are empty, `!!binary` is base64-decoded, other
    /// scalars keep their text, collections are type errors.
    fn string(&mut self, node: &Node) -> Result<Option<String>, ParseError> {
        let Kind::Scalar(value, ..) = &node.kind else {
            self.type_error(node, "string");
            return Ok(None);
        };
        Ok(Some(match node.tag().as_str() {
            "!!null" => String::new(),
            "!!binary" => {
                use base64::Engine;
                let data = base64::engine::general_purpose::STANDARD
                    .decode(value)
                    .map_err(|_| {
                        ParseError::Yaml("yaml: !!binary value contains invalid base64 data".into())
                    })?;
                String::from_utf8_lossy(&data).into_owned()
            }
            _ => value.clone(),
        }))
    }
}

struct Builder<'a> {
    events: &'a [(Event, Marker)],
    pos: usize,
    anchors: HashMap<usize, Node>,
}

impl Builder<'_> {
    fn node(&mut self) -> Result<Option<Node>, ParseError> {
        let Some((event, mark)) = self.events.get(self.pos).cloned() else {
            return Ok(None);
        };
        self.pos += 1;
        let line = mark.line();
        let (kind, anchor) = match event {
            Event::Scalar(value, style, anchor, tag) => (Kind::Scalar(value, style, tag), anchor),
            Event::Alias(id) => {
                let mut node =
                    self.anchors.get(&id).cloned().ok_or_else(|| {
                        ParseError::Yaml("yaml: unknown anchor referenced".into())
                    })?;
                node.alias = true;
                return Ok(Some(node));
            }
            Event::SequenceStart(anchor, _) => {
                while !matches!(
                    self.events.get(self.pos),
                    Some((Event::SequenceEnd, _)) | None
                ) {
                    self.node()?;
                }
                self.pos += 1;
                (Kind::Sequence, anchor)
            }
            Event::MappingStart(anchor, _) => {
                let mut pairs = Vec::new();
                while !matches!(
                    self.events.get(self.pos),
                    Some((Event::MappingEnd, _)) | None
                ) {
                    if let (Some(k), Some(v)) = (self.node()?, self.node()?) {
                        pairs.push((k, v));
                    }
                }
                self.pos += 1;
                (Kind::Mapping(pairs), anchor)
            }
            _ => return Ok(None),
        };
        let node = Node {
            kind,
            line,
            alias: false,
        };
        if anchor != 0 {
            self.anchors.insert(anchor, node.clone());
        }
        Ok(Some(node))
    }
}

/// Format a yaml-rust2 syntax error the way yaml.v3 reports libyaml errors.
fn syntax_error(text: &str, err: &yaml_rust2::ScanError) -> ParseError {
    let info = err.info();
    if info.ends_with("found unknown anchor") {
        let name: String = text[err.marker().index().min(text.len())..]
            .trim_start_matches('*')
            .chars()
            .take_while(|c| !c.is_whitespace() && !",[]{}".contains(*c))
            .collect();
        return ParseError::Yaml(format!("yaml: unknown anchor '{name}' referenced"));
    }
    let problem = match info.strip_prefix("while parsing ") {
        Some(rest) => rest.split_once(", ").map_or(info, |(_, p)| p),
        None => info,
    };
    ParseError::Yaml(format!("yaml: line {}: {problem}", err.marker().line()))
}

/// yaml.v3 `Unmarshal` of the frontmatter into Go's `skill.Skill{Name, Description}`.
fn decode_frontmatter(text: &str) -> Result<(String, String), ParseError> {
    let mut events = Events::default();
    Parser::new_from_str(text)
        .load(&mut events, false)
        .map_err(|e| syntax_error(text, &e))?;
    let first = events
        .0
        .iter()
        .position(|(e, _)| !matches!(e, Event::StreamStart | Event::DocumentStart))
        .unwrap_or(events.0.len());
    let mut builder = Builder {
        events: &events.0,
        pos: first,
        anchors: HashMap::new(),
    };
    let (mut name, mut description) = (String::new(), String::new());
    let Some(root) = builder.node()? else {
        return Ok((name, description));
    };
    let mut out = Unmarshal::default();
    match &root.kind {
        _ if root.is_null() => {}
        Kind::Mapping(pairs) => {
            for (i, (first, _)) in pairs.iter().enumerate() {
                for (later, _) in &pairs[i + 1..] {
                    if first.key_identity() == later.key_identity() {
                        out.errors.push(format!(
                            "line {}: mapping key {} already defined at line {}",
                            later.line,
                            crate::prompt::go_quote(first.key_identity().1),
                            first.line
                        ));
                    }
                }
            }
            if out.errors.is_empty() {
                for (key, value) in pairs {
                    let field = match out.string(key)? {
                        Some(k) if k == "name" => &mut name,
                        Some(k) if k == "description" => &mut description,
                        _ => continue,
                    };
                    if let Some(v) = out.string(value)? {
                        *field = v;
                    }
                }
            }
        }
        _ => out.type_error(&root, "skill.Skill"),
    }
    if !out.errors.is_empty() {
        return Err(ParseError::Yaml(format!(
            "yaml: unmarshal errors:\n  {}",
            out.errors.join("\n  ")
        )));
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

    /// Expected values are Go's `Parser.Parse` output for the same inputs.
    #[test]
    fn matches_go_yaml_v3_results() {
        let cases = [
            ("---\nname: [a, b]\ndescription: x\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into string"),
            ("---\nname: x\ndescription: {a: 1}\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!map into string"),
            ("---\nname: [a]\ndescription: [b]\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into string\n  line 2: cannot unmarshal !!seq into string"),
            ("---\nname: a\nname: b\ndescription: dup\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 2: mapping key \"name\" already defined at line 1"),
            ("---\nname: a\n\"name\": b\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 2: mapping key \"name\" already defined at line 1"),
            ("---\nname: a\nname: b\nname: c\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 2: mapping key \"name\" already defined at line 1\n  line 3: mapping key \"name\" already defined at line 1\n  line 3: mapping key \"name\" already defined at line 2"),
            ("---\nfoo: 1\nfoo: 2\nname: [x]\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 2: mapping key \"foo\" already defined at line 1"),
            ("---\njust a string\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `just a ...` into skill.Skill"),
            ("---\nlonger than ten chars\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `longer ...` into skill.Skill"),
            ("---\n- a\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into skill.Skill"),
            ("---\nname: *nope\n---\n", "ERR failed to parse YAML frontmatter: yaml: unknown anchor 'nope' referenced"),
            ("---\n\nname: [a]\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into string"),
            ("---\r\nname: [a]\r\n---\r\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into string"),
            ("---\nname: !!int 5\ndescription: !!binary aGk=\n---\n", "OK \"5\" \"hi\""),
            ("---\n[1]: x\nname: a\ndescription: b\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into string"),
            ("---\nname: &a [x]\ndescription: *a\n---\n", "ERR failed to parse YAML frontmatter: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into string\n  line 1: cannot unmarshal !!seq into string"),
        ];
        for (input, want) in cases {
            let got = match parsed(input) {
                Ok(s) => format!("OK {:?} {:?}", s.name, s.description),
                Err(e) => format!("ERR {e}"),
            };
            assert_eq!(got, want, "{input:?}");
        }
    }

    #[test]
    fn formats_syntax_errors_like_yaml_v3() {
        let err = parsed("---\nname: [unclosed\n---").unwrap_err().to_string();
        assert!(
            err.starts_with("failed to parse YAML frontmatter: yaml: line "),
            "{err}"
        );
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
