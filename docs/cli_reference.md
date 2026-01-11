````markdown
# Construct CLI Reference

**Construct CLI: Your command-line copilot for building with AI.**

Interact with, manage, and configure AI agents directly from your terminal. This document provides a complete reference for all `construct` commands, flags, and options.

## Global Options

-   `-v, --verbose`: Enable verbose output for detailed logging.
-   `--help`: Show help information for any command or subcommand.

## Command Groups

The `construct` CLI is organized into three logical groups:

* **Core Workflow**: Commands for your primary day-to-day interactions with agents.
* **Manage Resources**: Commands for creating, listing, and managing agents, models, and tasks.
* **System & Configuration**: Commands for configuring the CLI and managing the system daemon.

---

## Core Workflow

### `construct new`

Launch a new interactive chat session with an agent.

**Usage**
```bash
construct new [flags]
````

**Description**
Starts a real-time, interactive conversation with an AI agent in your terminal. This is the primary command for collaborative tasks like coding, debugging, and brainstorming. The session context, including messages and files, is saved automatically.

**Options**

  * `--agent <name|id>`: Start the session with a specific agent. Defaults to the last used agent.
  * `--workspace <path>`: Set the agent's working directory. Defaults to the current directory (`.`).

**Examples**

```bash
# Start a chat with the default agent
construct new

# Start a chat with a specific agent named 'coder'
construct new --agent coder

# Start a chat with an agent sandboxed in a different directory
construct new --workspace /path/to/project
```

### `construct resume`

Continue a previous chat session.

**Usage**

```bash
construct resume [task-id] [flags]
```

**Description**
Pick up a conversation where you left off. `construct resume` restores the full context of a previous session, including the agent, all messages, and the original workspace directory.

If no `task-id` is provided, an interactive menu will display recent sessions to choose from. Partial ID matching is supported.

**Options**

  * `--last`: Immediately resume the most recent session without showing the interactive picker.

**Examples**

```bash
# Show an interactive picker to select a recent session
construct resume

# Immediately jump into the most recent session
construct resume --last

# Resume a specific session by its ID
construct resume 01974c1d-0be8-70e1-88b4-ad9462fff25e
```

### `construct exec`

Execute a non-interactive task with an agent..

**Usage**

```bash
construct prompt "<question>" [flags]
```

**Description**
Sends a single prompt to an agent for immediate, non-interactive execution. This is ideal for scripting, running automated tasks, or integrating construct into other workflows and pipelines. The entire execution is saved as a task that can be inspected or resumed later with construct resume.

**Options**

  * `-a, --agent <name|id>`: Specify the agent to use by its name or ID.
  * `-w, --workspace <path>`: Set the agent's working directory.
  * `--max-turns <number>`: Set a maximum number of conversational turns for the agent to complete the task. (Default: 5)
  * `-f, --file <path>`: Add a file to the agent's context. Can be used multiple times.
  * `-c, --continue`: Continue the most recent task with this new question.

**Examples**

```bash
# Execute a simple command
construct exec "What are the top 5 features of Go 1.22?"

# Pipe a file into the agent as context for summarization
cat README.md | construct exec "Summarize this document."

# Instruct an agent to review specific files for bugs
construct exec "Review this code for potential race conditions" \
  --file ./cmd/server/main.go \
  --file ./pkg/worker/worker.go \
  --agent go-reviewer

# Get structured JSON output for scripting
construct exec "List all .go files in the workspace" --output json

# Give the agent more turns to complete a complex task
construct  "Draft a project proposal based on the attached spec" \
  --file ./specs/project-spec.md \
  --max-turns 10
