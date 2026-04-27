# CLAUDE.md — Project Guide for YAPL

> **Agents: Read `AGENTS.md` and the entire `SKILLS/` folder before doing anything.**

---

## What is YAPL?

YAPL (Yet Another Proton Launcher) is a Go CLI tool that solves the "it works on my machine" problem for Windows games and apps running on Linux via Proton/Wine. It creates **portable, reproducible environments**: define your exact Proton build, DXVK version, Wine prefix, and launch method in a JSON file, then package the whole thing into a tarball that unpacks and runs identically on any other Linux machine.

Primary use case: set up a game on one PC, package it, copy it to 10 LAN party machines, and have it work everywhere.

---

## Architecture

```
cmd/yapl/main.go          — CLI entry: flag parsing, command dispatch
internal/app/app.go       — Orchestrator: holds state, calls into packages
internal/config/config.go — Load/save runner.json and game.json/app.json
internal/dependency/      — Download/verify Proton, DXVK, Steam Runtime
internal/command/command.go — Build env vars, run wine/proton/container
internal/archive/archive.go — Extract/create tar.gz / tar.xz / tar.zst
internal/fs/fs.go         — Filesystem helpers (copy, exists, abs path)
```

### Flow

1. `main.go` parses flags and dispatches to `setup`, `package`, `run`, or `unpackage`
2. `initializeApp` loads `runner.json` (global tool URLs) and `game.json` / `app.json` (per-game config)
3. An `App` struct is created and the appropriate method called (`Setup`, `Run`, `Package`)
4. `App.Setup` / `App.Run` call `dependency.EnsureAll` (downloads if missing), `dependency.EnsureRuntime`, then `command.InitializePrefix`
5. `App.Run` then dispatches to one of three launch methods:
   - `direct` — calls `wine64`/`wine` directly from the Proton distribution
   - `container` — wraps execution in the Steam Linux Runtime
   - `umu` — delegates to `umu-launcher`

### Directory layout at runtime

```
./runner.json             — global config (Proton URLs, DXVK URLs, runtime URLs)
./proton/<version>/       — downloaded Proton builds
./dependencies/dxvk/      — DXVK versions
./dependencies/vkd3d/     — VKD3D versions
./dependencies/runtime/   — Steam Linux Runtime
./games/<name>/prefix/    — Wine prefix per game
./games/<name>/game.json  — per-game config
./apps/<name>/prefix/     — Wine prefix per app
./apps/<name>/app.json    — per-app config
```

---

## Key Files

| File | Role |
|------|------|
| `cmd/yapl/main.go` | Entry point; CLI flags and command switch |
| `internal/app/app.go` | `App` struct; `Setup`, `Run`, `Package` methods |
| `internal/config/config.go` | `Global`, `App` config structs; JSON load/save |
| `internal/command/command.go` | `buildProtonEnv`, `RunDirectly`, `RunInContainer`, `RunWithUMU`, `InitializePrefix` |
| `internal/dependency/dependency.go` | `EnsureAll`, `ensureProton`, `InstallCustomComponents` |
| `internal/dependency/runtime.go` | `EnsureRuntime`, runtime update check |
| `internal/archive/archive.go` | `Extract`, `Package`, `Unpackage` |
| `internal/fs/fs.go` | `CopyFile`, `CopyDir`, `DirExistsAndIsNotEmpty` |
| `prds/` | Active PRD/Task files |
| `prds/done/` | Completed PRD/Task files |
| `SKILLS/PRD/template.md` | Mandatory PRD template |

---

## Coding Standards

- **Language:** Go 1.24+
- **No external frameworks** — stdlib + the two existing compression libs only
- **Error handling:** Return errors up the call stack; never use `log.Fatalf` inside library packages (`internal/*`) — only in `main.go`
- **No comments** explaining what code does — name things clearly instead. Only comment *why* when it's non-obvious (a workaround, a hidden constraint)
- **No half-finished code** — every PR must be complete and testable
- **No unnecessary abstractions** — don't design for hypothetical futures

### Testing Rules

- Every package in `internal/` must have a `_test.go` file
- Tests must run with `go test ./...`
- Write the **failing** test first, then write the code to make it pass
- Test the behaviour, not the implementation — test via public functions
- Use table-driven tests where there are multiple input cases
- No mocking of the filesystem for unit tests — use `t.TempDir()` for real temp directories

---

## Mandatory Workflow

Read `AGENTS.md` for the authoritative version. Summary:

1. **Create or edit a PRD/Task** in `prds/` using `SKILLS/PRD/template.md` — set status to **In Progress**
2. **`git commit`** the PRD
3. **Write a failing test** that defines the behaviour required
4. **Run `go test ./...`** — confirm it fails
5. **Write the implementation code**
6. **Run `go test ./...`** — confirm all tests pass
7. **Mark the PRD as Complete** and move it to `prds/done/`
8. **`git commit`** the implementation + passing tests

Never skip steps. Never commit code that doesn't have tests.

---

## Building

```bash
go build -o yapl ./cmd/yapl/
```

## Testing

```bash
go test ./...
go test -v ./internal/...
```

## Skills

Before any task, read `SKILLS/PRD/SKILL/SKILL.md` to understand how to create worktrees and manage PRDs.
