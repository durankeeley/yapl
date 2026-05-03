# PRD-26 - `--workdir` Flag for Non-CWD Deployments

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** M
**Sprint:** 3 — Validation & Deployment Flexibility
**Tags:** `feature`, `cli`, `portability`
**Created:** 2026-04-27

---

## 1. Problem & Context

All of YAPL's path resolution is relative to the current working directory — `proton/`, `games/`, `dependencies/`, `runner.json`. This means `yapl` must always be invoked from the deployment root directory. There is no way to run `yapl` from a different directory, e.g. `yapl --workdir /opt/games run Doom` or invoke it from a shell script in `/usr/local/bin`.

* **Current State:** YAPL implicitly uses `os.Getwd()` (via relative paths) as the deployment root. No flag to override.
* **Impact/Need:** Cannot create a desktop launcher or systemd service that calls `yapl` with an absolute path without wrapping it in `cd /deployment/root && yapl ...`.

## 2. Proposed Solution

* Add a `--workdir <path>` global flag.
* When provided, `os.Chdir(workdir)` is called at startup in `main.go` before any other work begins.
* All existing relative-path logic continues to work unchanged — the working directory is simply set to the specified path.

## 3. Scope & Design Details

### Flag definition
* **Behavior/Rule:** Global flag `--workdir string` parsed before command dispatch in `main.go`.
* **Specifics:** Default is `""` (no chdir; use CWD as before). If non-empty, call `os.Chdir(workdir)` and return an error if it fails.

### Error handling
* **Behavior/Rule:** If `--workdir` is specified but the directory does not exist or is not accessible, print a clear error and exit.
* **Specifics:** `fmt.Fprintf(os.Stderr, "error: --workdir: %v\n", err)` then `os.Exit(1)` in `main.go`.

### No internal changes required
* **Behavior/Rule:** All internal packages continue to use relative paths. Only `main.go` changes.
* **Specifics:** The `os.Chdir` approach is the simplest correct implementation and requires zero changes to business logic.

### Documentation
* **Behavior/Rule:** Update `README.md` and `docs/developer-guide.md` with the new flag.
* **Specifics:** Include a desktop launcher example:
  ```
  [Desktop Entry]
  Exec=yapl --workdir /opt/lan-games run Doom
  ```

## 4. Execution & Milestones

- [ ] Write failing test: `TestMain_ChdirToWorkdirBeforeDispatch` (integration test using a temp dir)
- [ ] Add `--workdir` flag to `main.go`
- [ ] Call `os.Chdir` before command dispatch
- [ ] Add error handling for invalid path
- [ ] Update `README.md` with flag documentation and example
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `cmd/yapl/main.go` — new flag, `os.Chdir` call

### Risks & Considerations
* **`os.Chdir` is process-global:** It changes the working directory for the entire process. This is fine for a CLI tool; there is no concurrent code relying on CWD.
* **Symlinks:** `os.Chdir` resolves symlinks. `os.Getwd()` after `Chdir` returns the real path. This is expected and correct.
* **Test isolation:** Tests that rely on relative paths must set up their own `t.TempDir()` and `os.Chdir` — ensure test parallelism is disabled for path-sensitive tests (use `t.Parallel()` carefully).
