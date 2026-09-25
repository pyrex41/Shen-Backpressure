# phoenix-ash-tenant — Elixir/Ash target for Shen-Backpressure.
#
#   make ci      all gates (sb gates: gen, compile, test, shen tc+, tcb audit,
#                shen-derive, mutate-spec) + Dialyzer
#   make demo    watch the tracer, shen-derive and mutate-spec reject planted bugs
#
# Needs: Elixir >= 1.17 / OTP 27+, Go (builds sb + shengen-ex), and shen-erl
# (https://github.com/pyrex41/shen-erl, built with `make`). Point
# SHEN_ERL_ROOT at the checkout; the default is a sibling of this repo.

REPO          := $(abspath ../..)
SB            ?= $(REPO)/bin/sb
SHEN_ERL_ROOT ?= $(abspath $(REPO)/../shen-erl)
export SHEN_ERL_ROOT
export SHEN_ERL_EBIN ?= $(SHEN_ERL_ROOT)/ebin
export SB_SHEN       ?= $(SHEN_ERL_ROOT)/bin/shen-erl
export SHENGEN_EX_PATH ?= $(REPO)/bin/shengen-ex
export MIX_ENV       ?= test

.PHONY: ci deps sb gates gen gen-check compile test shen-check audit derive derive-regen mutate dialyzer demo clean

ci: deps gates dialyzer

deps:
	mix deps.get

sb:
	cd $(REPO)/cmd/sb && go build -o $(SB) .
	cd $(REPO)/cmd/shengen-ex && go build -o $(REPO)/bin/shengen-ex .

gates: sb
	$(SB) gates

# --- individual gates -------------------------------------------------------
gen: sb
	$(SB) gen

gen-check: sb
	$(SB) gen --check

compile:
	mix compile --warnings-as-errors

test:
	mix test

shen-check:
	./bin/shen-check.sh

audit: sb
	$(SB) audit

derive: sb
	$(SB) derive

derive-regen: sb
	$(SB) derive --regen

mutate: sb
	$(SB) mutate-spec

dialyzer:
	mix dialyzer

# --- demo: each planted bug must be caught -----------------------------------
demo: sb
	@echo "== 1. compile tracer refuses a forged %TenantAccess{} literal"
	@cp demo/hostile_forge.ex.bak lib/tenant/hostile.ex
	@if mix compile --force --warnings-as-errors 2>&1 | grep "Shen guard type"; then \
	  echo "   caught"; rm lib/tenant/hostile.ex; else rm lib/tenant/hostile.ex; echo "   NOT caught"; exit 1; fi
	@echo "== 2. shen-derive (shen-erl oracle) catches a case-folding bug in visible_titles/2"
	@cp lib/tenant/authz.ex /tmp/phoenix-ash-tenant-authz.ex && cp demo/broken_authz.ex.bak lib/tenant/authz.ex
	@if $(SB) derive 2>&1 | grep -A6 "cases disagree"; then \
	  cp /tmp/phoenix-ash-tenant-authz.ex lib/tenant/authz.ex; echo "   caught"; \
	else cp /tmp/phoenix-ash-tenant-authz.ex lib/tenant/authz.ex; echo "   NOT caught"; exit 1; fi
	@echo "== 3. mutate-spec fails when a premise has no hostile witness"
	@if $(SB) mutate-spec --hostile 'specs/hostile/0[1-6]*.shen' --json '' 2>&1 | grep "SURVIVED"; then \
	  echo "   caught"; else echo "   NOT caught"; exit 1; fi

clean:
	rm -rf _build .sb
