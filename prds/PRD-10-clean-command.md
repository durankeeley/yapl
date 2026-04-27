# PRD-10 - `clean` Command

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** Backlog
**Tags:** `feature`, `cli`, `cleanup`
**Created:** 2026-04-27

---

## 1. Problem & Context

There is no CLI command to remove a game prefix, a downloaded Proton version, or a dependency (DXVK, VKD3D, runtime). Users must locate and delete the directories manually. This is error-prone (wrong directory deleted, shared Proton left behind) and blocks scripted teardown on LAN party machines.

* **Current State:** No `clean` command. No programmatic way to remove YAPL-managed directories.
* **Impact/Need:** Operators cannot reset a broken prefix or free disk space without manual filesystem work.

## 2. Proposed Solution

* Add a `clean` command with sub-targets controlled by flags:
  - `--prefix` — deletes the game/app prefix directory
  - `--proton` — deletes the downloaded Proton version for this game
  - `--deps` — deletes DXVK and VKD3D for this game's configured versions
  - `--all` — equivalent to `--prefix --proton --deps`
* Require `--game` as normal. Print each path before deleting it. Prompt for confirmation unless `--yes` is given.

## 3. Scope & Design Details

### Confirmation prompt
* **Behavior/Rule:** Without `--yes`, print the paths that will be deleted and prompt `Delete the above? [y/N]: `. Default is No.
* **Specifics:** Read a single line from stdin. Any input other than `y` or `Y` aborts with no changes made.

### `--prefix`
* **Behavior/Rule:** Deletes `games/<name>/prefix/` (or `apps/<name>/prefix/`).
* **Specifics:** Does not delete the `game.json`/`app.json` config file.

### `--proton`
* **Behavior/Rule:** Deletes `proton/<proton_version>/` for the version configured in this game's `game.json`.
* **Specifics:** Warn if multiple games share this Proton version (scan all `game.json` files and print their names).

### `--deps`
* **Behavior/Rule:** Deletes `dependencies/dxvk/<version>/` and `dependencies/vkd3d/<version>/` for versions configured in this game.
* **Specifics:** Skip silently if a dependency version is not configured or directory does not exist.

### `--all`
* **Behavior/Rule:** Runs all three targets in order: prefix, proton, deps.
* **Specifics:** A single confirmation prompt lists all paths before deleting.

## 4. Execution & Milestones

- [ ] Write failing test: `TestClean_PrefixDeletesOnlyPrefixDir`
- [ ] Write failing test: `TestClean_ProtonWarnsWhenSharedByMultipleGames`
- [ ] Write failing test: `TestClean_DepsSkipsIfVersionNotConfigured`
- [ ] Add `Clean(targets CleanTargets) error` to `App` in `internal/app/app.go`
- [ ] Add `case "clean":` to `cmd/yapl/main.go` with flag parsing
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `cmd/yapl/main.go` — add `case "clean":`, flag parsing
* `internal/app/app.go` — `Clean(targets CleanTargets) error`
* `internal/app/app_test.go` — new tests

### Risks & Considerations
* **Shared Proton deletion:** Since Proton is shared across games, deleting it affects others. Warn loudly; do not block the operation.
* **`--yes` in automation:** Required for non-interactive scripts. Ensure the flag name does not conflict with existing flags.
* **Symlinks in prefix:** `os.RemoveAll` handles symlinks correctly (removes the link, not the target). Safe to use.
