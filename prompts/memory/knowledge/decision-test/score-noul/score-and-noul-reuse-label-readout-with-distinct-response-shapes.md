---
id: score-and-noul-reuse-label-readout-with-distinct-response-shapes
knowledge_id: score-and-noul-reuse-label-readout-with-distinct-response-shapes
title: Score and noul reuse label readout with distinct response shapes
status: current
category_path: decision-test/score-noul
created_at: 2026-09-20T01:17:46.887643Z
last_updated: 2026-09-20T01:17:46.887643Z
source_event_ids:
    - E-01M2XWET6J21KTXRDG93YNV40C
---

# Score and noul reuse the choice label path

## Shared readout
- Score and noul are not new model calls. Score levels `0..n-1` map onto labels A onward. Noul maps true to A and false to B regardless of JSON key order.

## Score semantics
- A score answer is the probability-weighted average of level indexes. Do not round to the nearest level or emit argmax as the score.

## Noul semantics
- A noul answer is only P(true). Omit confidence, top-level probabilities, and any boolean threshold from the noul response.
