\* A deliberately broad marketplace operations specification.
   It is larger than the focused repository examples and spans identity,
   catalog, inventory, pricing, orders, payments, shipping, support, and risk. *\

(datatype tenant-id
  X : string;
  ==============
  X : tenant-id;)

(datatype user-id
  X : string;
  ==============
  X : user-id;)

(datatype sku
  X : string;
  ==============
  X : sku;)

(datatype order-id
  X : string;
  ==============
  X : order-id;)

(datatype payment-id
  X : string;
  ==============
  X : payment-id;)

(datatype warehouse-id
  X : string;
  ==============
  X : warehouse-id;)

(datatype ticket-id
  X : string;
  ==============
  X : ticket-id;)

(datatype money
  X : number;
  (>= X 0) : verified;
  ======================
  X : money;)

(datatype quantity
  X : number;
  (>= X 0) : verified;
  ======================
  X : quantity;)

(datatype basis-points
  X : number;
  (>= X 0) : verified;
  (<= X 10000) : verified;
  =======================
  X : basis-points;)

(datatype tenant-member
  Tenant : tenant-id;
  User : user-id;
  Active : boolean;
  =================================
  [Tenant User Active] : tenant-member;)

(datatype catalog-item
  Tenant : tenant-id;
  Sku : sku;
  Price : money;
  Active : boolean;
  ===================================
  [Tenant Sku Price Active] : catalog-item;)

(datatype inventory-position
  Tenant : tenant-id;
  Warehouse : warehouse-id;
  Sku : sku;
  OnHand : quantity;
  Reserved : quantity;
  (>= (val OnHand) (val Reserved)) : verified;
  ========================================================
  [Tenant Warehouse Sku OnHand Reserved] : inventory-position;)

(datatype price-adjustment
  Subtotal : money;
  Discount : money;
  (>= (val Subtotal) (val Discount)) : verified;
  =============================================
  [Subtotal Discount] : price-adjustment;)

(datatype promotion-policy
  Tenant : tenant-id;
  Minimum : money;
  Rate : basis-points;
  UsageLimit : quantity;
  ============================================
  [Tenant Minimum Rate UsageLimit] : promotion-policy;)

(datatype order-line
  Sku : sku;
  Qty : quantity;
  UnitPrice : money;
  =================================
  [Sku Qty UnitPrice] : order-line;)

(datatype order-draft
  Tenant : tenant-id;
  Buyer : user-id;
  Lines : (list order-line);
  Subtotal : money;
  ==========================================
  [Tenant Buyer Lines Subtotal] : order-draft;)

(datatype inventory-reservation
  Position : inventory-position;
  Requested : quantity;
  (>= (- (val (on-hand Position)) (val (reserved Position))) (val Requested)) : verified;
  ===========================================================================
  [Position Requested] : inventory-reservation;)

(datatype payment-capture
  Payment : payment-id;
  Authorized : money;
  Captured : money;
  (>= (val Authorized) (val Captured)) : verified;
  =================================================
  [Payment Authorized Captured] : payment-capture;)

(datatype refund
  Capture : payment-capture;
  PreviouslyRefunded : money;
  Requested : money;
  (>= (- (val (captured Capture)) (val PreviouslyRefunded)) (val Requested)) : verified;
  =================================================================================
  [Capture PreviouslyRefunded Requested] : refund;)

(datatype shipment-allocation
  Warehouse : warehouse-id;
  Sku : sku;
  Available : quantity;
  Allocated : quantity;
  (>= (val Available) (val Allocated)) : verified;
  ========================================================
  [Warehouse Sku Available Allocated] : shipment-allocation;)

(datatype support-ticket
  Tenant : tenant-id;
  Requester : user-id;
  Ticket : ticket-id;
  Private : boolean;
  ===========================================
  [Tenant Requester Ticket Private] : support-ticket;)

(datatype risk-signal
  Velocity : number;
  Amount : money;
  AccountAgeDays : number;
  (>= Velocity 0) : verified;
  (>= AccountAgeDays 0) : verified;
  ===============================================
  [Velocity Amount AccountAgeDays] : risk-signal;)

(datatype risk-decision
  Signal : risk-signal;
  Score : number;
  (>= Score 0) : verified;
  (<= Score 100) : verified;
  =================================
  [Signal Score] : risk-decision;)

(datatype order-transition
  From : symbol;
  To : symbol;
  ActorTenant : tenant-id;
  OrderTenant : tenant-id;
  (= (val ActorTenant) (val OrderTenant)) : verified;
  ========================================================
  [From To ActorTenant OrderTenant] : order-transition;)

(datatype idempotency-record
  Tenant : tenant-id;
  Key : string;
  RequestHash : string;
  ResponseHash : string;
  =================================================
  [Tenant Key RequestHash ResponseHash] : idempotency-record;)

(datatype audit-event
  Tenant : tenant-id;
  Actor : user-id;
  Action : symbol;
  Resource : string;
  Timestamp : number;
  (>= Timestamp 0) : verified;
  =================================================
  [Tenant Actor Action Resource Timestamp] : audit-event;)

(datatype fulfillment-plan
  Order : order-id;
  Allocations : (list shipment-allocation);
  Complete : boolean;
  =================================================
  [Order Allocations Complete] : fulfillment-plan;)
