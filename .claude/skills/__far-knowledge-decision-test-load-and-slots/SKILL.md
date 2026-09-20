---
apiVersion: agent.meta/v1
kind: capability
id: __far-knowledge-decision-test-load-and-slots
title: "Far-Knowledge: Decision load and llama slots"
description: >-
  Use when changing load measurement, llama_parallel, run_llama_server
  --parallel, or concurrency versus slot comparisons.
user_visible: false
manual_only: false
status: current
body: inline
---

# Decision load, slots, and concurrency

## Current measurement rule
- With `-c 2048` and no `--kv-unified`, an explicit `-np` splits context per slot. Keep `-c` fixed and do not enable `--kv-unified` when comparing slot counts.
- Engine semaphore capacity must equal `llama_parallel`. Default remains 1 so callers that omit the setting stay serial.
- The `load` CLI `--slots` flag is a report label only; it does not reconfigure a running llama-server. Restart llama with `--parallel N` and set `llama_parallel: N` together when comparing slot sizes.

## Acceptance behavior
- llama.cpp b11056 defers work when every slot is busy instead of returning "slot unavailable". The deferred queue has no length cap.
- llama-server HTTP workers are capped at the base thread count plus 1024 dynamic threads, not by CPU or GPU utilization.
- Huma `http.Server` accepts without a concurrency cap. Inference concurrency is the engine semaphore. Health stays outside that semaphore.

## Prompt fit and VRAM
- The account.json direct prompt is about 109 tokens. Per-slot context after llama.cpp 256-padding still fits for slots 8 and 16 under `-c 2048`.
- Powers of two that divide 2048 (1, 2, 4, 8) keep total KV near 2048. Larger or non-dividing `-np` can pad `n_ctx_seq` to 256 and inflate total context (VRAM). On an 8 GB laptop GPU, exploratory ceiling is about 16; 32 is likely unsafe.
- Measured sweet spot for account.json direct on RTX 3070 Laptop 8GB: about slots 5 and concurrency 20 (peak rps near slots 5 / concurrency 40-50 with much higher per-request latency).
