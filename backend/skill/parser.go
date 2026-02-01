package skill

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

var (
	ErrMissingName        = errors.New("skill name is required")
	ErrMissingDescription = errors.New("skill description is required")
	ErrNameTooLong        = fmt.Errorf("skill name must not exceed %d characters", MaxNameLength)
	ErrDescriptionTooLong = fmt.Errorf("skill description must not exceed %d characters", MaxDescriptionLength)
	ErrInvalidNameFormat  = errors.New("skill name must be lowercase alphanumeric with hyphens only")
	ErrNameMismatch       = errors.New("skill name must match directory name")
	ErrNoFrontmatter      = errors.New("SKILL.md must contain YAML frontmatter delimited by ---")
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(content []byte, location string) (*Skill, error) {
	frontmatter, err := extractFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var skill Skill
	if err := yaml.Unmarshal(frontmatter, &skill); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	skill.Location = location

	return &skill, nil
}

func (p *Parser) Validate(skill *Skill, dirPath string) error {
	if skill.Name == "" {
		return ErrMissingName
	}

	if len(skill.Name) > MaxNameLength {
		return ErrNameTooLong
	}

	if !namePattern.MatchString(skill.Name) {
		return ErrInvalidNameFormat
	}

	if skill.Description == "" {
		return ErrMissingDescription
	}

	return nil
}

func extractFrontmatter(content []byte) ([]byte, error) {
	content = bytes.TrimSpace(content)

	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, ErrNoFrontmatter
	}

	content = content[3:]

	endIndex := bytes.Index(content, []byte("\n---"))
	if endIndex == -1 {
		endIndex = bytes.Index(content, []byte("\r\n---"))
	}

	if endIndex == -1 {
		return nil, ErrNoFrontmatter
	}

	frontmatter := bytes.TrimSpace(content[:endIndex])
	if len(frontmatter) == 0 {
		return nil, ErrNoFrontmatter
	}

	return frontmatter, nil
}
