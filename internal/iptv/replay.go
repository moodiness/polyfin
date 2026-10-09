package iptv

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A channel's archive is that of the first of its enabled streams, in its
// order, whose entry the provider keeps past programmes of: archiveSQL is
// its number of days, NULL for none, of l joined as in channelColumns.
const archiveSQL = `(SELECT e.catchup_days FROM iptv_streams s JOIN iptv_entries e ON e.addon_id = s.addon_id AND e.key = s.key
	WHERE s.addon_id = l.addon_id AND s.channel_id = l.id AND s.enabled AND e.catchup <> '' AND e.catchup_days > 0
	ORDER BY ` + streamOrder + ` LIMIT 1)`

// Archives maps the Stremio identifiers of the channels a source shows
// that have an archive, which their provider keeps past programmes in, to
// the days it reaches back. The map is shared: callers do not change it.
func (s *Service) Archives(ctx context.Context, source accounts.ID) (map[string]int, error) {
	s.mu.Lock()
	archives, ok := s.archives[source]
	s.mu.Unlock()
	if ok {
		return archives, nil
	}
	if err := s.ensureLineup(ctx, source); err != nil {
		return nil, err
	}
	result, err, _ := s.flight.Do("archives "+source.String(), func() (any, error) {
		rows, err := s.db.Query(ctx, `SELECT id, days FROM (SELECT l.id, `+archiveSQL+` AS days `+channelJoins+`
			WHERE l.addon_id = $1 AND i.live_tv AND `+shownSQL+`) a WHERE days IS NOT NULL`, source)
		if err != nil {
			return nil, err
		}
		archives := map[string]int{}
		var id string
		var days int
		if _, err := pgx.ForEachRow(rows, []any{&id, &days}, func() error {
			archives[prefix(source)+id] = days
			return nil
		}); err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.archives[source] = archives
		s.mu.Unlock()
		return archives, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]int), nil
}

// GuideArchives maps the channels of an XMLTV guide that line-up channels
// with an archive are mapped to, by their guide identifier, to the most
// days those archives reach back: the past programmes of the guide worth
// keeping that long. The channels of the guide's source not mapped yet,
// as on the guide's first download, which maps them once it is read,
// count by the guide identifier their list gives.
func (s *Service) GuideArchives(ctx context.Context, guide accounts.ID) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `SELECT xmltv_id, max(days) FROM (
			SELECT m.xmltv_id, a.days FROM live_guide_maps m
			JOIN iptv_lineup l ON l.addon_id = m.addon_id AND l.item_id = m.channel_id
			CROSS JOIN LATERAL (SELECT `+archiveSQL+` AS days) a
			WHERE m.guide_id = $1
			UNION ALL
			SELECT l.guide_id, a.days FROM live_guides g JOIN iptv_lineup l ON l.addon_id = g.addon_id
			CROSS JOIN LATERAL (SELECT `+archiveSQL+` AS days) a
			WHERE g.id = $1 AND l.guide_id <> ''
				AND NOT EXISTS (SELECT 1 FROM live_guide_maps m WHERE m.addon_id = l.addon_id AND m.channel_id = l.item_id)
		) t WHERE days IS NOT NULL GROUP BY xmltv_id`, guide)
	if err != nil {
		return nil, err
	}
	archives := map[string]int{}
	var id string
	var days int
	_, err = pgx.ForEachRow(rows, []any{&id, &days}, func() error {
		archives[id] = days
		return nil
	})
	return archives, err
}

// replayKey starts the identifier of a replay within its source.
const replayKey = "replay:"

// ReplayID is the Stremio identifier of the programme of a source's
// channel from start to end, played from its provider's archive: the
// channel's identifier with the programme's times. ok is false for a
// channel no IPTV source lists.
func ReplayID(channel string, start, end time.Time) (string, bool) {
	rest, ok := strings.CutPrefix(channel, IDPrefix)
	if !ok {
		return "", false
	}
	source, id, ok := strings.Cut(rest, ":")
	if !ok || id == "" {
		return "", false
	}
	return IDPrefix + source + ":" + replayKey + strconv.FormatInt(start.Unix(), 10) + "-" + strconv.FormatInt(end.Unix(), 10) + ":" + id, true
}

// parseReplay reads a replay's key within its source (see ReplayID).
func parseReplay(key string) (channel string, start, end time.Time, ok bool) {
	rest, ok := strings.CutPrefix(key, replayKey)
	if !ok {
		return "", time.Time{}, time.Time{}, false
	}
	times, channel, ok := strings.Cut(rest, ":")
	from, to, found := strings.Cut(times, "-")
	first, err1 := strconv.ParseInt(from, 10, 64)
	last, err2 := strconv.ParseInt(to, 10, 64)
	if !ok || !found || err1 != nil || err2 != nil || channel == "" || last <= first {
		return "", time.Time{}, time.Time{}, false
	}
	return channel, time.Unix(first, 0).UTC(), time.Unix(last, 0).UTC(), true
}

// replayStreams lists the stream of a replay: the address, in its
// provider's archive, of the programme of a channel that shows and has an
// archive, when the programme ended within the days the archive reaches.
// The address holds the account's credentials: it is never logged.
func (s *Service) replayStreams(ctx context.Context, source accounts.ID, key string) ([]stremio.Stream, error) {
	channel, start, end, ok := parseReplay(key)
	if !ok {
		return nil, nil
	}
	var name, stream, kind, template, zone, sourceKind string
	var days int
	var headers map[string]string
	err := s.db.QueryRow(ctx, `SELECT coalesce(l.name, l.provider_name), e.url, e.headers, e.catchup, e.catchup_days, e.catchup_source,
		i.timezone, a.kind
		FROM iptv_lineup l JOIN iptv_sources i ON i.addon_id = l.addon_id JOIN addons a ON a.id = l.addon_id
		JOIN iptv_categories c ON c.id = coalesce(l.moved_to, l.category_id)
		JOIN iptv_streams s ON s.addon_id = l.addon_id AND s.channel_id = l.id AND s.enabled
		JOIN iptv_entries e ON e.addon_id = s.addon_id AND e.key = s.key AND e.catchup <> '' AND e.catchup_days > 0
		WHERE l.addon_id = $1 AND l.id = $2 AND i.live_tv AND `+shownSQL+` ORDER BY `+streamOrder+` LIMIT 1`, source, channel).
		Scan(&name, &stream, &headers, &kind, &days, &template, &zone, &sourceKind)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	if end.After(now) || start.Before(now.Add(-time.Duration(days)*24*time.Hour)) {
		return nil, nil
	}
	// An Xtream account's timeshift is written in its server's zone; an M3U
	// playlist's dates in UTC.
	if sourceKind != addons.KindXtream {
		zone = ""
	}
	address, ok := catchupAddress(Catchup{Type: kind, Days: days, Source: template}, stream, start, end, now, providerZone(zone))
	if !ok {
		return nil, nil
	}
	result := stremio.Stream{Name: name, Description: "Replay", URL: address}
	if len(headers) > 0 {
		result.BehaviorHints.ProxyHeaders = &stremio.ProxyHeaders{Request: headers}
	}
	return []stremio.Stream{result}, nil
}
