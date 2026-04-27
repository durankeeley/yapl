# PRD-28a - setup --method flag: override launch method when creating new config

**Status:** `[x] Complete`
**Priority:** High
**Size:** S
**Sprint:** —
**Tags:** `bugfix`, `cli`, `setup`
**Created:** 2026-04-28
**GitHub Issue:** —

---

## 1. Problem & Context

When `yapl setup game "nfsp-mw2005"` runs and no `game.json` exists, `LoadOrCreateApp` always creates the config with `launch_method: "container"`. There is no way to tell `setup` to create the config with `launch_method: "direct"` before the file is written.

The user's intent was: "I know this game needs the direct method — create the config that way from the start." Instead they must run `setup`, wait for the full container init to finish, manually edit `game.json`, then re-run `setup`.

* **Current State:** `LoadOrCreateApp` hardcodes `LaunchMethod: "container"` in the generated default. The generated config is also passed straight to `InitializePrefix`, so the wrong prefix init path runs before the user even gets a chance to correct it.
* **Impact/Need:** Any user who wants a `direct`-method game must edit the config after setup has already run the wrong (heavier) init path.

---

## 2. Proposed Solution

Add a `--method <direct|container|umu>` flag to `main.go`. When provided:

- If no config file exists yet, use the supplied method when writing the default config instead of `"container"`.
- If a config file already exists, the flag is silently ignored (the file is the source of truth).

This keeps existing behaviour unchanged for all current users and gives a clean escape hatch for the new-config case.

---

## 3. Scope & Design Details

### CLI (`cmd/yapl/main.go`)

* **New flag:** `--method <string>` (default `""`, empty = use config file value or auto-default).
* Validate that if non-empty the value is one of `direct`, `container`, `umu`; print an error and exit if not.
* Pass `method` into `initializeApp` and then to `config.LoadOrCreateApp`.

### Config (`internal/config/config.go`)

* `LoadOrCreateApp` gains a new `defaultMethod string` parameter.
* When creating a new config: if `defaultMethod != ""`, use it as `LaunchMethod`; otherwise keep `"container"`.
* When a config file already exists: `defaultMethod` is ignored entirely.
* `LoadOrCreateApp` signature change: `func LoadOrCreateApp(appType, appName, configName string, globalCfg Global, defaultMethod string) (App, error)`

### `initializeApp` helper (`cmd/yapl/main.go`)

* Add `method string` to its signature and forward it to `config.LoadOrCreateApp`.

### No changes required

* `internal/command/command.go` — prefix init already dispatches on `appCfg.LaunchMethod`; now that the method is correct at creation time, it will route correctly.
* `internal/app/app.go` — no changes.

---

## 4. Execution & Milestones

- [ ] Write failing tests in `internal/config/config_test.go`:
  - `TestLoadOrCreateApp_MethodFlagOverridesDefault` — new config created with `defaultMethod: "direct"` has `LaunchMethod == "direct"`.
  - `TestLoadOrCreateApp_ExistingConfigIgnoresMethodFlag` — existing config with `LaunchMethod: "container"` is returned unchanged when `defaultMethod: "direct"` is passed.
- [ ] Run `go test ./...` — confirm new tests fail.
- [ ] Update `config.LoadOrCreateApp` signature to accept `defaultMethod string`.
- [ ] Update all call sites for `LoadOrCreateApp` (only `initializeApp` in `main.go`, and any existing tests).
- [ ] Update `initializeApp` to accept and forward `method string`.
- [ ] Add `--method` flag to `main.go`; validate value; pass to `initializeApp`.
- [ ] Run `go test ./...` — confirm all tests pass.
- [ ] Update `README.md` Flags table to document `--method`.

---

## 5. Technical Notes

### Key Files & Systems Targeted

* `cmd/yapl/main.go` — add `--method` flag, validate, pass to `initializeApp`
* `internal/config/config.go` — `LoadOrCreateApp` signature + default-config path
* `internal/config/config_test.go` — two new tests covering flag-present and flag-ignored cases

### Risks & Considerations

* **Signature change is a breaking internal API change:** Only one call site (`initializeApp`), so impact is minimal.
* **Existing config_test.go tests:** All existing `LoadOrCreateApp` calls must add `""` as the new last argument; update them all.
* **`--method` on `run`/`package`/`info`:** Flag is parsed globally but only meaningful for `setup`. Silently ignoring it for other commands is acceptable — document that it only applies to `setup`.
