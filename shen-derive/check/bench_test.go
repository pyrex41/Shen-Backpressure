package check

import (
	"fmt"
	"strings"
	"testing"
)

// electionN returns const overrides that scale the election example to
// n computers.
func electionN(n int) map[string]string {
	var names, ids, row, roles []string
	for i := range n {
		names = append(names, fmt.Sprintf("%q", string(rune('a'+i))))
		ids = append(ids, fmt.Sprint(i))
		row = append(row, "false")
		roles = append(roles, `"follower"`)
	}
	matrix := strings.Repeat("["+strings.Join(row, " ")+"] ", n)
	return map[string]string{
		"names":  "[" + strings.Join(names, " ") + "]",
		"ids":    "[" + strings.Join(ids, " ") + "]",
		"quorum": fmt.Sprint(n/2 + 1),
		"init":   "[[[" + strings.Join(roles, " ") + "] [" + matrix + "]]]",
	}
}

func BenchmarkElection(b *testing.B) {
	for _, n := range []int{3, 4, 5} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			t := &testing.T{}
			m := election(t, electionN(n), Names{Invariants: []string{"one-leader"}})
			for b.Loop() {
				res, err := m.Explore(ExploreOptions{})
				if err != nil || res.Violation != nil {
					b.Fatal(err, res.Violation)
				}
				b.ReportMetric(float64(res.States), "states")
			}
		})
	}
}
