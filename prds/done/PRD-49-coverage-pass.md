# PRD-49 - Test Coverage Pass for Long-Running and Legacy Paths

**Status:** `[x] Complete`
**Priority:** Medium
**Size:** M
**Sprint:** Phase 3 follow-up
**Tags:** `tests`, `coverage`, `refactor`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** After Phases 0–3 most packages are at 75–100% statement coverage, but four lag behind: `internal/server` (~54%: `Start` blocks forever), `internal/modules/console` (~38%: browser launch and the blocking run loop), `internal/dependency` (~54%: downloads and DLL installs predate the test rules) and `internal/command` (~60%: container and umu launch paths).
* **Impact/Need:** These are exactly the paths a LAN party exercises: serving, launching the console, downloading Proton, starting games.

## 2. Proposed Solution

* Make blocking entry points testable without changing behaviour: `server.Serve(ctx, listener)` (Start = listen + Serve), with the rescan and broadcast loops stopping when the context ends.
* Add Given/When/Then tests using real temp directories, `httptest` servers and fake executables (shell scripts) — never mocks of the filesystem.

## 3. Scope & Design Details

* **server**: Serve answers HTTP on a `:0` listener and returns when the context is cancelled.
* **modules/console**: `openBrowser` with a fake `PATH` (kiosk window exit ends the console; missing browser prints the URL).
* **dependency**: `ensure` downloads and extracts from a local HTTP server; unknown types/versions error; custom DLL install copies the right files.
* **command**: `RunInContainer` and `RunWithUMU` reach their runner with the expected arguments and environment, via fake entry-point and umu-run scripts.

## 4. Execution & Milestones

- [x] `server.Serve` + tests.
- [x] Console browser tests.
- [x] Dependency download/install tests.
- [x] Container and umu launch tests.
- [x] Coverage reported in the final summary.

## Outcome

* Coverage: server 54→75%, modules/console 38→87%, dependency 54→88%, command 60→79%; whole project 83%.
* Running the suite with `-race` exposed a real bug: `ExtractVerified` drained and hashed a zstd stream while the zstd decoder's background goroutine was still reading it, which could produce a wrong checksum. Decompressors are now closed (zstd's `Close` waits for its goroutine) before draining, `Archive.Extract` no longer leaks that goroutine, and `batect.yml` gains a `test` task running `go test -race`.
* `log.Printf` calls left in `command` and `dependency` were replaced with stderr writes, per the coding standards.
