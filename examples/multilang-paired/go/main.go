// CLI for the multilang-paired demo (Go implementation).
//
// Reads JSONL from stdin where each line is one fixture row:
//
//   {"caseId":"c1","customerId":"u1","items":[{"sku":"A","qty":1.0}],
//    "subtotal":50.0,"minSubtotal":50.0,"minItems":2}
//
// Emits one JSON object per line, in the same order, with the shape
//
//   {"caseId":"c1","ok":false,
//    "outcome":"…","reason":"…",
//    "itemCount":1,"hasBulkLine":false}
//
// `outcome` is one of: "eligible", "rejected-subtotal", "rejected-items",
// "rejected-cart-build", "rejected-item-build". `reason` is the
// constructor-emitted error message when ok is false, or "" otherwise.
//
// The exact same shape is produced by the TS, Py, and Rs CLIs; the
// parity check pairwise-diffs the output.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	mp "example.com/multilang-paired/multilang_paired"
)

type itemInput struct {
	Sku string  `json:"sku"`
	Qty float64 `json:"qty"`
}

type input struct {
	CaseID      string      `json:"caseId"`
	CustomerID  string      `json:"customerId"`
	Items       []itemInput `json:"items"`
	Subtotal    float64     `json:"subtotal"`
	MinSubtotal float64     `json:"minSubtotal"`
	MinItems    float64     `json:"minItems"`
}

// output is the per-row decision the CLI emits. The shape is identical
// across all four languages — the parity check pairwise-diffs lines.
// `reason` is deliberately omitted: each language's generated guards
// emit a slightly different error string (e.g. Go's
// `cart.subtotal must be >= minSubtotal` vs Py's `cart.subtotal() must
// be >= min_subtotal`) because they follow per-language identifier
// conventions. `outcome` carries the structured rejection category,
// which IS portable.
type output struct {
	CaseID      string `json:"caseId"`
	OK          bool   `json:"ok"`
	Outcome     string `json:"outcome"`
	ItemCount   int    `json:"itemCount"`
	HasBulkLine bool   `json:"hasBulkLine"`
}

// hasBulkLine mirrors the Shen define has-bulk-line? — a line item
// with qty >= 5 means a bulk discount kicker. Implemented inline in
// the CLI rather than calling a generated helper because:
//   - cmd/shengen (Go) only emits defines referenced from `verified`
//     premises, not free-standing ones (see
//     `cmd/shengen/main.go:1012, generateDefineHelpers`).
//   - cmd/shengen-ts has a known parser bug where the LAST clause's
//     `where`-guard is dropped (the emitted `hasBulkLine` returns true
//     for any non-empty list).
// The Py and Rs CLIs DO call the generated `has_bulk_line(…)` helper
// — see `py/main.py` and `rs/main.rs`. The behavioural contract
// (qty >= 5 on any line) is identical across all four; the
// implementation path differs only because of emitter-coverage gaps.
func hasBulkLine(items []mp.CartItem) bool {
	for _, it := range items {
		if it.Qty() >= 5 {
			return true
		}
	}
	return false
}

func processRow(in input) output {
	out := output{CaseID: in.CaseID, ItemCount: len(in.Items)}

	// Build typed CartItems via NewCartItem (verified premise: qty >= 1).
	items := make([]mp.CartItem, 0, len(in.Items))
	for _, it := range in.Items {
		ci, err := mp.NewCartItem(mp.NewSku(it.Sku), it.Qty)
		if err != nil {
			out.OK = false
			out.Outcome = "rejected-item-build"
			return out
		}
		items = append(items, ci)
	}

	// Build Cart (verified premise: subtotal >= 0).
	cart, err := mp.NewCart(in.Subtotal, mp.NewCustomerId(in.CustomerID), items)
	if err != nil {
		out.OK = false
		out.Outcome = "rejected-cart-build"
		return out
	}

	// Compute the auxiliary fields up front so they're reported in
	// every row, including rejection rows below.
	out.HasBulkLine = hasBulkLine(items)

	// Build DiscountEligible (cross-field premises:
	//   subtotal >= minSubtotal, itemCount >= minItems).
	// Each language's generated NewDiscountEligible(...) raises on the
	// FIRST failing premise in spec order; spec order is
	// (>= (head Cart) MinSubtotal) then (>= ItemCount MinItems), so
	// the subtotal-failure case fires first. We disambiguate by
	// re-checking the premises locally — the parity check requires
	// identical `outcome` strings across languages, independent of how
	// each language phrases its error message.
	_, err = mp.NewDiscountEligible(cart, float64(len(items)), in.MinSubtotal, in.MinItems)
	if err != nil {
		out.OK = false
		if !(in.Subtotal >= in.MinSubtotal) {
			out.Outcome = "rejected-subtotal"
		} else {
			out.Outcome = "rejected-items"
		}
		return out
	}

	out.OK = true
	out.Outcome = "eligible"
	return out
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var in input
		if err := json.Unmarshal([]byte(line), &in); err != nil {
			fmt.Fprintf(os.Stderr, "go: invalid input line %q: %v\n", line, err)
			os.Exit(1)
		}
		out := processRow(in)
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "go: encode error: %v\n", err)
			os.Exit(1)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "go: scanner error: %v\n", err)
		os.Exit(1)
	}
}
