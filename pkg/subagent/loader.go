package subagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Loader discovers and loads agent definitions from the filesystem.
type Loader struct {
	cwd        string
	userDir    string   // ~/.claude/agents/
	pluginDirs []string // plugin agent directories
}

// NewLoader creates a Loader that scans for agent definitions.
// cwd is used to find .claude/agents/ in the project.
// userDir defaults to ~/.claude/agents/ if empty.
func NewLoader(cwd string, userDir string, pluginDirs ...string) *Loader {
	if userDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			userDir = filepath.Join(home, ".claude", "agents")
		}
	}
	return &Loader{
		cwd:        cwd,
		userDir:    userDir,
		pluginDirs: pluginDirs,
	}
}

// LoadAll discovers and loads all file-based agent definitions.
// Returns a map of name → Definition. Higher-priority sources overwrite lower.
// Priority: Plugin (10) < User (20) < Project (30)
// Missing directories are silently skipped; malformed agent files return errors.
func (l *Loader) LoadAll() (map[string]Definition, error) {
	result := make(map[string]Definition)

	// 1. Plugin dirs (lowest priority)
	for _, dir := range l.pluginDirs {
		defs, err := l.scanDir(dir, SourcePlugin, 10)
		if err != nil {
			if isDirNotExist(err) {
				continue
			}
			return nil, err
		}
		for name, def := range defs {
			result[name] = def
		}
	}

	// 2. User agents (~/.claude/agents/)
	if l.userDir != "" {
		defs, err := l.scanDir(l.userDir, SourceUser, 20)
		if err != nil && !isDirNotExist(err) {
			return nil, err
		}
		for name, def := range defs {
			result[name] = def
		}
	}

	// 3. Project agents (.claude/agents/) — highest priority
	projectDir := filepath.Join(l.cwd, ".claude", "agents")
	defs, err := l.scanDir(projectDir, SourceProject, 30)
	if err != nil && !isDirNotExist(err) {
		return nil, err
	}
	for name, def := range defs {
		result[name] = def
	}

	return result, nil
}

// isDirNotExist returns true if the error is from a missing directory.
func isDirNotExist(err error) bool {
	return os.IsNotExist(err)
}

// scanDir reads all .md files from a directory and returns parsed definitions.
func (l *Loader) scanDir(dir string, source AgentSource, priority int) (map[string]Definition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	result := make(map[string]Definition)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		def, err := ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", entry.Name(), err)
		}

		def.Source = source
		def.Priority = priority
		result[def.Name] = *def
	}

	return result, nil
}
