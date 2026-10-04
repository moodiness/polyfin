package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/xmltv"
)

const (
	// GuideRefresh is how often an XMLTV guide is fetched again, and
	// GuideCheck how often the guides due are looked for (see
	// RefreshGuides).
	GuideRefresh = 12 * time.Hour
	GuideCheck   = 30 * time.Minute
	// A guide download must answer within guideAnswer, send something at
	// least every guideStall and end within guideTimeout.
	guideAnswer  = 30 * time.Second
	guideStall   = time.Minute
	guideTimeout = 10 * time.Minute
	// Programmes are kept from guidePast before a fetch to guideAhead days
	// after it.
	guidePast  = 24 * time.Hour
	guideAhead = 8
	// maxGuideProgrammes bounds the programmes a fetch keeps, held in
	// memory until the guide is read to its end.
	maxGuideProgrammes = 500_000
)

// Codes of the reasons a guide could not be fetched, which the admin app
// shows.
const (
	guideUnreachable         = "unreachable"
	guidePrivateNetwork      = "private_network"
	guideTooLarge            = "too_large"
	guideMalformed           = "malformed"
	guideChannelsUnreachable = "channels_unreachable"
)

var errTooManyProgrammes = errors.New("too many programmes")

// RefreshGuides fetches the XMLTV guides not fetched for GuideRefresh, or
// every guide when all is set, one after the other: each costs its source
// a download. The task scheduler runs it every GuideCheck, and an
// administrator runs it with all.
func (s *Service) RefreshGuides(ctx context.Context, all bool) error {
	sources, err := s.addons.GuideSources(ctx)
	if err != nil {
		return err
	}
	for _, src := range sources {
		if !all && src.CheckedAt != nil && s.now().Sub(*src.CheckedAt) < GuideRefresh {
			continue
		}
		if err := s.refreshGuide(ctx, src); err != nil {
			return err
		}
	}
	return nil
}

// RefreshGuide fetches the XMLTV guide of one of the scope's live TV
// catalogs now. How the fetch went is stored with the catalog; the error
// is only for a catalog without a guide (addons.ErrInvalidLibrary) or the
// database.
func (s *Service) RefreshGuide(ctx context.Context, scope addons.Scope, key addons.LibraryKey) error {
	src, err := s.addons.GuideSource(ctx, scope, key)
	if err != nil {
		return err
	}
	return s.refreshGuide(ctx, src)
}

// guideChannel is a channel of a catalog, as a guide is matched to it.
type guideChannel struct {
	id   accounts.ID
	name string
	// byID and byName are the guide channels it matched: by its Stremio ID,
	// and the first one declared under its name.
	byID, byName string
}

// refreshGuide fetches a catalog's guide and stores its programmes, once
// at a time for each address.
func (s *Service) refreshGuide(ctx context.Context, src addons.GuideSource) error {
	key := fmt.Sprintf("guide %v %s", src.Key, src.URL)
	_, err, _ := s.flight.Do(key, func() (any, error) {
		at := s.now()
		programmes, channels, matched, code := s.fetchGuide(ctx, src, at)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if code != "" {
			s.logger.Warn("A Live TV guide could not be fetched", "catalog", src.Catalog.ID, "reason", code)
		}
		err := s.addons.StoreGuideFetch(ctx, src.Key, src.URL, addons.GuideFetch{At: at, Channels: channels, Matched: matched, Error: code},
			func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, "DELETE FROM guide_programmes WHERE addon_id = $1 AND catalog_type = $2 AND catalog_id = $3",
					src.Key.AddonID, src.Key.CatalogType, src.Key.CatalogID); err != nil {
					return err
				}
				_, err := tx.CopyFrom(ctx, pgx.Identifier{"guide_programmes"},
					[]string{"channel_id", "addon_id", "catalog_type", "catalog_id", "starts_at", "ends_at", "title", "subtitle",
						"description", "categories", "season", "episode", "icon"},
					pgx.CopyFromSlice(len(programmes), func(i int) ([]any, error) {
						p := programmes[i]
						return []any{p.channel, src.Key.AddonID, src.Key.CatalogType, src.Key.CatalogID, p.Start, p.Stop, p.Title,
							p.SubTitle, p.Description, nonNilStrings(p.Categories), positive(p.Season), positive(p.Episode), p.Icon}, nil
					}))
				return err
			})
		return nil, err
	})
	return err
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func positive(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}

