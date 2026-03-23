.PHONY: all build test shen-check gates ralph clean

all: build test shen-check

build:
	go build ./cmd/server
	go build ./cmd/ralph

test:
	go test ./...

shen-check:
	@./bin/shen-check.sh

gates: test build shen-check
	@echo "All gates passed."

ralph:
	./run-ralph.sh

run:
	go run ./cmd/server

clean:
	rm -f server ralph data.db
	rm -f plans/backpressure.log
