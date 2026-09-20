---
id: decision-test-uses-llama-server-behind-a-jev-shaped-single-binary
knowledge_id: decision-test-uses-llama-server-behind-a-jev-shaped-single-binary
title: Decision-test uses llama-server behind a Jev-shaped single binary
status: current
category_path: decision-test/architecture
created_at: 2026-09-20T01:17:46.5757873Z
last_updated: 2026-09-20T01:17:46.5757873Z
source_event_ids:
    - E-01M2XPV01S90PK332Q0VTWYNBM
---

# Decision-test architecture

## Inference backend
- Call native llama-server over HTTP. Do not use wllama: wllama v3 is a WASM build of llama-server server-context and has no Go host runtime.
- Keep the llama HTTP client behind an Engine interface limited to label logprob readout, constrained generation, and health.

## Public API shape
- Expose Jev-shaped `POST /v1/systemone`. Map openjev flat state, question, and options into `questions`; that flat form is not the wire format.
- Ship CLI and API server from one `main` package so `scripts/process/build.sh` can emit a single binary.

## Direct readout
- Direct probabilities are a softmax over option-label logprobs only. llama-server pre-sampling `top_logprobs` ignore `logit_bias`, so bias is for sampling constraint, not probability shaping.

## Phase scope
- Foundation choice support is 2 to 20 options. Score and noul were added later by reusing the same label readout; they are not separate inference engines.
