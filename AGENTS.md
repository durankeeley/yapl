# AGENTS.md — AI Agent Rules for YAPL

All AI agents working in this repository **must** follow these rules before doing any work.

---

## 1. Read CLAUDE.md First

Before taking any action, read `CLAUDE.md` in full. It contains the project description, architecture, coding standards, and the mandatory workflow. CLAUDE.md is the source of truth for how work is done here.

## 2. Read the SKILLS Folder

Before starting any task, read every file under `SKILLS/`. Skills are reusable procedures that define *how* to do certain work (e.g., creating PRDs, creating git worktrees). You must follow the relevant skill for your task.

Key skills:
- `SKILLS/PRD/SKILL/SKILL.md` — how to create git worktrees for PRD/task branches
- `SKILLS/PRD/template.md` — the required template for all PRD files

## 3. Use the PRD Skill for Every Piece of Work

Every new feature, bug fix, or improvement **must** have a PRD or Task file. Use `SKILLS/PRD/template.md` as the template. Store PRDs in `prds/` and move them to `prds/done/` when complete.

## 4. Follow the Mandatory Workflow

Do not skip steps. The workflow is defined in CLAUDE.md under "Workflow". In short:

1. Create or edit the PRD/Task — mark as **In Progress**
2. `git commit`
3. Write a failing test for the work
4. Run tests (confirm failure)
5. Write the code
6. Run tests (confirm passing)
7. Mark PRD/Task as **Complete** and move to `prds/done/`
8. `git commit`

## 5. Tests Are Non-Negotiable

Every piece of new work must include tests. No exceptions. PRs without tests will be rejected.
