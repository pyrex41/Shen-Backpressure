\* ====================================================================
   specs/datatypes-only.shen — the same datatype block from core.shen
   with the (define has-bulk-line?) block stripped. Used only by the
   `shen-check-datatypes` gate so the SHEN tc+ port-validation surface
   stays exercised without tripping the same list-destructure +
   arithmetic-guard limitation that takes down
   `examples/payment/specs/core.shen`'s `processable` define.

   This file MUST be kept in sync with `specs/core.shen` by hand. The
   `tcb-audit-*` gates audit the codegen path (specs/core.shen → four
   `guards_gen.*` files), not this auxiliary spec. The
   datatypes-only spec is here for Shen tc+ smoke coverage only.
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

\* --- CartItem --- *\

(datatype cart-item
  Sku : sku;
  Qty : number;
  (>= Qty 1) : verified;
  =========================
  [Sku Qty] : cart-item;)

\* --- Cart --- *\

(datatype cart
  Subtotal : number;
  Customer : customer-id;
  Items : (list cart-item);
  (>= Subtotal 0) : verified;
  ===============================
  [Subtotal Customer Items] : cart;)

\* --- DiscountEligible --- *\

(datatype discount-eligible
  Cart : cart;
  ItemCount : number;
  MinSubtotal : number;
  MinItems : number;
  (>= (head Cart) MinSubtotal) : verified;
  (>= ItemCount MinItems) : verified;
  ============================================
  [Cart ItemCount MinSubtotal MinItems] : discount-eligible;)