```

-----

## Manage Resources

### Agent Commands: `construct agent`

Manage the AI agents that perform tasks.

#### `construct agent create <name>`

Define a new, reusable AI agent.

**Usage**

```bash
construct agent create <name> [flags]
```

**Description**
Creates a new agent by giving it a name, a system prompt, and assigning a model. The system prompt defines the agent's personality, goals, and constraints.

You must specify the agent's system prompt using one of `--prompt`, `--prompt-file`, or by piping it via `--prompt-stdin`.

**Arguments**

  * `<name>` (required): A unique, memorable name for the agent (e.g., `coder`, `sql-writer`).

**Options**

  * `-m, --model <model-name|id>` (required): The AI model the agent will use (e.g., `gpt-4o`).
  * `-p, --prompt <string>`: The system prompt that defines the agent's behavior.
  * `--prompt-file <path>`: Read the system prompt from a specified file.
  * `--prompt-stdin`: Read the system prompt from standard input (stdin).
  * `-d, --description <string>`: A brief description of what the agent does.

**Examples**

```bash
# Create a simple coding assistant
construct agent create "coder" \
  --model "gpt-4o" \
  --prompt "You are an expert Go developer. Your code is clean, efficient, and well-documented."

# Create an agent with a prompt from a file
construct agent create "sql-expert" \
  --model "claude-3-5-sonnet" \
  --prompt-file ./prompts/sql.txt

# Create an agent by piping the prompt
echo "You are a security expert reviewing code for vulnerabilities." | \
  construct agent create "reviewer" --model "gpt-4o" --prompt-stdin
```

#### `construct agent list`

List all available agents.

**Usage**

```bash
construct agent list [flags]
```

**Aliases**: `ls`

**Options**

  * `-m, --model <model-name|id>`: Filter agents by the model they use.
  * `-n, --name <string>`: Filter agents by name (supports partial matching).
  * `-l, --limit <number>`: Limit the number of results returned.
  * `--output <table|json|yaml|markdown>`: Specify the output format. Use `markdown` or `md` for markdown output.

**Examples**

```bash
# List all agents in a table
construct agent list

# Export agents as markdown for documentation
construct agent list --output markdown

# Find all agents using a specific model
construct agent ls --model "claude-3-5-sonnet"
```

#### `construct agent get <name|id>`

Inspect the details of a specific agent.

**Usage**

```bash
construct agent get <name|id> [flags]
```

**Examples**

```bash
# Get details for the 'coder' agent
construct agent get coder

# Get details and format as JSON
construct agent get 01974c1d-0be8-70e1-88b4-ad9462fff25e --output json
```

#### `construct agent edit <name|id>`

Edit an agent's configuration in your default editor.

**Usage**

```bash
construct agent edit <name|id>
```

**Description**
Opens the agent's configuration in your default text editor (`$EDITOR`). This provides a fast and powerful way to modify an agent's prompt, model, or description, similar to `kubectl edit`. The changes are applied when you save and close the file.

**Examples**

```bash
# Edit the 'coder' agent in your default editor
construct agent edit coder

# Use a specific editor for the session
EDITOR=vim construct agent edit sql-expert
```

#### `construct agent delete <name|id>...`

Permanently delete one or more agents.

**Usage**

```bash
construct agent delete <name|id>... [flags]
```

**Aliases**: `rm`

**Options**

  * `-f, --force`: Skip the confirmation prompt.

**Examples**

```bash
# Delete a single agent
construct agent delete coder

# Delete multiple agents at once
construct agent rm architect security-reviewer

# Force delete without a confirmation prompt
construct agent delete old-agent --force
```

### Task Commands: `construct task`

Manage tasks, which are the saved records of conversations.

#### `construct task create`

Create a new task without starting an interactive session.

**Usage**

```bash
construct task create [flags]
```

**Description**
Programmatically creates a new task (a container for a conversation) and assigns an agent to it. This is useful for setting up tasks that will be used later or by automated systems. To start an interactive chat, use `construct new`.

**Options**

  * `-a, --agent <name|id>` (required): The agent to assign to the task.
  * `-w, --workspace <path>`: The workspace directory for the task.

**Examples**

```bash
# Create a new task assigned to the 'coder' agent
construct task create --agent coder

# Create a task with a specific workspace
construct task create --agent sql-expert --workspace /path/to/db/repo
```

#### `construct task list`

List all tasks.

**Usage**

```bash
construct task list [flags]
```

**Aliases**: `ls`

**Options**

  * `-a, --agent <name|id>`: Filter tasks by the agent assigned to them.
  * `-l, --limit <number>`: Limit the number of results returned.
  * `--output <table|json|yaml|markdown>`: Specify the output format. Use `markdown` or `md` for markdown output.

**Examples**

```bash
# List all recent tasks
construct task list

