# PRD-33 — Background Services Support

**Status:** `[ ] In Progress`
**Priority:** Medium
**Size:** S
**Sprint:** 3
**Tags:** `run`, `config`, `lan`
**Created:** 2026-05-06
**GitHub Issue:** N/A

---

## 1. Problem & Context

Some Windows applications require a separate helper process to be running before the main executable starts. The canonical case is **Starcraft Remastered LAN play**: a background service (e.g. a LAN relay or battle.net proxy) must be launched first and left running, then the game is started. When the game exits the service should be torn down automatically.

The `feature/backgroundservice` branch attempted this but was never brought forward to `feature/yapl-v2`. That branch had several rough edges that should be addressed rather than blindly ported:

* **No stdout/stderr from services** — a crashed service gave no visible output.
* **Hard-coded 500 ms delay** — not enough for heavier services; no way to tune it.
* **Emojis in output** — inconsistent with the current codebase style.
* **`WineArch` field bundled in** — unrelated to this feature; win32 support was intentionally removed in PRD-2.
* **No tests** — the original commit had zero test coverage.

* **Current State:** `config.App` has no `background_services` field. `RunDirectly`, `RunInContainer`, and `RunWithUMU` launch only the main executable.
* **Impact/Need:** Users cannot run LAN-dependent games (or any app requiring a pre-launched helper process) without manual intervention outside of YAPL.

---

## 2. Proposed Solution

Add a `background_services` array to the `App` config (list of Windows executable paths relative to the Wine prefix). When the run command fires, YAPL starts each service via the same Wine/Proton environment, waits a configurable startup delay, then launches the main executable. On exit the services are killed.

---

## 3. Scope & Design Details

### Config (`internal/config/config.go`)

* **New field on `App`:**
  ```go
  BackgroundServices    []string `json:"background_services,omitempty"`
  BackgroundServiceDelayMs int  `json:"background_service_delay_ms,omitempty"`
  ```
* `BackgroundServiceDelayMs` defaults to `1000` (1 s) when zero — enough for most services without being configurable noise. Applied once after all services are started (not per-service), which is simpler and sufficient.
* Paths follow the same convention as `Executable`: relative to the Wine prefix root (e.g. `drive_c/Program Files/MyService/service.exe`).

### Command (`internal/command/command.go`)

* **New private function `launchBackgroundServices`:**
  ```
  func launchBackgroundServices(services []string, wineExe string, argsPrefix []string, env []string, absPrefix string, delayMs int) func()
  ```
  * Starts each service with `cmd.Start()` (non-blocking).
  * Routes `cmd.Stdout` and `cmd.Stderr` to `os.Stdout` / `os.Stderr` so crashes are visible.
  * After all services are started, sleeps `delayMs` milliseconds (default 1000 if caller passes 0).
  * Returns a cleanup `func()` that calls `p.Kill()` on every live process.
* **Wire into `RunDirectly`:** call before `executeCommand`, `defer cleanup()`.
* **Wire into `RunInContainer`:** call before `executeCommand`, `defer cleanup()`. The args prefix already contains the container invocation (`entryPointPath`, `--verb=…`, shimPath, protonScriptPath, protonVerb); re-use it for services too.
* **Wire into `RunWithUMU`:** call before `executeCommand`, `defer cleanup()`.
* A service that fails to start is logged as a warning and skipped — it does not abort the whole launch (the user may still want the game to run).

### No changes to

* `app.go` — orchestration is unchanged; background services are an internal detail of the command layer.
* Archive/package — background service executables live inside the prefix and are packaged naturally.

---

## 4. Execution & Milestones

- [ ] Add `BackgroundServices []string` and `BackgroundServiceDelayMs int` to `config.App`.
- [ ] Write failing tests in `internal/command/command_test.go` covering:
  - Services started before the main executable.
  - Cleanup func kills started processes.
  - A service that fails to start is skipped without aborting the run.
  - Zero-length `BackgroundServices` is a no-op (no delay, cleanup is a no-op).
- [ ] Implement `launchBackgroundServices` in `command.go`.
- [ ] Wire `launchBackgroundServices` + `defer cleanup()` into `RunDirectly`, `RunInContainer`, and `RunWithUMU`.
- [ ] Run `go test ./...` — all tests pass.
- [ ] Update `README.md` — document `background_services` and `background_service_delay_ms` config fields with a Starcraft LAN example.
- [ ] Move PRD to `prds/done/`.

---

## 5. Technical Notes

### Key Files & Systems Targeted

* `internal/config/config.go` — add two fields to `App` struct.
* `internal/command/command.go` — add `launchBackgroundServices`; wire into the three `Run*` functions.
* `internal/command/command_test.go` — new test cases.
* `README.md` — document the new config fields.

### Risks & Considerations

* **Service process leak on panic:** `defer cleanup()` only runs if the Go function returns normally or panics within a `recover`. If the process is hard-killed by the OS, child processes may be orphaned. This is acceptable for v1 — it matches how Steam handles helper processes.
* **Container mode args prefix:** `launchBackgroundServices` in container mode must receive the same `argsPrefix` slice used for the main executable (entry-point path, `--verb=…`, shim, proton script, verb). Care must be taken not to append the main exe's path into the services slice by accident — pass only the prefix, then append the service path inside `launchBackgroundServices`.
* **Delay placement:** sleeping after *all* services start (rather than after each one) avoids artificially serialising startup. For Starcraft LAN this is fine — the helper only needs to be alive before the game opens a socket, not before the next helper starts.
