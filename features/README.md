# Features

This directory contains individual features of the project.

Each feature is an independent module with its own codebase, 
tests, and configuration.

## Adding a Feature

Use `devctl scaffold features <template>` to generate a new feature structure.

## Directory Convention

```
features/
  <feature-name>/
    cmd/           # CLI entry points
    internal/      # Internal packages
    go.mod         # Go module definition
```
