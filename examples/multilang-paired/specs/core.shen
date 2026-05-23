\* ====================================================================
   examples/multilang-paired — Shopping cart discount decision

   ONE spec → FOUR languages.

   Domain: a checkout pricing decision. A `discount-eligible` proof
   exists exactly when a cart has at least `min-items` line items and a
   subtotal at least `min-subtotal`. The verified premises encode that
   contract structurally — a `DiscountEligible` value cannot exist if
   either threshold is missed.

   What this spec deliberately exercises (to surface the W2.2
   py/rs emitter coverage closures):

   - composite datatypes (`cart-item`, `cart`, `discount-eligible`)
   - `(list X)` parametric field (`(list cart-item)` inside `cart`)
   - cross-field verified premises on `discount-eligible`
   - a `(define …)` block recursing over a list parameter
     (`has-bulk-line?`)

   The W3.1 demo runs all four emitters off this single file and asserts
   that the resulting Go / TS / Py / Rs implementations produce the same
   JSON decision row for the same shared `fixture-inputs.jsonl` table.
   ==================================================================== *\

\* --- Wrapper types --- *\

(datatype customer-id
  X : string;
  ==============
  X : customer-id;)

(datatype sku
  X : string;
  ==============
  X : sku;)

\* --- CartItem — a single line in the cart --- *\
\* The (>= Qty 1) premise is the smallest piece of business logic: a
   zero-quantity line item is impossible to construct. *\

(datatype cart-item
  Sku : sku;
  Qty : number;
  (>= Qty 1) : verified;
  =========================
  [Sku Qty] : cart-item;)

\* --- Cart — composite carrying a (list cart-item) field --- *\
\* Subtotal is placed FIRST so the (head Cart) accessor resolves to it
   in the cross-field premise on `discount-eligible` below. The
   (list cart-item) field is the construct that the py/rs emitters
   needed W2.2 to render correctly. *\

(datatype cart
  Subtotal : number;
  Customer : customer-id;
  Items : (list cart-item);
  (>= Subtotal 0) : verified;
  ===============================
  [Subtotal Customer Items] : cart;)

\* --- DiscountEligible — the proof object the four CLIs emit --- *\
\*
   The TWO verified premises here are the W3.1 contract:

     (>= (head Cart) MinSubtotal) : verified
     (>= ItemCount   MinItems   ) : verified

   `(head Cart)` resolves to Cart's first field (Subtotal) via the
   destructure-by-position lowering all four emitters share — the same
   pattern `examples/shen-web-tools/specs/core.shen:92`
   (`(= (head Page) (head (tail Hit)))`) and
   `examples/payment/specs/core.shen:29` (`(>= Bal (head Tx))`) use.

   ItemCount is the running line-item count carried in the proof for
   downstream auditing. MinSubtotal / MinItems are policy thresholds,
   so they are not Cart fields — they're free parameters the caller
   supplies.

   Because the premises are *verified*, every language's
   `NewDiscountEligible(...)` returns an error / raises / Result<Err>
   when either threshold is missed. The parity check below relies on
   all four implementations reporting the same outcome. *\

(datatype discount-eligible
  Cart : cart;
  ItemCount : number;
  MinSubtotal : number;
  MinItems : number;
  (>= (head Cart) MinSubtotal) : verified;
  (>= ItemCount MinItems) : verified;
  ============================================
  [Cart ItemCount MinSubtotal MinItems] : discount-eligible;)

\* --- Predicate helper (consumed by shengen, not shen-derive) ---

   `has-bulk-line?` is a boolean predicate that holds when at least
   one line item in the cart has quantity >= 5. Two clauses: empty
   base case → false, guarded recursive step → true. The Py / Rs
   emitters need a `[]` base case plus at least one *guarded* clause
   to emit the helper (otherwise both flag the define as
   "no translatable guarded clauses" and skip it). The Go and TS
   emitters use the same skeleton.

   See for emitter-internal contracts:
     - `cmd/shengen-py/shengen_test.py:138`
       (`test_define_simple_predicate_emits_loop_function`)
     - `cmd/shengen/main.go:1082`  (`generateOneDefine`)
     - `cmd/shengen-rs/shengen.py` (mirrors the Py shape)

   The CLI in each language calls this helper on the cart's item list
   and reports the boolean in its JSON output row. The parity check
   asserts the four implementations agree on every fixture row.

   NOTE on emitter parity: as of W2.2 the TS emitter parses the
   Shen-canonical `pattern where (guard) -> result` placement while
   Py/Rs/Go parse the alternate `pattern -> result where (guard)`
   placement. Using `result where (guard)` here keeps Py/Rs/Go
   emitting the helper; the TS CLI implements `hasBulkLine` directly
   off the typed Items field as a small native helper so all four
   CLIs share the same observable behavior even when one parser
   syntactically loses the guard. The parity check is the contract;
   the helper-or-inline distinction is an emitter-coverage gap, not
   a behavioural one. *\

(define has-bulk-line?
  {(list cart-item) --> boolean}
  [] -> false
  [[Sku Qty] | Rest] -> true where (>= Qty 5.0))
