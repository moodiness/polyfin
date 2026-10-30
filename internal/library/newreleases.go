package library

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// New episodes and seasons.
//
// A series' poster may tell that an episode of it came out within the
// last newReleaseDays days, or that a season of it started within them
// (see Settings.PosterBadges). Both are read from the descriptions of the
// series kept in the database, whichever addon gave them, never asking an
// addon: a series none is kept of tells nothing. What is read is
// remembered for releaseStatusLife.

// ReleaseStatus is what a series' poster tells of its newest episodes.
type ReleaseStatus int

const (
	// NoRelease tells nothing.
	NoRelease ReleaseStatus = iota
	// NewEpisode tells an episode came out within newReleaseDays days.
	NewEpisode
	// NewSeason tells a season, other than the first, whose first
	// episode came out within newReleaseDays days.
	NewSeason
)

const (
	newReleaseDays    = 30
	releaseStatusLife = 10 * time.Minute
)

// ReleaseStatuses returns what the posters of the series among items
// tell, by item, leaving out those telling nothing.
func (s *Service) ReleaseStatuses(ctx context.Context, items []Item) (map[accounts.ID]ReleaseStatus, error) {
	result := map[accounts.ID]ReleaseStatus{}
	var types, ids []string
	wanted := map[string][]accounts.ID{}
	for _, item := range items {
		if item.Kind != KindSeries || item.StremioID == "" {
			continue
		}
		key := item.StremioType + "|" + item.StremioID
		if status, ok := s.releaseStatuses.Get(key); ok {
			if status != NoRelease {
				result[item.ID] = status
			}
			continue
		}
		if _, listed := wanted[key]; !listed {
			types, ids = append(types, item.StremioType), append(ids, item.StremioID)
		}
		wanted[key] = append(wanted[key], item.ID)
	}
	if len(wanted) == 0 {
		return result, nil
	}
	now := s.now()
	since := now.AddDate(0, 0, -newReleaseDays)
	// Dates are compared as text, as ISO 8601 sorts: the episodes from
	// the first day of the window on come whole, and of the earlier
	// ones only their seasons.
	rows, err := s.db.Query(ctx, `SELECT m.meta_type, m.meta_id,
			COALESCE((SELECT jsonb_agg(v) FROM jsonb_array_elements(m.meta->'videos') v WHERE v->>'released' >= $3), '[]'),
			COALESCE((SELECT jsonb_agg(DISTINCT v->'season') FROM jsonb_array_elements(m.meta->'videos') v
				WHERE v->>'released' < $3 AND jsonb_typeof(v->'season') IN ('number', 'string')), '[]')
		FROM metas m
		WHERE m.addon_id = ANY(ARRAY(SELECT id FROM addons)) AND m.meta_type = ANY($1) AND m.meta_id = ANY($2)
			AND m.fetched_at > $4 AND jsonb_typeof(m.meta->'videos') = 'array'`,
		types, ids, since.UTC().Format(time.DateOnly), now.Add(-metaKept))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[string]ReleaseStatus{}
	for rows.Next() {
		var metaType, metaID string
		var recentJSON, olderJSON []byte
		if err := rows.Scan(&metaType, &metaID, &recentJSON, &olderJSON); err != nil {
			return nil, err
		}
		var recent []stremio.Video
		var older []stremio.Number
		if json.Unmarshal(recentJSON, &recent) != nil || json.Unmarshal(olderJSON, &older) != nil {
			continue
		}
		key := metaType + "|" + metaID
		found[key] = max(found[key], releaseStatus(recent, older, since, now))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for key, items := range wanted {
		status := found[key]
		s.releaseStatuses.Put(key, status)
		if status == NoRelease {
			continue
		}
		for _, id := range items {
			result[id] = status
		}
	}
	return result, nil
}

// releaseStatus tells what a series' poster shows of its episodes
// released from since on, recent, the seasons of its earlier episodes
// being older: a new season when the latest season of the recent ones,
// past the first, has none earlier, else a new episode when any came out.
// Specials, season 0, are often extras: they tell nothing.
func releaseStatus(recent []stremio.Video, older []stremio.Number, since, now time.Time) ReleaseStatus {
	latest, seen := 0, false
	for _, video := range recent {
		date := parseDate(video.Released)
		if video.Season <= 0 || date == nil || date.Before(since) || date.After(now) || !released(video, now) {
			continue
		}
		seen = true
		latest = max(latest, int(video.Season))
	}
	switch {
	case !seen:
		return NoRelease
	case latest > 1 && !slices.Contains(older, stremio.Number(latest)):
		return NewSeason
	}
	return NewEpisode
}
