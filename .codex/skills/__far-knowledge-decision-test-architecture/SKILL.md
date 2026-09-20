---
apiVersion: agent.meta/v1
kind: capability
id: __far-knowledge-decision-test-architecture
title: "Far-Knowledge: Decision-test architecture"
description: >-
  Use when changing decision-test inference, API surface, or binary packaging.
  Native llama-server only; Jev-shaped /v1/systemone; one main package.
user_visible: false
manual_only: false
status: current
body: inline
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
- Foundation choice support is 2 to 20 options. Score and noul reuse the same label readout; they are not separate inference engines.
