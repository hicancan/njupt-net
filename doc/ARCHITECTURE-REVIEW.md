# Architecture Review

## Goal

The repository now targets a single-repo, Go-first `njupt-net` terminal system rather than a narrow CLI prototype.

The implementation is organized around one rule:

`protocol truth stays in typed kernel/protocol packages; CLI and workflows compose it, but do not redefine it.`

## Round 1: Baseline Rebuild

Round 1 rebuilt the project around explicit boundaries:

- `internal/kernel`
  - evidence levels
  - typed operation results
  - diagnostics and sentinel errors
  - transport contract
  - write readback/restore result models
- `internal/app`
  - config loading
  - output mode selection
  - confirmation policy
  - explicit runtime context
- `internal/selfservice`
  - Self login/logout/status
  - dashboard/service/setting/bill protocol implementations
- `internal/portal`
  - Portal 802 primary implementation
  - Portal 801 blocked admin-console probe
- `internal/workflow`
  - multi-step doctor and migration workflows

The old prototype kernel package was removed after the new typed kernel replaced it.

## Round 2: Self Domain And CLI Surface

Round 2 focused on turning the command tree into a typed client of the kernel:

- `self`
  - `login`
  - `logout`
  - `status`
  - `doctor`
- `dashboard`
  - `online-list`
  - `login-history`
  - `refresh-account-raw`
  - `mauth get`
  - `mauth toggle`
  - `offline`
- `service`
  - `binding get|set`
  - `consume get|set`
  - `mac list`
  - `migrate`
- `setting`
  - `person get|update`
- `bill`
  - `online-log`
  - `month-pay`
  - `operator-log`
- `portal`
  - `login`
  - `logout`
  - `login-801`
  - `logout-801`
- `raw`
  - `get`
  - `post`

Write operations now consistently follow the same readback-first shape:

`pre-state -> submit -> readback -> compare -> optional restore`

## Round 3: Delivery And Quality

Round 3 focused on removal of prototype smells and release readiness:

- CI now follows `go.mod` for version selection
- formatting, tests, vet, and staticcheck are enforced
- local smoke testing targets the new command tree
- the repository documents both capability coverage and architecture boundaries

## Current Design Decisions

### Single repository

The project stays in one repository because the kernel, CLI, workflow, and delivery surfaces are still evolving together. Splitting into separate modules now would add versioning cost before the API is truly stable.

### Python scripts are legacy references

Historical Python guard scripts remain useful as behavior samples, but they are not part of the formal release contract. Future runtime/guard logic should live in Go when it becomes part of the supported product surface.

### Evidence levels are first-class

The SSOT certainty model is not a documentation note; it is part of the runtime API:

- `confirmed`
- `guarded`
- `blocked`

This keeps CLI output, workflow decisions, and tests aligned with the reverse-engineered truth model.

### CLI bootstrap is intentionally lazy

Root flags are parsed first, but config and protocol clients are only constructed when a command truly needs them.

This keeps:

- `--help`
- shell completion
- `guard status --state-dir ...`
- `guard stop --state-dir ...`

independent from `credentials.json` when those commands do not need protocol access.

### Machine-readable diagnostics are schema-first

Runtime JSON output no longer depends on ad hoc diagnostic maps.

- kernel results expose stable `problems[].code` with typed `details`
- guard runtime emits stable `event.kind` records with typed `details`
- `guard status` is a typed nested contract instead of a flat bag of booleans
- free-form messages remain human-facing context, not the machine contract

The supported JSON contract now centers on:

- `OperationResult`
- typed `problem.code + details`
- typed nested `guard status`
- typed `guard event.kind + details`

### Guard is a formal runtime subsystem

The Go guard is no longer just a loop with PID files.

It now has:

- explicit schedule evaluation
- typed cycle health (`healthy`, `degraded`, `stopped`)
- typed nested status sections for binding, connectivity, portal, cycle, timing, and log pointers
- graceful stop requests before forced kill
- structured JSONL runtime events
- bounded log retention

This keeps the long-running model cross-platform while avoiding OS-specific service adapters.

### Machine-readable contracts are frozen

The current JSON output shape is now treated as a supported compatibility surface rather than an exploratory output format.

Protected machine contracts:

- top-level `OperationResult`
- typed `problem.code + details`
- typed nested `guard status`
- typed `guard event.kind + details`

Human-readable `message` strings remain useful context, but they are not the compatibility boundary.

### ImmortalWrt deployment is now part of the supported product surface

The repository now ships an official router deployment path through `scripts/install-immortalwrt.ps1`.

That deployment model is intentionally simple:

- local PowerShell orchestration
- remote `procd + guard run`
- state directory under `/tmp` to avoid flash churn

This keeps router deployment aligned with the same single-binary, cross-platform runtime model used elsewhere.

## Keep / Rewrite / Remove Summary

### Keep

- `internal/httpx`
- `scripts/build.sh`
- `scripts/build.ps1`
- the SSOT documents in `doc/`

### Rewritten

- `cmd/njupt-net/*`
- `internal/config`
- `internal/output`
- `internal/selfservice`
- `internal/portal`
- `internal/workflow`

### Removed

- the prototype kernel package
- legacy command paths based on hidden shared session state
- placeholder protocol/client shells with no typed behavior

## Remaining Follow-Up

The project is now structurally ready, but there are still worthwhile future improvements:

- add more fixtures for rare HTML variants
- increase selfservice and portal coverage further
- add session persistence when the supported UX requires it
- keep machine contracts stable and extend coverage and golden tests instead of reshaping JSON
