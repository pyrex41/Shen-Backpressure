// Command impl runs two goroutines contending for a spin lock and writes
// the behaviour it observed as a JSONL trace, one state of
// specs/mutex.shen per line. `shen-derive trace` then checks that the
// trace is a behaviour the spec allows.
//
// The refinement mapping from implementation state to spec state is
// record: every change to a process's pc is logged together with the
// lock bit, and a step that changes both (taking or releasing the lock)
// happens inside the same critical section of the recorder, so the log
// sees it as one atomic step, which is what the spec says it is. The
// recorder's mutex orders the log; it is not the lock under test.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
)

type system struct {
	held atomic.Bool // the lock under test

	mu  sync.Mutex // guards pcs and the trace
	pcs [2]string
	out *json.Encoder
}

// step applies change and logs the resulting state as one atomic step.
func (s *system) step(id int, change func() bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !change() {
		return false
	}
	s.out.Encode(map[string]any{
		"action": fmt.Sprintf("p%d", id+1),
		"state":  []any{s.pcs[0], s.pcs[1], s.held.Load()},
	})
	return true
}

// correct takes the lock with a compare-and-swap: seeing it free and
// taking it are one step.
func (s *system) correct(id int) {
	s.step(id, func() bool { s.pcs[id] = "want"; return true })
	for !s.step(id, func() bool {
		if !s.held.CompareAndSwap(false, true) {
			return false
		}
		s.pcs[id] = "crit"
		return true
	}) {
		runtime.Gosched()
	}
	s.step(id, func() bool { s.held.Store(false); s.pcs[id] = "idle"; return true })
}

// racy checks the lock and takes it in two separate steps.
func (s *system) racy(id int) {
	s.step(id, func() bool { s.pcs[id] = "want"; return true })
	for s.held.Load() {
		runtime.Gosched()
	}
	runtime.Gosched() // widen the window between the check and the take
	s.step(id, func() bool { s.held.Store(true); s.pcs[id] = "crit"; return true })
	s.step(id, func() bool { s.held.Store(false); s.pcs[id] = "idle"; return true })
}

func main() {
	racy := flag.Bool("racy", false, "use the check-then-take lock")
	rounds := flag.Int("rounds", 200, "critical sections per process")
	flag.Parse()

	s := &system{pcs: [2]string{"idle", "idle"}, out: json.NewEncoder(os.Stdout)}
	s.out.Encode([]any{"idle", "idle", false})

	var wg sync.WaitGroup
	for id := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range *rounds {
				if *racy {
					s.racy(id)
				} else {
					s.correct(id)
				}
			}
		}()
	}
	wg.Wait()
}
