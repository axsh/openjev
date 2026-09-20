---
apiVersion: agent.meta/v1
kind: capability
id: __far-knowledge-decision-test-build-and-test
title: "Far-Knowledge: Decision-test build and test"
description: >-
  Use when changing decision-test build scripts, Windows binary naming,
  integration tags, or JSON criteria order.
user_visible: false
manual_only: false
status: current
body: inline
---

# Decision-test build and integration conventions

## Build
- `scripts/process/build.sh` must compile the single main package path. Do not use `go build -o file ./...`; Go refuses to write multiple packages into one output file.
- On Windows, name the binary with a `.exe` suffix. `CreateProcess` will not run an extensionless path even when `os.Stat` succeeds.

## Integration tests
- `scripts/process/integration_test.sh` must pass `-tags integration`, or files with `//go:build integration` never run.
- Load-matrix runs can exceed Go's default 10-minute test timeout; the runner uses `-timeout 45m` for that reason.

## JSON key order
- Choice criteria and question objects must preserve JSON key order with a custom unmarshaler. Label letters and tie-breaks follow appearance order, and `encoding/json` maps discard it.
