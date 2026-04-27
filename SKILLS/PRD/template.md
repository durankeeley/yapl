# [Task/PRD #] - [Title/Short Description]

**Status:** `[ ] Pending` | `[ ] In Progress` | `[x] Complete`
**Priority:** [High / Medium / Low]
**Size:** [S / M / L]
**Sprint:** [Sprint Number/Name]
**Tags:** `tag1`, `tag2`
**Created:** [YYYY-MM-DD]
**GitHub Issue:** [#1](https://github.com/[org]/[repo]/issues/1)

---

## 1. Problem & Context
*What is the core issue, and why is this work necessary? Describe the current state, user impact, or technical debt.*

* **Current State:** [e.g., When a user presses Ctrl+n, a pane is created but orphaned if exited early.]
* **Impact/Need:** [e.g., The background process continues running invisibly, consuming resources with no way to close it.]

## 2. Proposed Solution
*What is the high-level approach to solving the problem?*

* [e.g., Apply Kent Beck "Tidy First" methodology specifically to YAML manifests.]
* [e.g., Create a placeholder `SessionState` immediately when a pane is created to ensure a dashboard card is always visible.]

## 3. Scope & Design Details
*Detailed breakdown of the changes, UI/UX behavior, structural clarity, or specific areas to review. (Keep fewest elements · reveal intention · no duplication).*

### [Area/Component 1]
* **Behavior/Rule:** [e.g., Placeholder sessions use a "No agent" status label and distinct border color.]
* **Specifics:** [e.g., Hardcoded secrets in `mgmt-api.yaml` — add banner comment for operators.]

### [Area/Component 2]
* **Behavior/Rule:** [e.g., Session Transition logic.]
* **Specifics:** [e.g., When a `SessionStart` event arrives, transition the placeholder into a real session seamlessly.]

## 4. Execution & Milestones
*Actionable checklist for the implementation phase.*

- [ ] Placeholder state created at generation time.
- [ ] Dashboard card rendered with distinct UI for placeholders.
- [ ] Seamless transition from placeholder to real session.
- [ ] Verify standard normalisation across labels (e.g., `app.kubernetes.io/managed-by`).

## 5. Technical Notes

### Key Files & Systems Targeted
* `path/to/file1.ext` — [Brief note on what changes here, e.g., `SessionState` struct]
* `path/to/file2.ext` — [Brief note on what changes here, e.g., `Ctrl+w` handler]

### Risks & Considerations
* **[Risk 1]:** [e.g., Session ID collision: The synthetic `pane-{id}` must not collide with UUIDs.]
* **[Risk 2]:** [e.g., Ensure any new shell logic used in cronjobs is POSIX compliant and not GNU-specific.]
