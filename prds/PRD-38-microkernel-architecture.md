# PRD-38 - Microkernel Architecture: Versioned Modules, Scoped Flags, Generated Help

**Status:** `[ ] Pending`
**Priority:** High
**Size:** L
**Sprint:** Phase 1 — Out-of-the-box foundation
**Tags:** `architecture`, `cli`, `kernel`, `versioning`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** `cmd/yapl/main.go` is one 340-line `switch`. Every flag lives in a single global `flag` set, so flags leak across commands (`--port` means two things, PRD-36 was caused by this). The usage text is a hand-written string that already disagrees with the code. Tunables such as port `8471`, the 3 s discovery timeout, the 30 s rescan and the 2 s broadcast are literals spread across packages. Nothing in YAPL has a version, so a LAN client cannot tell whether it is compatible with a server.
* **Impact/Need:** Phases 2 and 3 add about ten commands (push, upgrade, install, console, export, version). Adding them to the switch makes the file unmaintainable, and help would drift further. Per-part upgrades (PRD-45) need each part of YAPL to carry its own version.

## 2. Proposed Solution

A small kernel owns argument parsing, help and dispatch. Features live in **modules**: packages that declare a name, a semantic version and the commands they contribute. `main.go` only registers modules and hands `os.Args` to the kernel.

```
cmd/yapl/main.go ── registers ──► kernel ◄── Module{Name, Version, Commands}
                                     │
          ┌──────────────┬───────────┼─────────────┬──────────────┐
      lifecycle       registry      store        console         (later)
   setup/run/...    serve/push   pull/upgrade  console/export
```

## 3. Scope & Design Details

### `internal/version`
* `Parse("1.4.2")`, `Compare(a, b)`, `IsCompatible(a, b)` (same major). Non-semver strings fall back to string comparison so `"GE-Proton11-7"` still orders.

### `internal/defaults`
* Every shared tunable as a named, documented constant: `ServerPort`, `ConsolePort`, `DiscoveryTimeout`, `ManifestRefreshInterval`, `BroadcastInterval`, `DownloadTimeout`, `CopyBufferSize`, `DefaultFormat`, `DefaultLaunchMethod`.

### `internal/kernel`
* `Module` interface: `Name() string`, `Version() string`, `Commands() []Command`.
* `Command{Verb, Usage, Summary, Description, Flags func(*flag.FlagSet), Run func(*Context) error}`.
* `Context{Args []string, Flags *flag.FlagSet, Stdout, Stderr io.Writer, Kernel *Kernel}`.
* `Register` rejects duplicate verbs. `Execute(args)` returns an exit code.
* Each command gets its **own** `flag.FlagSet` (flags are scoped, as CLAUDE.md requires). Flags may appear before or after positional arguments.
* Built-in commands: `help [verb]`, `version`. `yapl <verb> --help` prints that command's usage, description and flags, all generated from the registration.
* `yapl version` prints `kernel`, every module and its version; `--output json` emits machine-readable JSON (used by `serve` in PRD-42).
* Errors from `Run` print as `error: <msg>` on stderr with exit 1; usage errors exit 2.

### Modules (this PRD)
* `internal/modules/lifecycle` — `setup`, `run`, `package`, `unpackage`, `info`, `list`, `clean`.
* `internal/modules/registry` — `serve`.
* `internal/modules/store` — `pull`.
* Each module package has `const Version = "1.0.0"`, bumped whenever its behaviour changes.

## 4. Execution & Milestones

- [ ] `version`, `defaults`, `kernel` packages with Given/When/Then tests.
- [ ] Modules for existing commands; `main.go` reduced to registration.
- [ ] Literals replaced with `defaults` constants in server, client, archive.
- [ ] CLI subprocess tests cover help, version, unknown command, flag scoping.
- [ ] README "Commands" and "Flags" sections regenerated from `yapl help`; developer guide gains an architecture section.

### Acceptance scenarios
* **Given** the kernel with the lifecycle module **when** I run `yapl help` **then** every registered verb is listed with its summary.
* **Given** `yapl serve` **when** I pass `--prefix` **then** it fails with a usage error, because `--prefix` belongs to `clean`.
* **Given** two modules declaring `run` **when** both register **then** registration fails naming the duplicate.
* **Given** `yapl version --output json` **then** the output lists every module and the kernel with semantic versions.

## 5. Technical Notes

### Key Files & Systems Targeted
* `cmd/yapl/main.go`, `internal/kernel/*`, `internal/version/*`, `internal/defaults/*`, `internal/modules/*`.

### Risks & Considerations
* **Behaviour parity:** CLI subprocess tests from PRD-36 stay green throughout; they test the binary, not internals.
* **Why not Go plugins:** `plugin` is Linux-only, fragile across toolchains and conflicts with "single static binary". Modules are compiled in; the version contract is what makes parts independently upgradable.
