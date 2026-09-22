package verify

// shensamples.go — W6.D. The sample table, written out in Shen source
// form so a Shen host can be asked the same question the evaluator was
// asked.
//
// W5 shipped blame with a `lowering` value that could never be
// produced: telling a lowering bug from an implementation bug needs a
// second oracle for what the spec means, and there was no Shen host.
// With one, the second oracle is the spec itself, running on the host.
// To ask it, sb needs the failing case's inputs as Shen text — which
// the generated Go test does not carry, since its cases are Go
// constructor calls.
//
// So `shen-derive verify --shen-samples-out FILE` writes them: per
// case, the Shen literal of each argument and of the evaluator's
// answer. sb reads that, evaluates `(<func> <arg>…)` on the host, and
// compares. Agreement blames the implementation on the strength of two
// oracles; disagreement blames the lowering, because the two things
// that are supposed to mean the same thing do not.

import (
	"encoding/json"
	"os"

	"github.com/pyrex41/Shen-Backpressure/shen-derive/core"
)

// ShenSampleCase is one case rendered for a Shen host.
type ShenSampleCase struct {
	// Name is the case id, matching the subtest name in the generated
	// Go test and the case_id of any counter-example it produces.
	Name string `json:"name"`
	// Args are the Shen literals of the case's arguments, in order.
	Args []string `json:"args"`
	// Expected is the Shen literal of what the *evaluator* computed.
	// It is deliberately not called "want": this is the first oracle's
	// answer, and the whole point of the file is to check it against a
	// second one.
	Expected string `json:"expected"`
	// Provenance carries the sample source, as the generated test's
	// comment does: "" for the boundary pool, "path:<n>", "falsify:<n>".
	Provenance string `json:"provenance,omitempty"`
}

// ShenSampleFile is the whole sidecar.
type ShenSampleFile struct {
	// Func is the (define …) the cases belong to.
	Func string `json:"func"`
	// Spec is the spec file path, so a consumer can load the same
	// spec the samples were generated from rather than guessing.
	Spec string `json:"spec"`
	// Cases are the renderable cases. A case whose values have no
	// faithful Shen literal is omitted and named in Skipped, because a
	// missing case makes the second oracle silent about it, whereas an
	// approximated one would make it confidently wrong.
	Cases []ShenSampleCase `json:"cases"`
	// Skipped names the cases left out, with the reason.
	Skipped []string `json:"skipped,omitempty"`
}

// ShenSamples renders the harness's cases. specPath is recorded as
// written, so a consumer loads the same spec file these samples were
// generated from rather than guessing at it.
func (h *Harness) ShenSamples(specPath string) *ShenSampleFile {
	out := &ShenSampleFile{Spec: specPath}
	if h.Config.Spec != nil {
		out.Func = h.Config.Spec.Name
	}
	for _, c := range h.Cases {
		args := make([]string, 0, len(c.Args))
		ok := true
		for _, a := range c.Args {
			lit, err := core.ShenLiteral(a.Value)
			if err != nil || !core.ShenRenderable(a.Value) {
				out.Skipped = append(out.Skipped, c.Name+": argument has no Shen literal ("+a.Value.String()+")")
				ok = false
				break
			}
			args = append(args, lit)
		}
		if !ok {
			continue
		}
		exp, err := core.ShenLiteral(c.Expected)
		if err != nil {
			out.Skipped = append(out.Skipped, c.Name+": expected value has no Shen literal ("+c.Expected.String()+")")
			continue
		}
		out.Cases = append(out.Cases, ShenSampleCase{
			Name:       c.Name,
			Args:       args,
			Expected:   exp,
			Provenance: c.Provenance,
		})
	}
	return out
}

// WriteShenSamples writes the sidecar as indented JSON.
func (h *Harness) WriteShenSamples(path, specPath string) error {
	data, err := json.MarshalIndent(h.ShenSamples(specPath), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
