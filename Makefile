.PHONY: help sync-skilldata build-sb build-shengen build-shengen-ts build-shen-derive build-shen-derive-ts build-all check-skilldata

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

# -trimpath keeps the build path out of the binary, so the same source
# and toolchain produce the same bytes from any checkout location. The
# discharge report's toolchain block records the shengen binary's
# sha256, and without -trimpath that hash would depend on where the
# repo happens to live. See docs/TRUST-MODEL.md (W5.1).
GOFLAGS_REPRO := -trimpath

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
