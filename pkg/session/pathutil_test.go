package session

import (
	"runtime"
	"testing"
)

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"unix root", "/", ""},
		{"unix absolute", "/Users/foo/bar", "Users-foo-bar"},
		{"unix deep path", "/home/user/projects/my-app", "home-user-projects-my-app"},
		{"empty string", "", ""},
		{"relative path", "foo/bar", "foo-bar"},
	}

	if runtime.GOOS == "windows" {
		// Windows paths use backslash
		tests = append(tests, struct {
			name string
			cwd  string
			want string
		}{"windows path", `C:\Users\foo`, `C:-Users-foo`})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizePath(tt.cwd)
			if got != tt.want {
				t.Errorf("SanitizePath(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestDefaultBaseDir(t *testing.T) {
	dir := DefaultBaseDir()
	if dir == "" {
		t.Error("DefaultBaseDir() returned empty string")
	}
	// Should end with .claude/projects
	if !containsSubpath(dir, ".claude") {
		t.Errorf("DefaultBaseDir() = %q, expected to contain .claude", dir)
	}
}

func containsSubpath(path, sub string) bool {
	for i := 0; i+len(sub) <= len(path); i++ {
		if path[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
