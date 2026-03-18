# njupt-net

`njupt-net` is a Go terminal system for NJUPT Self-Service and Portal workflows.

The repository name is still `njupt-net-cli`, but the shipped product surface is the `njupt-net` command and its typed protocol kernel.

## Source Of Truth

Protocol truth and behavior constraints come from:

- [doc/FINAL-SSOT.md](doc/FINAL-SSOT.md)
- [doc/IMPLEMENTATION-TASK.md](doc/IMPLEMENTATION-TASK.md)

Project-level architecture and capability tracking live in:

- [doc/ARCHITECTURE-REVIEW.md](doc/ARCHITECTURE-REVIEW.md)
- [doc/CAPABILITY-MATRIX.md](doc/CAPABILITY-MATRIX.md)

## Architecture

The repository is intentionally single-repo and Go-first.

- `cmd/njupt-net`: CLI entrypoint and command wiring
- `internal/kernel`: evidence model, typed results, diagnostics, transport contract
- `internal/selfservice`: Self-Service protocol implementation
- `internal/portal`: Portal 802 primary flow and 801 guarded fallback
- `internal/workflow`: higher-level repair and migration workflows
- `internal/app`: config, output, and explicit app context
- `scripts/`: supported build and smoke-test helpers
- `legacy/experimental/`: historical reverse-engineering and guard tooling

Historical Python guard scripts remain under `legacy/experimental/` only as behavior references. They are not part of the formal release surface.

## Command Tree

The command surface is organized around stable domains:

- `njupt-net self`
- `njupt-net dashboard`
- `njupt-net service`
- `njupt-net setting`
- `njupt-net bill`
- `njupt-net portal`
- `njupt-net raw`

## Build

Quick compile check:

```bash
go build ./...
```

Cross-build release targets:

```bash
./scripts/build.sh all
```

```powershell
.\scripts\build.ps1 -Mode all
```

## Quality Gates

Local verification:

```bash
go test ./...
go test -cover ./...
go vet ./...
```

The GitHub Actions release workflow also enforces:

- `gofmt`
- `go test -cover ./...`
- `go vet ./...`
- `staticcheck ./...`
- cross-platform release builds

## Local Smoke Test

The smoke script reads `credentials.json` and exercises the new command tree:

```powershell
.\scripts\test-local.ps1
```

Include side-effecting writes:

```powershell
.\scripts\test-local.ps1 -IncludeWriteOps
```

## Design Rules

- Protocol truth belongs in the kernel and protocol packages, not in CLI formatting code.
- `confirmed`, `guarded`, and `blocked` semantics must stay explicit.
- Write operations use readback verification and optional restore.
- Dead experimental code should not stay on the main path.
