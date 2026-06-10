package main

// shen-lua tier for the n-way differential: the Decidable-Shen-fragment
// evaluated on the REAL Shen kernel (shen-lua, a certified Shen 41.1 port),
// not a lowering. The runner (runtime/shen-lua/policy.lua) loads
// specs/core.shen itself and answers each request by asking the kernel's
// sequent-calculus typechecker whether the request's ground term inhabits the
// access type (tenant-access / resource-access), with `: verified` premises
// discharged by total ground evaluation — totality is exactly what the
// decidable-fragment certification (`sb policy --decidable`) establishes.
//
// Like the cedar/opa binaries, this tier is optional: no luajit on PATH (or a
// failed handshake) skips it gracefully. Set SHEN_LUA_DIR to a shen-lua
// checkout if the `shen` rock is not on the default luarocks package.path.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type shenLuaEvaluator struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// newShenLuaEvaluator starts the policy runner as a subprocess and performs
// the PING/READY handshake. Returns nil (with a reason) when the tier is
// unavailable; callers skip it like the opa tier.
func newShenLuaEvaluator(exampleRoot string) (*shenLuaEvaluator, string) {
	luajit, err := exec.LookPath("luajit")
	if err != nil {
		return nil, "no luajit binary in PATH"
	}
	runner := filepath.Join(exampleRoot, "runtime", "shen-lua", "policy.lua")
	if _, err := os.Stat(runner); err != nil {
		return nil, "runner not found: " + runner
	}
	spec := filepath.Join(exampleRoot, "specs", "core.shen")

	cmd := exec.Command(luajit, runner, "--spec", spec)
	cmd.Env = os.Environ() // pass SHEN_LUA_DIR / LUA_PATH through
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, "stdin pipe: " + err.Error()
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "stdout pipe: " + err.Error()
	}
	if err := cmd.Start(); err != nil {
		return nil, "start: " + err.Error()
	}
	ev := &shenLuaEvaluator{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}

	// Handshake with a timeout: boot is ~100ms warm, ~1.5s on a cold kernel
	// compile; 30s covers a first-ever run on a slow machine.
	type hs struct {
		line string
		err  error
	}
	ch := make(chan hs, 1)
	go func() {
		fmt.Fprintln(stdin, "PING")
		line, err := ev.stdout.ReadString('\n')
		ch <- hs{line, err}
	}()
	select {
	case h := <-ch:
		if h.err != nil || strings.TrimSpace(h.line) != "READY" {
			ev.Close()
			return nil, fmt.Sprintf("handshake failed (line=%q err=%v)", strings.TrimSpace(h.line), h.err)
		}
	case <-time.After(30 * time.Second):
		ev.Close()
		return nil, "handshake timeout"
	}
	return ev, ""
}

// Allow asks the kernel whether the sample's ground term inhabits the access
// type. Errors fail closed (deny) and are reported by the caller via the
// returned error.
func (ev *shenLuaEvaluator) Allow(s accessSample) (bool, error) {
	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
	_, err := fmt.Fprintf(ev.stdin, "CHECK\t%s\t%s\t%s\t%s\t%s\t%s\n",
		s.Level, s.PrincipalID, s.TenantID, s.ResourceID, b(s.IsMember), b(s.IsOwned))
	if err != nil {
		return false, err
	}
	line, err := ev.stdout.ReadString('\n')
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(line) == "allow", nil
}

func (ev *shenLuaEvaluator) Close() {
	if ev.stdin != nil {
		fmt.Fprintln(ev.stdin, "QUIT")
		ev.stdin.Close()
	}
	if ev.cmd != nil {
		done := make(chan struct{})
		go func() { ev.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			ev.cmd.Process.Kill()
		}
	}
}
