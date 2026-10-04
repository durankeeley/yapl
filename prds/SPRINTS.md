# YAPL Roadmap and Sprint Plan

## Product goal

A local "Docker Hub" for Windows games on Linux: the operator installs and packages a game once and pushes it to a LAN server. Every other machine pulls it with one button from a console-style, gamepad-driven launcher.

## Phases (current plan, 2026-10-04)

Phases supersede the sprint ordering below for anything they list. Within a phase, PRDs are worked top to bottom.

### Phase 0 — Stabilise ✅ Complete
| PRD | Title | Status |
|-----|-------|--------|
| PRD-33 | Background services | ✅ Done |
| PRD-8  | Config validation (cherry-picked from the prd-8 worktree) | ✅ Done |
| PRD-36 | Fix `clean` flag parsing | ✅ Done |
| PRD-37 | Merge bundled `runner.json` entries on unpackage | ✅ Done |

### Phase 1 — Out-of-the-box foundation ✅ Complete
| PRD | Title | Size | Status |
|-----|-------|------|--------|
| PRD-38 | Microkernel: versioned modules, scoped flags, generated help, `yapl version` | L | ✅ Done |
| PRD-20 | Centralise directory paths (`internal/paths`) | S | ✅ Done |
| PRD-26 | `--workdir` / `YAPL_HOME` | S | ✅ Done |
| PRD-19 | DXVK DirectX version validation | S | ✅ Done |
| PRD-39 | Working defaults catalog (pinned GE-Proton, DXVK, VKD3D, sniper) | M | ✅ Done |
| PRD-9  | Download progress (`internal/progress`) | S | ✅ Done |
| PRD-40 | `yapl install` from installer or folder, with executable detection | M | ✅ Done |

### Phase 2 — Registry server ("Docker Hub")
| PRD | Title | Size |
|-----|-------|------|
| PRD-41 | Package manifest: metadata, versions, checksums, components | M |
| PRD-42 | Registry API v1 and versioned storage | L |
| PRD-43 | Shared component layers and `yapl push` | L |
| PRD-44 | Pull v2: tags, verified streaming install, components, remembered server | L |
| PRD-45 | `yapl upgrade`: per-part upgrades (games keep saves, self, catalog) | L |

### Phase 3 — Console client
| PRD | Title | Size |
|-----|-------|------|
| PRD-46 | Console service: local API for library, store, jobs, sessions | L |
| PRD-47 | `yapl console`: gamepad-first full-screen launcher | L |
| PRD-48 | `yapl export`: desktop, Steam and autostart integration | M |

### Backlog (not in a phase)
PRD-11 (dependency upgrade flags, partly superseded by PRD-45), PRD-12 (`--no-prefix`), PRD-23/25/30 (wine builder ecosystem), PRD-35 (display resolution).

---

## Original sprint plan

Sprints are ordered by dependency and impact. Earlier sprints produce a cleaner, more reliable codebase that later sprints build on. Within each sprint, PRDs can be worked in parallel or in the order listed.

---

## Sprint 1 — Codebase Health
**Theme:** Remove dead code, fill test gaps, wire up code that already exists. No user-visible changes. Do this first so the codebase is clean before any new behaviour is added.

| PRD | Title | Size |
|-----|-------|------|
| PRD-15 | Remove unused Python fields from `VersionInfo` | S |
| PRD-16 | Remove deprecated `MustGetAbsolutePath` | S |
| PRD-27 | Wire `fs.CopyDir` into `app.Package` (replace manual copy loop) | S |
| PRD-22 | Fill test coverage gaps in `internal/app` | S |

**Why together:** All four are pure internal cleanup with no user-facing change. Touching the same handful of files once is more efficient than four separate PRs.

---

## Sprint 2 — Reliability (Error Propagation) ✅ Complete
**Theme:** YAPL currently swallows several network and I/O errors silently. Every PRD here fixes the same class of bug in the same layer of the code. Doing them together means each file is touched once, not five times across five PRs.

| PRD | Title | Size | Status |
|-----|-------|------|--------|
| PRD-6  | Add HTTP timeout to runtime HTTP client | S | ✅ Done |
| PRD-17 | Validate HTTP status code before reading runtime response body | S | ✅ Done |
| PRD-18 | Propagate extraction errors from `Unpackage` | S | ✅ Done |
| PRD-21 | Return `steam_appid.txt` write error instead of logging and continuing | S | ✅ Done |
| PRD-24 | Propagate runtime version-check errors instead of swallowing | S | ✅ Done |

**Why together:** All five are in the same problem family — silent failures in I/O paths. A user can't tell the difference between "download succeeded" and "download failed silently" right now. Fixing all five in one sprint closes the whole class.

---

