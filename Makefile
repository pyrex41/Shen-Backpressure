.PHONY: all build test shen-check run run-relaxed demo clean

all: build test shen-check

build:
	go build -o ralph ./cmd/ralph

test:
	go test ./...

shen-check:
	@./bin/shen-check.sh

run: build
	./ralph

run-relaxed: build
	./ralph --relaxed

demo: build
	RALPH_DEMO=1 ./ralph

clean:
	rm -f ralph
	rm -f plans/backpressure.log
