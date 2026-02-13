---
description: "Testing strategies and patterns for Go: table-driven tests, mocks, fixtures, and coverage"
when_to_use: "When writing, reviewing, or improving tests"
context: inline
---

# Go Testing Patterns

You are an expert in Go testing. Apply the following patterns and strategies when writing or reviewing tests.

## Table-Driven Tests

Always prefer table-driven tests for functions with multiple input/output combinations:

```go
func TestParseConfig(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    *Config
        wantErr string
    }{
        {
            name:  "valid config",
            input: `{"port": 8080}`,
            want:  &Config{Port: 8080},
        },
        {
            name:    "empty input",
            input:   "",
            wantErr: "unexpected end of JSON",
        },
        {
            name:    "negative port",
            input:   `{"port": -1}`,
            wantErr: "port must be positive",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseConfig([]byte(tt.input))
            if tt.wantErr != "" {
                require.ErrorContains(t, err, tt.wantErr)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

Key rules for table tests:
- Name each case descriptively (it appears in test output).
- Use `t.Run` for subtests so failures identify the exact case.
- Put `wantErr` as a string, not a bool -- it documents the expected failure.
- Use `t.Parallel()` in subtests when cases are independent and thread-safe.

## Test Helpers

Extract common setup into helpers that accept `testing.TB`:

```go
func setupTestDB(t testing.TB) *sql.DB {
    t.Helper()
    db, err := sql.Open("postgres", testDSN)
    require.NoError(t, err)
    t.Cleanup(func() { db.Close() })
    return db
}
```

Rules:
- Always call `t.Helper()` first so failures report the caller's line.
- Use `t.Cleanup()` instead of `defer` -- cleanup runs even if the test panics.
- Accept `testing.TB` (not `*testing.T`) so helpers work in benchmarks too.

## Mocking with Interfaces

Define small interfaces at the point of use and create test doubles:

```go
// In production code (service layer):
type UserStore interface {
    GetByID(ctx context.Context, id string) (*User, error)
}

// In test file:
type mockUserStore struct {
    getByIDFunc func(ctx context.Context, id string) (*User, error)
}

func (m *mockUserStore) GetByID(ctx context.Context, id string) (*User, error) {
    return m.getByIDFunc(ctx, id)
}

func TestUserService_GetUser(t *testing.T) {
    store := &mockUserStore{
        getByIDFunc: func(_ context.Context, id string) (*User, error) {
            if id == "123" {
                return &User{ID: "123", Name: "Alice"}, nil
            }
            return nil, ErrNotFound
        },
    }
    svc := NewUserService(store)

    user, err := svc.GetUser(context.Background(), "123")
    require.NoError(t, err)
    assert.Equal(t, "Alice", user.Name)
}
```

Rules:
- Keep interfaces small (1-3 methods). Don't mock large interfaces.
- Prefer hand-written mocks over generated ones for simple interfaces.
- Use `gomock` or `mockery` only when the interface is large or used across many tests.
- For HTTP clients, use `httptest.NewServer` instead of mocking.

## HTTP Handler Testing

Use `httptest` for testing HTTP handlers:

```go
func TestGetUser(t *testing.T) {
    handler := NewHandler(mockStore)
    
    req := httptest.NewRequest("GET", "/users/123", nil)
    rec := httptest.NewRecorder()
    
    handler.GetUser(rec, req)
    
    assert.Equal(t, http.StatusOK, rec.Code)
    
    var resp User
    err := json.NewDecoder(rec.Body).Decode(&resp)
    require.NoError(t, err)
    assert.Equal(t, "123", resp.ID)
}
```

For integration tests with a full router:

```go
func TestAPI_Integration(t *testing.T) {
    srv := httptest.NewServer(setupRouter(testDB))
    defer srv.Close()
    
    resp, err := http.Get(srv.URL + "/api/v1/users/123")
    require.NoError(t, err)
    defer resp.Body.Close()
    assert.Equal(t, http.StatusOK, resp.StatusCode)
}
```

## Test Fixtures and Golden Files

For complex expected outputs, use golden files:

```go
func TestRenderTemplate(t *testing.T) {
    got := RenderTemplate(input)
    
    golden := filepath.Join("testdata", t.Name()+".golden")
    if *update {
        os.WriteFile(golden, []byte(got), 0644)
    }
    
    want, err := os.ReadFile(golden)
    require.NoError(t, err)
    assert.Equal(t, string(want), got)
}
```

For test fixtures (input data):
- Store in `testdata/` directory (ignored by `go build`).
- Use `os.ReadFile(filepath.Join("testdata", "input.json"))`.
- Name files after the test: `testdata/TestParseConfig_valid.json`.

## Error Path Testing

Always test error paths, not just happy paths:

```go
func TestCreateUser_Errors(t *testing.T) {
    tests := []struct {
        name      string
        input     CreateUserRequest
        storeErr  error
        wantCode  int
    }{
        {
            name:     "empty name",
            input:    CreateUserRequest{Name: ""},
            wantCode: http.StatusBadRequest,
        },
        {
            name:     "duplicate email",
            input:    CreateUserRequest{Name: "Alice", Email: "a@b.com"},
            storeErr: ErrAlreadyExists,
            wantCode: http.StatusConflict,
        },
        {
            name:     "store failure",
            input:    CreateUserRequest{Name: "Alice", Email: "a@b.com"},
            storeErr: errors.New("connection refused"),
            wantCode: http.StatusInternalServerError,
        },
    }
    // ... run subtests
}
```

## Test Organization

- Test file lives next to source: `user_service.go` -> `user_service_test.go`.
- Use `_test` package suffix for black-box tests: `package service_test`.
- Use same package (no suffix) only when testing unexported functions.
- Group tests logically: one `Test` function per public method, with subtests for cases.
- Run `go test -race ./...` in CI to catch data races.
- Use `-count=1` to disable test caching during development: `go test -count=1 ./...`.
- Use `t.Skip("reason")` for tests that require external dependencies not available in CI.

## Benchmarks

Write benchmarks for performance-critical code:

```go
func BenchmarkParseConfig(b *testing.B) {
    data := []byte(`{"port": 8080, "host": "localhost"}`)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ParseConfig(data)
    }
}
```

- Call `b.ResetTimer()` after expensive setup.
- Use `b.ReportAllocs()` to track allocations.
- Run with `go test -bench=. -benchmem ./...`.
