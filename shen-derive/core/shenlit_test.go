package core

import "testing"

// TestShenLiteralRoundTripsThroughTheParser is the property that
// matters: what the renderer writes, core.ParseSexpr reads back as the
// same shape. The second oracle hands these literals to a Shen host,
// so a renderer that produced almost-right text would make the host
// answer a different question and the report blame the lowering for it.
func TestShenLiteralRoundTrips(t *testing.T) {
	cases := []struct {
		v    Value
		want string
	}{
		{IntVal(0), "0"},
		{IntVal(-7), "-7"},
		{FloatVal(2.5), "2.5"},
		// An integral float loses its fractional part: Shen's reader
		// gives the same number either way, and the short form is what
		// the generated Go test shows too.
		{FloatVal(100), "100"},
		{BoolVal(true), "true"},
		{BoolVal(false), "false"},
		{StringVal(""), `""`},
		{StringVal("alice"), `"alice"`},
		{ListVal(nil), "[]"},
		{ListVal{IntVal(1), IntVal(2)}, "[1 2]"},
		{ListVal{ListVal{FloatVal(5), StringVal("bob"), StringVal("bob")}}, `[[5 "bob" "bob"]]`},
		{&TupleVal{Fst: BoolVal(true), Snd: IntVal(3)}, "(@p true 3)"},
	}
	for _, c := range cases {
		got, err := ShenLiteral(c.v)
		if err != nil {
			t.Errorf("%s: %v", c.v, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.v, got, c.want)
		}
		if _, err := ParseSexpr(got); err != nil {
			t.Errorf("%q does not parse back: %v", got, err)
		}
	}
}

// TestShenLiteralRefusesWhatItCannotSay. A closure has no literal, and
// Shen string literals have no escapes, so a string holding a quote
// cannot be written at all. Both must be refused rather than
// approximated: an approximated goal would send the host a different
// question and its answer would be read as a disagreement.
func TestShenLiteralRefusesWhatItCannotSay(t *testing.T) {
	if _, err := ShenLiteral(&ClosureVal{Param: "X"}); err == nil {
		t.Error("a closure was given a Shen literal")
	}
	if _, err := ShenLiteral(&PrimPartial{Op: "+"}); err == nil {
		t.Error("a partial primitive was given a Shen literal")
	}
	if ShenRenderable(StringVal(`has "quote"`)) {
		t.Error("a string containing a quote was reported renderable")
	}
	if ShenRenderable(ListVal{StringVal(`a"b`)}) {
		t.Error("a list containing an unrenderable string was reported renderable")
	}
	if !ShenRenderable(ListVal{IntVal(1), StringVal("ok")}) {
		t.Error("an ordinary list was reported unrenderable")
	}
}
