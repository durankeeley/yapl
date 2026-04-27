# PRD-21 - Fix steam_appid.txt Write Error Handling

**Status:** `[ ] Pending`
**Priority:** Medium
**Size:** S
**Sprint:** Backlog
**Tags:** `bugfix`, `error-handling`, `command`
**Created:** 2026-04-27

---

## 1. Problem & Context

`internal/command/command.go` writes `steam_appid.txt` into the prefix directory before launching a game. The write is done with `os.WriteFile` and the error is handled with `log.Printf` ("warning: could not write steam_appid.txt") — then execution continues regardless. This has two problems:

1. `log.Printf` in an internal package violates coding standards (errors must be returned, not logged).
2. If the write fails (e.g., read-only filesystem, full disk), the game launches without `steam_appid.txt`. Some games require this file for Steam API initialisation and will crash or fail silently.

* **Current State:** Failed `steam_appid.txt` write is logged with `log.Printf` and silently ignored.
* **Impact/Need:** Games that require a valid `SteamAppId` may fail to launch with no clear error message.

## 2. Proposed Solution

* Change the write to return an error to the caller instead of logging.
* The caller (`RunDirectly`, `RunInContainer`, `RunWithUMU`) propagates the error up to `app.Run`, which surfaces it to `main.go`.
* This makes `steam_appid.txt` write failures visible and actionable.

## 3. Scope & Design Details

### Error propagation
* **Behavior/Rule:** Extract the `steam_appid.txt` write into a helper `writeSteamAppID(prefixPath, appID string) error`. If `os.WriteFile` returns an error, `writeSteamAppID` returns it.
* **Specifics:** Callers check the return value and return the error immediately.

### When SteamAppID is empty
* **Behavior/Rule:** If `appCfg.SteamAppID == ""`, skip writing `steam_appid.txt` entirely — no file, no error.
* **Specifics:** This matches the existing behaviour of falling back to `yapl-default` in `buildProtonEnv` for `UMU_ID`, but the file write should be skipped to avoid writing `steam_appid.txt` with an empty or placeholder ID.

### log.Printf removal
* **Behavior/Rule:** Remove all `log.Printf` calls related to `steam_appid.txt` from `command.go`.
* **Specifics:** Internal packages must not log; they must return errors.

## 4. Execution & Milestones

- [ ] Write failing test: `TestWriteSteamAppID_CreatesFileWithCorrectContent`
- [ ] Write failing test: `TestWriteSteamAppID_SkipsWhenAppIDEmpty`
- [ ] Extract `writeSteamAppID` helper in `command.go`
- [ ] Update callers to check and propagate the error
- [ ] Remove `log.Printf` from `steam_appid.txt` write path
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/command/command.go` — `writeSteamAppID` helper, updated callers
* `internal/command/command_test.go` — new tests

### Risks & Considerations
* **Read-only prefix:** If the prefix is on a read-only filesystem, this becomes a hard error rather than a warning. This is correct — the game can't launch from a read-only prefix anyway.
* **Multiple call sites:** `steam_appid.txt` is written in at least `RunDirectly` and `RunInContainer`. Both must be updated.
