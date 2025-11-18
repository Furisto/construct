# Tool Calling in Construct

## Introduction

Tool calling is fundamental to how AI coding agents interact with their environment. When an agent needs to read a file, search code, or execute a command, it must invoke tools that provide these capabilities. The design of this tool-calling interface profoundly affects what agents can accomplish and how efficiently they work.

Traditional AI agent systems use structured formats like JSON or templated text for tool calls. An agent generates a JSON object specifying the tool name and parameters, the system executes it, and returns results. While straightforward, this approach constrains agents to predefined patterns and struggles with tasks requiring composition, iteration, or conditional logic.

Construct takes a different approach: agents write executable JavaScript code to call tools. Instead of generating rigid data structures, agents write programs that combine tools using the full expressiveness of a programming language. This document explains how this works, why it provides significant advantages, and when it shines brightest.

## How It Works

In Construct, all tools are exposed as JavaScript functions in a sandboxed execution environment. When an agent needs to use tools, it writes JavaScript code that calls these functions directly:

```javascript
// Agent writes code like this
const result = grep({
  query: "authenticate",
  path: "/project/src",
  include_pattern: "*.ts"
});

print(`Found ${result.total_matches} matches`);
```

The environment provides:
- **Tool functions**: Each tool (`read_file`, `grep`, `execute_command`, etc.) is available as a global function
- **JavaScript runtime**: Full language features including variables, loops, conditionals, and error handling
- **Sandboxed execution**: Code runs in isolation with controlled access to the filesystem and system
- **Immediate feedback**: Results are returned directly, allowing multi-step logic in a single execution

This architecture enables agents to write sophisticated programs that compose multiple tools, make decisions based on results, and process data efficiently—all within a single turn.

## Key Advantages

### 1. Composability: Multiple Tools in One Execution

Traditional JSON-based tool calling requires separate invocations for each operation. Need to search five files? That's five separate tool calls, each requiring a round trip to the model.

With JavaScript, agents can compose multiple operations in a single execution:

```javascript
// Search multiple patterns and aggregate results
const patterns = ["TODO", "FIXME", "HACK", "XXX"];
const allMatches = [];

for (const pattern of patterns) {
  const result = grep({
    query: pattern,
    path: "/project/src",
    include_pattern: "*.go"
  });

  result.matches.forEach(match => {
    allMatches.push({
      pattern: pattern,
      file: match.file_path,
      line: match.line_number
    });
  });
}

print(`Found ${allMatches.length} total code annotations`);
```

This single execution performs four searches and aggregates results. With JSON tool calling, this would require at minimum four separate model invocations, plus additional logic to aggregate results.

### 2. Control Flow: Conditionals, Loops, and Logic

Real coding tasks rarely follow linear paths. You often need to make decisions based on what you discover:

```javascript
// Check if tests exist before running them
const testFiles = find_file({
  pattern: "**/*_test.go",
  path: "/project"
});

if (testFiles.files.length > 0) {
  print(`Found ${testFiles.files.length} test files, running tests...`);
  const result = execute_command("go test ./...");

  if (result.exitCode !== 0) {
    print("❌ Tests failed, analyzing output...");
    // Could search for specific error patterns here
  } else {
    print("✅ All tests passed");
  }
} else {
  print("⚠️ No test files found, skipping tests");
}
```

This conditional logic adapts the workflow based on discovered context. JSON-based approaches would require multiple round trips—one to discover files, another to decide whether to test, another to handle results.

### 3. Context Efficiency: Filter and Transform Data

Tool results can be large. A file might have thousands of lines, a search might return hundreds of matches. Returning everything to the model wastes context.

Writing code allows agents to filter and extract only what's needed:

```javascript
// Find all code-generated files efficiently
const result = grep({
  query: "^// Code generated",
  path: "/project",
  include_pattern: "*.go",
  max_results: 500
});

// Extract unique file paths only (not full match details)
const uniqueFiles = new Set();
result.matches.forEach(match => {
  uniqueFiles.add(match.file_path);
});

// Create clean output list
const fileList = Array.from(uniqueFiles)
  .map(f => f.replace('/project/', ''))
  .sort();

create_file('/project/GENERATED_FILES.txt', fileList.join('\n'));
print(`✅ Listed ${fileList.length} generated files`);
```

