# Fix Plan — Email CRUD App

## Foundation
- [x] Get go.mod dependencies resolved and `go build ./cmd/server` passing
- [x] Get `go build ./cmd/ralph` passing
- [x] Add first test (models.User.IsKnown) and get `go test ./...` passing
- [x] Verify all three gates pass together (go test, go build, shen-check)

## Templates & Frontend
- [x] Create base layout template (templates/layout.html) with htmx + alpine script tags
- [x] Create index/dashboard page (templates/index.html) listing campaigns and users
- [x] Create users list page (templates/users.html) with htmx CRUD (add/edit/delete)
- [x] Create campaigns list page (templates/campaigns.html) with htmx CRUD
- [x] Create copy variants page (templates/copy_variants.html) — manage copy per campaign keyed by (age_decade, state)
- [x] Create CTA landing page (templates/cta_landing.html) — shows tailored copy for known users
- [x] Create CTA demographics prompt page (templates/cta_prompt.html) — alpine form for unknown users to enter age_decade + state

## Core CTA Flow
- [x] Implement CTA landing handler: known user → tailored copy, unknown user → prompt
- [x] Implement demographics prompt submit: upgrade unknown → known, redirect to CTA landing
- [x] Add tests for CTA flow: known user sees copy, unknown user gets prompted, prompt upgrades profile

## Send Flow
- [x] Implement send email handler (simulated — records to email_sends table, returns confirmation)
- [x] Create send email UI (pick campaign + user, send, see result)

## Seed Data
- [x] Add seed data command or init: sample users (some known, some unknown), sample campaign, sample copy variants for a few decade/state combos

## Polish
- [x] Add minimal CSS (static/style.css) so the app looks decent
- [x] Add ability for known users to update their demographics from the CTA landing page
- [x] End-to-end manual test: create campaign → add copy variants → send email → click CTA → see tailored copy (or prompt flow)
