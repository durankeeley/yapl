# PRD-52 - Align New Commands With the Docker/kubectl CLI Style

**Status:** `[x] Complete`
**Priority:** High
**Size:** S
**Sprint:** Phase 3 follow-up
**Tags:** `cli`, `ux`
**Created:** 2026-10-05
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** PRD-29 set the shape `yapl <verb> [noun] <name> [flags]`, like `docker`/`kubectl`. An audit of the commands added since then found two that put magic words in the name slot:
  * `yapl pull list`: a game called "list" could never be pulled, and "list" isn't a verb.
  * `yapl upgrade self` / `yapl upgrade catalog`: "self" and "catalog" collide with game names.
* Everything else fits: `install game <name>`, `push <name>`, `pull <name[:version]>`, `upgrade <name>`, `export desktop|steam|autostart [name]`, `console`, `serve`, `version`, `help`.

## 2. Proposed Solution

| Before | After |
|---|---|
| `yapl pull list` | `yapl search [term]` (like `docker search`; term filters by name, title or tag) |
| `yapl pull` (no name) | usage error: a name is required |
| `yapl upgrade self` | `yapl upgrade --self` |
| `yapl upgrade catalog` | `yapl upgrade --catalog` |

* The old spellings give a usage error naming the new one, rather than trying to pull or upgrade a game called "list", "self" or "catalog".
* CLAUDE.md gains the rule: never put a magic word where a name goes; use a verb or a flag.
* The `store` module is bumped to 3.0.0 (breaking CLI change).

## 4. Execution & Milestones

- [x] Tests for `search`, `upgrade --self`, `upgrade --catalog` and the hints for old spellings.
- [x] Commands changed; CLI reference regenerated.
- [x] README, developer guide and CLAUDE.md updated.
