package appimport

import (
	"testing"

	"github.com/moodiness/polyfin/internal/metrics"
)

// The imports of an app connection count in the metrics, by app and
// outcome. Sequential: the other tests import connections to the same
// app, which count in the same series.
func TestImportsCountInTheMetrics(t *testing.T) {
	h := newConnectionHarness(t)
	root, alice := h.user(t, "root", true), h.user(t, "alice", false)
	account := Account{Name: "alice@example.com", Profiles: []Profile{
		{ID: "1", Name: "Main", Entries: []Entry{{Title: ref(t, "tt0000001", true, 0, 0), Played: true}}},
	}}
	const secret = "counted-secret-0123456789abcdef"
	h.source.set(func(f *fakeSource) { f.account, f.accepted[secret] = account, true })
	count := func(outcome string) float64 {
		return metrics.Value("polyfin_imports_total", map[string]string{"service": "stremio", "outcome": outcome})
	}
	succeeded, failed := count(metrics.Success), count(metrics.Failure)

	h.connected(t, root, alice, account, SignIn{Secret: secret, Owned: true}, Choice{Profile: "1"})
	h.importedNow(t, alice)
	if got := count(metrics.Success); got != succeeded+2 {
		t.Errorf("imports that succeeded: %v, before %v", got, succeeded)
	}
	h.source.set(func(f *fakeSource) { clear(f.accepted) })
	h.importedNow(t, alice)
	if got := count(metrics.Failure); got != failed+1 {
		t.Errorf("imports that failed: %v, before %v", got, failed)
	}
}
