---
apiVersion: agent.meta/v1
kind: capability
id: __far-knowledge-decision-test-score-noul
title: "Far-Knowledge: Score and noul semantics"
description: >-
  Use when changing score or noul request validation, aggregation, or response
  JSON shape in decision-test.
user_visible: false
manual_only: false
status: current
body: inline
---

# Score and noul reuse the choice label path

## Shared readout
- Score and noul are not new model calls. Score levels `0..n-1` map onto labels A onward. Noul maps true to A and false to B regardless of JSON key order.

## Score semantics
- A score answer is the probability-weighted average of level indexes. Do not round to the nearest level or emit argmax as the score.

## Noul semantics
- A noul answer is only P(true). Omit confidence, top-level probabilities, and any boolean threshold from the noul response.
