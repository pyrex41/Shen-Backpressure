\* ============================================================ *\
\* Dosage Calculator — Shen Sequent-Calculus Type Specs          *\
\* ============================================================ *\

\* --- Wrapper types (no validation) --- *\

(datatype patient-id
  X : string;
  ==============
  X : patient-id;)

(datatype drug-id
  X : string;
  ============
  X : drug-id;)

\* --- Constrained types (validated) --- *\

\* Patient weight: must be > 0 and <= 500 kg *\
(datatype patient-weight
  X : number;
  (> X 0) : verified;
  (<= X 500) : verified;
  =======================
  X : patient-weight;)

\* Non-negative dose value in mg *\
(datatype dose-value
  X : number;
  (>= X 0) : verified;
  =====================
  X : dose-value;)

\* --- Composite/Guarded types --- *\

\* Valid dose range: min and max in mg, computed for a patient's weight.
   Both must be non-negative, max >= min. *\
(datatype dose-range
  Min : number;
  Max : number;
  Weight : patient-weight;
  (>= Min 0) : verified;
  (>= Max 0) : verified;
  (<= Min Max) : verified;
  =========================
  [Min Max Weight] : dose-range;)

\* Proof that a proposed dose falls within the computed range *\
(datatype dose-in-range
  Dose : number;
  Range : dose-range;
  (>= Dose (head Range)) : verified;
  (<= Dose (head (tail Range))) : verified;
  ==========================================
  [Dose Range] : dose-in-range;)

\* --- Interaction checking --- *\
\* A contraindication pair: two drugs that must not be co-administered *\

(datatype contraindication
  DrugA : drug-id;
  DrugB : drug-id;
  ========================
  [DrugA DrugB] : contraindication;)

\* Helper: check if a drug pair (in either order) appears in a list of contraindications *\

(define pair-in-list?
  _ _ [] -> false
  A B [[X Y] | Rest] -> true  where (and (= A X) (= B Y))
  A B [[X Y] | Rest] -> true  where (and (= A Y) (= B X))
  A B [_ | Rest] -> (pair-in-list? A B Rest))

\* Helper: check that a drug has no contraindicated interaction with any medication *\

(define drug-clear-of-list?
  _ [] _ -> true
  Drug [Med | Meds] Pairs -> false where (pair-in-list? Drug Med Pairs)
  Drug [_ | Meds] Pairs -> (drug-clear-of-list? Drug Meds Pairs))

\* Interaction clearance: drug checked against ALL current medications,
   no contraindicated pair found in the known interactions list *\

(datatype interaction-clearance
  Drug : drug-id;
  Meds : (list drug-id);
  Pairs : (list contraindication);
  (drug-clear-of-list? Drug Meds Pairs) : verified;
  ==================================================
  [Drug Meds Pairs] : interaction-clearance;)

\* --- Safe administration: the final proof-carrying type --- *\
\* Requires BOTH dose-in-range AND interaction clearance *\

(datatype safe-administration
  DoseProof : dose-in-range;
  Clearance : interaction-clearance;
  ====================================
  [DoseProof Clearance] : safe-administration;)
