---
apiVersion: agent.meta/v1
kind: capability
id: record-far-knowledge
title: Record Far-Knowledge via record.sh
description: >-
  Shared skill for recording far-knowledge (cross-cutting knowledge not
  discoverable from nearby code) into the project memory system using
  ./scripts/code/agent/record.sh.
  Referenced by far-knowledge-memory policy and pre-push-knowledge-check capability.
bundle:
  - src: prompts/memory/schemas/agent-record-payload.schema.json
    dest: references/agent-record-payload.schema.json
scripts:
  - "scripts/code/agent/record.sh"
manual_only: true
body: inline
tags:
  - baseline

---

# Record Far-Knowledge via record.sh

This skill describes how to record far-knowledge into the project's
memory system using `./scripts/code/agent/record.sh`.

Payload schema: `references/agent-record-payload.schema.json`

Far-knowledge is knowledge that cannot be discovered by searching nearby
code (same package, imports, callers). It includes architecture decisions,
cross-cutting design patterns, conventions, lessons learned, and engineer
preferences.

This is a shared reference. It is invoked by:

- **far-knowledge-memory** policy: when architecture-impacting or
  cross-cutting changes are detected during or after implementation
- **pre-push-knowledge-check** capability: when inspecting changes
  before `git push`

## Recording Tool

This project uses `./scripts/code/agent/record.sh` to record
far-knowledge into the project's memory system.

You **MUST** use record.sh. Do **NOT** manually create or edit files
under `prompts/memory/`.

## Required Invocation

```bash
./scripts/code/agent/record.sh \
  --agent "<your-agent-name>" \
  --summary "<one-line description of the knowledge>" \
  --changed-paths-from-git \
  <category-flags> \
  --note "<note 1>" \
  --note "<note 2>" \
  ...
```

### Required Flags

Every invocation **MUST** include:

- `--agent "<your-agent-name>"` : Your agent name (one of: `antigravity`, `claude-code`, `codex`, `cursor`)
- `--summary "..."` : A one-line description of what changed
- `--changed-paths-from-git` : Automatically detects changed files from git
- At least one category flag (see below)

### Category Flags (pick all that apply)

| Flag | When to use |
|------|-------------|
| `--architecture-impact` | New/removed packages, changed module boundaries, new CLI commands, new API endpoints |
| `--memory-related` | Changes to memory system itself (`prompts/memory/`, `scripts/code/agent/`) |
| `--prompt-related` | Changes to prompt templates (`prompts/manifest/`) |
| `--agent-behavior-related` | Changes to agent rules, workflows, or skills (`.agents/`) |
| `--design-pattern` | Cross-cutting design patterns shared across modules |
| `--convention` | Conventions and style rules (log format, DB design, API design, comment style) |
| `--lesson-learned` | Lessons from past failures or review feedback |
| `--preference` | Engineer preferences or quality standards |

### Optional Flags

| Flag | Purpose |
|------|---------| 
| `--changed-path "<path>"` | Manually specify a changed path (repeatable) |
| `--dry-run` | Preview without recording |
| `--print-payload` | Print the JSON payload (useful with `--dry-run`) |

## Distance Judgment Guidelines

Use these criteria to decide whether to record:

1. **Can this knowledge be found by searching nearby code?** (same package, imports)
   -> If yes, skip recording.
2. **Would this knowledge help when developing an unrelated module?**
   -> If yes, record with `--design-pattern`.
3. **Is this knowledge based on past decisions/feedback and not readable from code?**
   -> If yes, record with `--lesson-learned`.
4. **Is this knowledge about engineer preferences or quality standards?**
   -> If yes, record with `--preference`.

## Writing Good Notes

Each `--note` value MUST capture a **single knowledge proposition**.
Focus on **why**, **what boundary**, **what constraint**, or **what decision** --
not on **what code was written**.

### Good Notes

```text
--note "API handlers must not access the database directly; all DB access goes through usecase services"
--note "Error responses in API handlers use pkg/apierror types; internal errors are logged but clients receive generic messages"
--note "Integration test names follow TestXxx_Scenario format to allow --specify filtering"
--note "Prefer explicit error wrapping with fmt.Errorf and %w over bare error returns for traceability"
```

### Bad Notes

```text
--note "Added a new function to handle tasks"
--note "Updated some files for better architecture"
--note "Pipeline: A -> B -> C -> D"
```

Each note MUST be a single proposition. Do not pack multiple concepts into one note.

## Note Content Guidelines

When writing notes, prioritize these types of knowledge:

| Type | What to capture |
|------|----------------|
| **Component responsibility** | What this part of the system is responsible for, and what it intentionally excludes |
| **Boundary** | Which layer may call which; which component owns which data |
| **Invariant** | Conditions that must remain true |
| **Design decision** | The selected design and why it was chosen |
| **Rejected alternative** | What was considered but not chosen, and why |
| **Cross-cutting pattern** | Patterns applicable across multiple modules |
| **Convention** | Naming, formatting, or structural rules |
| **Lesson learned** | Past mistakes or review feedback |
| **Risk / temporary** | Provisional implementations or tech debt |

## Dry-Run for Uncertain Cases

If you are unsure whether the change qualifies, run a dry-run first:

```bash
./scripts/code/agent/record.sh \
  --agent "<your-agent-name>" \
  --summary "<description>" \
  --changed-paths-from-git \
  --design-pattern \
  --note "<note>" \
  --dry-run --print-payload
```

## Report Format

After running record.sh, include the following in your log output:

```text
Far-knowledge intake: recorded.
Summary: <summary passed to record.sh>
Notes:
- <note 1>
- <note 2>
Flags: <flags used>
```

If no recording was made:

```text
Far-knowledge intake: no update.
Reason: <brief reason>
```

## User Report

After this skill completes, you **MUST** report the outcome to the user
with an explanation. Do not simply state the result -- explain the
reasoning so the user understands what happened and why.

### When knowledge was recorded

Explain to the user:
- **What knowledge was captured**: Describe the key insight, decision,
  or pattern that was recorded, in plain language the user can understand
  without reading the raw notes.
- **Why it qualifies as far-knowledge**: Briefly explain why this
  knowledge cannot be discovered from nearby code and how it would
  benefit future work in other parts of the codebase.

### When no knowledge was recorded

Explain to the user:
- **Why you decided not to record**: Describe which of the distance
  judgment criteria (see "Distance Judgment Guidelines" above) you
  evaluated and why none were met. Be concrete -- for example: "The
  change is localized to a single package and can be discovered from its
  direct imports, so it does not meet the far-knowledge threshold."
- **What would make it recordable**: If applicable, briefly mention what
  kind of additional context or scope would elevate the work to
  far-knowledge (e.g., "If this pattern were adopted across multiple
  modules, it would qualify as a cross-cutting design pattern.").

## Constraints

- Do not modify production code as part of knowledge intake.
- Do not create or edit files under `prompts/memory/` directly.
- Do not run `./scripts/code/prompt/compile.sh`, `./scripts/code/prompt/deploy.sh`,
  or `./scripts/code/prompt/update.sh` unless the user explicitly asks.
- Use record only to store long-term memory candidates.
- Do not edit canonical memory documents for intake.
