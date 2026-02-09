package subagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadWarning represents a non-fatal issue encountered during agent loading.
type LoadWarning struct {
	File  string // path to the file that caused the warning
	Error error  // the underlying error
}

func (w LoadWarning) String() string {
	return fmt.Sprintf("%s: %v", w.File, w.Error)
}

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
// Returns a map of name → Definition plus any warnings encountered.
// Higher-priority sources overwrite lower.
// Priority: Plugin (10) < User (20) < Project (30)
// Missing directories are silently skipped; malformed agent files produce warnings and are skipped.
// Only directory-level errors (permission denied etc.) are returned as hard errors.
func (l *Loader) LoadAll() (map[string]Definition, []LoadWarning, error) {
	result := make(map[string]Definition)
	var warnings []LoadWarning

	// 1. Plugin dirs (lowest priority)
	for _, dir := range l.pluginDirs {
		defs, w, err := l.scanDir(dir, SourcePlugin, 10)
		warnings = append(warnings, w...)
		if err != nil {
			if isDirNotExist(err) {
				continue
			}
			return nil, warnings, err
		}
		for name, def := range defs {
			result[name] = def
		}
	}

	// 2. User agents (~/.claude/agents/)
	if l.userDir != "" {
		defs, w, err := l.scanDir(l.userDir, SourceUser, 20)
		warnings = append(warnings, w...)
		if err != nil && !isDirNotExist(err) {
			return nil, warnings, err
		}
		for name, def := range defs {
			result[name] = def
		}
	}

	// 3. Project agents (.claude/agents/) — highest priority
	projectDir := filepath.Join(l.cwd, ".claude", "agents")
	defs, w, err := l.scanDir(projectDir, SourceProject, 30)
	warnings = append(warnings, w...)
	if err != nil && !isDirNotExist(err) {
		return nil, warnings, err
	}
	for name, def := range defs {
		result[name] = def
	}

	return result, warnings, nil
}

// isDirNotExist returns true if the error is from a missing directory.
func isDirNotExist(err error) bool {
	return os.IsNotExist(err)
}

// scanDir reads all .md files from a directory and returns parsed definitions.
// Individual file parse errors are collected as warnings and the file is skipped.
// Only directory-level errors are returned as hard errors.
func (l *Loader) scanDir(dir string, source AgentSource, priority int) (map[string]Definition, []LoadWarning, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	result := make(map[string]Definition)
	var warnings []LoadWarning
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			warnings = append(warnings, LoadWarning{
				File:  path,
				Error: readErr,
			})
			continue
		}
		def, fieldWarnings, parseErr := ParseContentWithWarnings(data, path)
		for _, fw := range fieldWarnings {
			warnings = append(warnings, LoadWarning{
				File:  path,
				Error: fmt.Errorf("%s", fw),
			})
		}
		if parseErr != nil {
			warnings = append(warnings, LoadWarning{
				File:  path,
				Error: parseErr,
			})
			continue // skip malformed file
		}

		def.Source = source
		def.Priority = priority
		result[def.Name] = *def
	}

	return result, warnings, nil
}
