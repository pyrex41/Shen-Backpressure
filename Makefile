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
# The canonical copy lives at sb/; cmd/sb/skilldata/ is a mirror rebuilt from it.
# Always run sync-skilldata before build-sb if you edited anything under sb/.
sync-skilldata:
	rsync -a --delete sb/ cmd/sb/skilldata/

check-skilldata:
	@diff -qr sb/ cmd/sb/skilldata/ && echo "skilldata in sync" || \
	  (echo "skilldata drift — run 'make sync-skilldata'" && exit 1)

build-sb: sync-skilldata
	cd cmd/sb && go build -o ../../bin/sb .

build-shengen:
	cd cmd/shengen && go build -o ../../bin/shengen .

build-shengen-ts:
	cd cmd/shengen-ts && npm install && npm run build

build-shen-derive:
	cd shen-derive && go build -o ../bin/shen-derive ./cmd/shen-derive

build-shen-derive-ts:
	cd cmd/shen-derive-ts && npm install && npm run build

build-all: build-sb build-shengen build-shengen-ts build-shen-derive build-shen-derive-ts
