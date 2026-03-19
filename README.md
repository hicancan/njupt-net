# njupt-net

[中文文档](README.zh-CN.md) | English

[![Verify](https://github.com/hicancan/njupt-net-cli/actions/workflows/release.yml/badge.svg)](https://github.com/hicancan/njupt-net-cli/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/hicancan/njupt-net-cli)](https://github.com/hicancan/njupt-net-cli/blob/main/go.mod)
[![Latest Release](https://img.shields.io/github/v/release/hicancan/njupt-net-cli)](https://github.com/hicancan/njupt-net-cli/releases)
[![Repo Stars](https://img.shields.io/github/stars/hicancan/njupt-net-cli?style=social)](https://github.com/hicancan/njupt-net-cli/stargazers)

`njupt-net` is a Go-first terminal system for NJUPT Self-Service and Portal workflows.

It is not a thin wrapper around a few endpoints. It is a typed protocol kernel plus a disciplined CLI/runtime layer designed for:

- confirmed vs guarded vs blocked capability modeling
- readback-first write verification
- stable machine-readable JSON output
- cross-platform guard runtime with explicit status and event logs
- long-term maintainability in a single repository and a single binary

The repository name remains `njupt-net-cli`, but the supported product surface is the `njupt-net` command.

## Table Of Contents

- [Why This Project](#why-this-project)
- [Highlights](#highlights)
- [Architecture](#architecture)
- [Command Surface](#command-surface)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Machine-Readable Contract](#machine-readable-contract)
- [Guard Runtime](#guard-runtime)
- [Evidence Model](#evidence-model)
- [Quality Gates](#quality-gates)
- [Project Layout](#project-layout)
- [Documentation](#documentation)

## Why This Project

NJUPT networking is not one clean API. It is a combination of:

- the Self-Service system
- the Portal gateway
- HTML pages with embedded state
- JSON and JSONP endpoints
- guarded and blocked paths that cannot be treated as fully confirmed protocol truth

`njupt-net` exists to make that environment operable without hiding uncertainty.

Instead of pretending every endpoint is equally trustworthy, the project turns reverse-engineered certainty into first-class runtime semantics.

## Highlights

- Typed kernel with explicit `confirmed`, `guarded`, and `blocked` evidence levels
- Stable `--output json` contract for automation and scripting
- Confirmed Self login chain with protected-page readback
- Readback-first write flows for binding, consume-protect, and mauth toggling
- Portal 802 as the primary implementation, 801 retained as guarded fallback
- Cross-platform Go guard runtime with:
  - day `B` / night `W` scheduling
  - aggressive recovery without `logout`
  - typed nested status
  - structured JSONL events
  - graceful stop-first supervision
- Architecture guardrail tests to keep protocol truth out of the wrong layers

## Architecture

The project is intentionally a modular monolith.

```text
cmd/njupt-net
  -> internal/app
      -> internal/kernel
      -> internal/selfservice
      -> internal/portal
      -> internal/workflow
      -> internal/runtime/guard
```

### Design Rules

- `cmd/njupt-net`
  - flag parsing and command wiring only
- `internal/app`
  - lazy config loading
  - renderer selection
  - client/session factories
  - confirmation policy
- `internal/kernel`
  - evidence levels
  - typed operation results
  - typed problem catalog
  - transport contract
  - writeflow semantics
- `internal/selfservice`
  - Self protocol truth only
  - endpoint/request orchestration
  - parser/helper functions
  - typed model mapping
- `internal/portal`
  - Portal request building
  - raw/JSONP parsing
  - ret_code classification
  - typed model mapping
- `internal/workflow`
  - pure use-cases only
  - no concrete transport construction
- `internal/runtime/guard`
  - typed runtime state machine
  - scheduler, probe, runner, supervisor, store

This keeps the CLI thin and keeps protocol truth where it belongs.

## Command Surface

`njupt-net` currently exposes 8 domain groups and 32 leaf commands.

### Top-Level Groups

- `self`
- `dashboard`
- `service`
- `setting`
- `bill`
- `portal`
- `raw`
- `guard`

### Full Command Tree

```text
njupt-net
  self
    login
    logout
    status
    doctor
  dashboard
    online-list
    login-history
    refresh-account-raw
    offline
    mauth get
    mauth toggle
  service
    binding get
    binding set
    consume get
    consume set
    mac list
    migrate
  setting
    person get
    person update
  bill
    month-pay
    online-log
    operator-log
  portal
    login
    logout
    login-801
    logout-801
  raw
    get
    post
  guard
    run
    start
    stop
    status
    once
```

### Capability Map

| Domain | Commands | Purpose |
| --- | --- | --- |
| `self` | login, logout, status, doctor | authoritative Self auth and diagnosis |
| `dashboard` | online sessions, login history, mauth, guarded offline | operational state and guarded actions |
| `service` | binding, consume, MAC, migrate | broadband state and controlled write flows |
| `setting` | person get/update | guarded user-security surface |
| `bill` | month-pay, online-log, operator-log | billing and usage records |
| `portal` | 802 login/logout, 801 fallback login/logout | gateway authentication |
| `raw` | GET and POST probes | low-level debugging |
| `guard` | run, start, stop, status, once | scheduled daemon and recovery runtime |

## Quick Start

### 1. Build

```bash
go build ./...
```

Cross-platform release builds:

```bash
bash ./scripts/build.sh all
```

```powershell
.\scripts\build.ps1 -Mode all
```

### 2. Create `credentials.json`

Minimal example:

```json
{
  "accounts": {
    "B": {
      "username": "your-student-id",
      "password": "your-password"
    },
    "W": {
      "username": "your-student-id",
      "password": "your-password"
    }
  },
  "cmcc": {
    "account": "your-mobile-broadband-account",
    "password": "your-mobile-broadband-password"
  },
  "self": {
    "baseURL": "http://10.10.244.240:8080",
    "timeoutSeconds": 10
  },
  "portal": {
    "baseURL": "https://10.10.244.11:802/eportal/portal",
    "fallbackBaseURLs": [
      "https://p.njupt.edu.cn:802/eportal/portal"
    ],
    "isp": "mobile",
    "timeoutSeconds": 8,
    "insecureTLS": true
  },
  "guard": {
    "stateDir": "dist/guard",
    "probeIntervalSeconds": 3,
    "bindingCheckIntervalSeconds": 180,
    "timezone": "Asia/Shanghai",
    "schedule": {
      "dayProfile": "B",
      "nightProfile": "W",
      "nightStart": "23:30",
      "nightEnd": "07:00"
    }
  }
}
```

### 3. Run Common Commands

```bash
njupt-net self login --profile B
njupt-net self status --profile B
njupt-net service binding get --profile B
njupt-net portal login --profile B --ip 10.163.177.138
njupt-net guard start --replace
njupt-net guard status --output json
```

### 4. Local Smoke Test

```powershell
.\scripts\test-local.ps1
```

Read-only smoke:

```powershell
.\scripts\test-local.ps1 -ReadOnly -SkipPortal
```

## Configuration

`njupt-net` resolves config in this order:

1. `--config <path>`
2. `NJUPT_NET_CONFIG`
3. `credentials.json`

Environment overrides also exist for selected runtime defaults:

- `NJUPT_NET_OUTPUT`
- `NJUPT_NET_SELF_BASE_URL`
- `NJUPT_NET_PORTAL_BASE_URL`
- `NJUPT_NET_PORTAL_ISP`
- `NJUPT_NET_INSECURE_TLS`
- `NJUPT_NET_SELF_TIMEOUT`
- `NJUPT_NET_PORTAL_TIMEOUT`

Commands that do not require protocol credentials should still work without a readable config file, for example:

- `njupt-net --help`
- `njupt-net completion ...`
- `njupt-net guard status --state-dir ...`
- `njupt-net guard stop --state-dir ...`

## Machine-Readable Contract

`--output json` is a supported machine interface.

### Operation Output

All commands return a typed `OperationResult` envelope:

- `level`
- `success`
- `message`
- `data`
- `problems`
- `raw`

### Problems

Problems are a versioned machine contract.

Each problem exposes:

- `code`
- `message`
- `details`

Major typed families include:

- portal problems
- readback / restore / state-comparison problems
- invalid-config problems
- guarded / blocked capability problems

### Guard Status

`guard status --output json` uses a nested typed contract:

- `running`
- `health`
- `desiredProfile`
- `scheduleWindow`
- `binding`
- `connectivity`
- `portal`
- `cycle`
- `timing`
- `log`

### Guard Events

Guard runtime events are emitted as JSONL records with stable `kind` values plus typed `details`.

Supported kinds:

- `startup`
- `schedule-switch`
- `binding-audit`
- `portal-login`
- `binding-repair`
- `degraded`
- `shutdown`
- `fatal`

Human-readable output can evolve more freely than these JSON contracts.

## Guard Runtime

The Go guard is the supported long-running runtime model.

### Behavior

- day profile: `B`
- night profile: `W`
- probe interval: `3s` by default
- no proactive `logout`
- immediate recovery chain on connectivity loss
- graceful stop request before forced kill

### Runtime Files

Default state directory:

```text
dist/guard
```

Important files:

- `status.json`
- `current-log.txt`
- `current-events.txt`
- `logs/*.log`
- `logs/*.events.jsonl`
- `launcher.pid`
- `worker.pid`

### Health Model

- `healthy`
  - desired profile is correct
  - binding is correct
  - final connectivity is restored
- `degraded`
  - one full recovery cycle finished without reaching the target state
- `stopped`
  - guard is not running

## Evidence Model

The reverse-engineered certainty model is part of the runtime API.

### `confirmed`

Safe to implement as a normal supported capability.

Examples:

- Self login chain with readback
- broadband binding set with readback
- consume-protect write with readback
- mauth toggle with before/after verification
- Portal 802 primary login flow

### `guarded`

Exposed, but intentionally conservative.

Examples:

- force offline
- Portal 801 fallback
- selected write paths whose success semantics are environment-sensitive

### `blocked`

Known endpoint surface exists, but success semantics are not yet strong enough to promise as fully supported truth.

Examples:

- blocked/guarded user-security update paths
- unconfirmed Portal ret_code families

## Quality Gates

Local gates:

```bash
go test ./...
go test -cover ./...
go vet ./...
```

```powershell
.\scripts\build.ps1 -Mode all
.\scripts\test-local.ps1 -ReadOnly -SkipPortal
```

GitHub Actions enforces:

- formatting
- tests
- coverage run
- `go vet`
- `staticcheck`
- release build matrix

## Project Layout

```text
.
├── cmd/njupt-net
├── doc
├── internal/app
├── internal/kernel
├── internal/portal
├── internal/runtime/guard
├── internal/selfservice
├── internal/workflow
└── scripts
```

## Documentation

Source-of-truth and architecture documents:

- [doc/FINAL-SSOT.md](doc/FINAL-SSOT.md)
- [doc/IMPLEMENTATION-TASK.md](doc/IMPLEMENTATION-TASK.md)
- [doc/ARCHITECTURE-REVIEW.md](doc/ARCHITECTURE-REVIEW.md)
- [doc/CAPABILITY-MATRIX.md](doc/CAPABILITY-MATRIX.md)

## Repository Notes

- The repo name is `njupt-net-cli`, but the supported product name is `njupt-net`.
- Historical Python/PowerShell guard helpers are no longer the supported runtime path.
- The mainline product is the Go command, the typed kernel, and the Go guard runtime.
