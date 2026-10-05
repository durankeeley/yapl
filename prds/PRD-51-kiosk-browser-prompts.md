# PRD-51 - No Browser Prompts in the Console Kiosk

**Status:** `[ ] In Progress`
**Priority:** High
**Size:** S
**Sprint:** Phase 3 follow-up
**Tags:** `bug`, `console`, `browser`
**Created:** 2026-10-05
**GitHub Issue:** N/A

---

## 1. Problem & Context

* **Current State:** On first launch of `yapl console` with Google Chrome 150, a "Can't update Chrome" prompt covers the console. The console runs Chrome with a dedicated profile, Chrome's outdated-build detector fires, and on Linux Chrome cannot update itself. Closing the kiosk window also leaves a "restore pages?" bubble on the next start.
* **Impact/Need:** A console must never show browser chrome or prompts, and there's no mouse on the couch to dismiss them.

## 2. Proposed Solution

Chromium-family launches add:
* `--simulate-outdated-no-au=<far-future date>`: tells the outdated-build detector that auto-update is off until then, so the update prompt never appears.
* `--check-for-update-interval=<one year>`: no update checks during a session.
* `--hide-crash-restore-bubble`: no "restore pages?" after the kiosk window is closed.
* `--disable-features=Translate`: no translate prompt over game descriptions.

Values are named constants. Firefox is unaffected.

## 4. Execution & Milestones

- [ ] Browser command test updated (Given/When/Then).
- [ ] Flags added.
- [ ] README console section mentions prompts are suppressed.