// storedProgramme is a programme of a guide, for the channel identified.
type storedProgramme struct {
	xmltv.Programme
	channel accounts.ID
}

// fetchGuide lists the catalog's channels, then reads its guide and keeps
// the programmes of the channels it covers within the window around at.
// code is empty on success, else the reason it failed.
//
// A guide channel matches a catalog channel by its identifier equal to the
// channel's Stremio ID, else by a display name equal to the channel's
// name once both are normalized (see xmltv.NormalizeName). Each channel
// takes one guide channel: the one matched by identifier, else the first
// declared under its name. XMLTV declares channels before programmes;
// a programme of a channel not declared yet only counts when its channel
// is a Stremio ID.
func (s *Service) fetchGuide(ctx context.Context, src addons.GuideSource, at time.Time) (kept []storedProgramme, channels, matched int, code string) {
	settings := s.settings()
	v := view{deadline: at, lookups: new(atomic.Int32), held: new(atomic.Bool),
		catalogLimit: settings.CatalogLimit, channelLimit: settings.ChannelLimit}
	catalog := source{addon: installed{addon: src.Addon, confined: src.Confined, shared: src.Scope.Owner == nil}, catalog: src.Catalog}
	metas, _, err := s.window(ctx, v, catalog, 0, v.limit(catalog))
	if err != nil {
		return nil, 0, 0, guideChannelsUnreachable
	}
	var list []*guideChannel
	byStremioID := map[string]*guideChannel{}
	byName := map[string][]*guideChannel{}
	for _, meta := range metas {
		if meta.ID == "" || byStremioID[meta.ID] != nil {
			continue
		}
		c := &guideChannel{id: itemID(channelKey(meta.ID)), name: meta.Name}
		list = append(list, c)
		byStremioID[meta.ID] = c
		if name := xmltv.NormalizeName(meta.Name); name != "" {
			byName[name] = append(byName[name], c)
		}
	}
	if len(list) == 0 {
		return nil, 0, 0, ""
	}

	from, to := at.Add(-guidePast), at.AddDate(0, 0, guideAhead)
	wanted := map[string]bool{}
	programmes := map[string][]xmltv.Programme{}
	count := 0
	err = s.readGuide(ctx, src, xmltv.Options{Language: settings.Language},
		func(channel xmltv.Channel) error {
			if c := byStremioID[channel.ID]; c != nil {
				c.byID, wanted[channel.ID] = channel.ID, true
			}
			for _, name := range channel.Names {
				for _, c := range byName[xmltv.NormalizeName(name)] {
					if c.byName == "" {
						c.byName, wanted[channel.ID] = channel.ID, true
					}
				}
			}
			return nil
		},
		func(p xmltv.Programme) error {
			if !p.Stop.After(from) || !p.Start.Before(to) {
				return nil
			}
			if !wanted[p.Channel] {
				c := byStremioID[p.Channel]
				if c == nil {
					return nil
				}
				c.byID, wanted[p.Channel] = p.Channel, true
			}
			if count++; count > maxGuideProgrammes {
				return errTooManyProgrammes
			}
			programmes[p.Channel] = append(programmes[p.Channel], p)
			return nil
		})
	switch {
	case err == nil:
	case errors.Is(err, stremio.ErrPrivateNetwork):
		return nil, 0, 0, guidePrivateNetwork
	case errors.Is(err, xmltv.ErrTooLarge), errors.Is(err, errTooManyProgrammes):
		return nil, 0, 0, guideTooLarge
	case errors.Is(err, xmltv.ErrMalformed):
		return nil, 0, 0, guideMalformed
	default:
		return nil, 0, 0, guideUnreachable
	}
	for _, c := range list {
		guide := c.byID
		if guide == "" {
			guide = c.byName
		}
		starts := map[int64]bool{}
		for _, p := range programmes[guide] {
			// A channel has one programme at a time from each start.
			if !starts[p.Start.UnixNano()] {
				starts[p.Start.UnixNano()] = true
				kept = append(kept, storedProgramme{Programme: p, channel: c.id})
			}
		}
		if len(starts) > 0 {
			matched++
		}
	}
	return kept, len(list), matched, ""
}

