# PRD 1 - Production Readiness & Code Improvements

**Status:** `[ ] Pending` | `[x] In Progress` | `[ ] Complete`
**Priority:** High
**Size:** L
**Sprint:** 1
**Tags:** `testing`, `reliability`, `error-handling`, `security`
**Created:** 2026-04-27
**GitHub Issue:** N/A

---

## 1. Problem & Context

The codebase has zero test coverage and several reliability, error-handling, and security issues that make it unsuitable for production use. Each issue is independently small, but together they represent real risks when YAPL is used at a LAN party or in an automated deployment.

* **Current State:**
  - No `_test.go` files exist anywhere in the project
  - `executeCommand` logs errors but always returns `nil`, so callers cannot detect launch failures
  - `MustGetAbsolutePath` and `getProtonInfo` call `log.Fatalf` inside library packages, making them impossible to test and bypassing Go's error-return convention
  - HTTP downloads have no timeout — a stalled server will hang YAPL forever
  - The path traversal security check in `extractTar` is fragile: comparing against a `destPath` that may not end in `/` can allow a path like `destPath/../escape` to pass the check
  - `CopyDir` uses `filepath.Walk` which does not copy symlinks — it copies the target file instead, breaking Proton distributions that rely on symlinks
  - `buildDllOverridesString` iterates over a map with non-deterministic key order, producing inconsistent `WINEDLLOVERRIDES` strings

* **Impact/Need:** A launcher that silently ignores game crashes, hangs on bad network connections, or corrupts a Proton copy during setup is not reliable enough for the stated use case.

---

## 2. Proposed Solution

Fix each issue in isolation so each change is easy to review and test. Add a comprehensive test suite using Go's standard `testing` package and `t.TempDir()` for filesystem operations.

---

## 3. Scope & Design Details

### 3a. Test Suite (all packages)

* **Rule:** Every package under `internal/` must have at least one `_test.go` file exercising its public API.
* **Specifics:**
  - `internal/fs` — test `CopyFile`, `CopyDir`, `DirExistsAndIsNotEmpty`, `MustGetAbsolutePath` using `t.TempDir()`
  - `internal/archive` — test `Extract` (local `.tar.gz` file), `Package`, `Unpackage`, path traversal rejection, `trimArchiveSuffix`
  - `internal/config` — test `LoadOrCreateGlobal` (missing file creates default), `LoadOrCreateApp`, round-trip JSON
  - `internal/dependency` — test `InstallCustomComponents`, `patchProtonForWin32`, `getInfo` error cases with fake config
  - `internal/command` — test `buildProtonEnv`, `buildDllOverridesString`, `getWineArch`, `getProtonPath`, `restructureProtonPrefix` using `t.TempDir()`

### 3b. Fix `executeCommand` — Propagate Exit Errors

* **Behavior/Rule:** If the child process exits non-zero, `executeCommand` must return the error so callers can act on it.
* **Specifics:** Remove the `log.Printf` that swallows the error and change the return to pass `cmd.Run()` errors back. Update all callers that currently rely on the swallowed error.
* **File:** `internal/command/command.go:304`

### 3c. Remove `log.Fatalf` from Library Code

* **Behavior/Rule:** Library functions must never call `log.Fatalf`. They must return an `error` instead.
* **Specifics:**
  - `fs.MustGetAbsolutePath` → rename to `GetAbsolutePath`, return `(string, error)`. Update all call sites.
  - `command.getProtonInfo` → return `(config.VersionInfo, error)` instead of `log.Fatalf`. Propagate errors up through `RunDirectly`, `RunInContainer`, `RunWithUMU`, `buildProtonEnv`.
* **Files:** `internal/fs/fs.go:14`, `internal/command/command.go:354`

### 3d. HTTP Download Timeout

* **Behavior/Rule:** All HTTP downloads must use a client with a configurable timeout. Default: 30 minutes (sufficient for large runtimes).
* **Specifics:** Replace bare `http.Get` calls in `archive/archive.go` and `dependency/runtime.go` with a package-level `http.Client` that has a `Timeout` of `30 * time.Minute`.
* **Files:** `internal/archive/archive.go:98`, `internal/dependency/runtime.go:82`, `internal/dependency/runtime.go:122`

### 3e. Fix Path Traversal Check in `extractTar`

* **Behavior/Rule:** The traversal guard must be airtight. Append a `/` to `destPath` before comparing so that a path like `/tmp/dst/../escape` cannot slip through.
* **Specifics:** Change `!strings.HasPrefix(target, destPath)` to `!strings.HasPrefix(target, filepath.Clean(destPath)+string(filepath.Separator))`. Also handle the case where `target == destPath` (the root dir itself).
* **File:** `internal/archive/archive.go:153`

### 3f. Fix `CopyDir` Symlink Handling

* **Behavior/Rule:** `CopyDir` must preserve symlinks rather than dereference them. Proton distributions contain many symlinks; dereferencing them wastes disk space and can break lookup paths.
* **Specifics:** Replace `filepath.Walk` with `os.ReadDir` + recursive descent that checks `os.Lstat` and calls `os.Symlink` for symlink entries.
* **File:** `internal/fs/fs.go:62`

### 3g. Deterministic DLL Override Order

* **Behavior/Rule:** `buildDllOverridesString` must produce the same output for the same input.
* **Specifics:** Collect map keys into a slice, `sort.Strings` it, then build the output in sorted order.
* **File:** `internal/command/command.go:343`

---

## 4. Execution & Milestones

- [ ] Write failing tests for `internal/fs` (CopyFile, CopyDir symlinks, DirExistsAndIsNotEmpty)
- [ ] Fix `CopyDir` to handle symlinks — tests pass
- [ ] Write failing tests for `internal/archive` (extract, package, path traversal rejection)
- [ ] Fix path traversal check — tests pass
- [ ] Write failing tests for `internal/config` (load/create round-trip)
- [ ] Write failing tests for `internal/command` (buildProtonEnv, buildDllOverridesString sorted, restructureProtonPrefix)
- [ ] Fix `buildDllOverridesString` to sort keys — tests pass
- [ ] Fix `executeCommand` to return errors — tests pass
- [ ] Remove `log.Fatalf` from `MustGetAbsolutePath` and `getProtonInfo` — update all call sites — tests pass
- [ ] Add HTTP timeout to `archive` and `runtime` packages — tests pass (mock server or skip network)
- [ ] All `go test ./...` green
- [ ] Move PRD to `prds/done/`

---

## 5. Technical Notes

### Key Files & Systems Targeted

* `internal/fs/fs.go` — `CopyDir` (symlink fix), `MustGetAbsolutePath` (error return)
* `internal/archive/archive.go` — path traversal fix, HTTP timeout
* `internal/command/command.go` — `executeCommand` error propagation, `buildDllOverridesString` sort, `getProtonInfo` error return
* `internal/dependency/runtime.go` — HTTP timeout
* All `internal/*_test.go` — new files

### Risks & Considerations

* **Call-site churn for `MustGetAbsolutePath`:** It is called in `app.go`, `command.go`, and `dependency.go`. Each call site needs to handle the returned error. Accept the churn — the current design hides real failures.
* **`buildProtonEnv` signature:** Once `getProtonInfo` returns an error, `buildProtonEnv` needs to return `([]string, error)`. This ripples to all three `Run*` functions. Map each change explicitly so nothing is missed.
* **HTTP timeout on slow connections:** 30 minutes is generous; a Steam Runtime download is ~2 GB. Make the timeout a named constant so it's easy to adjust.
