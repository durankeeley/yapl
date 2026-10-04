# PRD-47 - `yapl console`: Gamepad-First Full-Screen Launcher

**Status:** `[x] Complete`
**Priority:** High
**Size:** L
**Sprint:** Phase 3 — Console client
**Tags:** `console`, `ui`, `gamepad`
**Created:** 2026-10-04
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** No UI. Players at a LAN party need to type commands.
* **Impact/Need:** The goal is a PS2-like experience: power on, pick a game with the controller, play. Install new games from the LAN server the same way.

## 2. Proposed Solution

`yapl console` starts the console service (PRD-46), serves an embedded single-page UI on `http://127.0.0.1:<ConsolePort>`, and opens it full-screen in the first available browser in kiosk/app mode.

* UI assets are plain HTML/CSS/JS embedded with `embed` — no frameworks, no build step, no new Go dependencies.
* Navigation is spatial (D-pad / left stick / arrow keys move focus to the nearest tile in that direction) and implemented as pure functions in `nav.js`, tested with `node --test` (Go test runs it when `node` exists, skips otherwise).

## 3. Scope & Design Details

### Screens
* **Library** — grid of cover tiles (generated gradient tile with initials when no cover). A = play when installed, otherwise details; X = details; Start = play.
* **Store** — server catalog; badges: Installed / Update available. A = install; progress bar on the tile while the job runs.
* **Details** — title, version, description, players, tags; buttons Play / Update / Uninstall (uninstall asks for a second confirm).
* **System** — yapl and module versions, server address and status, workdir.
* **Now playing** overlay while a session runs, with "Stop game".

### Input mapping (named constants in `input.js`)
| Gamepad (standard mapping) | Keyboard | Action |
|---|---|---|
| D-pad / left stick | Arrows | move focus |
| A (0) | Enter | select |
| B (1) | Escape / Backspace | back |
| LB/RB (4/5) | Q / E | switch tab |
| Start (9) | Space | play focused game |
* Stick dead zone and repeat delay are named constants.

### Browser launch (`internal/console/browser.go`)
* Candidates in order: `chromium`, `chromium-browser`, `google-chrome`, `brave-browser` (`--kiosk --app=<url>`), `firefox` (`--kiosk <url>`), then `xdg-open <url>`.
* Flags: `--no-browser`, `--port`, `--windowed` (app mode without kiosk), plus `--server`/`--auth`/`--password` like `pull`.
* The browser gets its own profile in `<library>/.console-browser`, so kiosk mode works even when the user's browser is already open and the console never touches their normal profile.
* When YAPL started the browser itself in kiosk mode, closing that window shuts the console down: closing the console screen ends everything.
* Server selection: `--server`, else `client.json` (if it answers), else LAN discovery; with none found the console starts offline (library only) and says so on the System tab.
* `yapl console` lives in its own versioned module (`internal/modules/console`).

## 4. Execution & Milestones

- [x] `nav.js` spatial navigation + input mapping, with node tests.
- [x] Embedded assets served with correct content types; Go tests check every referenced asset exists.
- [x] Browser selection tests (fake PATH with stub executables).
- [x] `console` command; README "Console mode" section with screenshots-in-words.

### Acceptance scenarios
* **Given** tiles in a 3-column grid **when** focus is on tile 1 and I press Down **then** focus moves to tile 4.
* **Given** focus on the last tile of a row **when** I press Right **then** focus stays (no wrap into the next row).
* **Given** only firefox on PATH **when** the console starts **then** it runs `firefox --kiosk http://127.0.0.1:8472/`.
* **Given** `--no-browser` **then** only the URL is printed.
