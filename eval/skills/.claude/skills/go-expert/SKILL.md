---
description: "Go coding expert with deep knowledge of idioms, error handling, and standard library"
when_to_use: "When writing, reviewing, or debugging Go code"
context: inline
---

# Go Expert Knowledge

You have deep expertise in Go programming. Apply the following idioms and patterns when writing or reviewing Go code.

## Error Handling

- Always wrap errors with context using `fmt.Errorf("doing X: %w", err)` to build a chain of context.
- Use `errors.Is()` and `errors.As()` for error inspection, never compare error strings.
- Define sentinel errors as package-level `var` with `errors.New()`:
  ```go
  var ErrNotFound = errors.New("not found")
  ```
- For errors that carry structured data, define a custom type implementing `error`:
  ```go
  type ValidationError struct {
      Field   string
      Message string
  }
  func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }
  ```
- Return early on errors. The happy path should be at the left margin.

## Naming and Style

- Use short, descriptive variable names. Receivers are 1-2 letters (e.g., `s` for a server, `r` for a reader).
- Exported names use MixedCaps. No underscores in Go names (except test functions and generated code).
- Interfaces describe behavior: name them with `-er` suffix when possible (`Reader`, `Writer`, `Closer`).
- Accept interfaces, return structs. This maximizes flexibility for callers.
- Package names are short, lowercase, singular nouns (e.g., `http`, `json`, `user`).

## Struct Design

- Use struct embedding for composition, not inheritance:
  ```go
  type Server struct {
      http.Server           // embedded: promotes methods
      logger      *slog.Logger // unexported field
  }
  ```
- Use functional options for complex constructors:
  ```go
  type Option func(*Config)
  func WithTimeout(d time.Duration) Option { return func(c *Config) { c.Timeout = d } }
  func New(opts ...Option) *Server { /* apply opts */ }
  ```
- Zero values should be useful. Design structs so `var s MyStruct` is a valid starting state.

## Concurrency

- Never start a goroutine without knowing how it will stop. Use `context.Context` for cancellation.
- Use `errgroup.Group` for coordinating parallel work with error propagation:
  ```go
  g, ctx := errgroup.WithContext(ctx)
  g.Go(func() error { return doWork(ctx) })
  if err := g.Wait(); err != nil { /* handle */ }
  ```
- Prefer channels for communication, mutexes for state. But `sync.Mutex` is fine for simple cases.
- Use `sync.Once` for lazy initialization, not double-checked locking.
- Buffer channels when the producer shouldn't block: `ch := make(chan Item, 100)`.

## Standard Library

- Use `slog` (Go 1.21+) for structured logging, not `log` or `fmt.Println`:
  ```go
  slog.Info("request handled", "method", r.Method, "path", r.URL.Path, "duration", elapsed)
  ```
- Use `net/http` directly for HTTP servers. `http.HandlerFunc` adapters are idiomatic.
- Use `encoding/json` with struct tags. Use `json.NewDecoder` for streaming, `json.Unmarshal` for known-size data.
- Use `os.ReadFile` / `os.WriteFile` for simple file I/O. Use `bufio.Scanner` for line-by-line.
- Use `filepath.Join` for paths, never string concatenation with `/`.
- Use `testing` + `t.Run` for subtests. Use `testify/assert` only if already in the project.

## Code Organization

- Keep `main()` thin: parse flags, wire dependencies, call `run()` that returns an error.
- Group related functions in the same file. One type per file is NOT required in Go.
- Use internal packages (`internal/`) to prevent external imports of implementation details.
- Use `//go:embed` for static assets that need to be in the binary.

## Performance

- Profile before optimizing. Use `pprof` and benchmarks (`func BenchmarkX(b *testing.B)`).
- Preallocate slices when the size is known: `make([]T, 0, expectedLen)`.
- Use `strings.Builder` for building strings in a loop, not `+=`.
- Use `sync.Pool` for frequently allocated short-lived objects.
