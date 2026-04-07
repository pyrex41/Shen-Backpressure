# Email CRUD Demo

Demonstrates Shen-Backpressure with a personalized email campaign app, built entirely by a Ralph loop.

## The Prompt

This entire app was generated from a single prompt to `/sb:init`:

> This is going to be an email sending app w/ some basic crud operations as a demo in go. We are going to use ralph w/ shen to do it. The app will send out emails to users with a call to action, then on click, show users custom-tailored copy. Copy will depend on the age (what decade) and location (what state) of the end person. If known, we should show by default, if unknown, prompt user on their arrival (they can always update this info if incorrect). Use go for the backend, simple htmx / alpine as needed and vanilla js on the frontend. Should be easy.

Ralph took it from there — generating Shen specs, scaffolding the app, implementing features one plan item at a time, with all three gates (tests, build, Shen type check) enforcing correctness each iteration.

## What It Does

1. **Admin creates campaigns** with email copy variants keyed by `(age_decade, state)`
2. **Emails go out** with a CTA link
3. **User clicks the CTA**:
   - **Known user** (has age + state on file) → sees tailored copy immediately
   - **Unknown user** → prompted for age decade and US state, then sees tailored copy
4. **Users can always update** their demographics

## The Key Invariant

From `specs/core.shen` — tailored copy can only be delivered to a user with known demographics that match the copy's target:

```shen
(datatype copy-delivery
  Profile : known-profile;
  Copy : copy-content;
  (= (tail (tail (head Profile))) (tail Copy)) : verified;
  =========================================================
  [Profile Copy] : copy-delivery;)
```

Unknown users must go through the prompt flow first (`prompt-required` → `profile-upgrade` → `safe-copy-view`).

## Stack

- **Backend**: Go stdlib `net/http`, SQLite via `go-sqlite3`
- **Frontend**: htmx + alpine.js + vanilla JS, server-rendered HTML templates
- **No frameworks**

## Running

```bash
cd examples/email-crud
go build -o server ./cmd/server
./server
# → http://localhost:8080
```

## Running the Ralph Loop (to continue development)

```bash
./run-ralph.sh
# or: make run
```

This calls the configured harness in a loop, validating each iteration against `go test`, `go build`, and `./bin/shen-check.sh`.
