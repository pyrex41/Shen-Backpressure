.PHONY: help sync-skilldata build-sb build-shengen build-shengen-ts build-shen-derive build-shen-derive-ts build-all check-skilldata shen-go

help:
	@echo "Shen-Backpressure build targets:"
	@echo ""
	@echo "  sync-skilldata     Copy canonical sb/ into cmd/sb/skilldata/ (run before build-sb)"
	@echo "  check-skilldata    Verify cmd/sb/skilldata/ matches canonical sb/ (no changes)"
	@echo "  build-sb           Build the sb engine binary (embeds skilldata)"
	@echo "  build-shengen      Build Go shengen"
	@echo "  build-shengen-ts   Build TypeScript shengen"
	@echo "  build-shen-derive  Build Go shen-derive"
	@echo "  build-shen-derive-ts  Build TypeScript shen-derive"
	@echo "  build-all          Sync skilldata + build all supported binaries"
	@echo "  shen-go            Build the Go port of Shen into bin/shen (the gate-4 / flow host)"

# The sb CLI embeds the SKM skill bundle via //go:embed skilldata/*.
# Canonical source: sb/. Mirror (checked in for offline-friendly fresh
# clones): cmd/sb/skilldata/. Edits to the bundle land in sb/; the mirror
# is rebuilt before every binary build by build-sb (and by cmd/sb/build.sh).
# `make check-skilldata` enforces equality in CI.
sync-skilldata:
	rm -rf cmd/sb/skilldata
	mkdir -p cmd/sb/skilldata
	cp -R sb/. cmd/sb/skilldata/

check-skilldata:
	@diff -qr sb/ cmd/sb/skilldata/ && echo "skilldata in sync" || \
	  (echo "skilldata drift — run 'make sync-skilldata'" && exit 1)

# -trimpath keeps the build path out of the binary and -buildvcs=false
# keeps the revision stamp out of it, so the same source and toolchain
# produce the same bytes from any checkout, tracked or not. The
# discharge report's toolchain block records the shengen binary's
# sha256; without both flags that hash would change every time the repo
# moved or anyone made a commit, which would make it useless as an
# identifier for the emitter. See docs/TRUST-MODEL.md (W5.1).
GOFLAGS_REPRO := -trimpath -buildvcs=false

build-sb: sync-skilldata
	cd cmd/sb && go build $(GOFLAGS_REPRO) -o ../../bin/sb .

build-shengen:
	cd cmd/shengen && go build $(GOFLAGS_REPRO) -o ../../bin/shengen .

build-shengen-ts:
	cd cmd/shengen-ts && npm install && npm run build

build-shen-derive:
	cd shen-derive && go build $(GOFLAGS_REPRO) -o ../bin/shen-derive .

build-shen-derive-ts:
	cd cmd/shen-derive-ts && npm install && npm run build

build-all: build-sb build-shengen build-shengen-ts build-shen-derive build-shen-derive-ts

# --- The Shen host (W6) ----------------------------------------------
#
# Gate 4 (tc+) and the Shen flow engine both need a Shen host. The
# repository does not vendor one: this target fetches and builds the Go
# port, which is the only host that installs without a Lisp or Scheme
# toolchain, and drops its launcher at bin/shen — the last name
# ResolveShenHost looks for on PATH, and the one `[shen] bin` points at
# in both examples.
#
# GOTOOLCHAIN=auto is required, not a convenience: shen-go needs Go
# 1.27 and this repository pins go1.24.7 for reproducible emitter
# builds. `auto` lets the shen-go build download the toolchain it asks
# for without changing anything about the toolchain that builds sb,
# shengen or shen-derive.
#
# SHEN_GO_CACHE is where the clone lives. Outside the repository by
# default, so a `git clean` does not throw away a 1-minute build, and
# overridable for an air-gapped runner that already has the source.
SHEN_GO_REPO  ?= https://github.com/pyrex41/shen-go
SHEN_GO_CACHE ?= $(HOME)/.cache/shen-backpressure/shen-go

shen-go:
	@if [ ! -d "$(SHEN_GO_CACHE)/.git" ]; then \
	  echo "cloning $(SHEN_GO_REPO) → $(SHEN_GO_CACHE)"; \
	  mkdir -p "$(dir $(SHEN_GO_CACHE))"; \
	  git clone --depth 1 "$(SHEN_GO_REPO)" "$(SHEN_GO_CACHE)"; \
	else \
	  echo "using existing clone at $(SHEN_GO_CACHE)"; \
	fi
	cd "$(SHEN_GO_CACHE)" && GOTOOLCHAIN=auto $(MAKE) shen
	mkdir -p bin
	cp "$(SHEN_GO_CACHE)/shen" bin/shen
	@echo "bin/shen: $$(./bin/shen --version)"
	@echo ""
	@echo "Gate 4 and the Shen flow engine will now find it. To make it"
	@echo "explicit, set SHEN=\$$PWD/bin/shen or [shen] bin in sb.toml."
