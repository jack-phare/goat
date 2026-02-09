package prompt

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// SkillWatcher watches skill directories for changes and updates the registry.
type SkillWatcher struct {
	registry *SkillRegistry
	dirs     []string
	debounce time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewSkillWatcher creates a new SkillWatcher.
func NewSkillWatcher(registry *SkillRegistry, dirs []string) *SkillWatcher {
	return &SkillWatcher{
		registry: registry,
		dirs:     dirs,
		debounce: 500 * time.Millisecond,
	}
}

// Start begins watching directories for changes. Call the returned cancel to stop.
func (w *SkillWatcher) Start(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancel = cancel
	w.mu.Unlock()

	// Add directories and their immediate subdirs
	for _, dir := range w.dirs {
		if err := w.addDirAndChildren(watcher, dir); err != nil {
			// Log but don't fail — directory might not exist yet
			log.Printf("skill watcher: skipping %s: %v", dir, err)
		}
	}

	go w.run(ctx, watcher)
	return nil
}

// Stop stops the watcher.
func (w *SkillWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

// addDirAndChildren adds a directory and its immediate children to the watcher.
func (w *SkillWatcher) addDirAndChildren(watcher *fsnotify.Watcher, dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	if err := watcher.Add(dir); err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // not critical
	}
	for _, entry := range entries {
		if entry.IsDir() {
			subdir := filepath.Join(dir, entry.Name())
			_ = watcher.Add(subdir)
		}
	}
	return nil
}

// run is the main event loop for the watcher.
func (w *SkillWatcher) run(ctx context.Context, watcher *fsnotify.Watcher) {
	defer watcher.Close()

	var debounceTimer *time.Timer
	pendingReload := false

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Only care about SKILL.md files
			if filepath.Base(event.Name) != "SKILL.md" && !isSkillDir(event.Name) {
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}

			// Schedule a debounced reload
			pendingReload = true
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(w.debounce, func() {
				if pendingReload {
					w.reload(event)
					pendingReload = false
				}
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("skill watcher error: %v", err)
		}
	}
}

// isSkillDir checks if a path is a potential skill directory (parent of SKILL.md).
func isSkillDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// reload processes a file system event and updates the registry.
func (w *SkillWatcher) reload(event fsnotify.Event) {
	path := event.Name

	// Resolve to SKILL.md path
	skillFile := path
	if filepath.Base(path) != "SKILL.md" {
		skillFile = filepath.Join(path, "SKILL.md")
	}

	// Determine skill name from directory
	skillName := filepath.Base(filepath.Dir(skillFile))

	switch {
	case event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0:
		// Skill deleted
		w.registry.Unregister(skillName)
		log.Printf("skill watcher: removed skill %q", skillName)

	case event.Op&(fsnotify.Write|fsnotify.Create) != 0:
		// Skill created or modified
		entry, err := ParseSkillFile(skillFile)
		if err != nil {
			log.Printf("skill watcher: error reloading %s: %v", skillFile, err)
			return
		}
		w.registry.Register(*entry)
		log.Printf("skill watcher: reloaded skill %q", entry.Name)
	}
}
