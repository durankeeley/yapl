# PRD-22 - Add Test Coverage for internal/app Package

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** M
**Sprint:** 1 — Codebase Health
**Tags:** `testing`, `app`, `coverage`
**Created:** 2026-04-27

---

## 1. Problem & Context

`internal/app/app.go` is the central orchestrator for all YAPL commands (`Setup`, `Run`, `Package`, `Unpackage`). It is the only package in `internal/` with no test file. The package orchestrates multiple sub-systems (dependency, command, archive, config) and its integration logic is currently untested. Bugs in the orchestration layer (wrong order of operations, missing error propagation, wrong directory passed to sub-calls) would only surface at runtime.

* **Current State:** `internal/app/` has no `app_test.go` file.
* **Impact/Need:** The highest-level code in the project is completely untested. A regression here breaks the entire tool.

## 2. Proposed Solution

* Create `internal/app/app_test.go` with Gherkin-style Given-When-Then tests that exercise the `App` struct methods using real temp directories.
* Focus on the orchestration logic: correct sequencing, error propagation, and correct directory resolution.
* Use `t.TempDir()` for filesystem isolation; do not mock sub-packages.

## 3. Scope & Design Details

### Tests to cover

#### Setup
* **Given** a valid config with no dependencies configured, **when** `Setup()` is called, **then** it returns no error and the prefix directory is created.
* **Given** a config referencing a proton version not in runner.json, **when** `Setup()` is called, **then** it returns a descriptive error.

#### Package
* **Given** a game directory exists with a `game.json` and some files, **when** `Package()` is called, **then** an archive is created at the expected path.
* **Given** the game directory does not exist, **when** `Package()` is called, **then** it returns an error.

#### Unpackage
* **Given** a valid archive at a known path, **when** `Unpackage()` is called, **then** the game directory is extracted correctly.
* **Given** the archive path does not exist, **when** `Unpackage()` is called, **then** it returns an error.

#### Run (limited — cannot actually launch games in CI)
* **Given** a config with an invalid `launch_method`, **when** `Run()` is called, **then** it returns an error before attempting to launch.

### Test structure
* **Behavior/Rule:** Each test creates its own `t.TempDir()` as the working directory and writes minimal config files into it.
* **Specifics:** Use `os.Chdir` or pass the working directory to `App` — whichever approach the existing code supports. Check `app.go` for how the working directory is set.

## 4. Execution & Milestones

- [ ] Read `internal/app/app.go` thoroughly to understand the `App` struct and how it resolves paths
- [ ] Write failing tests for `Setup`, `Package`, `Unpackage`, and `Run` error cases
- [ ] Make tests pass (no implementation changes expected — this is pure test addition)
- [ ] Confirm `go test ./internal/app/` passes
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/app/app_test.go` — new file
* `internal/app/app.go` — read-only reference

### Risks & Considerations
* **Working directory sensitivity:** Much of YAPL's path resolution uses relative paths. Tests must either `os.Chdir` to `t.TempDir()` or the `App` struct must accept a working directory parameter. Check the existing code first.
* **Download avoidance:** Tests must not trigger actual downloads. Use configs that reference local paths (`vinfo.Path`) rather than URLs.
* **`Run()` tests:** Do not attempt to actually launch a game. Only test the pre-launch validation (config errors, missing proton, etc.).
