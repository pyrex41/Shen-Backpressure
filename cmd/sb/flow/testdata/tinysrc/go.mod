// Fixture module for the flow golden test. It is deliberately tiny
// and has no dependencies, so `scip-go` can index it offline and the
// resulting index.scip stays small enough to check in.
//
// Regenerate the fixture with:
//
//	cd cmd/sb/flow/testdata/tinysrc && scip-go --output ../tiny.scip
//	cd ../../.. && go test ./flow -run TestGoldenFacts -update
module tiny

go 1.24