This code processes 500 potential matches but only extracts unique file paths, significantly reducing context usage. The agent gets exactly the information it needs without overwhelming the context window with redundant data.

### 4. Self-Correction: Data-Driven Decision Making

Effective agents adapt their approach based on what they discover. Writing code makes this natural:

```javascript
// Systematically update routes, handling each file appropriately
const routeFiles = find_file({
  pattern: "**/*route*.ts",
  path: "/project/src"
});

print(`Processing ${routeFiles.files.length} route files`);

for (const routeFile of routeFiles.files) {
  // Check if this file actually needs changes
  const unprotectedEndpoints = regex_search({
    query: "router\\.(get|post|put|delete)\\([^,]+,\\s*(?!authenticateToken)",
    path: routeFile
  });

  if (unprotectedEndpoints.total_matches > 0) {
    print(`${routeFile}: Found ${unprotectedEndpoints.total_matches} unprotected endpoints`);

    // Build edits dynamically based on actual findings
    const edits = [{
      old: "import express from 'express';",
      new: "import express from 'express';\nimport { authenticateToken } from '../middleware/auth';"
    }];

    unprotectedEndpoints.matches.forEach(match => {
      edits.push({
        old: match.line,
        new: match.line.replace(/(\([^,]+,\s*)/, '$1authenticateToken, ')
      });
    });

    edit_file(routeFile, edits);
    print(`✅ Protected ${unprotectedEndpoints.total_matches} endpoints`);
  } else {
    print(`${routeFile}: Already secure - skipping`);
  }
}
```

The agent discovers files, examines each one individually, and only makes changes where needed. It builds edits dynamically based on actual code patterns found. This adaptive approach is natural in code but awkward to express in static JSON.

### 5. Natural for Language Models

Modern LLMs are trained extensively on code. They understand loops, conditionals, variable assignment, and function composition intuitively. Asking them to write JavaScript to call tools leverages this existing capability.

Research and industry adoption validate this approach: Wang et al. (2024) demonstrated that code-based tool calling achieves up to 20% higher success rates compared to JSON and text-based approaches across 17 different language models. Anthropic has also experimented with code execution for tool calling, reporting significant improvements in efficiency and capability. The models aren't learning a new tool-calling convention—they're using skills they already have.

### 6. Reduced Communication Overhead

Each model invocation has latency—time for the request, inference, and response. Complex tasks can require dozens of tool calls with JSON-based approaches.

Writing code reduces this overhead by batching operations and executing control flow locally. Conditional logic and loops run in the execution environment rather than requiring repeated model evaluations, significantly improving time-to-first-token latency:

```javascript
// Single execution gathering all git context needed
const status = execute_command("git status --porcelain");
const staged = execute_command("git diff --cached --stat");
const recentCommits = execute_command("git log --oneline -5");

print("=== Git Status ===");
print(status.stdout);

print("\n=== Staged Changes ===");
print(staged.stdout);

print("\n=== Recent Commits ===");
print(recentCommits.stdout);

// Now make informed decision based on all context
if (staged.stdout.trim().length > 0) {
  print("✅ Ready to commit");
} else {
  print("⚠️ No staged changes");
}
```

Three command executions, analysis, and decision making in one turn. This efficiency compounds across complex workflows.

### 7. Privacy and Data Handling

When processing sensitive information, code execution provides an additional layer of control. Intermediate results can remain in the execution environment without flowing through the model's context:

```javascript
// Process sensitive data without exposing it to the model
const users = read_file('/project/data/users.json');
const parsedUsers = JSON.parse(users.content);

// Filter and aggregate without sending raw data to model context
const summary = {
  total: parsedUsers.length,
  activeCount: parsedUsers.filter(u => u.status === 'active').length,
  roles: [...new Set(parsedUsers.map(u => u.role))]
};

// Only the summary enters the model's context
print(`User summary: ${JSON.stringify(summary)}`);

// Write detailed report to file (stays in execution environment)
const detailedReport = parsedUsers.map(u =>
  `${u.id}: ${u.email} - ${u.role}`
).join('\n');
create_file('/project/user_report.txt', detailedReport);
```

