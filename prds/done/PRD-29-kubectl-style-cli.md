# PRD-29 - kubectl-Style CLI and xz Default Format

**Status:** `[x] Complete`
**Priority:** High
**Size:** M
**Tags:** `cli`, `refactor`, `ux`
**Created:** 2026-04-28

---

## 1. Problem & Context

* **Current State:** Every command requires `--game <name>` or `--app <name>` flags. The type (`game` vs `app`) must always be specified even though YAPL can look it up. The default package format is `gz` (slower, larger than `xz`).
* **Impact/Need:** The flag-based style is verbose and doesn't match modern CLI conventions. Users want to type `yapl run "nfs"` instead of `yapl --game "nfs" run`. The auto-detection of `games/` vs `apps/` already exists in `ListAll`; it should power all commands.

## 2. Proposed Solution

Switch to kubectl-style positional arguments: `<verb> [type] <name> [flags]`.

* For commands that operate on an existing entry (`run`, `package`, `info`): name alone is sufficient — YAPL searches `games/` then `apps/`.
* For `setup` and `unpackage`: type (`game` or `app`) is required because YAPL needs to know where to create or extract.
* `list` is unchanged (no name needed).
* Remove `--game` and `--app` flags.
* Change the default `--format` to `xz`.

## 3. Scope & Design Details

### New CLI shapes

```
yapl run <name> [flags]
yapl package <name> [--format xz|gz|zst] [--bundle-deps] [--yes]
yapl info <name>
yapl list
yapl setup game <name> [flags]
yapl setup app  <name> [flags]
yapl unpackage game <file> [<file>...]
yapl unpackage app  <file> [<file>...]
```

Flags that remain unchanged: `--config`, `--upgrade-proton`, `--format`, `--bundle-deps`, `--yes`, `--debug`, `--steam`.

### Auto-detection (`app.Find`)

New exported function `app.Find(name string) (appType, appName string, err error)`:
* Checks `games/<name>/game.json` and `apps/<name>/app.json`.
* Exactly one found → returns that type and name.
* Both found → error: `'<name>' exists in both games/ and apps/; specify type with 'setup game|app'`.
* Neither found → error: `'<name>' not found in games/ or apps/`.

### `setup` auto-detect for re-runs

`setup` with a type + name always creates/re-runs. An existing entry can also be re-set-up using just `setup <name>` (auto-detect), but a fresh entry always requires the explicit `game|app` sub-type.

Actually, keep it simple: `setup` always requires `game|app` as Arg(1). This avoids ambiguity.

### `cmd/yapl/main.go` changes

* Remove `--game` and `--app` flag definitions.
* Parse `command = flag.Arg(0)`.
* For `run`, `package`, `info`: `name = flag.Arg(1)`, call `app.Find(name)` to get type+name.
* For `setup`: `subType = flag.Arg(1)` ("game"/"app"), `name = flag.Arg(2)`.
* For `unpackage`: `subType = flag.Arg(1)` ("game"/"app"), `archives = flag.Args()[2:]`.
* For `list`: no change.
* Change `flag.String("format", "gz", ...)` → `flag.String("format", "xz", ...)`.

### Error messages

Helpful errors guide users to the right syntax:
* Missing name: `usage: yapl <command> <name>`
* Unknown type for setup/unpackage: `type must be 'game' or 'app'`

## 4. Execution & Milestones

- [ ] Add `app.Find(name)` with tests
- [ ] Refactor `main.go`: remove `--game`/`--app`, add positional dispatch
- [ ] Change default format to `xz`
- [ ] Update `README.md` with new CLI syntax
- [ ] All tests pass (`go test ./...`)
- [ ] PRD moved to done/

## 5. Technical Notes

### Key Files
* `internal/app/app.go` — add `Find(name string) (string, string, error)`
* `internal/app/app_test.go` — tests for `Find`
* `cmd/yapl/main.go` — full dispatch rewrite
* `README.md` — update all command examples

### Risks & Considerations
* **Breaking change:** Old `--game`/`--app` invocations no longer work. Any scripts using the old style need updating.
* **Ambiguity:** If the same name exists in both `games/` and `apps/`, `Find` errors. This is intentional — names should be unique across the two directories.