// readGuide downloads a guide and reads it (see xmltv.Read). The address
// never appears in errors: it may embed credentials.
func (s *Service) readGuide(ctx context.Context, src addons.GuideSource, options xmltv.Options,
	channel func(xmltv.Channel) error, programme func(xmltv.Programme) error) error {
	ctx, cancel := context.WithTimeout(ctx, guideTimeout)
	defer cancel()
	answered := time.AfterFunc(guideAnswer, cancel)
	response, err := s.client.Open(ctx, http.MethodGet, src.URL, http.Header{"Accept": {"application/xml, text/xml, */*"}}, src.Confined)
	answered.Stop()
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", stremio.ErrUnreachable, response.StatusCode)
	}
	body := &stallReader{r: response.Body, timer: time.AfterFunc(guideStall, cancel)}
	defer body.timer.Stop()
	return xmltv.Read(body, options, channel, programme)
}

// stallReader cancels a download, through its timer, when nothing comes
// for guideStall.
type stallReader struct {
	r     io.Reader
	timer *time.Timer
}

func (r *stallReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.timer.Reset(guideStall)
	}
	return n, err
}

// guidePrograms lists the programmes the XMLTV guides of the view's live
// TV catalogs give the channels that overlap [from, to), but those of
// skip, which have Native EPG programmes. A channel two catalogs list
// takes the programmes of the first one's guide.
func (s *Service) guidePrograms(ctx context.Context, v view, channels map[string]Item, skip map[string]bool, from, to time.Time) ([]Item, []record, error) {
	order := map[addons.LibraryKey]int{}
	for i, src := range v.channels {
		if src.guide {
			key := addons.LibraryKey{AddonID: src.addon.addon.ID, CatalogType: src.catalog.Type, CatalogID: src.catalog.ID}
			if _, ok := order[key]; !ok {
				order[key] = i
			}
		}
	}
	if len(order) == 0 {
		return nil, nil, nil
	}
	byID := map[accounts.ID]Item{}
	ids := make([]accounts.ID, 0, len(channels))
	for stremioID, channel := range channels {
		if !skip[stremioID] {
			byID[channel.ID] = channel
			ids = append(ids, channel.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT channel_id, addon_id, catalog_type, catalog_id, starts_at, ends_at, title, subtitle,
		description, categories, coalesce(season, 0), coalesce(episode, 0), icon
		FROM guide_programmes WHERE channel_id = ANY($1::uuid[]) AND ends_at > $2 AND starts_at < $3 ORDER BY starts_at`, ids, from, to)
	if err != nil {
		return nil, nil, err
	}
	type row struct {
		storedProgramme
		catalog int
	}
	var found []row
	first := map[accounts.ID]int{}
	var r row
	var key addons.LibraryKey
	if _, err := pgx.ForEachRow(rows, []any{&r.channel, &key.AddonID, &key.CatalogType, &key.CatalogID, &r.Start, &r.Stop, &r.Title,
		&r.SubTitle, &r.Description, &r.Categories, &r.Season, &r.Episode, &r.Icon}, func() error {
		index, ok := order[key]
		if !ok {
			return nil
		}
		r.catalog = index
		if current, seen := first[r.channel]; !seen || index < current {
			first[r.channel] = index
		}
		found = append(found, r)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	var items []Item
	var records []record
	for _, r := range found {
		if first[r.channel] != r.catalog {
			continue
		}
		channel := byID[r.channel]
		video := guideVideo(r.Programme)
		program, ok := programItem(channel, video, r.SubTitle)
		if !ok {
			continue
		}
		items = append(items, program)
		records = append(records, record{ID: program.ID, Key: programKey(channel.StremioID, programmeID(video)), Kind: KindProgram,
			Parent: &channel.ID, Channel: channel.StremioID, Video: &video, Confined: v.channels[r.catalog].addon.confined,
			EpisodeTitle: r.SubTitle})
	}
	return items, records, nil
}

// guideVideo describes an XMLTV programme as the Native EPG does, which
// finds it again (see program). It has no identifier: a programme is
// identified by its start.
func guideVideo(p xmltv.Programme) stremio.Video {
	return stremio.Video{
		Title:     p.Title,
		Overview:  p.Description,
		Thumbnail: p.Icon,
		Season:    stremio.Number(p.Season),
		Episode:   stremio.Number(p.Episode),
		StartTime: p.Start.UTC().Format(time.RFC3339),
		EndTime:   p.Stop.UTC().Format(time.RFC3339),
		Genres:    slices.Clip(p.Categories),
	}
}
