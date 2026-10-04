# PRD-36 - Fix `clean` Flag Parsing

**Status:** `[ ] In Progress`
**Priority:** High
**Size:** S
**Sprint:** Phase 0 — Stabilise
**Tags:** `bug`, `cli`, `clean`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** `yapl clean "Doom" --prefix` exits with `flag provided but not defined: -prefix`. The `--prefix`, `--proton`, `--deps` and `--all` flags are registered inside the `clean` branch of `main`, after the first `flag.Parse` call has already rejected them.
* **Impact/Need:** The `clean` command is unusable. Every documented invocation fails.

## 2. Proposed Solution

* Register the `clean` flags together with the other flags, before the first parse.
* Add black-box CLI tests for `cmd/yapl` that run the real binary entry point in a subprocess, so the behaviour stays covered through the kernel refactor planned in PRD-38.

## 3. Scope & Design Details

### CLI harness (`cmd/yapl/main_test.go`)
* **Behavior/Rule:** The test binary re-executes itself with `YAPL_TEST_MAIN=1`, which calls `main()` with the supplied arguments in a temp working directory. Tests assert on exit code and combined output.

### `clean` flags
* **Behavior/Rule:** `yapl clean <name> --prefix` reaches app lookup and reports `'<name>' not found in games/ or apps/` when the game is missing, and deletes the prefix when it exists and `--yes` is given.

## 4. Execution & Milestones

- [ ] Failing CLI tests for `clean` written and confirmed failing.
- [ ] Flags registered before parse.
- [ ] `go test ./...` passes.
- [ ] README/CLI help reviewed.

## 5. Technical Notes

### Key Files & Systems Targeted
* `cmd/yapl/main.go` — move flag registration.
* `cmd/yapl/main_test.go` — new subprocess CLI harness.

### Risks & Considerations
* **Subprocess tests:** They must set the working directory to `t.TempDir()` so they never touch real `games/` folders.
