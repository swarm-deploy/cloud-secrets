## Project Context
- Name of project = cloud-secrets
- This project implements service background service that synchronizes secrets from external secret managers with Docker Swarm

## Projects Rules
- Before starting any coding task, load and follow all files in `.ai/rules/*.md`
- Treat frontmatter as policy:
- `apply: always` means rule is always active.
- `apply: by file patterns` + `globs` means apply only to matching files.
- `alwaysApply: true` means apply regardless of globs.
- In the first progress update, briefly state which `.ai` rules were loaded.

## Project structure
- `./internal` - Backend on Golang
- `./internal/config` - Configuration of service
- `./internal/engine` - Package with working Swarm API
- `./internal/metrics` - Metrics for Swarm Deploy
- `./internal/providers` - Secret providers
- `./internal/sync` - Sync pipeline
