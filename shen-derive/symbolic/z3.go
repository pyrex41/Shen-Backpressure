package symbolic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultSolverTimeout bounds a single satisfiability query. Path
// conditions in this fragment are small; a query that needs longer than
// this is reported unknown rather than allowed to stall the gate.
const DefaultSolverTimeout = 5 * time.Second

// ErrSolverUnavailable is returned by FindSolver when no solver binary
// is on PATH. Callers degrade: paths are still enumerated, feasibility
// stays unknown, and the harness proceeds with the existing sampler.
var ErrSolverUnavailable = errors.New("no SMT solver found on PATH (looked for z3)")

// Z3 drives Z3 over a subprocess. Deliberately not the Go binding: the
// dependency is then a binary on PATH, which lets the whole feature
// degrade cleanly when it is absent (see the W2 decision note in the
// verifier-throughput roadmap).
type Z3 struct {
	// Path is the z3 executable. Empty means "z3", resolved via PATH.
	Path string
	// Timeout bounds one query. Zero means DefaultSolverTimeout.
	Timeout time.Duration
	// UseTempFile passes the script as a file argument instead of on
	// stdin. Both are supported; stdin (`z3 -in`) is the default
	// because it leaves nothing behind on disk.
	UseTempFile bool
}

// FindSolver returns a Z3 driver when a z3 binary is on PATH, and
// ErrSolverUnavailable otherwise.
func FindSolver(timeout time.Duration) (Solver, error) {
	p, err := exec.LookPath("z3")
	if err != nil {
		return nil, ErrSolverUnavailable
	}
	if timeout <= 0 {
		timeout = DefaultSolverTimeout
	}
	return &Z3{Path: p, Timeout: timeout}, nil
}

// Name identifies the solver in report output.
func (z *Z3) Name() string { return "z3" }

// Check renders q to SMT-LIB2 and runs it through z3.
func (z *Z3) Check(q *Query) (*SolverResult, error) {
	script := RenderScript(q)
	timeout := z.Timeout
	if timeout <= 0 {
		timeout = DefaultSolverTimeout
	}
	bin := z.Path
	if bin == "" {
		bin = "z3"
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// -T:<seconds> is z3's own wall-clock cap; the context is the
	// belt-and-braces outer bound in case the binary ignores it.
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 1
	}
	args := []string{fmt.Sprintf("-T:%d", secs)}

	var cmd *exec.Cmd
	if z.UseTempFile {
		f, err := os.CreateTemp("", "shen-derive-smt-*.smt2")
		if err != nil {
			return nil, err
		}
		path := f.Name()
		defer os.Remove(path)
		if _, err := f.WriteString(script); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		args = append(args, filepath.Clean(path))
		cmd = exec.CommandContext(ctx, bin, args...)
	} else {
		args = append(args, "-in")
		cmd = exec.CommandContext(ctx, bin, args...)
		cmd.Stdin = strings.NewReader(script)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return &SolverResult{Status: StatusUnknown, Model: map[string]string{}, Raw: stdout.String()}, nil
	}

	res, parseErr := parseSolverOutput(stdout.String())
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("z3: %v (stderr: %s)", runErr, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("z3: %w", parseErr)
	}
	// z3 exits non-zero for `unsat` in some builds; a parsed status is
	// authoritative, so runErr is only reported when nothing parsed.
	return res, nil
}
