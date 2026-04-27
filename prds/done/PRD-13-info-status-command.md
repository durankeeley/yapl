# PRD-13 - `info` / Status Command

**Status:** `[x] Complete`
**Priority:** Low
**Size:** S
**Sprint:** Backlog
**Tags:** `feature`, `cli`, `diagnostics`
**Created:** 2026-04-27

---

## 1. Problem & Context

There is no command to display the current resolved state of a game: which Proton path will actually be used, whether dependencies are downloaded, whether the prefix is initialised, and what environment variables would be set. Operators must manually inspect the filesystem and read the docs to answer these questions, making remote support and debugging slow.

* **Current State:** No introspection command. Operators rely on running `setup` or `run` and reading error output.
* **Impact/Need:** Hard to diagnose misconfiguration without running code; no way to verify a machine is ready before a LAN event.

## 2. Proposed Solution

* Add an `info` command that prints the resolved configuration and readiness state for the specified game, without modifying any state.
* Output sections: Config summary, Dependency status (present / missing), Prefix status (initialised / not initialised), Resolved Proton path.

## 3. Scope & Design Details

### Output format
* **Behavior/Rule:** Human-readable, key-value style. No JSON (that adds complexity with no gain for the target audience).
* **Specifics:**
  ```
  Game:          Doom
  Config file:   games/Doom/game.json
  Proton:        ge-proton-9  [/home/user/yapl/proton/ge-proton-9]  ✓ present
  DXVK:          2.3          [dependencies/dxvk/2.3]               ✓ present
  VKD3D:         (not set)
  Runtime:       (not set)
  Prefix:        games/Doom/prefix                                   ✓ initialised
  Launch method: direct
  Executable:    drive_c/Games/Doom/doom.exe
  ```

### Readiness check
* **Behavior/Rule:** For each dependency, check whether the directory exists and is non-empty using `fs.DirExistsAndIsNotEmpty`. For the prefix, use `prefixIsInitialized`.
* **Specifics:** Print `✓ present` or `✗ missing` next to each item. Do not download or create anything.

### No side effects
* **Behavior/Rule:** `info` must be read-only. It must not call `EnsureAll`, `InitializePrefix`, or any mutating function.
* **Specifics:** Enforce by construction — `Info()` only reads config and calls check functions.

## 4. Execution & Milestones

- [ ] Write failing test: `TestInfo_PrintsProtonStatusPresent`
- [ ] Write failing test: `TestInfo_PrintsProtonStatusMissing`
- [ ] Write failing test: `TestInfo_PrintsPrefixInitialisedStatus`
- [ ] Add `Info() error` to `App` in `internal/app/app.go`
- [ ] Add `case "info":` to `cmd/yapl/main.go`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `cmd/yapl/main.go` — add `case "info":`
* `internal/app/app.go` — add `Info() error`
* `internal/app/app_test.go` — new tests

### Risks & Considerations
* **`command.prefixIsInitialized` is unexported:** Either export it (`PrefixIsInitialized`) or duplicate the check logic in `app.go`. Prefer exporting to avoid duplication.
* **Colour output:** Keep output plain text; do not add colour escape codes. The `✓`/`✗` symbols provide sufficient visual distinction in most terminals.
