package agent

import "testing"

func TestParseSlashCommand_ValidCommand(t *testing.T) {
	name, args, ok := ParseSlashCommand(`/commit -m "fix bug"`, nil)
	if !ok {
		t.Fatal("expected isSlash=true")
	}
	if name != "commit" {
		t.Errorf("name = %q, want %q", name, "commit")
	}
	if args != `-m "fix bug"` {
		t.Errorf("args = %q", args)
	}
}

func TestParseSlashCommand_NoArgs(t *testing.T) {
	name, args, ok := ParseSlashCommand("/deploy", nil)
	if !ok {
		t.Fatal("expected isSlash=true")
	}
	if name != "deploy" {
		t.Errorf("name = %q, want %q", name, "deploy")
	}
	if args != "" {
		t.Errorf("args = %q, want empty", args)
	}
}

func TestParseSlashCommand_RegularMessage(t *testing.T) {
	_, _, ok := ParseSlashCommand("just a regular message", nil)
	if ok {
		t.Error("expected isSlash=false for regular message")
	}
}

func TestParseSlashCommand_BuiltinHelp(t *testing.T) {
	_, _, ok := ParseSlashCommand("/help", nil)
	if ok {
		t.Error("expected isSlash=false for built-in /help")
	}
}

func TestParseSlashCommand_BuiltinClear(t *testing.T) {
	_, _, ok := ParseSlashCommand("/clear", nil)
	if ok {
		t.Error("expected isSlash=false for built-in /clear")
	}
}

func TestParseSlashCommand_BuiltinCompact(t *testing.T) {
	_, _, ok := ParseSlashCommand("/compact", nil)
	if ok {
		t.Error("expected isSlash=false for built-in /compact")
	}
}

func TestParseSlashCommand_EmptyInput(t *testing.T) {
	_, _, ok := ParseSlashCommand("", nil)
	if ok {
		t.Error("expected isSlash=false for empty input")
	}
}

func TestParseSlashCommand_JustSlash(t *testing.T) {
	_, _, ok := ParseSlashCommand("/", nil)
	if ok {
		t.Error("expected isSlash=false for just /")
	}
}

func TestParseSlashCommand_WithWhitespace(t *testing.T) {
	name, args, ok := ParseSlashCommand("  /review-pr 123  ", nil)
	if !ok {
		t.Fatal("expected isSlash=true")
	}
	if name != "review-pr" {
		t.Errorf("name = %q, want %q", name, "review-pr")
	}
	if args != "123" {
		t.Errorf("args = %q, want %q", args, "123")
	}
}

func TestParseSlashCommand_UnknownCommand(t *testing.T) {
	name, _, ok := ParseSlashCommand("/unknown-skill", nil)
	if !ok {
		t.Fatal("expected isSlash=true for unknown command")
	}
	if name != "unknown-skill" {
		t.Errorf("name = %q", name)
	}
}

func TestParseSlashCommand_QualifiedName(t *testing.T) {
	name, args, ok := ParseSlashCommand("/ms-office-suite:pdf document.pdf", nil)
	if !ok {
		t.Fatal("expected isSlash=true")
	}
	if name != "ms-office-suite:pdf" {
		t.Errorf("name = %q", name)
	}
	if args != "document.pdf" {
		t.Errorf("args = %q", args)
	}
}
