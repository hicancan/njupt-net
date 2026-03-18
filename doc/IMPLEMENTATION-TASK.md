# IMPLEMENTATION-TASK.md

## Goal

Implement a complete Go project for the `njupt-net-cli` **CLI kernel** based only on `doc/FINAL-SSOT.md`.

This repository is for the **CLI kernel only**.  
Do not implement GUI, daemon/background service, browser extension, or OS-specific Wi-Fi automation unless `Final-SSOT.md` explicitly requires a minimal adapter abstraction.

## Source of truth

`doc/FINAL-SSOT.md` is the **only protocol and behavior source of truth**.

Rules:

1. Do **not** infer protocol behavior from historical reverse-engineering notes.
2. Do **not** search for or import assumptions from older NJUPT scripts unless the SSOT explicitly incorporates them.
3. If something is marked `confirmed`, implement it directly.
4. If something is marked `guarded`, implement it with conservative runtime behavior and explicit warnings/errors.
5. If something is marked `blocked`, do **not** fabricate success semantics. Expose it only in a clearly guarded/experimental way or omit command exposure if the SSOT says so.

## Required implementation scope

Implement the full Go CLI kernel with these characteristics:

### 1. Project structure
Use a clean Go layout oriented around **kernel + outer layer**:

- `cmd/`
- `internal/kernel/` or equivalent kernel domain package(s)
- `internal/selfservice/`
- `internal/portal/`
- `internal/workflow/`
- `internal/config/`
- `internal/output/`
- `internal/httpx/` or similar shared HTTP/session utilities

The exact package naming may vary, but the architecture must preserve:
- protocol/domain kernel isolation
- protocol implementation separation
- workflow separation
- CLI separation from protocol logic

### 2. Implement all confirmed CLI kernel capabilities
This includes all confirmed and implementable guarded functionality defined in the SSOT, especially:

- Self login flow
- Self logout flow
- login diagnosis / doctor-style checks if specified
- dashboard online list
- login history
- refresh-account raw probe behavior
- mauth state read
- mauth toggle
- operator binding state read
- operator binding write workflow with readback verification
- consume protect read
- consume protect write workflow with readback verification
- MAC list retrieval
- bill/log retrieval endpoints covered by SSOT
- Portal 802 login/logout
- Portal 801 fallback login/logout path, if SSOT requires exposure

### 3. Mandatory behavior rules
The implementation must obey these semantics everywhere they apply:

- Never treat HTTP 200 alone as business success.
- For write operations, use:
  - `PreRead`
  - `Submit`
  - `ReadBack`
  - optional `Restore`
- Login success must not rely only on HTML text hints.
- Use protected-page readability / verified session state as the authoritative login success signal.
- Use endpoint-specific parsing strategy instead of assuming uniform JSON.
- Preserve raw response snippets or structured diagnostics where the SSOT says they matter.

### 4. Capability levels
Represent feature certainty explicitly in code where useful:

- `confirmed`
- `guarded`
- `blocked`

This can be implemented as enums, typed constants, metadata, or structured command annotations.

### 5. CLI design
Implement a practical CLI with subcommands that map cleanly to the SSOT.  
The CLI should be suitable as the primary human and scripting interface.

Requirements:
- machine-readable JSON output mode
- readable human output mode
- explicit non-zero exit codes on failure
- `--yes` or equivalent guard for side-effecting commands
- dry-run support where sensible for guarded write workflows
- config file and env var support

### 6. Safety requirements
For side-effecting operations:
- require explicit confirmation unless clearly safe
- support readback verification
- never silently report success without evidence
- surface guarded/blocked semantics clearly in command output

### 7. Configuration
Support a configuration model suitable for local CLI usage:
- base URLs
- credentials and account fields
- portal parameters
- output mode
- optional insecure TLS flag where required by SSOT realities
- session/cookie persistence if useful

### 8. Tests
Add tests for:
- parsers
- token extraction
- endpoint response interpretation
- write-operation readback verification logic
- error classification
- capability-level handling

Mocked/fixture-based tests are acceptable.

### 9. Documentation
Include:
- build instructions
- command usage
- config format
- examples
- explanation of confirmed vs guarded vs blocked behavior

## Explicit non-goals

Unless explicitly required by `Final-SSOT.md`, do not implement:

- GUI
- resident daemon/service
- Wi-Fi connection management
- platform-specific SSID switching
- auto-discovery of OS network adapters
- browser automation
- unrelated campus-network tools

## Code quality expectations

The generated code must be:

- idiomatic Go
- modular and readable
- production-oriented
- minimally dependent on external libraries unless a library provides clear value
- easy to audit against the SSOT

## Delivery expectation

Produce a complete buildable Go project that:
1. compiles successfully,
2. exposes the CLI kernel commands,
3. follows the SSOT semantics exactly,
4. clearly distinguishes confirmed, guarded, and blocked behavior,
5. is ready for iterative extension without redesigning the core.

## Final instruction

When implementation choices are ambiguous, prefer:

1. fidelity to `doc/FINAL-SSOT.md`
2. conservative runtime behavior
3. explicit diagnostics
4. strict kernel/outer-layer separation
5. maintainability over cleverness