## Sprint 3 — Validation & Deployment Flexibility
**Theme:** Catch bad config early rather than failing mid-run, let YAPL be invoked from anywhere, and manage the host environment around game launches. PRD-8 is foundational for PRD-19, PRD-25 (Sprint 5), and PRD-35. PRD-20 is foundational for PRD-26.

| PRD | Title | Size |
|-----|-------|------|
| PRD-8  | Config validation — catch missing required fields before setup/run | S |
| PRD-19 | DXVK DirectX version validation (`"9"`, `"10"`, or `"11"` only) | S |
| PRD-20 | Centralise directory path constants into `internal/paths` | S |
| PRD-26 | `--workdir <path>` global flag for non-CWD deployments | M |
| PRD-35 | Display resolution management — switch host display + patch game configs around `run` | L |

**Why together:** PRD-8 + PRD-19 both validate config fields — they share the same validation infrastructure. PRD-20 + PRD-26 both deal with how YAPL resolves paths — PRD-20 refactors path strings into constants, and PRD-26 adds the ability to root those paths at an arbitrary directory. PRD-35 builds on PRD-8's validation pass to validate the `display` config block before attempting any `xrandr` calls.

**kubectl-style note for PRD-26:** `--workdir` is the right name — it mirrors how other tools expose a "context root" override (e.g. `make -C`, `docker-compose --project-directory`). Keep it as a global flag placed before the command.

---

## Sprint 4 — Dependency & Package UX
**Theme:** The most visible day-to-day improvements. Progress bars, upgrade shortcuts, and packaging flexibility. None depends on anything outside Sprint 1–3 being done first (though PRD-9's progress work may cleanly reuse the HTTP client from PRD-6).

| PRD | Title | Size |
|-----|-------|------|
| PRD-9  | Download progress indicator (byte count / percentage for all HTTP downloads) | S |
| PRD-11 | Upgrade flags: `--upgrade-deps`, `--upgrade-runtime`, `--upgrade-all` alongside existing `--upgrade-proton` | S |
| PRD-12 | `--no-prefix` flag for `package` — exclude Wine prefix from the archive | S |

**Why together:** All three improve the core `setup`/`run`/`package` workflow that most users touch every session. PRD-9's progress reporting will make PRD-11's forced re-downloads much less anxiety-inducing. PRD-12 completes the packaging story.

**kubectl-style note for PRD-11:** Keep as flags on `setup` and `run` (consistent with existing `--upgrade-proton`). An `upgrade` subcommand would add a new verb and a new learning curve for a feature that is always run alongside setup/run anyway.

---

## Sprint 5 — Wine Builder Ecosystem
**Theme:** Connect locally-built Wine to YAPL's dependency management. PRD-30 is standalone and the most impactful for anyone using the `direct` launch method. PRD-23 modernises the wine builder. PRD-25 ties them together via a new `yapl add proton` command.

| PRD | Title | Size |
|-----|-------|------|
| PRD-30 | DXVK/VKD3D auto-install into Wine prefix for `direct` launch method | M |
| PRD-23 | Wine builder: switch to single WoW64 build, add `.tar.xz` output | M |
| PRD-25 | Wine builder integration: `local_archive` config field + `yapl add proton` command | M |

**Why together:** PRD-30 is usable standalone (the most impactful fix for direct-method users), but PRD-23 and PRD-25 are tightly coupled — the whole point of PRD-23 is to produce a tarball that PRD-25 knows how to register and extract.

**kubectl-style note for PRD-25:** Replace the proposed `yapl add --name <n> --archive <path>` with:

```
yapl add proton <name> <archive-path>
```

This follows the established YAPL pattern of `yapl <verb> <resource-type> <name>` (same as `yapl setup game <name>`, `yapl unpackage game <file>`). Example:

```bash
yapl add proton wine-11.7 ./wine-builder/output/wine-11.7.tar.xz
```

This writes the `local_archive` entry into `runner.json` automatically.

---

## Dependency Map

```
Sprint 1 (clean code) ✅
  └─► Sprint 2 (reliability) ✅
        └─► Sprint 3 (validation + workdir + display)
              ├─► PRD-8 validates config → PRD-25 (Sprint 5) + PRD-35 use same infra
              ├─► PRD-20 centralises paths → PRD-26 uses them
              └─► PRD-35 display management (depends on PRD-8 for validation pass)
Sprint 4 (UX) — independent, can start after Sprint 1
Sprint 5 (wine ecosystem) — PRD-30 independent; PRD-25 needs PRD-8 from Sprint 3
```

---

## What is not in any sprint

- **PRD-29 (kubectl-style CLI):** Already done.
- **SSH key auth for `serve`:** Descoped from PRD-32 pending a decision on adding `golang.org/x/crypto/ssh`.
