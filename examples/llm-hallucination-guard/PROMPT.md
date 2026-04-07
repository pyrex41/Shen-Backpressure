Demo of closed enumerations as LLM hallucination prevention. An LLM generates structured JSON describing a dashboard layout. Each panel must have a valid "kind" field from a fixed set. Hallucinated kinds are rejected by the guard type constructor.

Stack: Go stdlib net/http. No frameworks.

Scenario: An LLM generates JSON describing a dashboard layout. The spec defines exactly which panel kinds are valid.

Shen spec (specs/core.shen):

```shen
(datatype panel-kind
  X : string;
  (element? X ["bar-chart" "line-chart" "pie-chart" "table" "metric-card"
               "scatter-plot" "heatmap" "timeline" "map" "text-block"]) : verified;
  =================================================================================
  X : panel-kind;)

(datatype dashboard-panel
  Kind : panel-kind;
  Title : string;
  DataSource : string;
  (not (= Title "")) : verified;
  (not (= DataSource "")) : verified;
  ====================================
  [Kind Title DataSource] : dashboard-panel;)

(datatype dashboard-layout
  Panels : (list dashboard-panel);
  Title : string;
  (not (= Title "")) : verified;
  ================================
  [Panels Title] : dashboard-layout;)
```

Build a Go service that:
1. Accepts JSON from an LLM (simulated) describing a dashboard layout
2. Parses JSON into raw structs, then validates each panel through the guard type constructors
3. Valid panels pass through; hallucinated panel kinds (e.g., "gauge", "sparkline", "3d-globe") are rejected with clear error messages
4. Empty titles and empty data sources are also rejected
5. Returns either a validated DashboardLayout or structured errors

Demonstrate:
- Valid LLM output (all panel kinds in the allowed set) -> success
- LLM output with hallucinated panel kind "gauge" -> rejection with message "gauge is not a valid panel-kind"
- LLM output with empty title -> rejection
- Show the error messages that would be fed back to the LLM in a Ralph loop

This is the "backpressure on LLM output" story made concrete.
Target: ~30 lines of spec, ~80 lines of Go, clear before/after of valid vs invalid LLM output.
