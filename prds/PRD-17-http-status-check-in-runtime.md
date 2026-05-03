# PRD-17 - Validate HTTP Status Codes in dependency/runtime.go

**Status:** `[ ] Pending`
**Priority:** High
**Size:** S
**Sprint:** 2 — Reliability
**Tags:** `bugfix`, `http`, `runtime`, `error-handling`
**Created:** 2026-04-27

---

## 1. Problem & Context

`internal/dependency/runtime.go` makes HTTP GET requests (for version manifest fetching and post-install fixup) but does not validate the HTTP response status code before reading the body. `archive.go` was fixed in PRD-1 to check `resp.StatusCode != http.StatusOK`, but `runtime.go` was missed. A 404, 503, or rate-limit response body would be silently treated as valid content, potentially corrupting the runtime version state or causing obscure downstream failures.

* **Current State:** `runtime.go` reads `resp.Body` without checking `resp.StatusCode`.
* **Impact/Need:** Error pages (HTML 404s, JSON error responses from CDNs) are parsed as manifest data, causing silent state corruption or confusing parse errors.

## 2. Proposed Solution

* After every `httpClient.Get(url)` call in `runtime.go`, check `resp.StatusCode == http.StatusOK` before reading the body.
* If the status is not 200, close the body, and return a descriptive error including the URL and status code.
* Match the pattern already established in `archive.go`.

## 3. Scope & Design Details

### Status check pattern
* **Behavior/Rule:** Immediately after `resp, err := httpClient.Get(url)` and after checking `err != nil`, add:
  ```go
  if resp.StatusCode != http.StatusOK {
      resp.Body.Close()
      return fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
  }
  ```
* **Specifics:** Apply to every HTTP GET in `runtime.go` — both the version manifest fetch and the post-install fixup fetch.

### Error message format
* **Behavior/Rule:** Include the URL and `resp.Status` (which contains both code and text, e.g. `"404 Not Found"`) in the error.
* **Specifics:** Do not log; return the error to the caller.

## 4. Execution & Milestones

- [ ] Write failing test: `TestFetchRuntimeVersion_ReturnsErrorOnNon200Status` (use `httptest.Server` returning 404)
- [ ] Write failing test: `TestPostInstallRuntimeFixup_ReturnsErrorOnNon200Status`
- [ ] Add status code checks after each `httpClient.Get` in `runtime.go`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/dependency/runtime.go` — status checks after each HTTP GET
* `internal/dependency/runtime_test.go` — new tests using `httptest.Server`

### Risks & Considerations
* **Body leak:** Must call `resp.Body.Close()` before returning the error to avoid a goroutine/fd leak.
* **Redirect handling:** `http.Client` follows redirects by default, so a successful redirect to a valid resource still returns 200. No special redirect handling needed.
