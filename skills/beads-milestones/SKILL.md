---
name: beads-milestones
description: Organize and execute coding work using Beads milestones, child issues, and the bv milestone tree. Use when asked to plan milestones, group existing issues, work through a tracked milestone, or keep Beads progress current.
---

# Beads milestones

Maintain a useful shared work tree: the agent updates structured tracker data,
and the user watches the same work in `bv`. Apply this workflow when Beads
tracking is requested or established by the repository, rather than creating
a tracker for every coding question.

## Representation and tools

- A milestone is an ordinary **epic**, conventionally labeled `milestone`.
  This fork treats all epics, and any issue with that label, as milestones.
- Membership is an explicit **parent-child** relationship. Labels, title
  prefixes, and dotted issue IDs alone do not establish membership.
- Nest milestones under a project epic when helpful. Give each milestone an
  observable outcome and completion criteria; create actionable child tasks.
- Use the repository's selected `br` or `bd` CLI for writes. Do not edit the
  database or JSONL export directly, or run both trackers against one workspace.
- Run commands from the project owning `.beads`, following its `AGENTS.md` and
  configuration. An available executable or the JSONL filename alone does not
  identify the tracker. Check the installed CLI's help for supported flags.
- For agent inspection use `--json` or `bv --robot-*`. Bare `bv` opens the
  human interface and blocks an unattended agent session.

## Inspect before changing the plan

The commands below use `br`, the tracker installed on this machine:

```bash
br list --type epic --all --limit 0 --json
br epic status --json
br show "$milestone_id" --json
br ready --json
```

Use real returned IDs and reuse matching milestones instead of duplicating
them. `br show` returns an array: its `dependents` entries with
`dependency_type == "parent-child"` are immediate children. Blocking dependents
are a different relationship. Inspect nested milestones recursively as needed.
Include closed issues when reporting completion.

Optional work selection:

```bash
bv --robot-next
bv --robot-triage
```

Keep the user's selected milestone and scope. A global recommendation does not
override them. Recheck the current issue, prerequisites, and assignee before
claiming work. Robot output is a recommendation, not a reservation. Workspace
display IDs may be namespaced: use the returned live action's local ID and
working directory. Historical/imported views are not mutation routes.

## Create or organize milestones

When planning is authorized, use concrete outcome titles and completion criteria.
For example:

```bash
milestone_id="$(br create --title 'M1: Publish the compiler' \
  --type epic --labels milestone \
  --description 'Done when compilation succeeds on the agreed eval set and the integration tests pass.' \
  --silent)"

task_id="$(br create --title 'Implement compiler entry point' \
  --type task --parent "$milestone_id" --silent)"
```

To create a nested milestone, also pass `--parent "$project_id"` when creating
its epic. Use IDs returned by the tracker; do not manufacture dotted IDs.

To move an existing issue into a milestone, first inspect its current parent,
then use `br update "$task_id" --parent "$milestone_id" --json`. This replaces
its previous parent, so do it when that reorganization is intended. Preserve
unrelated labels, descriptions, and blocking dependencies.

A prerequisite is separate from hierarchy. For an actual ordering constraint:

```bash
br dep add "$later_task_id" "$prerequisite_id" --type blocks --json
br dep cycles --json
```

Here the first issue waits for the second. Do not add blocking edges merely to
make the tree look nested, or make every task wait for its own milestone to close.

## Work and keep status honest

1. Inspect the selected task with `br show`; check readiness and existing
   ownership. Claim only the work being started with
   `br update "$task_id" --claim --json`. If another agent owns it or the claim
   fails, inspect current state rather than forcing or stealing the claim.
2. Implement and validate the task. Use `br comments add "$task_id" --message
   "$summary" --json` to record actual progress, validation results, commit
   references, and remaining blockers. Set `summary` to the real evidence.
3. Close completed work with `br close "$task_id" --reason "$completion_reason"
   --json`. Set the reason to what was delivered and verified. Leave unfinished
   work open/in progress; represent genuine prerequisites as dependencies.
4. After writes, run `br sync --flush-only` to refresh the export read by `bv`.
   Check failures rather than assuming the viewer received the update.
5. Close a milestone only when its descendants and its stated completion criteria
   are satisfied. The tree counts **closed descendant leaf issues**, excludes
   milestone containers, and does not automatically close an epic at 100%.

For status requests, report milestone outcomes, completed/total leaf work,
active issues, and concrete blockers. Link evidence where available. Tracker
epic rollups may use different counting rules from the viewer; label counts
accurately instead of presenting those rollups as the viewer's leaf totals.

## If the project uses bd

Follow its installed help for `create --type epic --labels milestone --parent`,
`update --parent`, `ready`, `show`, `update --claim`, and `close` equivalents.
Refresh the viewer's compatibility export with
`bd export -o .beads/issues.jsonl` (or the export path configured for this project).
Do not initialize `br` as a replacement for an existing `bd` workspace.

## Human handoff

The milestone fork opens the tree with `bv`; `bv --classic` opens the original
interface. Keys: `m` new milestone, `M` milestone under the selected item, `n`
child issue, arrows navigate/fold, Space toggles, `o`/`O` expand/collapse all,
Enter details, Esc back, `r` refresh, `q` quit. Agent changes appear automatically
within about two seconds after the export is refreshed. Branch folds persist.

For a bounded terminal preview, use
`bv --debug-render milestones --debug-width 110 --debug-height 42`.
Explicit recipes and multi-repository workspaces retain the original interface.
