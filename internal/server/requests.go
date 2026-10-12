package server

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/metrics"
)

// countRequests counts every request in the metrics, by the API it is
// of, with the status of its answer and how long it took: until the
// handler returned, the whole body of a stream sent. It runs inside
// underJellyfinBase, and so sees paths without the base.
func countRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		status := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(status, r)
		metrics.HTTPRequest(apiOf(r), status.code(), time.Since(started))
	})
}

// apiOf names the API a request is of: the web client's, under
// jellyfinBase too; at the root only (see rootOnly), the admin app's, its
// API included, and Polyfin's own monitoring routes; and the Jellyfin
// API, which serves every other path.
func apiOf(r *http.Request) string {
	path, root := r.URL.Path, basePath(r) == ""
	switch {
	case path == "/web" || strings.HasPrefix(path, webPrefix):
		return metrics.APIWeb
	case root && (path == "/admin" || strings.HasPrefix(path, adminPrefix)):
		return metrics.APIAdmin
	case root && (path == "/healthz" || path == "/readyz" || path == "/metrics"):
		return metrics.APIOther
	}
	return metrics.APIJellyfin
}

// statusWriter keeps the status of the answer written through it. It
// passes on what the writers below it offer: streams flushed, files
// sent by the system, WebSockets taken over.
type statusWriter struct {
	http.ResponseWriter
	status   int
	hijacked bool
}

// code is the status of the answer: 101 for a connection taken over, as
// a WebSocket's, and 200 for an answer the handler left empty.
func (w *statusWriter) code() int {
	switch {
	case w.status != 0:
		return w.status
	case w.hijacked:
		return http.StatusSwitchingProtocols
	}
	return http.StatusOK
}

func (w *statusWriter) WriteHeader(status int) {
	// Informational answers, such as 103, come before the final one.
	if w.status == 0 && status >= http.StatusOK {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusWriter) ReadFrom(r io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if from, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return from.ReadFrom(r)
	}
	return io.Copy(struct{ io.Writer }{w.ResponseWriter}, r)
}

func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// Unwrap returns the underlying writer, for http.ResponseController.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
