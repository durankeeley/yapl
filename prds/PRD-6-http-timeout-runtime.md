# PRD-6 - HTTP Timeout in dependency/runtime.go

**Status:** `[ ] Pending`
**Priority:** High
**Size:** S
**Sprint:** Backlog
**Tags:** `bugfix`, `http`, `runtime`, `reliability`
**Created:** 2026-04-27

---

## 1. Problem & Context

PRD-1 added a 30-minute `http.Client` with timeout to `internal/archive/archive.go`, but `internal/dependency/runtime.go` makes its own `http.Get` calls (for fetching the runtime version manifest) using the default `http.Client`, which has no timeout. A stalled or slow server will cause `yapl` to hang indefinitely with no way to cancel short of a SIGKILL.

* **Current State:** `runtime.go` calls `http.Get(url)` with the default client (no timeout).
* **Impact/Need:** A hanging network request during `setup` or `run` blocks the user's terminal forever and cannot be interrupted cleanly.

## 2. Proposed Solution

* Define a package-level `var httpClient = &http.Client{Timeout: 30 * time.Minute}` in `runtime.go` (matching the convention already established in `archive.go`).
* Replace all `http.Get(...)` calls in `runtime.go` with `httpClient.Get(...)`.

## 3. Scope & Design Details

### Shared client definition
* **Behavior/Rule:** A single `var httpClient` at the top of `runtime.go`, identical timeout to `archive.go` (30 minutes).
* **Specifics:** Do not import `archive` just to reuse the client — each file owns its own client definition. Duplication here is intentional (avoids a shared-state dependency between unrelated packages).

### Replacement scope
* **Behavior/Rule:** Every `http.Get` in `runtime.go` is replaced; no bare `http.Get` remains.
* **Specifics:** Grep confirms `runtime.go` is the only file in `dependency/` with bare `http.Get`.

## 4. Execution & Milestones

- [ ] Write failing test: `TestFetchRuntimeVersion_UsesClientWithTimeout` (inject a slow test server, assert the call does not hang beyond the timeout)
- [ ] Add `var httpClient = &http.Client{Timeout: 30 * time.Minute}` to `runtime.go`
- [ ] Replace `http.Get` calls with `httpClient.Get`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/dependency/runtime.go` — add `httpClient`, replace `http.Get` calls
* `internal/dependency/runtime_test.go` — new test (create file if not present)

### Risks & Considerations
* **Test server:** The timeout test needs a `httptest.Server` that sleeps longer than the injected timeout. Use a very short timeout (e.g., 50ms) for the test client to keep CI fast; the production timeout is left at 30 minutes.
* **Variable shadowing:** Ensure the package-level `httpClient` does not shadow any local variable named `httpClient` in existing functions.
