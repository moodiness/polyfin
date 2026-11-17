package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// programIndex finds the XMLTV programmes of the guides by their
// identifier, which no item keeps (see unsavedProgram): an app opens a
// programme it listed before a restart, or asks for an identifier of
// nothing, another server's or a folder of its own. It is built from the
// guides on the first lookup that needs it, and again on the first lookup
// after a download or a mapping changes the guides (see guideState), one
// build at a time: lookups that need it meanwhile wait for it.
type programIndex struct {
	// building holds the one build under way.
	building chan struct{}
	// table is the index last built; nil before the first build.
	table *programTable
}

func newProgramIndex() programIndex {
	return programIndex{building: make(chan struct{}, 1)}
}

// programTable is the index of the guides in one state: each programme's
// identifier, sorted, with where it is in its guide. It is packed, about
// 28 bytes a programme and channel pair, columns rather than a map: a
// large guide maps a few thousand channels to over 100,000 programmes.
type programTable struct {
	state string
	// ids are the programmes' identifiers, sorted; starts, in
	// microseconds since 1970, and channels, indexes into maps, are
	// theirs.
	ids      []accounts.ID
	starts   []int64
	channels []uint32
	maps     []programChannel
}

// programChannel is a channel mapped to a guide channel: its Stremio
// identifier, and the guide and XMLTV channel its programmes are in.
type programChannel struct {
	channel string
	guide   accounts.ID
	xmltvID string
}

// find returns where the programme id is in its guide.
func (t *programTable) find(id accounts.ID) (programRef, bool) {
	i, found := slices.BinarySearchFunc(t.ids, id, compareIDs)
	if !found {
		return programRef{}, false
	}
	c := t.maps[t.channels[i]]
	return programRef{channel: c.channel, guide: c.guide, xmltvID: c.xmltvID, start: time.UnixMicro(t.starts[i]).UTC()}, true
}

// size is about the memory the table holds, in bytes.
func (t *programTable) size() int {
	n := len(t.ids)*16 + len(t.starts)*8 + len(t.channels)*4
	for _, c := range t.maps {
		n += 48 + len(c.channel) + len(c.xmltvID)
	}
	return n
}

func compareIDs(a, b accounts.ID) int { return bytes.Compare(a[:], b[:]) }

// programTable returns the index of the guides in state, building it when
// the one kept is of another state. ctx bounds the wait for a build under
// way as well as the build.
func (s *Service) programTable(ctx context.Context, state string) (*programTable, error) {
	select {
	case s.programs.building <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.programs.building }()
	if t := s.programs.table; t != nil && t.state == state {
		return t, nil
	}
	t, err := s.indexPrograms(ctx, state)
	if err != nil {
		return nil, err
	}
	s.programs.table = t
	return t, nil
}

// indexPrograms reads the programmes of the channels mapped to a guide,
// in the guides' current downloads, and indexes them by identifier: a
// hash of the channel's Stremio identifier and the programme's start (see
// guideItem). It reads one row per mapped channel, its starts gathered
// from the guide's index of programmes by channel: grouping the joined
// rows instead sorts them all, on disk past work_mem.
func (s *Service) indexPrograms(ctx context.Context, state string) (*programTable, error) {
	began := time.Now()
	rows, err := s.db.Query(ctx, `SELECT c.data->'meta'->>'id', m.guide_id, m.xmltv_id,
		coalesce((SELECT array_agg((extract(epoch FROM p.starts_at) * 1000000)::bigint) FROM live_guide_programmes p
			WHERE p.guide_id = g.id AND p.generation = g.generation AND p.xmltv_id = m.xmltv_id), '{}')
		FROM live_guide_maps m JOIN items c ON c.id = m.channel_id AND c.kind = 'channel'
		JOIN live_guides g ON g.id = m.guide_id
		WHERE c.data->'meta'->>'id' IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type entry struct {
		id      accounts.ID
		start   int64
		channel uint32
	}
	var entries []entry
	var maps []programChannel
	var key []byte
	for rows.Next() {
		var c programChannel
		var starts []int64
		if err := rows.Scan(&c.channel, &c.guide, &c.xmltvID, &starts); err != nil {
			return nil, err
		}
		channel := uint32(len(maps))
		maps = append(maps, c)
		// The key itemID hashes, as programKey and guideVideo make it.
		key = append(append(append(key[:0], "polyfin:item:program|"...), c.channel...), '|')
		prefix := len(key)
		for _, start := range starts {
			key = time.UnixMicro(start).UTC().AppendFormat(key[:prefix], time.RFC3339)
			sum := sha256.Sum256(key)
			e := entry{start: start, channel: channel}
			copy(e.id[:], sum[:16])
			entries = append(entries, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b entry) int { return compareIDs(a.id, b.id) })
	// A channel mapped twice lists its programmes twice: either names them.
	entries = slices.CompactFunc(entries, func(a, b entry) bool { return a.id == b.id })
	t := &programTable{state: state, ids: make([]accounts.ID, len(entries)), starts: make([]int64, len(entries)),
		channels: make([]uint32, len(entries)), maps: maps}
	for i, e := range entries {
		t.ids[i], t.starts[i], t.channels[i] = e.id, e.start, e.channel
	}
	s.logger.Debug("The guides' programmes were indexed", "programmes", len(t.ids), "channels", len(maps), "bytes", t.size(),
		"took", time.Since(began).Round(time.Millisecond))
	return t, nil
}
