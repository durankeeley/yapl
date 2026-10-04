# PRD-46 - Console Service: Local API for Library, Store, Jobs and Sessions

**Status:** `[x] Complete`
**Priority:** High
**Size:** L
**Sprint:** Phase 3 — Console client
**Tags:** `console`, `client`, `api`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** Everything is a CLI command. A couch/console UI needs a programmatic interface: list what's installed, browse the server, start an install and watch its progress, launch and stop a game.
* **Impact/Need:** The console UI (PRD-47) must stay a thin view. All behaviour belongs in a tested Go service.

## 2. Proposed Solution

`internal/console` exposes a JSON HTTP API bound to `127.0.0.1` only. It is composed from four small services behind interfaces, so each can be tested and replaced on its own:

| Service | Responsibility |
|---|---|
| `Library` | installed games/apps: name, title, version, cover, readiness |
| `Store` | remote catalog via `client`, annotated installed / upgradable |
| `Jobs` | long-running installs/upgrades with progress (`progress.Update`) |
| `Sessions` | launch a game as a child `yapl run` process in its own process group, track it, stop it, log to `games/<Name>/last-run.log` |

### API
| Method | Path |
|---|---|
| GET | `/api/system` → kernel/module versions, workdir, server address + reachability |
| GET | `/api/library` · `/api/library/{name}` · `/api/library/{name}/cover` |
| POST | `/api/library/{name}/launch` · DELETE `/api/library/{name}` (uninstall) |
| GET | `/api/store` · `/api/store/{name}/cover` |
| POST | `/api/store/{name}/install` · `/api/library/{name}/upgrade` → `{job_id}` |
| GET | `/api/jobs` · `/api/jobs/{id}` → `{state, stage, done, total, error}` |
| GET | `/api/session` · POST `/api/session/stop` |

## 3. Scope & Design Details

* One game session at a time; launching while one runs → 409.
* Launching runs `os.Executable() --workdir <root> run <name>` so a crashing game or Wine can never take the console down (process isolation, microservice-style).
* Jobs are in-memory; finished jobs are kept for `JobRetention` (named constant) for the UI to read the outcome.
* Uninstall requires `?confirm=<name>` to guard against accidental deletion.
* Cover fallbacks: no cover → 404, UI renders a generated tile.
* Bound to loopback only; the store's server password comes from `--password` or `YAPL_PASSWORD`, held in memory.
* **Browser safety:** any web page can make a browser send requests to `127.0.0.1`. Every request must carry a loopback `Host` (`127.0.0.1`, `localhost`, `[::1]`), which blocks DNS rebinding, and every mutating request (POST/DELETE) must carry the `X-Yapl-Console: 1` header. Cross-origin pages cannot add that header without a CORS preflight, which the service never approves.
* **Jobs run one at a time** in a single worker (queued jobs report `queued`), because two pulls writing `runner.json` concurrently would race.

## 4. Execution & Milestones

- [x] Library service tests on `t.TempDir()` libraries.
- [x] Jobs tests (progress, failure, retention) with a fake installer.
- [x] Sessions tests with a fake executable script (start, status, stop kills group, exit recorded).
- [x] HTTP handler tests via `httptest` for every route.

### Acceptance scenarios
* **Given** two installed games, one with `installed.json` version 1.0.0 **and** the server offering 1.1.0 **when** I GET `/api/store` **then** that game is marked `upgradable`.
* **Given** a running session **when** I POST another launch **then** I get 409 naming the running game.
* **Given** an install job **when** the download progresses **then** `/api/jobs/{id}` reports increasing `done` with a fixed `total`, and finally `state: "done"`.
* **Given** DELETE `/api/library/Doom` without `confirm=Doom` **then** 400 and nothing is deleted.
