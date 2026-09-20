---
id: build-one-windows-exe-and-keep-integration-tags-plus-json-order
knowledge_id: build-one-windows-exe-and-keep-integration-tags-plus-json-order
title: Build one Windows exe and keep integration tags plus JSON order
status: current
category_path: decision-test/build-and-test
created_at: 2026-09-20T01:17:46.741512Z
last_updated: 2026-09-20T01:17:46.741512Z
source_event_ids:
    - E-01M2XTE7QSC9A8HPV276EME0PY
    - E-01M2XQY3463WH0WQN208XTBPZT
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
