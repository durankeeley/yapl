# PRD-7 - `list` Command

**Status:** `[x] Complete`
**Priority:** Medium
**Size:** S
**Sprint:** Backlog
**Tags:** `feature`, `cli`, `discoverability`
**Created:** 2026-04-27

---

## 1. Problem & Context

There is no way to enumerate which games and apps are configured under the current YAPL working directory. Users must manually browse the `games/` and `apps/` filesystem directories. On a shared LAN machine with many titles this is cumbersome, and the machine operator has no quick way to confirm what is available.

* **Current State:** Four commands exist (`setup`, `run`, `package`, `unpackage`); none lists available titles.
* **Impact/Need:** Operators and users cannot discover what is installed without a shell and filesystem knowledge.

## 2. Proposed Solution

* Add a `list` command that scans `games/` and `apps/` directories and prints each title name with its configured `proton_version`, `launch_method`, and `executable`.
* No `--game` flag required; `list` operates on the whole working directory.

## 3. Scope & Design Details

### Output format
* **Behavior/Rule:** Print one line per title. Columns: `TYPE`, `NAME`, `PROTON`, `METHOD`, `EXECUTABLE`.
* **Specifics:** Use tab-separated output so it reads cleanly in a terminal and pipes well to `column -t`.

Example output:
```
TYPE   NAME       PROTON        METHOD     EXECUTABLE
game   Doom       ge-proton-9   direct     drive_c/Games/Doom/doom.exe
app    SteamTool  system        direct     drive_c/steamcmd/steamcmd.exe
```

### Directory scanning
* **Behavior/Rule:** Scan both `games/` and `apps/` if they exist; skip gracefully if absent.
* **Specifics:** For each subdirectory, attempt to load `game.json` (for `games/`) or `app.json` (for `apps/`). Skip any subdirectory that does not contain the config file.

### Error handling
* **Behavior/Rule:** A malformed config in one entry prints a warning line and continues; it does not abort the whole listing.
* **Specifics:** `fmt.Fprintf(os.Stderr, "warning: %s: %v\n", name, err)` then continue.

## 4. Execution & Milestones

- [ ] Write failing test: `TestListGames_PrintsAllConfiguredTitles`
- [ ] Write failing test: `TestListGames_SkipsDirectoryWithNoConfigFile`
- [ ] Write failing test: `TestListGames_HandlesMissingGamesDir`
- [ ] Add `List()` method to `App` in `internal/app/app.go`
- [ ] Add `case "list":` to the switch in `cmd/yapl/main.go`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `cmd/yapl/main.go` — add `case "list":`
* `internal/app/app.go` — add `List()` method
* `internal/app/app_test.go` — new tests (create file if not present)

### Risks & Considerations
* **Column width:** Do not right-pad to fixed width with spaces in code — output raw tabs and let the user pipe to `column -t`. This avoids encoding a width assumption.
* **No `--game` flag:** `list` ignores `--game`; document this in the help text.
