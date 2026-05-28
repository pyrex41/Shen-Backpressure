module ralph-shen-agent

go 1.24.7

require (
	github.com/pyrex41/Shen-Backpressure/shen-derive v0.0.0
	golang.org/x/sync v0.12.0
)

// Profile-B (`:runtime-via :eval`) guard constructors call the embedded
// Shen evaluator host. The shen-derive module is local to this repo.
replace github.com/pyrex41/Shen-Backpressure/shen-derive => ../../shen-derive
