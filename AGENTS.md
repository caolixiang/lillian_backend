# AGENTS.md

## Scope

- This file applies to the repository root and all subdirectories.

<!-- my-coding-memo-skill:start -->
## Coding Memo Workflow

### Instruction Priority And Exceptions

1. Treat platform and runtime constraints as higher priority than this workflow.
2. Treat direct developer instructions as higher priority than this workflow.
3. If a higher-priority instruction conflicts with this workflow, call out the conflict before proceeding.
4. Record every approved exception in today's `docs/plan/YYYYMMDD.md` under `## Exception Log`.

### Workflow Files

- Store plan files at `docs/plan/YYYYMMDD.md`.
- Store memo files at `docs/memo/YYYYMMDD.md`.
- Keep reusable templates at `docs/plan/TEMPLATE.md` and `docs/memo/TEMPLATE.md`.
- Use the user's timezone for every `YYYYMMDD` value unless the user explicitly requests a different timezone.

### Daily Operating Rules

1. Read the latest plan in `docs/plan/` and the latest memo in `docs/memo/` before resuming work, especially after context compression or a new conversation.
2. Create today's plan and memo files before implementation when they do not already exist.
3. Execute work in numbered phases, even if the task only needs one phase.
4. Keep each phase scoped, implemented, accepted, and logged separately.
5. Create one code commit after each completed phase unless the user explicitly says not to commit.
6. Update today's plan and memo after each phase commit with the commit hash and summary, but do not create a same-day follow-up commit just for those doc updates.
7. On the next workday, create a docs-only rollover commit for the previous day's `docs/plan/YYYYMMDD.md` and `docs/memo/YYYYMMDD.md` if they still have uncommitted changes.
8. Record tests, exceptions, and follow-up work in today's plan.

### Plan Requirements

The daily plan file must include:

- `# Today's Goals`
- `## Scope`
- `## Phase Plan`
- `## Phase Acceptance Log`
- `## Test Log`
- `## Commit Log`
- `## Exception Log`
- `## Tomorrow Plan`

### Memo Requirements

The daily memo file must include:

- `# YYYYMMDD Memo`
- `## Items`
- `## Commits`
- `## Notes`

### Commit Rules

1. Finish phase self-checks before creating the phase code commit.
2. Exclude today's `docs/plan/YYYYMMDD.md` and `docs/memo/YYYYMMDD.md` from same-day phase commits.
3. Keep one code commit scoped to one phase unless the user explicitly requests otherwise.
4. If the user explicitly says not to commit, keep the phase and validation records anyway.

### Test Logging

1. Record each validation command and result in the daily plan file.
2. If a test is skipped, record the reason in the same file.
<!-- my-coding-memo-skill:end -->
