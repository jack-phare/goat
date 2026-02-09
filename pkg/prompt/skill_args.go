package prompt

import (
	"strings"
)

// SubstituteArgs replaces $argName placeholders in a skill body with provided arguments.
// argDefs lists the expected argument names. argsStr is the raw argument string.
// Arguments are assigned positionally: the first whitespace-separated token maps to
// argDefs[0], the second to argDefs[1], etc. The last argument captures all remaining text.
// If fewer args are provided than defined, unmatched placeholders are left in place.
func SubstituteArgs(body string, argDefs []string, argsStr string) (string, error) {
	if len(argDefs) == 0 {
		return body, nil
	}

	argsStr = strings.TrimSpace(argsStr)
	if argsStr == "" {
		return body, nil
	}

	// Split args: first N-1 by whitespace, last captures remainder
	values := splitArgs(argsStr, len(argDefs))

	result := body
	for i, argName := range argDefs {
		if i < len(values) {
			result = strings.ReplaceAll(result, "$"+argName, values[i])
		}
		// If no value provided for this arg, leave the placeholder
	}

	return result, nil
}

// splitArgs splits s into at most n tokens. The last token captures all remaining text.
func splitArgs(s string, n int) []string {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []string{s}
	}

	var tokens []string
	remaining := s
	for i := 0; i < n-1; i++ {
		remaining = strings.TrimLeft(remaining, " \t")
		if remaining == "" {
			break
		}
		// Handle quoted strings
		if remaining[0] == '"' || remaining[0] == '\'' {
			quote := remaining[0]
			end := strings.IndexByte(remaining[1:], quote)
			if end >= 0 {
				tokens = append(tokens, remaining[1:1+end])
				remaining = remaining[2+end:]
				continue
			}
		}
		idx := strings.IndexAny(remaining, " \t")
		if idx < 0 {
			tokens = append(tokens, remaining)
			remaining = ""
			break
		}
		tokens = append(tokens, remaining[:idx])
		remaining = remaining[idx+1:]
	}
	remaining = strings.TrimSpace(remaining)
	if remaining != "" {
		tokens = append(tokens, remaining)
	}
	return tokens
}
