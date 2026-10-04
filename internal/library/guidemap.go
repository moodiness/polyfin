package library

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/xmltv"
)

// Errors of guide mapping.
var (
	ErrInvalidMode    = errors.New("unknown mapping mode")
	ErrInvalidMapping = errors.New("the catalog's guides do not have this guide channel")
	// errChannelsUnreachable is a catalog whose channels could not be
	// listed to map them.
	errChannelsUnreachable = errors.New("the catalog's channels could not be listed")
)

// Automatic mapping modes: AutomapUnmapped maps only the channels without
// a mapping, AutomapRemap every channel, dropping manual mappings; the
// automatic one, after every download, recomputes every mapping but the
// manual ones.
const (
	AutomapUnmapped  = "unmapped"
	AutomapRemap     = "remap"
	automapAutomatic = "automatic"
)

// AutomapResult counts a catalog's channels, those mapped after mapping,
// and those whose mapping changed.
type AutomapResult struct {
	Channels, Mapped, Changed int
}

// catalogChannels lists a live TV catalog's channels: an IPTV source's
// whole line-up, or what a Stremio catalog lists now.
func (s *Service) catalogChannels(ctx context.Context, c addons.LiveCatalog) ([]stremio.Meta, error) {
	if c.Addon.IPTV() {
		if s.iptv == nil {
			return nil, nil
		}
		return s.iptv.MappingChannels(ctx, c.Addon.ID)
	}
	settings := s.settings()
	v := view{deadline: s.now(), lookups: new(atomic.Int32), held: new(atomic.Bool),
		catalogLimit: settings.CatalogLimit, channelLimit: settings.ChannelLimit}
	src := source{addon: installed{addon: c.Addon, confined: c.Confined, shared: c.Scope.Owner == nil}, catalog: c.Catalog}
	metas, _, err := s.window(ctx, v, src, 0, v.limit(src))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	return slices.DeleteFunc(metas, func(m stremio.Meta) bool {
		duplicate := m.ID == "" || seen[m.ID]
		seen[m.ID] = true
		return duplicate
	}), nil
}

type guideMap struct {
	guide  *accounts.ID
	xmltv  *string
	manual bool
}

func (s *Service) catalogMaps(ctx context.Context, key addons.LibraryKey) (map[accounts.ID]guideMap, error) {
	rows, err := s.db.Query(ctx, `SELECT channel_id, guide_id, xmltv_id, manual FROM live_guide_maps
		WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`, key.AddonID, key.CatalogType, key.CatalogID)
	if err != nil {
		return nil, err
	}
	maps := map[accounts.ID]guideMap{}
	var id accounts.ID
	var m guideMap
	_, err = pgx.ForEachRow(rows, []any{&id, &m.guide, &m.xmltv, &m.manual}, func() error { maps[id] = m; return nil })
	return maps, err
}

