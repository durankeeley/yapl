# PRD-5 - Fix `system` Proton Version in EnsureAll

**Status:** `[x] Complete`
**Priority:** High
**Size:** S
**Sprint:** Backlog
**Tags:** `bugfix`, `system-wine`, `dependency`
**Created:** 2026-04-27

---

## 1. Problem & Context

`EnsureAll` calls `ensureProton`, which looks up `appCfg.ProtonVersion` in `globalCfg.ProtonVersions`. When `proton_version` is `"system"`, `ensureProton` returns an error (`proton version 'system' not defined in runner.json`) unless the user manually adds a `"system": {}` entry to `runner.json`. `InitializePrefix` has a special-case for `"system"` that works correctly, but it is never reached because `EnsureAll` errors first.

* **Current State:** `EnsureAll` → `ensureProton` fails for `proton_version: "system"` unless user adds a dummy entry in `runner.json`.
* **Impact/Need:** Users following the documented `"proton_version": "system"` workflow get a confusing error that contradicts the documentation.

## 2. Proposed Solution

* Add an early-return guard in `ensureProton`: if `appCfg.ProtonVersion == "system"`, return `nil` immediately — there is nothing to download or validate.
* Remove the requirement for a `"system"` entry in `runner.json`.
* Update the default `runner.json` template to omit the `"system"` key.

## 3. Scope & Design Details

### ensureProton guard
* **Behavior/Rule:** `if appCfg.ProtonVersion == "system" { return nil }` as the very first statement in `ensureProton`.
* **Specifics:** No download, no path check, no URL lookup. System Wine detection is entirely `InitializePrefix`'s responsibility.

### runner.json defaults
* **Behavior/Rule:** The auto-generated `runner.json` must not include a `"system"` entry under `proton_versions`.
* **Specifics:** Check `config.go` `defaultGlobalConfig()` and remove the entry if present.

### Documentation
* **Behavior/Rule:** `docs/developer-guide.md` "Using system Wine" section already says to set `proton_version: "system"` and omit from `runner.json`. Code must now match that claim.

## 4. Execution & Milestones

- [ ] Write failing test: `TestEnsureAll_SystemProtonVersionIsNoop` (EnsureAll returns nil for `proton_version: "system"` without any runner.json entry)
- [ ] Add `if appCfg.ProtonVersion == "system" { return nil }` to `ensureProton`
- [ ] Verify no `"system"` entry in `defaultGlobalConfig()`
- [ ] All tests pass
- [ ] Mark Complete, move to `prds/done/`

## 5. Technical Notes

### Key Files & Systems Targeted
* `internal/dependency/dependency.go` — guard in `ensureProton` (line ~34)
* `internal/config/config.go` — verify `defaultGlobalConfig()` has no `"system"` entry
* `internal/dependency/dependency_test.go` — new test

### Risks & Considerations
* **Case sensitivity:** The constant `"system"` must be compared as a string literal; no normalisation needed given that `config.go` already uses lowercase throughout.
* **No regression to other paths:** The guard must only short-circuit the `"system"` case; all other version lookups must behave exactly as before.
