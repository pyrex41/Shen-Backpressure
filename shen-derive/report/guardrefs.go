package report

import (
	"bufio"
	"os"
	"strings"
)

// readGuardLines scans a shengen-emitted guards file for
// "// Shen: (datatype NAME)" comments and returns a map from Shen
// datatype name to the 1-based line number of the comment. The file
// is scanned best-effort: missing or unreadable files yield an empty
// map and a nil error so the discharge-report classifier can still
// produce useful output without code references.
func readGuardLines(path string) (map[string]int, error) {
	out := map[string]int{}
	if path == "" {
		return out, nil
	}
	f, err := os.Open(path)
	if err != nil {
		// Best-effort: missing guard file is not fatal.
		return out, nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	const prefix = "// Shen: (datatype "
	const suffix = ")"
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// Point at the next non-blank line — the type declaration —
		// rather than the comment itself, to mirror what readers
		// expect from a "guards_gen.go:42" reference.
		if scanner.Scan() {
			lineNo++
			out[name] = lineNo
		} else {
			out[name] = lineNo
		}
	}
	return out, nil
}