// automap maps a catalog's channels to the channels of its guides (see
// xmltv.Matcher), every channel or those of only, in mode.
func (s *Service) automap(ctx context.Context, c addons.LiveCatalog, mode string, only map[accounts.ID]bool) (AutomapResult, error) {
	metas, err := s.catalogChannels(ctx, c)
	if err != nil {
		if ctx.Err() != nil {
			return AutomapResult{}, ctx.Err()
		}
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) {
			return AutomapResult{}, err
		}
		s.logger.Warn("A live TV catalog could not be listed to map its guides", "catalog", c.Catalog.ID, "error", err)
		return AutomapResult{}, errChannelsUnreachable
	}
	existing, err := s.catalogMaps(ctx, c.Key)
	if err != nil {
		return AutomapResult{}, err
	}
	matcher := xmltv.NewMatcher(s.settings().Language)
	ids := make([]accounts.ID, len(metas))
	for i, meta := range metas {
		ids[i] = itemID(channelKey(meta.ID))
		matcher.Add(meta.ID, meta.GuideID, meta.Name)
	}
	titles := make([]map[string]int, len(c.Guides))
	for position, g := range c.Guides {
		rows, err := s.db.Query(ctx, `SELECT xmltv_id, names FROM live_guide_channels WHERE guide_id = $1 AND generation = $2 ORDER BY xmltv_id`,
			g.ID, g.Generation)
		if err != nil {
			return AutomapResult{}, err
		}
		var channel xmltv.Channel
		if _, err := pgx.ForEachRow(rows, []any{&channel.ID, &channel.Names}, func() error {
			matcher.Declare(position, channel)
			return nil
		}); err != nil {
			return AutomapResult{}, err
		}
		counts := map[string]int{}
		rows, err = s.db.Query(ctx, `SELECT xmltv_id, count(DISTINCT title) FROM live_guide_programmes WHERE guide_id = $1 AND generation = $2
			GROUP BY xmltv_id`, g.ID, g.Generation)
		if err != nil {
			return AutomapResult{}, err
		}
		var id string
		var n int
		if _, err := pgx.ForEachRow(rows, []any{&id, &n}, func() error { counts[id] = n; return nil }); err != nil {
			return AutomapResult{}, err
		}
		titles[position] = counts
	}
	chosen := matcher.Choose(func(m xmltv.Match) int { return titles[m.Guide][m.ID] })
	var setChannels, setGuides []accounts.ID
	var setIDs []string
	var clear []accounts.ID
	result := AutomapResult{Channels: len(metas)}
	for i, id := range ids {
		if only != nil && !only[id] {
			continue
		}
		current, mapped := existing[id]
		if mode == AutomapUnmapped && mapped || mode == automapAutomatic && current.manual {
			continue
		}
		if chosen[i] == nil {
			if mapped {
				clear = append(clear, id)
				result.Changed++
			}
			continue
		}
		guide, xmltvID := c.Guides[chosen[i].Guide].ID, chosen[i].ID
		if !mapped || current.manual || current.guide == nil || *current.guide != guide || *current.xmltv != xmltvID {
			result.Changed++
		}
		setChannels, setGuides, setIDs = append(setChannels, id), append(setGuides, guide), append(setIDs, xmltvID)
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM live_guide_maps WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3
			AND channel_id = ANY($4)`, c.Key.AddonID, c.Key.CatalogType, c.Key.CatalogID, clear); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO live_guide_maps (addon_id, catalog_type, catalog_id, channel_id, guide_id, xmltv_id)
			SELECT $1, $2, $3, c, g, x FROM unnest($4::uuid[], $5::uuid[], $6::text[]) AS t(c, g, x)
			ON CONFLICT (addon_id, catalog_type, catalog_id, channel_id) DO UPDATE SET guide_id = excluded.guide_id,
				xmltv_id = excluded.xmltv_id, manual = false`,
			c.Key.AddonID, c.Key.CatalogType, c.Key.CatalogID, setChannels, setGuides, setIDs); err != nil {
			return err
		}
		if only == nil && !c.Addon.IPTV() {
			// Channels the catalog no longer lists lose their automatic mappings.
			if _, err := tx.Exec(ctx, `DELETE FROM live_guide_maps WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3
				AND NOT manual AND NOT (channel_id = ANY($4))`, c.Key.AddonID, c.Key.CatalogType, c.Key.CatalogID, ids); err != nil {
				return err
			}
		}
		err := tx.QueryRow(ctx, `SELECT count(*) FROM live_guide_maps WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3
			AND guide_id IS NOT NULL AND channel_id = ANY($4)`, c.Key.AddonID, c.Key.CatalogType, c.Key.CatalogID, ids).Scan(&result.Mapped)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE libraries SET guide_channels = $4, guide_matched = $5 WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3`,
			c.Key.AddonID, c.Key.CatalogType, c.Key.CatalogID, result.Channels, result.Mapped)
		return err
	})
	return result, err
}

// LineupChanged maps an IPTV source's channels to its guides again, after
// its line-up changed (see iptv.Service.OnChange).
func (s *Service) LineupChanged(ctx context.Context, source accounts.ID) {
	catalogs, err := s.addons.LiveCatalogs(ctx, false)
	if err != nil {
		return
	}
	for _, c := range catalogs {
		if c.Addon.ID == source {
			if _, err := s.automap(ctx, c, automapAutomatic, nil); err != nil && ctx.Err() == nil && !errors.Is(err, errChannelsUnreachable) {
				s.logger.Warn("An IPTV source's channels could not be mapped to its guides", "error", err)
			}
		}
	}
}

// CatalogGuides describes a live TV catalog's guides and how its channels
// are mapped: how many it has (when last mapped), those mapped, and those
// mapped (or unmapped) by hand.
type CatalogGuides struct {
	Guides                   []addons.Guide
	Channels, Mapped, Manual int
}

// CatalogGuides describes one of the scope's live TV catalogs' guides.
func (s *Service) CatalogGuides(ctx context.Context, scope addons.Scope, key addons.LibraryKey) (CatalogGuides, error) {
	c, err := s.addons.LiveCatalogOf(ctx, scope, key)
	if err != nil {
		return CatalogGuides{}, err
	}
	result := CatalogGuides{Guides: c.Guides}
	err = s.db.QueryRow(ctx, `SELECT l.guide_channels,
		(SELECT count(*) FROM live_guide_maps m WHERE m.addon_id = l.addon_id AND m.catalog_type = l.catalog_type AND m.catalog_id = l.catalog_id AND m.guide_id IS NOT NULL),
		(SELECT count(*) FROM live_guide_maps m WHERE m.addon_id = l.addon_id AND m.catalog_type = l.catalog_type AND m.catalog_id = l.catalog_id AND m.manual)
		FROM libraries l WHERE l.addon_id = $1 AND l.catalog_type = $2 AND l.catalog_id = $3`, key.AddonID, key.CatalogType, key.CatalogID).
		Scan(&result.Channels, &result.Mapped, &result.Manual)
	return result, err
}

// SetCatalogGuides replaces one of the scope's live TV catalogs' guides
// (see addons.Store.SetGuides), downloads the new ones and maps the
// catalog's channels again. A confined catalog's new addresses must not be
// on a local network.
func (s *Service) SetCatalogGuides(ctx context.Context, scope addons.Scope, key addons.LibraryKey, list []addons.GuideAddress) (CatalogGuides, error) {
	c, err := s.addons.LiveCatalogOf(ctx, scope, key)
	if err != nil {
		return CatalogGuides{}, err
	}
	if c.Confined {
		for _, entry := range list {
			if parsed, err := url.Parse(strings.TrimSpace(entry.URL)); entry.ID == nil && err == nil {
				if err := stremio.CheckPublic(ctx, parsed.Hostname()); err != nil {
					return CatalogGuides{}, err
				}
			}
		}
	}
	added, err := s.addons.SetGuides(ctx, scope, key, list)
	if err != nil {
		return CatalogGuides{}, err
	}
	if c, err = s.addons.LiveCatalogOf(ctx, scope, key); err != nil {
		return CatalogGuides{}, err
	}
	if c.Addon.Enabled {
		fresh := slices.DeleteFunc(slices.Clone(c.Guides), func(g addons.Guide) bool { return !slices.Contains(added, g.ID) })
		if err := s.refreshCatalog(ctx, c, fresh); err != nil {
			return CatalogGuides{}, err
		}
	}
	return s.CatalogGuides(ctx, scope, key)
}

// Automap maps one of the scope's live TV catalogs' channels in mode,
// AutomapUnmapped or AutomapRemap.
func (s *Service) Automap(ctx context.Context, scope addons.Scope, key addons.LibraryKey, mode string) (AutomapResult, error) {
	if mode != AutomapUnmapped && mode != AutomapRemap {
		return AutomapResult{}, ErrInvalidMode
	}
	c, err := s.addons.LiveCatalogOf(ctx, scope, key)
	if err != nil {
		return AutomapResult{}, err
	}
	result, err := s.automap(ctx, c, mode, nil)
	if errors.Is(err, errChannelsUnreachable) {
		err = stremio.ErrUnreachable
	}
	return result, err
}

// GuideProgramme is a programme of a guide channel.
type GuideProgramme struct {
	Title      string
	Start, End time.Time
}

// GuideChannel is a channel of a catalog's guide, with what it airs now.
type GuideChannel struct {
	Guide         accounts.ID
	GuidePosition int
	ID            string
	Names         []string
	Icon          string
	Now           *GuideProgramme
}

// GuideChannels lists the channels of one of the scope's live TV
// catalogs' guides, of one guide when guide is set, those whose names or
// identifier hold q, in guide order then name. It reports how many match.
func (s *Service) GuideChannels(ctx context.Context, scope addons.Scope, key addons.LibraryKey, q string, guide *accounts.ID, offset, limit int) (int, []GuideChannel, error) {
	if _, err := s.addons.LiveCatalogOf(ctx, scope, key); err != nil {
		return 0, nil, err
	}
	args := []any{key.AddonID, key.CatalogType, key.CatalogID, s.now()}
	filter := ""
	if guide != nil {
		args = append(args, *guide)
		filter += " AND g.id = $5"
	}
	if strings.TrimSpace(q) != "" {
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(iptv.Fold(strings.TrimSpace(q)))+"%")
		filter += " AND c.search LIKE $" + strconv.Itoa(len(args))
	}
	rows, err := s.db.Query(ctx, `SELECT g.id, g.position, c.xmltv_id, c.names, c.icon, p.title, p.starts_at, p.ends_at, count(*) OVER ()
		FROM live_guides g JOIN live_guide_channels c ON c.guide_id = g.id AND c.generation = g.generation
		LEFT JOIN LATERAL (SELECT title, starts_at, ends_at FROM live_guide_programmes p WHERE p.guide_id = g.id AND p.generation = g.generation
			AND p.xmltv_id = c.xmltv_id AND p.starts_at <= $4 AND p.ends_at > $4 ORDER BY p.starts_at DESC LIMIT 1) p ON true
		WHERE g.addon_id = $1 AND g.catalog_type = $2 AND g.catalog_id = $3`+filter+`
		ORDER BY g.position, lower(coalesce(c.names[1], c.xmltv_id)), c.xmltv_id OFFSET `+strconv.Itoa(max(offset, 0))+` LIMIT `+strconv.Itoa(cmp.Or(limit, 100)), args...)
	if err != nil {
		return 0, nil, err
	}
	total := 0
	channels, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (GuideChannel, error) {
		var c GuideChannel
		var title *string
		var start, end *time.Time
		if err := row.Scan(&c.Guide, &c.GuidePosition, &c.ID, &c.Names, &c.Icon, &title, &start, &end, &total); err != nil {
			return GuideChannel{}, err
		}
		if title != nil {
			c.Now = &GuideProgramme{Title: *title, Start: *start, End: *end}
		}
		return c, nil
	})
	if channels == nil {
		channels = []GuideChannel{}
	}
	return total, channels, err
}

// ChannelMapping is a catalog's channel with the guide channel it takes.
type ChannelMapping struct {
	Channel accounts.ID
	Name    string
	Number  int
	GuideID string
	Mapping *Mapping
}

// Mapping is a guide channel a channel takes: a guide and its channel and
// first name, both nil for a manual "no guide".
type Mapping struct {
	Guide        *accounts.ID
	GuideChannel *string
	Name         string
	Manual       bool
}

// Mapping states.
const (
	MappingAll      = "all"
	MappingMapped   = "mapped"
	MappingUnmapped = "unmapped"
	MappingManual   = "manual"
)

// mappingNames finds the first names of guide channels.
func (s *Service) mappingNames(ctx context.Context, maps map[accounts.ID]guideMap) (map[string]string, error) {
	var guides []accounts.ID
	var ids []string
	for _, m := range maps {
		if m.guide != nil {
			guides, ids = append(guides, *m.guide), append(ids, *m.xmltv)
		}
	}
	rows, err := s.db.Query(ctx, `SELECT c.guide_id, c.xmltv_id, coalesce(c.names[1], '') FROM unnest($1::uuid[], $2::text[]) AS t(g, x)
		JOIN live_guides g ON g.id = t.g JOIN live_guide_channels c ON c.guide_id = g.id AND c.generation = g.generation AND c.xmltv_id = t.x`,
		guides, ids)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	var guide accounts.ID
	var id, name string
	_, err = pgx.ForEachRow(rows, []any{&guide, &id, &name}, func() error { names[guide.String()+"\x00"+id] = name; return nil })
	return names, err
}

func describeMapping(m guideMap, mapped bool, names map[string]string) *Mapping {
	if !mapped {
		return nil
	}
	result := &Mapping{Guide: m.guide, GuideChannel: m.xmltv, Manual: m.manual}
	if m.guide != nil {
		result.Name = names[m.guide.String()+"\x00"+*m.xmltv]
	}
	return result
}

// Mappings lists one of the scope's live TV catalogs' channels with their
// mapping, in the catalog's order: all of them, or those mapped, unmapped
// or mapped by hand, those whose name or guide identifier holds q. It
// reports how many match.
func (s *Service) Mappings(ctx context.Context, scope addons.Scope, key addons.LibraryKey, state, q string, offset, limit int) (int, []ChannelMapping, error) {
	state = cmp.Or(state, MappingAll)
	if !slices.Contains([]string{MappingAll, MappingMapped, MappingUnmapped, MappingManual}, state) {
		return 0, nil, ErrInvalidMode
	}
	c, err := s.addons.LiveCatalogOf(ctx, scope, key)
	if err != nil {
		return 0, nil, err
	}
	metas, err := s.catalogChannels(ctx, c)
	if err != nil {
		return 0, nil, err
	}
	maps, err := s.catalogMaps(ctx, key)
	if err != nil {
		return 0, nil, err
	}
	needle := iptv.Fold(strings.TrimSpace(q))
	var matching []ChannelMapping
	for i, meta := range metas {
		id := itemID(channelKey(meta.ID))
		m, mapped := maps[id]
		switch {
		case state == MappingMapped && (!mapped || m.guide == nil),
			state == MappingUnmapped && mapped && m.guide != nil,
			state == MappingManual && (!mapped || !m.manual),
			needle != "" && !strings.Contains(iptv.Fold(meta.Name+"\n"+meta.GuideID), needle):
			continue
		}
		number := meta.ChannelNumber
		if number == 0 {
			number = i + 1
		}
		matching = append(matching, ChannelMapping{Channel: id, Name: meta.Name, Number: number, GuideID: meta.GuideID})
	}
	total := len(matching)
	offset = min(max(offset, 0), total)
	page := matching[offset:min(offset+cmp.Or(limit, 100), total)]
	names, err := s.mappingNames(ctx, maps)
	if err != nil {
		return 0, nil, err
	}
	for i := range page {
		m, mapped := maps[page[i].Channel]
		page[i].Mapping = describeMapping(m, mapped, names)
	}
	if page == nil {
		page = []ChannelMapping{}
	}
	return total, page, nil
}

// catalogChannel finds a channel of a catalog by its item identifier.
func (s *Service) catalogChannel(ctx context.Context, c addons.LiveCatalog, channel accounts.ID) (ChannelMapping, error) {
	metas, err := s.catalogChannels(ctx, c)
	if err != nil {
		return ChannelMapping{}, err
	}
	for i, meta := range metas {
		if itemID(channelKey(meta.ID)) == channel {
			number := cmp.Or(meta.ChannelNumber, i+1)
			return ChannelMapping{Channel: channel, Name: meta.Name, Number: number, GuideID: meta.GuideID}, nil
		}
	}
	return ChannelMapping{}, ErrNotFound
}

func (s *Service) describedMapping(ctx context.Context, key addons.LibraryKey, channel ChannelMapping) (ChannelMapping, error) {
	maps, err := s.catalogMaps(ctx, key)
	if err != nil {
		return ChannelMapping{}, err
	}
	names, err := s.mappingNames(ctx, maps)
	if err != nil {
		return ChannelMapping{}, err
	}
	m, mapped := maps[channel.Channel]
	channel.Mapping = describeMapping(m, mapped, names)
	return channel, nil
}

// SetMapping maps a channel of one of the scope's live TV catalogs by
// hand: to a channel of one of its guides, or, both nil, to no guide.
func (s *Service) SetMapping(ctx context.Context, scope addons.Scope, key addons.LibraryKey, channel accounts.ID, guide *accounts.ID, xmltvID *string) (ChannelMapping, error) {
	if (guide == nil) != (xmltvID == nil) {
		return ChannelMapping{}, ErrInvalidMapping
	}
	c, err := s.addons.LiveCatalogOf(ctx, scope, key)
	if err != nil {
		return ChannelMapping{}, err
	}
	described, err := s.catalogChannel(ctx, c, channel)
	if err != nil {
		return ChannelMapping{}, err
	}
	if guide != nil {
		var found bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM live_guides g JOIN live_guide_channels c ON c.guide_id = g.id AND c.generation = g.generation
			WHERE g.id = $1 AND g.addon_id = $2 AND g.catalog_type = $3 AND g.catalog_id = $4 AND c.xmltv_id = $5)`,
			*guide, key.AddonID, key.CatalogType, key.CatalogID, *xmltvID).Scan(&found); err != nil {
			return ChannelMapping{}, err
		}
		if !found {
			return ChannelMapping{}, ErrInvalidMapping
		}
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO live_guide_maps (addon_id, catalog_type, catalog_id, channel_id, guide_id, xmltv_id, manual)
		VALUES ($1, $2, $3, $4, $5, $6, true) ON CONFLICT (addon_id, catalog_type, catalog_id, channel_id)
		DO UPDATE SET guide_id = excluded.guide_id, xmltv_id = excluded.xmltv_id, manual = true`,
		key.AddonID, key.CatalogType, key.CatalogID, channel, guide, xmltvID); err != nil {
		return ChannelMapping{}, err
	}
	if err := s.countMapped(ctx, key); err != nil {
		return ChannelMapping{}, err
	}
	return s.describedMapping(ctx, key, described)
}

// ClearMapping drops a channel's manual mapping; it is mapped
// automatically again at once.
func (s *Service) ClearMapping(ctx context.Context, scope addons.Scope, key addons.LibraryKey, channel accounts.ID) (ChannelMapping, error) {
	c, err := s.addons.LiveCatalogOf(ctx, scope, key)
	if err != nil {
		return ChannelMapping{}, err
	}
	described, err := s.catalogChannel(ctx, c, channel)
	if err != nil {
		return ChannelMapping{}, err
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM live_guide_maps WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3 AND channel_id = $4`,
		key.AddonID, key.CatalogType, key.CatalogID, channel); err != nil {
		return ChannelMapping{}, err
	}
	if _, err := s.automap(ctx, c, automapAutomatic, map[accounts.ID]bool{channel: true}); err != nil && !errors.Is(err, errChannelsUnreachable) {
		return ChannelMapping{}, err
	}
	return s.describedMapping(ctx, key, described)
}

// countMapped updates how many of a catalog's channels are mapped.
func (s *Service) countMapped(ctx context.Context, key addons.LibraryKey) error {
	_, err := s.db.Exec(ctx, `UPDATE libraries l SET guide_matched = least(guide_channels, (SELECT count(*) FROM live_guide_maps m
		WHERE m.addon_id = l.addon_id AND m.catalog_type = l.catalog_type AND m.catalog_id = l.catalog_id AND m.guide_id IS NOT NULL))
		WHERE l.addon_id = $1 AND l.catalog_type = $2 AND l.catalog_id = $3`, key.AddonID, key.CatalogType, key.CatalogID)
	return err
}
