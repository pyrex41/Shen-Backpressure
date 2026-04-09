package main

import "testing"

func TestParseRewriteArgsWithBinding(t *testing.T) {
	opts, err := parseRewriteArgs([]string{
		"--bind", "?h=\\x z -> z - x",
		"negate", ".", "foldr", "(+)", "0",
		"foldr-fusion",
	})
	if err != nil {
		t.Fatalf("parseRewriteArgs: %v", err)
	}
	if opts.RuleName != "foldr-fusion" {
		t.Fatalf("rule = %q, want foldr-fusion", opts.RuleName)
	}
	if opts.Expr != "negate . foldr (+) 0" {
		t.Fatalf("expr = %q", opts.Expr)
	}
	if _, ok := opts.Extra["?h"]; !ok {
		t.Fatal("missing ?h binding")
	}
}

func TestParseRewriteArgsRejectsNonMetaBinding(t *testing.T) {
	_, err := parseRewriteArgs([]string{
		"--bind", "h=\\x z -> z - x",
		"negate . foldr (+) 0",
		"foldr-fusion",
	})
	if err == nil {
		t.Fatal("expected non-meta binding to be rejected")
	}
}

func TestParseRewriteArgsRejectsDuplicateBinding(t *testing.T) {
	_, err := parseRewriteArgs([]string{
		"--bind", "?h=\\x z -> z - x",
		"--bind", "?h=\\x z -> z + x",
		"negate . foldr (+) 0",
		"foldr-fusion",
	})
	if err == nil {
		t.Fatal("expected duplicate binding to be rejected")
	}
}
