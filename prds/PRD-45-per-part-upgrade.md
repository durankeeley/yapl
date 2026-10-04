# PRD-45 - `yapl upgrade`: Upgrade Only the Parts That Changed

**Status:** `[ ] In Progress`
**Priority:** High
**Size:** L
**Sprint:** Phase 2 — Registry server
**Tags:** `upgrade`, `client`, `versioning`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** The only upgrade path is `--upgrade-proton` (re-download one Proton build). A new version of a game means deleting it and pulling again, which also deletes saves. Ten LAN machines running different YAPL builds have no way to converge.
* **Impact/Need:** Everything in YAPL is now versioned: games (PRD-41), components (immutable, PRD-43), the binary's modules and the defaults catalog (PRD-38/39). Upgrades should move only the parts whose version changed.

## 2. Proposed Solution

```
yapl upgrade                 # show what is outdated: games, yapl itself, catalog
yapl upgrade "Doom"          # pull the newer Doom; keep saves
yapl upgrade --all           # every outdated game, plus the catalog
yapl upgrade self            # replace this binary with the server's newer yapl
yapl upgrade catalog         # add newer pinned components to runner.json
```

## 3. Scope & Design Details

### Plan (`internal/upgrade`)
* `Plan(local LocalState, remote RemoteState) []Item` — pure function: compares `installed.json` versions to the server catalog, local kernel version to the server's, local catalog version to the compiled-in one. Each item: `{Part, Name, From, To}`. Easy to table-test.

### Game upgrade
* Uses PRD-44's verified install into staging. Before the swap, copy preserved paths from the old install into staging: always `prefix/drive_c/users` and `prefix/user.reg` (per-user settings and saves), plus `metadata.preserve` globs relative to the game directory (for games that save next to the exe, e.g. `prefix/drive_c/Games/Half-Life/cstrike/config.cfg`). `system.reg` is **not** preserved: it holds machine-wide install keys a new version may legitimately change.
* Swap: rename old → `.<Name>.old`, rename staging → `<Name>`, delete old. On failure, restore old.
* New components the new version needs are pulled; old components are left (another game may use them; `clean` handles removal).

### Self upgrade
* Server `GET /v1/yapl/binary` serves its own executable (`os.Executable()`); `/v1/info.kernel` advertises the version.
* Client downloads to a temp file next to `os.Executable()`, verifies the `X-Yapl-SHA256` header, `chmod 0755`, renames over itself.
* "Newer" is decided per part: the server binary is newer only if no module (or the kernel) is older than the local one and at least one is newer. A divergent build is refused. `/v1/info` also reports `platform` (`linux/amd64`), and a binary for another platform is refused. Only offered when the server's kernel version is newer: YAPL never downgrades itself. `/v1/yapl/binary` is a permanent path, so even a client on an old API version can always fetch a newer binary.

### Catalog upgrade
* `MergeGlobal(local, catalog.Default())`, then set `defaults` and `catalog_version` to the catalog's. Existing games keep the explicit versions in their `game.json`, so only new games pick up the new defaults.

## 4. Execution & Milestones

- [ ] `upgrade.Plan` table tests.
- [ ] Preserve-and-swap tests (saves survive, rollback on failure).
- [ ] Self-upgrade endpoint + client tests (writes into a temp "executable" path in tests).
- [ ] `upgrade` command; README "Upgrading" section; PRD-11 marked superseded for game/self paths.

### Acceptance scenarios
* **Given** Doom 1.0.0 installed with a save in `prefix/drive_c/users/steamuser/Saved Games/doom.sav` **and** 1.1.0 on the server **when** I `upgrade Doom` **then** `installed.json` says 1.1.0 and the save file is unchanged.
* **Given** everything current **when** I run `yapl upgrade` **then** it prints "Everything is up to date".
* **Given** a server running an older kernel than the client **when** I `upgrade self` **then** nothing is downloaded and I'm told the client is already newer.
