package subagent

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchDebounce = 200 * time.Millisecond

// WatchDirs configures which directories to watch for agent file changes.
type WatchDirs struct {
	ProjectDir string   // .claude/agents/ in the project
	UserDir    string   // ~/.claude/agents/
	PluginDirs []string // plugin agent directories
}

// Watch monitors agent definition directories for file changes and calls Reload
// when .md files are created, modified, or removed. The method blocks until ctx
// is cancelled. Running agents are NOT affected — only new spawns use reloaded definitions.
func (m *Manager) Watch(ctx context.Context, dirs WatchDirs) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Add directories to watch
	watchPaths := collectWatchPaths(dirs)
	for _, dir := range watchPaths {
		_ = watcher.Add(dir) // ignore errors for missing dirs
	}

	if len(watchPaths) == 0 {
		// Nothing to watch — block until ctx cancelled
		<-ctx.Done()
		return ctx.Err()
	}

	var (
		debounceTimer *time.Timer
		mu            sync.Mutex
		pending       bool
	)

	// CWD for reload — derive from ProjectDir
	cwd := ""
	if dirs.ProjectDir != "" {
		// ProjectDir is typically <cwd>/.claude/agents
		cwd = filepath.Dir(filepath.Dir(dirs.ProjectDir))
	}

	doReload := func() {
		mu.Lock()
		pending = false
		mu.Unlock()
		if cwd != "" {
			m.Reload(cwd)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Only care about .md files
			if !strings.HasSuffix(event.Name, ".md") {
				continue
			}
			// Only care about create/write/remove
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove) == 0 {
				continue
			}

			// Debounce: reset timer on each event
			mu.Lock()
			if !pending {
				pending = true
				debounceTimer = time.AfterFunc(watchDebounce, doReload)
			} else {
				debounceTimer.Reset(watchDebounce)
			}
			mu.Unlock()

		case _, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			// Ignore watcher errors — not fatal
		}
	}
}

// collectWatchPaths gathers all valid directories to watch.
func collectWatchPaths(dirs WatchDirs) []string {
	var paths []string
	if dirs.ProjectDir != "" {
		paths = append(paths, dirs.ProjectDir)
	}
	if dirs.UserDir != "" {
		paths = append(paths, dirs.UserDir)
	}
	paths = append(paths, dirs.PluginDirs...)
	return paths
}
