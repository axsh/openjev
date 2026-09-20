
## Mandatory Record After Far-Knowledge Changes

When you complete a coherent task boundary (e.g. after `git commit`, before `git push`),
you **MUST** invoke the **record-far-knowledge** skill if any of the following occurred:

### Architecture Signals
- A new Go package (`internal/`, `pkg/`, `cmd/`) was added or removed
- A new CLI subcommand or API endpoint was introduced
- Data models or database schemas were added or modified
- Module boundaries or dependency relationships changed
- New wrapper scripts (`scripts/`) were created
- Agent-facing configuration or workflow files were changed
- A design decision was made that future agents should know about

### Cross-Cutting Signals
- A design pattern was applied that should be shared across modules
- A convention or style rule was established or clarified
- A lesson was learned from a past failure or code review feedback
- An engineer preference or quality standard was communicated

Do **NOT** skip this step. Even if you are unsure whether the change qualifies,
run record with `--dry-run` first to inspect the payload.

### Execution Timing

Run record **once per coherent task boundary**. Typical timing:
1. After completing a logical unit of work and running `git commit`
2. Before running `git push`
3. If multiple commits form one logical change, record once after the last commit

### How to Record

Use the **record-far-knowledge** skill for full details on:

- Required command invocation (`./scripts/code/agent/record.sh`)
- Required and category flags (architecture + far-knowledge flags)
- Distance judgment guidelines
- How to write good notes
- Dry-run for uncertain cases
- Command examples and report format

## General Rules

Use record only to store long-term memory candidates.
Do not edit canonical memory documents for intake.
Do not run `./scripts/code/prompt/compile.sh`, `./scripts/code/prompt/deploy.sh`, or `./scripts/code/prompt/update.sh`
unless the user explicitly asks for consolidation or deployment.
