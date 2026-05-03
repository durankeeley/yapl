# PRD-20 - Centralise Hardcoded Directory Path Constants

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** 3 — Validation & Deployment Flexibility
**Tags:** `refactor`, `maintainability`, `config`
**Created:** 2026-04-27

---

## 1. Problem & Context

The strings `"proton"`, `"dependencies"`, `"dependencies/runtime"`, `"dependencies/umu-launcher"`, `"games"`, and `"apps"` are hardcoded as string literals scattered across at least five files:

- `internal/dependency/runtime.go` — `"dependencies/runtime"`
- `internal/dependency/dependency.go` — `"dependencies"`, `"proton"`
- `internal/command/command.go` — `"dependencies/runtime"`, `"dependencies/umu-launcher"`, `"proton"`
- `internal/app/app.go` — `"games"`, `"apps"`

When the directory layout needs to change (e.g., adding a `--workdir` flag, or renaming `dependencies` to `libs`), every occurrence must be found and updated manually. There is no single place to change the layout.

* **Current State:** Directory names are inline string literals with no central definition.
* **Impact/Need:** Layout changes require a multi-file find-and-replace with high chance of missing an instance, causing subtle runtime path errors.

## 2. Proposed Solution

* Add a `dirs` package (or a `paths` file inside `internal/fs/`) that exports path constants and constructor functions:
  ```go
  const (
      ProtonDir      = "proton"
      DependencyDir  = "dependencies"
      GamesDir       = "games"
      AppsDir        = "apps"
  )
  func ProtonPath(version string) string { return filepath.Join(ProtonDir, version) }
  func DependencyPath(name, version string) string { return filepath.Join(DependencyDir, name, version) }
  func RuntimePath(name string) string { return filepath.Join(DependencyDir, "runtime", name) }
  ```
* Replace all inline string literals throughout the codebase with calls to these functions/constants.

## 3. Scope & Design Details

### Location
* **Behavior/Rule:** Add the constants and path functions to `internal/fs/paths.go` (new file in the existing `fs` package, not a new package — avoids an import cycle).
* **Specifics:** Keep it simple — constants and pure string-manipulation functions only. No I/O.

### Replacement scope
* **Behavior/Rule:** Every hardcoded path string in `dependency/`, `command/`, and `app/` is replaced with a call to the relevant function.
* **Specifics:** Use `grep -r '"proton"\|"dependencies"\|"games"\|"apps"' internal/` to enumerate all instances before and after.

### No behaviour change
* **Behavior/Rule:** This is a pure refactor. No new behaviour, no new config options. All existing tests must pass unchanged.

## 4. Execution & Milestones

- [ ] Write failing test: `TestProtonPath_ReturnsCorrectPath` (and similar for each path function)
- [ ] Create `internal/fs/paths.go` with constants and path functions
- [ ] Replace inline string literals in `dependency/`, `command/`, `app/`
- [ ] `grep` confirms no remaining hardcoded `"proton"`, `"dependencies"`, `"games"`, `"apps"` literals in `internal/`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/fs/paths.go` — new file, constants + path functions
* `internal/dependency/dependency.go` — replace literals
* `internal/dependency/runtime.go` — replace literals
* `internal/command/command.go` — replace literals
* `internal/app/app.go` — replace literals
* `internal/fs/fs_test.go` — add path function tests

### Risks & Considerations
* **Import cycle:** `paths.go` lives in the `fs` package which is already imported by `dependency`, `command`, and `app`. No new import cycle is introduced.
* **`go vet` / `staticcheck`:** After replacement, run `go vet ./...` to confirm no inadvertent changes.
