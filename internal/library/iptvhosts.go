package library

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/addons"
)

// iptvHostsLife is how long the hosts of IPTV sources are read once for.
const iptvHostsLife = 10 * time.Minute

// iptvHosts remembers the hosts of IPTV sources' addresses, read from the
// sources, and the hosts channel logos were served from. Its zero value is
// ready to use.
type iptvHosts struct {
	mu      sync.Mutex
	read    time.Time
	sources map[string]bool
	logos   map[string]bool
}

// logo remembers the host of a channel logo's address.
func (h *iptvHosts) logo(address string) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.logos == nil {
		h.logos = map[string]bool{}
	}
	// Logos come from a few hosts; the bound only guards against a list
	// naming endless ones.
	if len(h.logos) < 10000 {
		h.logos[parsed.Host] = true
	}
}

// IPTVHost reports whether host serves an IPTV source, whoever added it,
// or the logos of its channels: such hosts are often the provider's panel,
// which refuses many requests at once.
func (s *Service) IPTVHost(ctx context.Context, host string) bool {
	s.iptvHosts.mu.Lock()
	defer s.iptvHosts.mu.Unlock()
	if s.iptvHosts.logos[host] {
		return true
	}
	if time.Since(s.iptvHosts.read) > iptvHostsLife {
		rows, err := s.db.Query(ctx, "SELECT manifest_url FROM addons WHERE kind IN ($1, $2)", addons.KindM3U, addons.KindXtream)
		if err == nil {
			sources := map[string]bool{}
			var address string
			if _, err = pgx.ForEachRow(rows, []any{&address}, func() error {
				if parsed, err := url.Parse(address); err == nil && parsed.Host != "" {
					sources[parsed.Host] = true
				}
				return nil
			}); err == nil {
				s.iptvHosts.sources, s.iptvHosts.read = sources, time.Now()
			}
		}
	}
	return s.iptvHosts.sources[host]
}
