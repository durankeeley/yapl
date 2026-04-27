#!/usr/bin/env bash
set -euo pipefail

# Create a git worktree for Task or PRD work
# Usage: create-worktree.sh <branch-name>
# Examples: 
#   create-worktree.sh prd-54-dashboard-card-every-pane
#   create-worktree.sh task-98-deep-tidy-k8s-manifests

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <branch-name>" >&2
    echo "Example: $0 task-98-deep-tidy-k8s-manifests" >&2
    exit 1
fi

branch_name="$1"
repo_name=$(basename "$(git rev-parse --show-toplevel)")
worktree_path="../${repo_name}-${branch_name}"

# Check if branch already exists
if git show-ref --verify --quiet "refs/heads/${branch_name}"; then
    echo "Error: Branch '${branch_name}' already exists" >&2
    git branch --list "${branch_name}"
    exit 1
fi

# Check if worktree path already exists
if [[ -d "${worktree_path}" ]]; then
    echo "Error: Worktree path '${worktree_path}' already exists" >&2
    exit 1
fi

# Check if worktree is already registered
if git worktree list | grep -q "${branch_name}"; then
    echo "Error: Worktree for '${branch_name}' already exists" >&2
    git worktree list | grep "${branch_name}"
    exit 1
fi

# Create the worktree
git worktree add "${worktree_path}" -b "${branch_name}" main

# Initialize submodules in the new worktree
(cd "${worktree_path}" && git submodule update --init --recursive)

echo ""
echo "Worktree created successfully:"
echo "  Path:   ${worktree_path}"
echo "  Branch: ${branch_name}"
echo ""
echo "To start working:"
echo "  cd ${worktree_path}"
