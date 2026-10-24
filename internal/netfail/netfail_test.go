package netfail

import "strings"

// failure names how err failed by this package's reports, joined by
// commas: a socket error makes exactly one of them true.
func failure(err error) string {
	var names []string
	for _, report := range []struct {
		name string
		is   func(error) bool
	}{
		{"refused", Refused},
		{"unreachable", Unreachable},
		{"timed out", TimedOut},
		{"reset", Reset},
		{"broken pipe", BrokenPipe},
	} {
		if report.is(err) {
			names = append(names, report.name)
		}
	}
	return strings.Join(names, ",")
}
