package silo

import (
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// unknownOperations tells which operations of Silo's API the apps ask
// that Polyfin does not serve, as the Jellyfin API's handler tells those
// Jellyfin apps ask: each method and path shape at most once an hour, so
// that an app polling one does not flood the log.
type unknownOperations struct {
	logger *slog.Logger
	now    func() time.Time

	mu     sync.Mutex
	logged map[string]time.Time
}

const (
	unknownInterval = time.Hour
	// maxUnknown bounds the shapes remembered: past that many within the
	// hour, the requests come from a scanner, not from the apps.
	maxUnknown = 1000
	// maxLogged bounds each value logged.
	maxLogged = 256
)

func newUnknownOperations(logger *slog.Logger, now func() time.Time) *unknownOperations {
	return &unknownOperations{logger: logger, now: now, logged: map[string]time.Time{}}
}

func (u *unknownOperations) report(r *http.Request) {
	// The method and path alone, without identifiers: query strings carry
	// tokens and search terms.
	method, path := truncate(r.Method), truncate(pathShape(r.URL.Path))
	key := method + " " + path
	now := u.now()
	u.mu.Lock()
	if at, ok := u.logged[key]; ok && now.Sub(at) < unknownInterval {
		u.mu.Unlock()
		return
	}
	if len(u.logged) >= maxUnknown {
		maps.DeleteFunc(u.logged, func(_ string, at time.Time) bool { return now.Sub(at) >= unknownInterval })
	}
	if len(u.logged) >= maxUnknown {
		u.mu.Unlock()
		return
	}
	u.logged[key] = now
	u.mu.Unlock()
	u.logger.Info("A Silo app asked for something Polyfin does not serve", "method", method, "path", path)
}

// truncate keeps the first maxLogged bytes of s, without splitting a
// character, marking what it cut.
func truncate(s string) string {
	if len(s) <= maxLogged {
		return s
	}
	end := maxLogged
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "…"
}

// pathShape replaces the identifiers in a path with {id}, so that requests
// for different items are one operation, and no item or user shows in the
// log: numbers, 32-digit hexadecimal ids and dashed UUIDs, alone or before
// an extension.
func pathShape(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		name, extension := segment, ""
		if dot := strings.IndexByte(segment, '.'); dot > 0 {
			name, extension = segment[:dot], segment[dot:]
		}
		if isIdentifier(name) {
			segments[i] = "{id}" + extension
		}
	}
	return strings.Join(segments, "/")
}

func isIdentifier(segment string) bool {
	switch {
	case segment == "":
		return false
	case strings.Trim(segment, "0123456789") == "":
		return true
	case len(segment) == 36 && segment[8] == '-' && segment[13] == '-' && segment[18] == '-' && segment[23] == '-':
		segment = strings.ReplaceAll(segment, "-", "")
	}
	return len(segment) == 32 && strings.Trim(segment, "0123456789abcdefABCDEF") == ""
}
