package hooks

import (
	"context"
	"time"
)

// defaultAsyncTimeout is the default timeout for async hooks (30 seconds).
const defaultAsyncTimeout = 30

// executeAsync runs a hook callback asynchronously with a timeout.
// If the hook returns an AsyncHookJSONOutput, it waits for the async result
// by re-running the callback with the async timeout.
func executeAsync(ctx context.Context, hook HookCallback, input any, asyncTimeout int) (HookJSONOutput, error) {
	if asyncTimeout <= 0 {
		asyncTimeout = defaultAsyncTimeout
	}

	asyncCtx, cancel := context.WithTimeout(ctx, time.Duration(asyncTimeout)*time.Second)
	defer cancel()

	// Re-execute the hook with the async timeout context.
	// The hook implementation is responsible for blocking until completion
	// within the async timeout period.
	return hook(input, "", asyncCtx)
}
