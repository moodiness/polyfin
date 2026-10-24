package admin

import (
	"net/http"
	"runtime/pprof"
	"time"
)

// downloadGoroutines sends what every goroutine of the server is doing,
// with its stack and how long it has waited, as Go prints it when the
// program ends on a panic: what to send with a report of a request that
// never answers. It holds no address or secret the log would redact, but
// the paths of the source files.
func (h *handler) downloadGoroutines(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="polyfin-goroutines-`+time.Now().UTC().Format("20060102-150405")+`.txt"`)
	w.Header().Set("Cache-Control", "no-store")
	_ = pprof.Lookup("goroutine").WriteTo(w, 2)
}
