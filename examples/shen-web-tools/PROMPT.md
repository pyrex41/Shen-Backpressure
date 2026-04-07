Research assistant where all application logic is written in Shen, running natively on Common Lisp (SBCL). Arrow.js handles the frontend. The LLM cannot cite sources it did not actually retrieve — grounding is enforced at the type level.

Stack: SBCL + Shen (application logic), Common Lisp (I/O bridge), Arrow.js (frontend), hunchentoot (HTTP server). No JavaScript frameworks.

Architecture: Shen decides WHAT to do. CL does the I/O. Arrow renders the result.

Domain entities:
- Search queries with bounded max results
- Search hits (title, url, snippet)
- Fetched pages (url, content, timestamp)
- Grounded sources (fetched page paired with its search hit, URL must match)
- AI prompts (system + user, both non-empty)
- Research summaries (require grounded sources)
- Pipeline states (idle -> searching -> fetching -> generating -> complete)

Key invariants (specs/core.shen):
- grounded-source: the URL of the fetched page must equal the URL of the search hit it came from — `(= (head Page) (head (tail Hit))) : verified`
- research-summary requires a list of grounded-source values — no ungrounded AI output
- Pipeline state machine: each stage carries the outputs of the previous stage, cannot skip steps
- search-query: max results bounded 1-20
- url: length > 8 characters

Medicare subdomain (specs/medicare.shen):
- medicare-plan-type: closed enumeration of exactly 6 valid plan type strings
- panel-kind: closed enumeration of 15 valid dashboard panel types — rejects LLM-hallucinated panel names
- grounded-layout: requires a medicare-result to construct — no rendering hallucinated data

The grounding invariant is the most important: you cannot construct a research-summary without grounded-source values, and those require URL-matched pairs of fetched pages and search hits. The LLM's output quality is enforced by the type system, not by hope.

Use /sb:ralph-scaffold to extend. The Shen application code is the novel part — start from src/app.shen.
