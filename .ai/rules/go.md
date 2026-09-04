---
name: go
description: Rules for writing code in Go
globs: ["**/*.go"]
apply: by file patterns
---

# Comments
- Each interface method must have a comment.
- Each structure exported field must have a comment.

# Go Logging Rules

- **Primary Library**: Always use the `log/slog` standard library for all logging needs.
- **Format**: Ensure all logs are structured (JSON format where possible) using key-value pairs.
- **Error Handling**: When logging an error, use `slog.ErrorContext`.
- **Avoid**: Never log raw error strings without context or PII.
- Always log with Go context. Use `slog.InfoContext` instead of `slog.Info`

# Go Testing
- For asserts use library `github.com/stretchr/testify/assert`. Example: `assert.Equal(t, 123, 123, "they should be equal")`
- For stoppable asserts use library `github.com/stretchr/testify/require`. Example: `require.Equal(t, 123, 123, "they should be equal")`
- Use table-driven pattern

## Mocking
- Do not write handwritten fakes, stubs, or mocks when the task requires mocking dependencies.
- For mocks always use `go.uber.org/mock/gomock` and generated mocks.
- Place generated mocks next to the package that owns the mocked interface.
- Do not introduce ad-hoc test helper types that imitate production interfaces unless explicitly requested.

# Environment
- Environment variables are described in the `Config` structure
- Secret values like API key or DSN must be loaded from mounted file.
- Every time you change configs update the documentation in the `docs/**` folder

## Structure initialization
- For DTO use default initialization.
- For service components use constructor functions like New{StructName} to initialize structs. In structure methods not repeat dependency validation.
- Do not use nil-guard for injected dependencies.
- All injected dependencies are mandatory unless explicitly marked optional.
- Never mask wiring errors with fallback behavior.