# List tasks assigned to the 'coder' agent, in JSON format
construct task ls --agent "coder" --output json
```

#### `construct task get <task-id>`

Inspect the details of a specific task.

**Usage**

```bash
construct task get <task-id> [flags]
```

**Examples**

```bash
# Get details for a specific task
construct task get 01974c1d-0be8-70e1-88b4-ad9462fff25e

# Get task details and format as YAML
construct task get 01974c1d-0be8-70e1-88b4-ad9462fff25e --output yaml
```

#### `construct task delete <task-id>...`

Permanently delete one or more tasks.

**Usage**

```bash
construct task delete <task-id>... [flags]
```

**Aliases**: `rm`

**Options**

  * `-f, --force`: Skip the confirmation prompt.

**Examples**

```bash
# Delete a single task
construct task delete 01974c1d-0be8-70e1-88b4-ad9462fff25e

# Delete multiple tasks at once
construct task rm 01974c1d-0be8-70e1-88b4-ad9462fff25e 01974c1d-0be8-70e1-88b4-ad9462fff26f
```

### Message Commands: `construct message`

Interact directly with the messages within a task.

#### `construct message create <task-id> <content>`

Add a message to a task programmatically.

**Usage**

```bash
construct message create <task-id> <content> [flags]
```

**Description**
Appends a new message to a task's history. This is an advanced command, typically used for scripting or integrating external tools with Construct tasks.

**Arguments**

  * `<task-id>` (required): The ID of the task to add the message to.
  * `<content>` (required): The text content of the message.

**Examples**

```bash
# Add a user message to an existing task
construct message create "01974c1d-0be8-70e1-88b4-ad9462fff25e" "Please check the file again."
```

#### `construct message list`

List messages.

**Usage**

```bash
construct message list [flags]
```

**Aliases**: `ls`, `msg ls`

**Description**
Lists messages, typically filtered by a specific task. Useful for reviewing or exporting a conversation history.

**Options**

  * `-t, --task <task-id>`: (Recommended) Filter messages by task ID.
  * `-a, --agent <name|id>`: Filter by the agent that participated in the conversation.
  * `-r, --role <user|assistant>`: Filter messages by the role of the author.
  * `--output <table|json|yaml|markdown>`: Specify the output format. 

**Examples**

```bash
# List all messages for a specific task
construct message list --task "01974c1d-0be8-70e1-88b4-ad9462fff25e"

# List only the assistant's responses in that task
construct message list --task "01974c1d-0be8-70e1-88b4-ad9462fff25e" --role assistant
```

### Model Commands: `construct model`

Manage the large language models available to agents.

#### `construct model create <name>`

Register a new large language model for use by agents.

**Usage**

```bash
construct model create <name> [flags]
```

**Description**
Makes a specific model from a provider (like `gpt-4o` from OpenAI) available to `construct`. You must configure a provider before you can create a model.

**Arguments**

  * `<name>` (required): The official name of the model (e.g., `gpt-4o`, `claude-3-5-sonnet-20240620`).

**Options**

  * `-p, --provider <name|id>` (required): The name or ID of the model provider.
  * `-w, --context-window <number>` (required): The maximum context window size for the model.

**Examples**

```bash
# Register GPT-4o from the 'openai-prod' provider
construct model create "gpt-4o" --provider "openai-prod" --context-window 128000

# Register Claude Sonnet 3.5 from the 'anthropic' provider
construct model create "claude-3-5-sonnet-20240620" --provider "anthropic" --context-window 200000
```

#### `construct model list`

List all registered models.

**Usage**

```bash
construct model list [flags]
```

**Aliases**: `ls`

**Options**

  * `-p, --provider <name|id>`: Filter models by their provider.
  * `--output <table|json|yaml|markdown>`: Specify the output format.

**Examples**

```bash
# List all available models
construct model list

