Standalone demo of a pipeline state machine where each stage is a separate type carrying the previous stage's output. Skipping a stage is a compile error.

Stack: Go stdlib. No frameworks. Self-contained.

Show a simplified 3-stage pipeline (search -> fetch -> summarize) in Go where:
1. Each stage is a separate type carrying the previous stage's output
2. Attempting to skip a stage is a compile error (show the error message)
3. The correct path compiles and runs

Shen spec (specs/core.shen):

```shen
(datatype search-result
  Query : string;
  Hits : (list string);
  (not (= Query "")) : verified;
  ================================
  [Query Hits] : search-result;)

(datatype fetched-content
  Search : search-result;
  Pages : (list string);
  ========================
  [Search Pages] : fetched-content;)

(datatype summary
  Content : fetched-content;
  Text : string;
  (not (= Text "")) : verified;
  ==============================
  [Content Text] : summary;)
```

The key insight: `fetched-content` requires a `search-result`. `summary` requires a `fetched-content`. You cannot summarize without fetching, and you cannot fetch without searching. The state machine is the type system.

Create:
- specs/core.shen (the spec above, refined as needed)
- internal/shenguard/guards_gen.go (generated via shengen)
- cmd/demo/main.go — demonstrates the correct pipeline AND shows the compile error for skipping

Target: ~30 lines of spec, ~80 lines of application code, clear compile error demonstration.
