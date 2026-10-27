package iptv

import (
	"context"
	"errors"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// streamAgent is the User-Agent of a source's streams when their list
// names none: own, the one its administrator set, else BrowserUserAgent
// for an M3U playlist or an Xtream account (browser), none for a tuner,
// whose streams Polyfin requests as its own.
type streamAgent struct {
	own     string
	browser bool
}

// streamAgent reads a source's (see streamAgent).
func (s *Service) streamAgent(ctx context.Context, source accounts.ID) (streamAgent, error) {
	var agent streamAgent
	var kind string
	err := s.db.QueryRow(ctx, "SELECT i.user_agent, a.kind FROM iptv_sources i JOIN addons a ON a.id = i.addon_id WHERE i.addon_id = $1", source).
		Scan(&agent.own, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return streamAgent{}, nil
	}
	agent.browser = browsed(kind)
	return agent, err
}

// browsed reports whether the streams of a kind of source default to
// BrowserUserAgent: those of M3U playlists and Xtream accounts.
func browsed(kind string) bool {
	return kind == addons.KindM3U || kind == addons.KindXtream
}

// apply gives stream the headers it is requested with: those its list
// gives, and the source's User-Agent when they name none. The one its
// administrator set is a header the stream needs, as its list's are;
// BrowserUserAgent only goes with Polyfin's own requests (see
// stremio.Stream.UserAgent), so that an app that can reach the source
// itself is still sent there with its own.
func (a streamAgent) apply(stream *stremio.Stream, headers map[string]string) {
	if !hasHeader(headers, "User-Agent") {
		switch {
		case a.own != "":
			headers = maps.Clone(headers)
			if headers == nil {
				headers = map[string]string{}
			}
			headers["User-Agent"] = a.own
		case a.browser:
			stream.UserAgent = BrowserUserAgent
		}
	}
	if len(headers) > 0 {
		stream.BehaviorHints.ProxyHeaders = &stremio.ProxyHeaders{Request: maps.Clone(headers)}
	}
}

// hasHeader reports whether headers name name, whatever its letter case.
func hasHeader(headers map[string]string, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}
