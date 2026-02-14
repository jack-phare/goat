package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMCPConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	os.WriteFile(path, []byte(`{
		"filesystem": {
			"type": "stdio",
			"command": "npx",
			"args": ["-y", "@anthropic/mcp-server-filesystem", "/workspace"]
		}
	}`), 0644)

	servers, err := loadMCPConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	fs, ok := servers["filesystem"]
	if !ok {
		t.Fatal("expected 'filesystem' key")
	}
	if fs.Type != "stdio" {
		t.Errorf("expected type 'stdio', got %q", fs.Type)
	}
	if fs.Command != "npx" {
		t.Errorf("expected command 'npx', got %q", fs.Command)
	}
	if len(fs.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(fs.Args))
	}
}

func TestLoadMCPConfig_MultipleServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	os.WriteFile(path, []byte(`{
		"fs": {
			"type": "stdio",
			"command": "npx",
			"args": ["-y", "@anthropic/mcp-server-filesystem"]
		},
		"remote": {
			"type": "http",
			"url": "https://example.com/mcp"
		}
	}`), 0644)

	servers, err := loadMCPConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers["fs"].Type != "stdio" {
		t.Errorf("expected fs type 'stdio', got %q", servers["fs"].Type)
	}
	if servers["remote"].Type != "http" {
		t.Errorf("expected remote type 'http', got %q", servers["remote"].Type)
	}
	if servers["remote"].URL != "https://example.com/mcp" {
		t.Errorf("expected remote URL, got %q", servers["remote"].URL)
	}
}

func TestLoadMCPConfig_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	os.WriteFile(path, []byte(`{}`), 0644)

	_, err := loadMCPConfig(path)
	if err == nil {
		t.Fatal("expected error for empty config")
	}
	if got := err.Error(); got != "MCP config is empty (no servers defined)" {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestLoadMCPConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	os.WriteFile(path, []byte(`not json`), 0644)

	_, err := loadMCPConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadMCPConfig_MissingFile(t *testing.T) {
	_, err := loadMCPConfig("/nonexistent/path/mcp.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
