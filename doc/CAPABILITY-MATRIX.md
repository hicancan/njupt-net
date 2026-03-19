# Capability Matrix

This matrix tracks the current implementation surface against the SSOT certainty model.

## Self

| Domain | Command / Endpoint | Level | Status | Notes |
| --- | --- | --- | --- | --- |
| Auth | `self login` / `/Self/login/*` | confirmed | implemented | checkcode fetch, randomCode, verify, protected-page readback |
| Auth | `self logout` / `/Self/login/logout` | confirmed | implemented | verified through post-logout status check |
| Auth | `self status` | confirmed | implemented | reads dashboard and service protected pages |
| Auth | `self doctor` | confirmed | implemented | composed login + status workflow |
| Dashboard | `dashboard online-list` / `/Self/dashboard/getOnlineList` | confirmed | implemented | typed `OnlineSession` rows |
| Dashboard | `dashboard login-history` / `/Self/dashboard/getLoginHistory` | confirmed | implemented | typed stable columns + raw row preservation |
| Dashboard | `dashboard refresh-account-raw` / `/Self/dashboard/refreshaccount` | confirmed | implemented | intentionally exposed as raw probe |
| Dashboard | `dashboard mauth get` / `/Self/dashboard/refreshMauthType` | confirmed | implemented | normalized to `on/off/unknown` |
| Dashboard | `dashboard mauth toggle` / `/Self/dashboard/oprateMauthAction` | confirmed | implemented | verified by before/after state flip |
| Dashboard | `dashboard offline` / `/Self/dashboard/tooffline` | guarded | implemented | pre-checks target session and re-reads online list |
| Service | `service binding get` / `/Self/service/operatorId` | confirmed | implemented | typed `OperatorBinding` |
| Service | `service binding set` / `/Self/service/bind-operator` | confirmed | implemented | readback and optional restore |
| Service | `service consume get` / `/Self/service/consumeProtect` | confirmed | implemented | typed `ConsumeProtectState` |
| Service | `service consume set` / `/Self/service/changeConsumeProtect` | confirmed | implemented | readback and optional restore |
| Service | `service mac list` / `/Self/service/getMacList` | confirmed | implemented | typed count + raw row list |
| Setting | `setting person get` / `/Self/setting/personList` | guarded | implemented | exposes guarded person state and raw HTML |
| Setting | `setting person update` / `/Self/setting/updateUserSecurity` | blocked | implemented as blocked shell | submit path exists, success semantics intentionally blocked |
| Bill | `bill online-log` / `/Self/bill/getUserOnlineLog` | confirmed | implemented | typed shared list result |
| Bill | `bill month-pay` / `/Self/bill/getMonthPay` | confirmed | implemented | typed shared list result |
| Bill | `bill operator-log` / `/Self/bill/getOperatorLog` | confirmed | implemented | typed shared list result |

## Portal

| Domain | Command / Endpoint | Level | Status | Notes |
| --- | --- | --- | --- | --- |
| Portal 802 | `portal login` / `:802/eportal/portal/login` | confirmed | implemented | JSONP parsing, ret_code classification, fallback host retry |
| Portal 802 | `portal logout` / `:802/eportal/portal/logout` | confirmed | implemented | transport-confirmed request path |
| Portal 801 | `portal login-801` | guarded | implemented | raw guarded fallback, no false success claims |
| Portal 801 | `portal logout-801` | guarded | implemented | raw guarded fallback, no false success claims |

## Workflow

| Domain | Command / Workflow | Level | Status | Notes |
| --- | --- | --- | --- | --- |
| Doctor | `self doctor` | confirmed | implemented | typed composition of login and status |
| Migration | `service migrate` | confirmed | implemented | source clear then target bind with readback |
| Guard Runtime | `guard run|start|stop|status|once` | confirmed | implemented | typed schedule/runtime status, structured events, graceful stop-first supervision |

## Diagnostics And Raw Access

| Domain | Command | Level | Status | Notes |
| --- | --- | --- | --- | --- |
| Raw | `raw get` | confirmed | implemented | optional Self login before probe |
| Raw | `raw post` | confirmed | implemented | optional Self login before probe |

## Test Sources

The implementation uses three kinds of evidence in tests:

- inline HTML/JSON fixtures for stable protocol shapes
- transport mocks for write/readback flows
- `httptest` servers for `internal/httpx`

Future fixture expansion should prefer committed HTML/JSON samples when new page variants are discovered in the field.

Machine-readable output contracts also rely on:

- stable `problem.code` values with typed `details`
- stable `guard event.kind` values with typed `details`
- typed nested `guard status` output for binding, connectivity, portal, cycle, timing, and log state, including probe and switch outcome data
- `guard status` health values: `healthy`, `degraded`, `stopped`

These JSON shapes are now treated as frozen compatibility contracts. Future work should expand tests and fixtures rather than reshape the payloads without an explicit breaking release.

## Supported Deployment Surfaces

- local CLI use on desktop/server systems
- cross-platform Go guard runtime
- official ImmortalWrt router deployment through `scripts/install-immortalwrt.ps1` with router-side `procd + guard run`
