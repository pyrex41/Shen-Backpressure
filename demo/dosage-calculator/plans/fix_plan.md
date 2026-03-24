# Dosage Calculator — Implementation Plan

- [ ] Define domain types (Patient, Drug, DoseRange, Interaction, SafeAdministration)
- [ ] Write Shen specs for all invariants (weight bounds, dose range validity, interaction clearance, safe administration proof chain)
- [ ] Generate guard types via shengen
- [ ] Implement SQLite persistence layer
- [ ] Implement domain logic (dose calculation, interaction checking, proof construction)
- [ ] Implement HTTP handlers (GET/POST patients, drugs, calculate, administer, history)
- [ ] Build htmx dashboard frontend
- [ ] Integration tests for proof chain (dose out of range rejected, interaction found rejected, valid administration accepted)
