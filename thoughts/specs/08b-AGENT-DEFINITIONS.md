# Spec 08b: Agent Definition Loading

**Go Package**: `pkg/subagent/` (loader subsystem)
**Depends on**: Spec 08 (AgentDefinition type), Spec 05 (prompt assembly)
**Source References**:
- [Sub-agents docs: Configure subagents](https://code.claude.com/docs/en/sub-agents.md)
- Claude Code CHANGELOG entries for v2.1.32-2.1.33

---

## 1. Purpose

Agent definitions can come from multiple sources: built-in defaults, file-based Markdown definitions, CLI flags, and plugins. This spec covers how definitions are discovered, parsed, and resolved when multiple sources define the same agent name.

---

## 2. Definition Sources & Priority

Definitions are loaded from multiple scopes. When multiple definitions share the same `name`, the highest-priority source wins.

| Priority | Source | Location | Lifetime |
|----------|--------|----------|----------|
| 1 (highest) | CLI flag | `--agents '{...}'` JSON | Current session only |
| 2 | Project | `.claude/agents/*.md` | Persistent, checked into VCS |
| 3 | User | `~/.claude/agents/*.md` | Persistent, all projects |
| 4 (lowest) | Plugin | `<plugin-dir>/agents/*.md` | Installed with plugin |
| — | Built-in | Compiled into binary | Always available |

Built-in agents (Explore, Plan, general-purpose, etc.) can be overridden by any file-based definition with the same name.

---

## 3. File Format

Agent definitions are Markdown files with YAML frontmatter. The YAML frontmatter maps to `AgentDefinition` fields. The Markdown body becomes the `Prompt` (system prompt).

```markdown
---
name: code-reviewer
description: Reviews code for quality and best practices
tools: Read, Glob, Grep, Bash
disallowedTools: Write, Edit
model: sonnet
permissionMode: default
maxTurns: 30
memory: user
skills:
  - api-conventions
  - error-handling-patterns
hooks:
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: "./scripts/validate-command.sh"
  PostToolUse:
    - matcher: "Edit|Write"
      hooks:
        - type: command
          command: "./scripts/run-linter.sh"
---

You are a senior code reviewer ensuring high standards of code quality.

When invoked:
1. Run git diff to see recent changes
2. Focus on modified files
3. Begin review immediately

Review checklist:
- Code is clear and readable
- No duplicated code
- Proper error handling
- No exposed secrets or API keys
```

---

## 4. Frontmatter Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | — | Unique identifier, lowercase + hyphens |
| `description` | string | Yes | — | When Claude should delegate to this agent |
| `tools` | comma-separated string or list | No | inherit all | Allowed tools; supports `Task(type)` syntax |
| `disallowedTools` | comma-separated string or list | No | — | Tools to deny |
| `model` | string | No | `inherit` | `sonnet`, `opus`, `haiku`, or `inherit` |
| `permissionMode` | string | No | — | `default`, `acceptEdits`, `delegate`, `dontAsk`, `bypassPermissions`, `plan` |
| `maxTurns` | int | No | — | Max agentic turns |
| `skills` | list | No | — | Skill names to preload (full content injected) |
| `memory` | string | No | — | `user`, `project`, or `local` |
| `mcpServers` | list | No | — | MCP server names or inline definitions |
| `hooks` | map | No | — | Scoped lifecycle hooks |
| `criticalSystemReminder_EXPERIMENTAL` | string | No | — | Critical reminder text |

### 4.1 Tools Field Parsing

The `tools` field accepts either a comma-separated string or a YAML list:

```yaml
# String format
tools: Read, Grep, Glob, Bash, Task(worker, researcher)

# List format
tools:
  - Read
  - Grep
  - Glob
  - Bash
  - Task(worker, researcher)
```

Both parse to `[]string{"Read", "Grep", "Glob", "Bash", "Task(worker, researcher)"}`.

---

## 5. Go Types

```go
// Loader discovers and parses agent definitions from all sources.
type Loader struct {
    cwd        string
    userDir    string   // ~/.claude/agents/
    pluginDirs []string // plugin agent directories
}

func NewLoader(cwd string) *Loader

// LoadAll discovers definitions from all file-based sources.
// Does NOT include CLI flag agents (those are passed separately).
func (l *Loader) LoadAll() (map[string]AgentDefinition, error)

// LoadFile parses a single agent definition Markdown file.
func (l *Loader) LoadFile(path string) (*AgentDefinition, error)

// Resolve merges all sources with priority resolution.
// builtIn: compiled-in defaults
// fileBased: from LoadAll()
// cliAgents: from --agents JSON flag
func Resolve(builtIn, fileBased map[string]AgentDefinition, cliAgents map[string]AgentDefinition) map[string]AgentDefinition
```

---

## 6. Loading Flow

```go
func (l *Loader) LoadAll() (map[string]AgentDefinition, error) {
    result := make(map[string]AgentDefinition)

    // 1. Load plugin agents (lowest priority file-based source)
    for _, dir := range l.pluginDirs {
        defs, err := l.loadDir(dir, AgentSourcePlugin)
        if err != nil { return nil, err }
        for name, def := range defs {
            result[name] = def
        }
    }

    // 2. Load user agents (overrides plugins)
    userDefs, err := l.loadDir(l.userDir, AgentSourceUser)
    if err != nil { return nil, err }
    for name, def := range userDefs {
        result[name] = def
    }

    // 3. Load project agents (overrides user)
    projectDir := filepath.Join(l.cwd, ".claude", "agents")
    projectDefs, err := l.loadDir(projectDir, AgentSourceProject)
    if err != nil { return nil, err }
    for name, def := range projectDefs {
        result[name] = def
    }

    return result, nil
}

func (l *Loader) loadDir(dir string, source AgentSource) (map[string]AgentDefinition, error) {
    result := make(map[string]AgentDefinition)
    entries, err := os.ReadDir(dir)
    if os.IsNotExist(err) {
        return result, nil // directory doesn't exist, skip
    }
    if err != nil {
        return nil, err
    }

    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        def, err := l.LoadFile(filepath.Join(dir, entry.Name()))
        if err != nil {
            return nil, fmt.Errorf("loading %s: %w", entry.Name(), err)
        }
        def.Source = source
        result[def.Name] = *def
    }
    return result, nil
}
```

---

## 7. Frontmatter Parsing

```go
func (l *Loader) LoadFile(path string) (*AgentDefinition, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    // Split frontmatter from body
    frontmatter, body, err := splitFrontmatter(data)
    if err != nil {
        return nil, fmt.Errorf("parsing frontmatter in %s: %w", path, err)
    }

    // Parse YAML frontmatter into AgentDefinition
    var def AgentDefinition
    if err := yaml.Unmarshal(frontmatter, &def); err != nil {
        return nil, fmt.Errorf("parsing YAML in %s: %w", path, err)
    }

    // Markdown body becomes the system prompt
    def.Prompt = strings.TrimSpace(string(body))

    // Validate required fields
    if def.Name == "" {
        return nil, fmt.Errorf("%s: name is required", path)
    }
    if def.Description == "" {
        return nil, fmt.Errorf("%s: description is required", path)
    }

    return &def, nil
}

// splitFrontmatter separates YAML frontmatter (between --- delimiters) from body.
func splitFrontmatter(data []byte) (frontmatter []byte, body []byte, err error) {
    s := string(data)
    if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
        return nil, nil, fmt.Errorf("file must start with --- frontmatter delimiter")
    }

    // Find closing ---
    rest := s[4:] // skip opening ---\n
    idx := strings.Index(rest, "\n---\n")
    if idx == -1 {
        idx = strings.Index(rest, "\n---\r\n")
    }
    if idx == -1 {
        // Check if file ends with ---
        if strings.HasSuffix(strings.TrimSpace(rest), "---") {
            trimmed := strings.TrimSpace(rest)
            return []byte(trimmed[:len(trimmed)-3]), nil, nil
        }
        return nil, nil, fmt.Errorf("no closing --- frontmatter delimiter found")
    }

    return []byte(rest[:idx]), []byte(rest[idx+5:]), nil // +5 = \n---\n
}
```

---

## 8. Resolution (Priority Merge)

```go
func Resolve(builtIn, fileBased, cliAgents map[string]AgentDefinition) map[string]AgentDefinition {
    result := make(map[string]AgentDefinition)

    // 1. Start with built-in (lowest priority)
    for name, def := range builtIn {
        def.Source = AgentSourceBuiltIn
        result[name] = def
    }

    // 2. Overlay file-based (already priority-ordered by LoadAll)
    for name, def := range fileBased {
        result[name] = def
    }

    // 3. Overlay CLI agents (highest priority)
    for name, def := range cliAgents {
        def.Source = AgentSourceCLIFlag
        result[name] = def
    }

    return result
}
```

---

## 9. CLI `--agents` Flag

The `--agents` flag accepts a JSON object where keys are agent names and values are agent configs:

```bash
claude --agents '{
  "code-reviewer": {
    "description": "Expert code reviewer. Use proactively after code changes.",
    "prompt": "You are a senior code reviewer...",
    "tools": ["Read", "Grep", "Glob", "Bash"],
    "model": "sonnet"
  }
}'
```

### 9.1 Parsing

```go
// ParseCLIAgents parses the --agents JSON flag value.
func ParseCLIAgents(jsonStr string) (map[string]AgentDefinition, error) {
    if jsonStr == "" {
        return nil, nil
    }

    var raw map[string]AgentDefinition
    if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
        return nil, fmt.Errorf("parsing --agents JSON: %w", err)
    }

    // Set names from map keys
    result := make(map[string]AgentDefinition)
    for name, def := range raw {
        def.Name = name
        def.Source = AgentSourceCLIFlag
        result[name] = def
    }
    return result, nil
}
```

---

## 10. `/agents` Interactive Command

The `/agents` command provides an interactive management interface:
- **List**: Show all available subagents (built-in, user, project, plugin) with source indicators
- **Create**: Guided creation or Claude-generated definitions
- **Edit**: Modify existing subagent configuration
- **Delete**: Remove custom subagents (cannot delete built-in)
- **Inspect**: Show which agent wins when duplicates exist

This is a UI-layer feature. The subagent package exposes the data; the CLI/UI renders the interface.

```go
// AgentInfo provides display information for the /agents command.
type AgentInfo struct {
    Name        string
    Description string
    Model       string
    Source      AgentSource
    Tools       []string
    IsActive    bool // true if this definition wins priority resolution
    FilePath    string // empty for built-in/CLI
}

func (m *Manager) ListAgentInfo() []AgentInfo
```

---

## 11. Hot Reload

Agents defined via `/agents create` are available immediately without session restart. File-based agents discovered at session start are loaded once. The `/agents` command can trigger a reload:

```go
func (m *Manager) Reload(cwd string) error {
    loader := NewLoader(cwd)
    fileBased, err := loader.LoadAll()
    if err != nil {
        return err
    }

    m.mu.Lock()
    defer m.mu.Unlock()

    // Re-resolve with current CLI agents and built-ins
    m.agents = Resolve(m.builtIn, fileBased, m.cliAgents)
    return nil
}
```

---

## 12. Verification Checklist

- [ ] **File discovery**: `.claude/agents/`, `~/.claude/agents/`, plugin dirs scanned
- [ ] **Frontmatter parsing**: YAML frontmatter + Markdown body correctly split
- [ ] **All fields parsed**: name, description, tools, disallowedTools, model, permissionMode, maxTurns, skills, memory, mcpServers, hooks, criticalReminder
- [ ] **Tools string parsing**: Both comma-separated string and YAML list accepted
- [ ] **Task(type) syntax**: Preserved through parsing into AgentDefinition.Tools
- [ ] **Priority resolution**: CLI > Project > User > Plugin > Built-in
- [ ] **Name collisions**: Higher-priority source wins silently
- [ ] **Required fields**: name and description validated; error on missing
- [ ] **CLI --agents flag**: JSON parsed correctly, names from keys
- [ ] **Built-in override**: File-based agent with same name as built-in wins
- [ ] **Missing directories**: Non-existent agent dirs are skipped gracefully
- [ ] **Hot reload**: `/agents` command triggers re-discovery and resolution
- [ ] **AgentInfo**: List endpoint provides source, priority, active status for UI
