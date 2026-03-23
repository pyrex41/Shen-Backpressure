# Fix Plan — Email CRUD App

## Foundation
- [ ] Get go.mod dependencies resolved and `go build ./cmd/server` passing
- [ ] Get `go build ./cmd/ralph` passing
- [ ] Add first test (models.User.IsKnown) and get `go test ./...` passing
- [ ] Verify all three gates pass together (go test, go build, shen-check)

## Templates & Frontend
- [ ] Create base layout template (templates/layout.html) with htmx + alpine script tags
- [ ] Create index/dashboard page (templates/index.html) listing campaigns and users
- [ ] Create users list page (templates/users.html) with htmx CRUD (add/edit/delete)
- [ ] Create campaigns list page (templates/campaigns.html) with htmx CRUD
- [ ] Create copy variants page (templates/copy_variants.html) — manage copy per campaign keyed by (age_decade, state)
- [ ] Create CTA landing page (templates/cta_landing.html) — shows tailored copy for known users
- [ ] Create CTA demographics prompt page (templates/cta_prompt.html) — alpine form for unknown users to enter age_decade + state

## Core CTA Flow
- [ ] Implement CTA landing handler: known user → tailored copy, unknown user → prompt
- [ ] Implement demographics prompt submit: upgrade unknown → known, redirect to CTA landing
- [ ] Add tests for CTA flow: known user sees copy, unknown user gets prompted, prompt upgrades profile

## Send Flow
- [ ] Implement send email handler (simulated — records to email_sends table, returns confirmation)
- [ ] Create send email UI (pick campaign + user, send, see result)

## Seed Data
- [ ] Add seed data command or init: sample users (some known, some unknown), sample campaign, sample copy variants for a few decade/state combos

## Polish
- [ ] Add minimal CSS (static/style.css) so the app looks decent
- [ ] Add ability for known users to update their demographics from the CTA landing page
- [ ] End-to-end manual test: create campaign → add copy variants → send email → click CTA → see tailored copy (or prompt flow)
