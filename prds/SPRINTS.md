# YAPL Sprint Plan

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

## Sprint 2 — Reliability (Error Propagation)
**Theme:** YAPL currently swallows several network and I/O errors silently. Every PRD here fixes the same class of bug in the same layer of the code. Doing them together means each file is touched once, not five times across five PRs.

| PRD | Title | Size |
|-----|-------|------|
| PRD-6  | Add HTTP timeout to runtime HTTP client | S |
| PRD-17 | Validate HTTP status code before reading runtime response body | S |
| PRD-18 | Propagate extraction errors from `Unpackage` | S |
| PRD-21 | Return `steam_appid.txt` write error instead of logging and continuing | S |
| PRD-24 | Propagate runtime version-check errors instead of swallowing | S |

**Why together:** All five are in the same problem family — silent failures in I/O paths. A user can't tell the difference between "download succeeded" and "download failed silently" right now. Fixing all five in one sprint closes the whole class.

---

## Sprint 3 — Validation & Deployment Flexibility
**Theme:** Catch bad config early rather than failing mid-run, and let YAPL be invoked from anywhere (scripts, desktop launchers, systemd services). PRD-8 is foundational for PRD-19 and PRD-25 (Sprint 5). PRD-20 is foundational for PRD-26.

| PRD | Title | Size |
|-----|-------|------|
| PRD-8  | Config validation — catch missing required fields before setup/run | S |
| PRD-19 | DXVK DirectX version validation (`"9"`, `"10"`, or `"11"` only) | S |
| PRD-20 | Centralise directory path constants into `internal/paths` | S |
| PRD-26 | `--workdir <path>` global flag for non-CWD deployments | M |

**Why together:** PRD-8 + PRD-19 both validate config fields — they share the same validation infrastructure. PRD-20 + PRD-26 both deal with how YAPL resolves paths — PRD-20 refactors path strings into constants, and PRD-26 adds the ability to root those paths at an arbitrary directory.

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
Sprint 1 (clean code)
  └─► Sprint 2 (reliability) — cleaner code = safer error propagation changes
        └─► Sprint 3 (validation + workdir)
              ├─► PRD-8 validates config → PRD-25 (Sprint 5) uses same infra
              └─► PRD-20 centralises paths → PRD-26 uses them
Sprint 4 (UX) — independent, can start after Sprint 1
Sprint 5 (wine ecosystem) — PRD-30 independent; PRD-25 needs PRD-8 from Sprint 3
```

---

## What is not in any sprint

- **PRD-29 (kubectl-style CLI):** Already done.
- **SSH key auth for `serve`:** Descoped from PRD-32 pending a decision on adding `golang.org/x/crypto/ssh`.
