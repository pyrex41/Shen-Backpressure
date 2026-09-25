\* shengen-ex feature fixture: every construct the Elixir emitter lowers. *\

(datatype account-id
  X : string;
  ==============
  X : account-id;)

(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(datatype currency
  X : symbol;
  (element? X [usd eur gbp]) : verified;
  =======================================
  X : currency;)

(datatype money
  Amt : amount;
  Cur : currency;
  ======================
  [Amt Cur] : money;)

(datatype transfer
  From : account-id;
  To : account-id;
  Value : money;
  (not (= From To)) : verified;
  (> (head Value) 0) : verified;
  ==================================
  [From To Value] : transfer;)

(datatype balance-checked
  Bal : number;
  Tx : transfer;
  (>= Bal (head (head (tail (tail Tx))))) : verified;
  ====================================================
  [Bal Tx] : balance-checked;)

(datatype safe-transfer
  Tx : transfer;
  Check : balance-checked;
  (= Tx (head (tail Check))) : verified;
  ======================================
  [Tx Check] : safe-transfer;)

\* alias over a guard type *\
(datatype settled
  T : safe-transfer;
  ==================
  T : settled;)

\* tagged sum type: literal constructor tags are not host fields *\
(datatype step-wait
  Seconds : amount;
  ===============================
  [wait Seconds] : workflow-step;)

(datatype step-call
  Target : account-id;
  Retries : number;
  (<= Retries 3) : verified;
  ======================================
  [call Target Retries] : workflow-step;)

(datatype workflow
  Steps : (list workflow-step);
  Owner : account-id;
  (> (length Steps) 0) : verified;
  (allowed-owner? Owner Steps) : verified;
  ========================================
  [Steps Owner] : workflow;)

(datatype audited
  Actor : account-id;
  Note : string;
  (not (= Note "")) : verified; \* :runtime-via auditLog *\
  ============================
  [Actor Note] : audited;)

(define allowed-owner?
  {account-id --> (list workflow-step) --> boolean}
  _ [] -> true
  Owner [[call Owner _] | Rest] -> (allowed-owner? Owner Rest)
  _ [[call _ _] | _] -> false
  Owner [_ | Rest] -> (allowed-owner? Owner Rest))

(define total-retries
  {(list workflow-step) --> number}
  [] -> 0
  [[call _ N] | Rest] -> (+ N (total-retries Rest))
  [_ | Rest] -> (total-retries Rest))

(define classify
  {number --> symbol}
  X -> small where (< X 10)
  X -> medium where (and (>= X 10) (< X 100))
  _ -> large)

(define same?
  {string --> string --> boolean}
  X X -> true
  _ _ -> false)
