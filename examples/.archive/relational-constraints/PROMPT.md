Standalone demo of relational cross-type constraints — a verified premise that compares subfields of two different composite types.

Stack: Go stdlib. Self-contained.

Domain: Regional order pricing where the currency in an order must match the currency of its region config.

Shen spec (specs/core.shen):

```shen
(datatype currency-code
  X : string;
  (element? X ["USD" "EUR" "GBP" "JPY" "CAD"]) : verified;
  ==========================================================
  X : currency-code;)

(datatype region-config
  Region : string;
  Currency : currency-code;
  TaxRate : number;
  (>= TaxRate 0) : verified;
  (<= TaxRate 1) : verified;
  =============================
  [Region Currency TaxRate] : region-config;)

(datatype order-pricing
  Config : region-config;
  Price : number;
  Currency : currency-code;
  (= (head (tail Config)) Currency) : verified;
  (>= Price 0) : verified;
  ============================================
  [Config Price Currency] : order-pricing;)
```

The key invariant: `(= (head (tail Config)) Currency) : verified` — the currency passed to the order must be the same object as the currency embedded in the region config. `head (tail Config)` navigates the region-config composite to extract its Currency field.

Build a Go program demonstrating:
1. Construct a region-config for "US" with "USD" currency and 0.08 tax rate (succeeds)
2. Construct an order-pricing that uses the same USD currency from the region (succeeds)
3. Attempt to construct an order-pricing with a different currency ("EUR") while using the US region config (fails — currency mismatch)
4. Show how the generated Go code translates the `head (tail Config)` accessor chain into field access

This illustrates relational invariants — constraints that span multiple types — in ~25 lines of spec.

Create:
- specs/core.shen
- internal/shenguard/guards_gen.go (generated)
- cmd/demo/main.go (demonstrates correct + incorrect construction)
