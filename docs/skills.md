# Skills

Skills teach your agent how to handle specific tasks. For example, the `remotion-best-practices` skill knows how to create videos with Remotion, and the `web-design-guidelines` skill knows how to build polished web interfaces.

For more on what skills are and how they work, see [agentskills.io](https://agentskills.io/).

## Install your first skill

```bash
construct skill install vercel-labs/agent-skills --skill web-design-guidelines
```

## Install from different sources

Skills can be installed from GitHub, GitLab, or any URL pointing to a `SKILL.md` file.

**GitHub**

```bash
# A specific skill from a repository
construct skill install vercel-labs/agent-skills --skill web-design-guidelines

# All skills from a repository
construct skill install vercel-labs/agent-skills

# From a specific branch
construct skill install github.com/vercel-labs/agent-skills/tree/v2/web-design-guidelines
```

**GitLab**

```bash
construct skill install gitlab.com/group/project/-/tree/main/skills
```

**Direct URL**

```bash
construct skill install https://example.com/path/to/SKILL.md
```

## Manage installed skills

**List skills**

```bash
construct skill list
```

**Update skills**

```bash
# Update all skills
construct skill update

# Update a specific skill
construct skill update web-design-guidelines
```

**Delete a skill**

```bash
construct skill delete web-design-guidelines
```

**Reinstall a skill**

```bash
construct skill install vercel-labs/agent-skills --skill web-design-guidelines --force
```

## Repository vs user skills

Skills can live in two places:

- **Repository skills** in `.construct/skills/` — shared with collaborators, only available in that repository
- **User skills** in `~/.config/construct/skills/` — available everywhere, just for you

When both exist with the same name, the repository skill wins.

Install commands always install to the user location. To create a repository skill, add it directly to `.construct/skills/<skill-name>/`.

## Next steps

- [Create your own skill](./skill-authoring.md)
- [Browse community skills](https://skills.sh/)
