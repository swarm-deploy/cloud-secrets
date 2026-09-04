# Contributing

Thank you for your interest in contributing to **cloud-secrets**.

Contributions are welcome, including bug fixes, new secret providers, tests, documentation, and other improvements.

For significant architectural changes or changes to the provider contract or synchronization model, please open an issue before starting implementation.

## Development

### Requirements

- Go version specified in `go.mod`
- Docker and Swarm mode
- `golangci-lint` for linting

Clone the repository:

```bash
git clone https://github.com/swarm-deploy/cloud-secrets.git
cd cloud-secrets
```

Run the tests:

```bash
make test
```

Run the linter:

```bash
make lint
```

Build the Docker image:

```bash
make build
```

Run end-to-end tests:

```bash
make e2e
```

## Pull Requests

Please keep pull requests focused on a single logical change.

Before submitting a pull request:

- add tests for new behavior when applicable;
- add a regression test for bug fixes when practical;
- update documentation for user-facing changes;
- run `make test` and `make lint`;
- avoid unrelated refactoring.

The pull request description should explain what changed and why.

## Adding a Provider

Providers implement the contract defined in `internal/providers/contracts/provider.go`:

```go
type Provider interface {
	// Definition returns human-readable provider definition.
	Definition() ProviderDefinition
	// GetSecretPayload retrieves latest secret payload by provider path returned from ListSecrets.
	GetSecretPayload(ctx context.Context, path string) ([]byte, error)
	// ListSecrets lists secret metadata without loading payload.
	ListSecrets(ctx context.Context) (map[string]Secret, error)
}

type Secret struct {
	// VersionID is the latest external version identifier.
	VersionID string
	// Path is the provider path within the synchronization scope.
	Path string
	// FullPath is the full secret path in external storage.
	FullPath string
}

type ProviderDefinition struct {
	Name string
	URL  string
}
```

When adding a provider:

- add the implementation under `internal/providers/<provider>`;
- add and validate provider-specific configuration;
- register the provider in `internal/config/provider.go`;
- wire provider creation in `internal/providers/factory.go`;
- avoid loading secret payloads during `ListSecrets`;
- provide a stable `VersionID` that changes when the external secret changes;
- never log secret payloads or credentials;
- add unit tests;
- add end-to-end tests when practical;
- document configuration and usage under `docs/`;
- add the provider to the supported providers list in `README.md`.

Provider-specific behavior should remain inside the provider implementation. The synchronizer should not need to understand provider-specific APIs or data models.

## Issues

When reporting a bug, please include enough information to reproduce the problem:

- cloud-secrets version
- provider
- configuration, environment variables
- expected behavior
- actual behavior
- relevant logs
- reproduction steps

Never include real tokens, passwords, secret payloads, SecretIDs, or other credentials in issues, pull requests, tests, or logs.

## Documentation

Documentation changes are welcome.

When introducing or changing user-facing configuration or behavior, update the corresponding documentation as part of the same pull request.

## License

By contributing to this project, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