The raw user data never enters the model's context—only the aggregated summary does. The detailed report is written directly to a file. This pattern allows agents to work with sensitive data while minimizing exposure, providing better privacy and security guarantees than approaches where all tool results flow through the model.

## Comparison: JSON vs JavaScript

Consider a task: "Find all TypeScript files that import 'express', check if they have proper error handling, and create a report."

**JSON Approach:**
1. Call `find_files` tool → get list of TS files
2. For each file, call `read_file` tool → N more tool calls
3. For each file content, call `analyze_imports` tool → N more tool calls
4. For each file with express, call `check_error_handling` tool → M more tool calls
5. Call `create_file` tool with report → 1 more tool call

Total: 2N + M + 2 model invocations (where N = total files, M = files with express)

**JavaScript Approach:**
```javascript
// Single execution
const tsFiles = find_file({
  pattern: "**/*.ts",
  path: "/project/src"
});

const report = [];

for (const file of tsFiles.files) {
  const content = read_file(file);

  if (content.content.includes("import")) {
    const hasExpress = /from ['"]express['"]/.test(content.content);
    const hasTryCatch = /try\s*{/.test(content.content);

    if (hasExpress) {
      report.push({
        file: file,
        hasErrorHandling: hasTryCatch
      });
    }
  }
}

// Generate markdown report
const markdown = report.map(item =>
  `- ${item.file}: ${item.hasErrorHandling ? '✅' : '❌'} Error handling`
).join('\n');

create_file('/project/EXPRESS_AUDIT.md', markdown);
print(`✅ Audited ${report.length} files`);
```

Total: 1 model invocation

Writing code isn't always better—for simple single-tool calls, JSON is perfectly adequate. But for anything involving iteration, composition, or conditional logic, the code-based approach provides substantial advantages.

## Research Foundation

This approach is grounded in peer-reviewed research. Wang et al. (2024) published "Executable Code Actions Elicit Better LLM Agents" at ICML 2024, demonstrating that code-based tool calling significantly outperforms traditional approaches.

Key findings:
- **20% higher success rate** across 17 different language models on API-Bank and M3ToolEval benchmarks
- **30% fewer steps required** to complete tasks, reducing token usage proportionally
- **Self-debugging capability**: Agents can observe error messages and revise their approach autonomously
- **Expanded action space**: Access to software packages and libraries beyond hand-crafted tools

The research shows this isn't a marginal improvement—it's a fundamental advantage in how agents reason about and execute tool-based tasks.

## When This Approach Shines

Writing JavaScript to call tools provides the most benefit in these scenarios:

**Multi-file operations**: Tasks requiring iteration over files, directories, or search results. The ability to loop naturally and make per-item decisions is powerful.

**Conditional workflows**: When the next action depends on what you discover. Real software engineering is full of "if this, then that" logic that code handles elegantly.

**Data processing and aggregation**: Extracting, transforming, and combining information from multiple sources. Being able to filter, map, and reduce data prevents context bloat.

**Exploratory tasks**: Understanding a codebase, finding patterns, or investigating issues. The ability to make decisions based on discoveries and adapt the search strategy mid-execution is valuable.

**Complex edits**: Making systematic changes across multiple files based on actual code patterns, not assumptions. Dynamic edit generation based on search results.

For simple tasks—reading a single file, running one command—traditional JSON tool calling is perfectly adequate. The advantages emerge when tasks require composition, logic, and adaptation.

## Conclusion

Tool calling design shapes what AI agents can accomplish. By using executable code instead of static data structures, Construct enables agents to think and act more like programmers—composing tools, making decisions, and adapting to discoveries.

This isn't just a technical choice; it's a fundamental enabler of more capable, efficient agents. The research validates it, and the practical benefits are clear: fewer round trips, better context usage, and more sophisticated workflows.

When you see a Construct agent efficiently processing dozens of files, making context-aware decisions, and adapting its strategy based on what it finds—that's this approach at work.

## References

Wang, X., Chen, Y., Yuan, L., Zhang, Y., Li, Y., Peng, H., & Ji, H. (2024). Executable Code Actions Elicit Better LLM Agents. *International Conference on Machine Learning (ICML) 2024*. https://arxiv.org/abs/2402.01030

Anthropic Engineering. (2024). Code Execution with MCP. https://www.anthropic.com/engineering/code-execution-with-mcp
