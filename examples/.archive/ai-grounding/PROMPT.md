# AI Output Grounding — Anti-Hallucination Types

Makes hallucination a **type error**. Ungrounded AI output cannot reach users.

## Proof Chain

```
source-url ─► fetched-document (URL + content + timestamp)
                    │
              grounded-citation (claim + source proof)
                    │
              grounded-summary (output + sources, count > 0)
                    │
              calibrated-confidence (score ~ source count)
                    │
              safe-output (grounded + calibrated)

fetched-document ─► relevant-retrieval (score ≥ threshold)
                          │
                    rag-pipeline-output (retrieval + safe output)
```

## Key Invariants

- Cannot render AI output without source documents
- Cannot cite a document that wasn't fetched
- Confidence must be proportional to evidence
- Tool calls require stated justification + user intent
- Retrieved chunks must meet relevance threshold

## Applicable To

- RAG systems (retrieval-augmented generation)
- AI agents with tool use
- Research assistants
- Any system where AI output faces users

Extends the pattern from `demo/shen-web-tools/` to a general framework.

See `specs/core.shen` for the full specification.
