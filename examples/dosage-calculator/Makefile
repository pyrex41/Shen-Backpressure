.PHONY: all build test shen-check shengen run run-relaxed demo clean

all: shengen test build shen-check

shengen:
	./bin/shengen-codegen.sh

build: shengen
	go build -o server ./cmd/server

test: shengen
	go test ./...

shen-check:
	./bin/shen-check.sh

run: build
	./ralph

run-relaxed: build
	./ralph --relaxed

demo: build
	RALPH_DEMO=1 ./ralph

clean:
	rm -f server ralph bin/shengen plans/backpressure.log
