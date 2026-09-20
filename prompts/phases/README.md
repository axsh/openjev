# Phases

This directory organizes specifications and plans by project phase.

## Structure

```
phases/
  000-foundation/      # Phase 0: Foundation
    branches/
      000-branch-name/ # Feature Branch Name
        ideas/         # Specification documents
        plans/         # Implementation plans
    refs/              # References for this phase
  001-<next-phase>/    # Phase 1: ...
    branches/
      000-branch-name/ # Feature Branch Name
        ideas/         # Specification documents
        plans/         # Implementation plans
    refs/              # References for this phase
```

## Workflow

1. Create a specification in `<branch-name>/ideas/`
2. Review and approve the specification
3. Create an implementation plan in `<branch-name>/plans/`
4. Review and approve the plan
5. Execute the plan
