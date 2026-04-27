# PRD-4 - Winetricks Integration

**Status:** `[x] Complete`
**Priority:** Medium
**Size:** M
**Sprint:** Backlog
**Tags:** `winetricks`, `dependencies`, `feature`
**Created:** 2026-04-27

---

## 1. Problem & Context

`Winetricks []string` is declared in `config.App` and set to `[]string{}` in the defaults, but the field is never read or acted on anywhere in `app.go`, `dependency.go`, or `command.go`. Any user who sets `"winetricks": ["vcrun2022", "dotnet48"]` in their `game.json` will get no effect and no warning.

* **Current State:** Field is declared and serialised but silently ignored at runtime.
* **Impact/Need:** Users cannot install Windows redistributables (Visual C++, .NET, DirectX legacy components) that many games require. No error is surfaced; it just doesn't work.

## 2. Proposed Solution

* Add `ensureWinetricks(prefixPath string, packages []string, env []string) error` in `internal/dependency/`.
* Call it from `app.Setup()` and `app.Run()` after the Wine prefix is initialised, passing the Proton env so winetricks runs inside the correct prefix.
* Detect winetricks in `PATH` with `exec.LookPath`; return a clear error if it is absent.
* Skip the step entirely when `Winetricks` is nil or empty.

## 3. Scope & Design Details

### Dependency detection
* **Behavior/Rule:** `exec.LookPath("winetricks")` before attempting any installs.
* **Specifics:** Return `fmt.Errorf("winetricks not found in PATH; install it to use the winetricks config option")` — not a fatal, propagated as an error.

### Package installation
* **Behavior/Rule:** Run `winetricks --unattended <pkg1> <pkg2> ...` once with all packages in a single invocation to avoid repeated Wine startup overhead.
* **Specifics:** Pass the full Proton env (from `buildProtonEnv`) so winetricks operates on the correct `WINEPREFIX`.

### Idempotency
* **Behavior/Rule:** Winetricks installs are not re-run if the prefix already has them. Winetricks itself tracks installed verbs via `$WINEPREFIX/winetricks.log`; we rely on that.
* **Specifics:** No additional state file needed.

### Config
* **Behavior/Rule:** `"winetricks": []` (empty array) is a no-op. Omitting the key entirely is also a no-op.
* **Specifics:** No config schema changes needed; field already exists.

## 4. Execution & Milestones

- [ ] Write failing test: `TestEnsureWinetricks_SkipsWhenListEmpty`
- [ ] Write failing test: `TestEnsureWinetricks_ErrorsWhenWinetricksNotInPath`
- [ ] Write failing test: `TestEnsureWinetricks_RunsSingleInvocationWithAllPackages`
- [ ] Implement `ensureWinetricks` in `internal/dependency/dependency.go`
- [ ] Call from `app.Setup()` and `app.Run()` after `InitializePrefix`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/dependency/dependency.go` — add `ensureWinetricks`
* `internal/app/app.go` — call `ensureWinetricks` in `Setup()` and `Run()`
* `internal/dependency/dependency_test.go` — new tests

### Risks & Considerations
* **winetricks absent on CI:** Tests that call the real binary must skip with `t.Skip` when winetricks is not in PATH; other tests mock or avoid the binary.
* **Long-running installs:** winetricks can take minutes for large packages (dotnet48). No timeout should be set; the user is waiting intentionally.
