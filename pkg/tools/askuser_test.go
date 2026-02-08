package tools

import (
	"context"
	"strings"
	"testing"
)

type mockHandler struct {
	answers map[string]string
	err     error
}

func (m *mockHandler) AskQuestions(_ context.Context, _ []QuestionSpec) (map[string]string, error) {
	return m.answers, m.err
}

func TestAskUser_WithMockHandler(t *testing.T) {
	tool := &AskUserQuestionTool{
		Handler: &mockHandler{
			answers: map[string]string{"Auth method": "JWT"},
		},
	}

	out, err := tool.Execute(context.Background(), map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Which auth method?",
				"header":      "Auth method",
				"multiSelect": false,
				"options": []any{
					map[string]any{"label": "JWT", "description": "JSON Web Tokens"},
					map[string]any{"label": "OAuth", "description": "OAuth 2.0"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "JWT") {
		t.Errorf("expected JWT in output, got %q", out.Content)
	}
}

func TestAskUser_NilHandler(t *testing.T) {
	tool := &AskUserQuestionTool{}
	out, err := tool.Execute(context.Background(), map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Test?",
				"header":      "Test",
				"multiSelect": false,
				"options": []any{
					map[string]any{"label": "A", "description": "Option A"},
					map[string]any{"label": "B", "description": "Option B"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for nil handler")
	}
	if !strings.Contains(out.Content, "not available") {
		t.Errorf("expected 'not available' error, got %q", out.Content)
	}
}

func TestAskUser_TooManyQuestions(t *testing.T) {
	tool := &AskUserQuestionTool{Handler: &mockHandler{}}
	questions := make([]any, 5)
	for i := range questions {
		questions[i] = map[string]any{
			"question":    "Q?",
			"header":      "H",
			"multiSelect": false,
			"options": []any{
				map[string]any{"label": "A", "description": "A"},
				map[string]any{"label": "B", "description": "B"},
			},
		}
	}

	out, err := tool.Execute(context.Background(), map[string]any{
		"questions": questions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for too many questions")
	}
}

func TestAskUser_TooManyOptions(t *testing.T) {
	tool := &AskUserQuestionTool{Handler: &mockHandler{}}
	out, err := tool.Execute(context.Background(), map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Pick one?",
				"header":      "Choice",
				"multiSelect": false,
				"options": []any{
					map[string]any{"label": "A", "description": "A"},
					map[string]any{"label": "B", "description": "B"},
					map[string]any{"label": "C", "description": "C"},
					map[string]any{"label": "D", "description": "D"},
					map[string]any{"label": "E", "description": "E"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for too many options")
	}
}

func TestAskUser_TooFewOptions(t *testing.T) {
	tool := &AskUserQuestionTool{Handler: &mockHandler{}}
	out, err := tool.Execute(context.Background(), map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Pick one?",
				"header":      "Choice",
				"multiSelect": false,
				"options": []any{
					map[string]any{"label": "A", "description": "A"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for too few options")
	}
}

func TestAskUser_HeaderTooLong(t *testing.T) {
	tool := &AskUserQuestionTool{Handler: &mockHandler{}}
	out, err := tool.Execute(context.Background(), map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Pick one?",
				"header":      "This header is way too long",
				"multiSelect": false,
				"options": []any{
					map[string]any{"label": "A", "description": "A"},
					map[string]any{"label": "B", "description": "B"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for long header")
	}
}

func TestAskUser_MissingRequiredFields(t *testing.T) {
	tool := &AskUserQuestionTool{Handler: &mockHandler{}}

	// Missing question
	out, _ := tool.Execute(context.Background(), map[string]any{
		"questions": []any{
			map[string]any{
				"header":      "Test",
				"multiSelect": false,
				"options": []any{
					map[string]any{"label": "A", "description": "A"},
					map[string]any{"label": "B", "description": "B"},
				},
			},
		},
	})
	if !out.IsError {
		t.Error("expected error for missing question field")
	}

	// Missing header
	out, _ = tool.Execute(context.Background(), map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Test?",
				"multiSelect": false,
				"options": []any{
					map[string]any{"label": "A", "description": "A"},
					map[string]any{"label": "B", "description": "B"},
				},
			},
		},
	})
	if !out.IsError {
		t.Error("expected error for missing header field")
	}
}

func TestAskUser_EmptyQuestions(t *testing.T) {
	tool := &AskUserQuestionTool{Handler: &mockHandler{}}
	out, _ := tool.Execute(context.Background(), map[string]any{
		"questions": []any{},
	})
	if !out.IsError {
		t.Error("expected error for empty questions")
	}
}
