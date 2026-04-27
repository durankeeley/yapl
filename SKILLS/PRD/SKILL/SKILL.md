---
name: worktree-task-prd
description: Create a git worktree for Task or PRD work with a descriptive branch name. Infers ID from context or asks user.
---

# Create Git Worktree for Task/PRD

Create a git worktree with a descriptive branch name based on the Task or PRD title. This ensures feature branches have human-readable names that describe what the work is about, accommodating both large features (PRDs) and smaller technical tasks.

## Workflow

### Step 1: Identify the Task or PRD

Try to infer the Task or PRD number from the current conversation. Look for references like "PRD 54", "PRD #54", "Task 98", "prd-54", or "task-98".

If not found in context, ask the user: "Which Task or PRD should I create a worktree for? (e.g., Task 98 or PRD 54)"

### Step 2: Get the Title

If the file content is already in the conversation context, extract the title from the primary header.

Otherwise, read the corresponding file. Files are typically stored in directories like `prds/` or `tasks/` (or a unified directory depending on the repository structure) with a naming pattern including the number:
```bash
find . -type f \( -path "*/prds/*" -o -path "*/tasks/*" \) | grep "[0-9]"
```

The title is on the first line in the unified format: 
`# [Task/PRD Number] - [Title/Short Description]`
*(Note: Fallback to checking for the em-dash format `# PRD #[number]: [Title]` if an dash `—` is not present).*

### Step 3: Generate Descriptive Branch Name

Convert the Task/PRD title to a branch-friendly name:
1. Identify the type and prefix accordingly: `task-[number]-` or `prd-[number]-`
2. Extract the title after the dash (`-`) or colon (`:`)
3. Convert to lowercase
4. Replace spaces with hyphens
5. Remove special characters except hyphens and dots
6. Keep it concise (truncate if very long, ideally under 50 characters)

**Examples:**
- "# Task 98 — Deep tidy of all Kubernetes manifests" → `task-98-deep-tidy-k8s-manifests`
- "# PRD #54: Always Create Dashboard Card for Every Pane" → `prd-54-dashboard-card-every-pane`
- "# PRD 290 — Skills Distribution System" → `prd-290-skills-distribution`

### Step 4: Create the Worktree

Run the `create-worktree.sh` script from this skill's directory:
```bash
.claude/skills/worktree-prd/create-worktree.sh [branch-name]
```

The script will:
- Check if the branch or worktree already exists (exits with error if so)
- Get the repository name dynamically
- Create the worktree at `../${repo_name}-${branch-name}`
- Initialize submodules in the new worktree
- Output the path and instructions for the user

If the script fails due to an existing branch/worktree, inform the user and ask how to proceed.

## Guidelines

- **Descriptive names**: Branch names should describe the work, not just contain the Task/PRD number.
- **Consistent format**: Always prefix the worktree directory with the repository name.
- **Base on main**: Always branch from `main` for new feature/task work unless specified otherwise.
- **Clean names**: Keep branch names concise but descriptive, stripping unnecessary filler words (e.g., "update-to", "fix-for").
```