# List all models available from the 'anthropic' provider
construct model ls --provider "anthropic"
```

### Provider Commands: `construct provider`

Manage integrations with model providers like OpenAI and Anthropic.

#### `construct provider create <name>`

Configure a new model provider integration.

**Usage**

```bash
construct provider create <name> [flags]
```

**Description**
Connects `construct` to an external AI model provider. This step is required to gain access to models. API credentials can be provided via flags or environment variables (e.g., `$OPENAI_API_KEY`, `$ANTHROPIC_API_KEY`).

**Arguments**

  * `<name>` (required): A unique name for this provider configuration (e.g., `openai-personal`, `anthropic-work`).

**Options**

  * `-t, --type <openai|anthropic>` (required): The type of the model provider.
  * `-k, --api-key <string>`: The API key. If omitted, the corresponding environment variable will be used.

**Examples**

```bash
# Create an OpenAI provider, using the API key from the environment
export OPENAI_API_KEY="sk-..."
construct provider create "openai-prod" --type openai

# Create an Anthropic provider, passing the API key directly
construct provider create "anthropic-dev" --type anthropic --api-key "sk-ant-..."
```

#### `construct provider list`

List all configured model providers.

**Usage**

```bash
construct provider list [flags]
```

**Aliases**: `ls`, `mp ls`

**Options**

  * `-t, --type <openai|anthropic>`: Filter providers by type.
  * `--output <table|json|yaml|markdown>`: Specify the output format.

**Examples**

```bash
# List all configured providers
construct provider list
```

#### `construct provider delete <name|id>...`

Permanently delete one or more model providers.

**Usage**

```bash
construct provider delete <name|id>... [flags]
```

**Aliases**: `rm`, `mp rm`

**Description**
Deletes a provider configuration. **Warning**: This action will also delete all models that depend on this provider.

**Options**

  * `-f, --force`: Skip the confirmation prompt.

**Examples**

```bash
# Delete the 'anthropic-dev' provider
construct provider delete anthropic-dev
```

-----

## System & Configuration

### Config Commands: `construct config`

Manage CLI configuration settings.

#### `construct config set <key> <value>`

Set a configuration value.

**Usage**

```bash
construct config set <key> <value>
```

**Description**
Sets a persistent configuration key-value pair. Use dot notation for nested keys.

**Examples**

```bash
# Set the default agent for the 'new' command
construct config set cmd.new.agent "coder"

# Set the default output format to JSON
construct config set output.format "json"
```

#### `construct config get <key>`

Get a configuration value.

**Usage**

```bash
construct config get <key>
```

**Examples**

```bash
# Get the default agent for the 'new' command
construct config get cmd.new.agent
```

#### `construct config list`

List all current configuration values.

**Usage**

```bash
construct config list
```

### Daemon Commands: `construct daemon`

Manage the `construct` background daemon.

#### `construct daemon install`

Install and enable the Construct daemon as a system service.

**Usage**

```bash
construct daemon install [flags]
```

**Description**
Installs the daemon using the appropriate service manager for your OS (e.g., launchd on macOS, systemd on Linux). The daemon is required for most `construct` operations.

**Examples**

```bash
# Install the daemon with default settings
construct daemon install
```

#### `construct daemon run`

Run the daemon process in the foreground.

**Usage**

```bash
construct daemon run [flags]
```

**Description**
Starts the daemon process directly in the current terminal. This is useful for debugging and development. For normal use, `construct daemon install` is recommended.

**Options**

  * `--listen-http <address>`: The address and port to listen on (e.g., `127.0.0.1:8080`).

#### `construct daemon stop`

Stop the running daemon service.

**Usage**

```bash
construct daemon stop
```

#### `construct daemon uninstall`

Uninstall the Construct daemon from the system.

**Usage**

```bash
construct daemon uninstall [flags]
```

**Options**

  * `-y, --yes`: Skip the confirmation prompt.

-----

### Utility Commands

#### `construct version`

Print the version of the Construct CLI.

**Usage**

```bash
construct version
```

#### `construct update`

Update the Construct CLI to the latest version.

**Usage**

```bash
construct update
```

```
```