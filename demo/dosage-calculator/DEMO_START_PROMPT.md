Clinical dosage calculator in Go where administering a drug requires proving: the dose is within the weight-based safe range AND no contraindicated drug interactions exist. A SafeAdministration can only be constructed by providing both proofs — the Go compiler makes it impossible to skip the interaction check.

Stack: Go stdlib net/http, SQLite, htmx frontend for a clinical dashboard. No frameworks.

Domain entities:
- Patients with weight (kg) and current medication list
- Drugs with per-weight dosage ranges (min/max mg per kg)
- Known drug interactions (contraindication pairs)
- Administration records with full proof chain

Invariants:
- Patient weight must be > 0 and <= 500 kg
- A dose range is valid only when max >= min and both are non-negative
- A dose range must be computed for the patient's specific weight class
- Drug interaction clearance requires checking the new drug against ALL current medications — no contraindicated pairs
- A safe administration requires: valid dose range for this patient's weight, dose within that range, AND interaction clearance
- Dose outside range → construction error. Interaction found → construction error. No shortcut.

Operations:
- GET /patients → list patients with current meds
- POST /patients → add patient (weight, current meds)
- GET /drugs → list drugs with dosage tables
- POST /calculate → given patient + drug + proposed dose: compute weight-based range, check interactions, return SafeAdministration or detailed rejection
- POST /administer → record a SafeAdministration (requires proof chain)
- GET /patients/:id/history → administration history with proofs
- Dashboard showing patients, drugs, and a dosage calculator form

Use /sb:ralph-scaffold to set up the Ralph loop with four-gate backpressure, then run the loop to build it out.
