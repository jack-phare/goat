package prompt

import (
	"testing"
)

func TestSubstituteArgs_Single(t *testing.T) {
	body := "Deploy to $environment now."
	result, err := SubstituteArgs(body, []string{"environment"}, "production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Deploy to production now." {
		t.Errorf("result = %q", result)
	}
}

func TestSubstituteArgs_Multiple(t *testing.T) {
	body := "Deploy $app to $environment with $flags."
	result, err := SubstituteArgs(body, []string{"app", "environment", "flags"}, "myapp staging --dry-run --verbose")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Deploy myapp to staging with --dry-run --verbose." {
		t.Errorf("result = %q", result)
	}
}

func TestSubstituteArgs_LastArgCapturesRemainder(t *testing.T) {
	body := "Run: $command"
	result, err := SubstituteArgs(body, []string{"command"}, "git commit -m 'fix bug'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Run: git commit -m 'fix bug'" {
		t.Errorf("result = %q", result)
	}
}

func TestSubstituteArgs_MissingArgLeavesPlaceholder(t *testing.T) {
	body := "Deploy to $environment with $flags."
	result, err := SubstituteArgs(body, []string{"environment", "flags"}, "production")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Deploy to production with $flags." {
		t.Errorf("result = %q", result)
	}
}

func TestSubstituteArgs_NoArgsDefined_NoArgsProvided(t *testing.T) {
	body := "Static body."
	result, err := SubstituteArgs(body, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Static body." {
		t.Errorf("result = %q", result)
	}
}

func TestSubstituteArgs_EmptyArgsString(t *testing.T) {
	body := "Deploy to $env."
	result, err := SubstituteArgs(body, []string{"env"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Deploy to $env." {
		t.Errorf("result = %q (expected placeholder left in place)", result)
	}
}

func TestSubstituteArgs_QuotedArgs(t *testing.T) {
	body := "Message: $msg to $target."
	result, err := SubstituteArgs(body, []string{"msg", "target"}, `"hello world" prod`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Message: hello world to prod." {
		t.Errorf("result = %q", result)
	}
}

func TestSubstituteArgs_MultipleOccurrences(t *testing.T) {
	body := "$name says hello, $name says goodbye."
	result, err := SubstituteArgs(body, []string{"name"}, "Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Alice says hello, Alice says goodbye." {
		t.Errorf("result = %q", result)
	}
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  []string
	}{
		{"a b c", 3, []string{"a", "b", "c"}},
		{"a b c d e", 2, []string{"a", "b c d e"}},
		{"single", 1, []string{"single"}},
		{"a", 3, []string{"a"}},
		{"", 2, nil},
		{`"hello world" foo`, 2, []string{"hello world", "foo"}},
	}

	for _, tt := range tests {
		got := splitArgs(tt.input, tt.n)
		if len(got) != len(tt.want) {
			t.Errorf("splitArgs(%q, %d) = %v, want %v", tt.input, tt.n, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitArgs(%q, %d)[%d] = %q, want %q", tt.input, tt.n, i, got[i], tt.want[i])
			}
		}
	}
}
