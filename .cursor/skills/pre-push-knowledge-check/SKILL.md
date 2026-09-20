---
name: pre-push-knowledge-check
description: Before git push, inspect all committed changes and determine whether they contain far-knowledge that should be recorded via record-far-knowledge skill.
disable-model-invocation: false
---


# Pre-Push Knowledge Check

You are an AI coding agent working in this repository.

Before running `git push`, you **MUST** inspect the changes being pushed and decide
whether they contain far-knowledge that should be recorded for future maintainers.

Far-knowledge is knowledge that cannot be discovered by searching nearby code
(same package, imports, callers). Your goal is to capture knowledge that will
help a future developer understand:

- Why the system has this structure
- Where responsibilities and boundaries are
- What constraints must not be broken
- What design decisions were made
- What cross-cutting patterns should be followed
- What conventions and style rules exist
- What lessons were learned from past mistakes
- What engineer preferences guide quality standards

## Step 1: Inspect the Changes Being Pushed

Run the following commands to understand what will be pushed:

```bash
git log --oneline origin/HEAD..HEAD
git diff origin/HEAD..HEAD --stat
git diff origin/HEAD..HEAD
```

If there are no unpushed commits, report the following and stop:

```text
Far-knowledge intake: no unpushed commits found. Skipping.
```

## Step 2: Determine Whether a Knowledge Note Is Needed

Create a knowledge note **only when** the changes being pushed include
at least one of the following signals:

### Architecture Signals
1. A new module, package, directory, service, command, API, component was added.
2. Responsibilities were moved, split, merged, or clarified.
3. A boundary was introduced or changed.
4. Dependency direction changed or should be protected.
5. State ownership changed.
6. Data ownership, lifecycle, source of truth, or consistency rule changed.
7. A non-obvious design decision was made.
8. A rejected alternative is visible or can be inferred.
9. A temporary implementation, shortcut, or technical debt was introduced.
10. An invariant or rule must remain true for the system to work.
11. Error handling, retry, timeout, transaction behavior changed.
12. Authentication, authorization, permission, or trust boundary changed.
13. The change affects future extensibility or automation flow.
14. The change would be hard to understand later by reading only the code.

### Cross-Cutting Signals
15. A design pattern was applied that should be shared across modules.
16. A convention or style rule was established or clarified.
17. A lesson was learned from a past failure or code review feedback.
18. An engineer preference or quality standard was communicated.

If **none** of these apply, report the following and stop:

```text
Far-knowledge intake: no far-knowledge detected.
Reason: <brief reason>
```

## Step 3: What NOT to Record

Do **not** create knowledge notes for:

- Trivial refactoring
- Formatting-only changes
- Local implementation details obvious from nearby code
- Bug fixes that do not change assumptions
- Comments that merely repeat the code
- Temporary debug code
- Mechanical renames with no structural meaning

Knowledge notes must not become noisy commit summaries.

## Step 4: Record via record-far-knowledge

If knowledge signals were detected in Step 2,
use the **record-far-knowledge** skill to record the knowledge.

Read the `record-far-knowledge` capability for full details on:

- Required command invocation and flags
- Category flag selection (architecture + far-knowledge flags)
- Distance judgment guidelines
- How to write good notes
- Dry-run for uncertain cases
- Report format

## Step 5: Final Check Before Push

Before allowing `git push` to proceed, confirm:

1. All unpushed commits were inspected.
2. Knowledge signals were evaluated against the checklist.
3. record-far-knowledge was invoked if signals were detected.
4. Report was produced (either "recorded" or "no update").

## Interaction Rules

- Do not ask the user whether to create a note unless the change is ambiguous.
- Prefer recording with a `PROVISIONAL` prefix when the signal is important
  but partially inferred.
- Do not modify production code as part of this knowledge intake.
