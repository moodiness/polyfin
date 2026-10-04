package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/xmltv"
)

const (
	// GuideCheck is how often the guides due are looked for (see
	// RefreshGuides); they are due after the settings' LiveTvRefreshHours.
	GuideCheck = 30 * time.Minute
	// A guide download must answer within guideAnswer, send something at
	// least every guideStall and end within guideTimeout.
	guideAnswer  = 30 * time.Second
	guideStall   = time.Minute
	guideTimeout = 10 * time.Minute
	// Programmes are kept from guidePast before a download to guideAhead
	// days after it.
	guidePast  = 24 * time.Hour
	guideAhead = 8
	// maxGuideProgrammes and maxGuideChannels bound what a download keeps;
	// programmes are written as they are read, channels kept in memory
	// until the guide's end.
	maxGuideProgrammes = 3_000_000
	maxGuideChannels   = 200_000
)

// Codes of the reasons a guide could not be fetched, which the admin app
// shows.
const (
	guideUnreachable    = "unreachable"
	guidePrivateNetwork = "private_network"
	guideTooLarge       = "too_large"
	guideMalformed      = "malformed"
	// guideChannelsUnreachable is a guide fetched for a catalog whose
	// channels could not be listed to map them.
	guideChannelsUnreachable = "channels_unreachable"
)

var errTooManyProgrammes = errors.New("too many programmes")

// SpoolGuidesIn sets the folder XMLTV guides in ZIP archives are written
// to while they are read, which reading one needs: a folder of Polyfin's
// cache, writable in a read-only container. What a previous run left there
// is removed. It is called before guides are fetched.
func (s *Service) SpoolGuidesIn(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	s.guideDir = dir
	return nil
}

// RefreshGuides fetches the XMLTV guides not fetched for the settings'
// LiveTvRefreshHours, or every guide when all is set, one after the other:
// each costs its source a download. A catalog that fetched one maps its
// channels to its guides again. The task scheduler runs it every
// GuideCheck, and an administrator runs it with all.
func (s *Service) RefreshGuides(ctx context.Context, all bool) error {
	catalogs, err := s.addons.LiveCatalogs(ctx, true)
	if err != nil {
		return err
	}
	interval := time.Duration(s.settings().LiveTvRefreshHours) * time.Hour
	for _, c := range catalogs {
		due := slices.DeleteFunc(slices.Clone(c.Guides), func(g addons.Guide) bool {
			return !all && g.CheckedAt != nil && s.now().Sub(*g.CheckedAt) < interval
		})
		if len(due) > 0 {
			if err := s.refreshCatalog(ctx, c, due); err != nil {
				return err
			}
		}
	}
	return nil
}

// RefreshGuide fetches every XMLTV guide of one of the scope's live TV
// catalogs now, then maps its channels again. How each download went is
// stored with its guide; the error is only for an unknown catalog
// (addons.ErrInvalidLibrary) or the database. A turned-off addon's guides
// are kept, and fetched once it is on.
func (s *Service) RefreshGuide(ctx context.Context, scope addons.Scope, key addons.LibraryKey) error {
	c, err := s.addons.LiveCatalogOf(ctx, scope, key)
	if err != nil || !c.Addon.Enabled {
		return err
	}
	return s.refreshCatalog(ctx, c, c.Guides)
}

// refreshCatalog fetches guides of a catalog, then maps its channels again.
func (s *Service) refreshCatalog(ctx context.Context, c addons.LiveCatalog, guides []addons.Guide) error {
	ids := make([]accounts.ID, 0, len(guides))
	for _, g := range guides {
		if err := s.fetchGuide(ctx, c, g); err != nil {
			return err
		}
		ids = append(ids, g.ID)
	}
	// The downloads made new generations current.
	c, err := s.addons.LiveCatalogOf(ctx, c.Scope, c.Key)
	if err != nil {
		return err
	}
	_, err = s.automap(ctx, c, automapAutomatic, nil)
	if errors.Is(err, errChannelsUnreachable) {
		// The guides fetched are kept, but tell they map nothing.
		_, err = s.db.Exec(ctx, "UPDATE live_guides SET error = $2 WHERE id = ANY($1) AND error = ''", ids, guideChannelsUnreachable)
	}
	return err
}

// fetchGuide downloads a guide of a catalog and stores its channels and
// the programmes within the window around the download, once at a time
// for each guide. They are written under the guide's next generation as
// the guide is read, then made the current one; a failure keeps the last
// download's and records why.
func (s *Service) fetchGuide(ctx context.Context, c addons.LiveCatalog, g addons.Guide) error {
	_, err, _ := s.flight.Do("guide "+g.ID.String(), func() (any, error) {
		at := s.now()
		generation := g.Generation + 1
		// What an interrupted download left is dropped first.
		for _, table := range []string{"live_guide_programmes", "live_guide_channels"} {
			if _, err := s.db.Exec(ctx, "DELETE FROM "+table+" WHERE guide_id = $1 AND generation <> $2", g.ID, g.Generation); err != nil {
				return nil, err
			}
		}
		channels, programmes, err := s.downloadGuide(ctx, c, g, generation, at)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		code := ""
		switch {
		case err == nil:
		case errors.Is(err, stremio.ErrPrivateNetwork):
			code = guidePrivateNetwork
		case errors.Is(err, xmltv.ErrTooLarge), errors.Is(err, errTooManyProgrammes):
			code = guideTooLarge
		case errors.Is(err, xmltv.ErrMalformed):
			code = guideMalformed
		default:
			var pgErr interface{ SQLState() string }
			if errors.As(err, &pgErr) {
				return nil, err
			}
			code = guideUnreachable
		}
		if code != "" {
			s.logger.Warn("A Live TV guide could not be fetched", "catalog", c.Catalog.ID, "reason", code)
			for _, table := range []string{"live_guide_programmes", "live_guide_channels"} {
				if _, err := s.db.Exec(ctx, "DELETE FROM "+table+" WHERE guide_id = $1 AND generation = $2", g.ID, generation); err != nil {
					return nil, err
				}
			}
			_, err := s.db.Exec(ctx, "UPDATE live_guides SET checked_at = $2, error = $3 WHERE id = $1", g.ID, at, code)
			return nil, err
		}
		return nil, pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `UPDATE live_guides SET generation = $2, checked_at = $3, fetched_at = $3, error = '', channels = $4,
				programmes = $5 WHERE id = $1`, g.ID, generation, at, channels, programmes)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			for _, table := range []string{"live_guide_programmes", "live_guide_channels"} {
				if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE guide_id = $1 AND generation <> $2", g.ID, generation); err != nil {
					return err
				}
			}
			return nil
		})
	})
	return err
}

// programmeRows feeds the programmes read from a guide to a COPY as they
// come.
type programmeRows struct {
	rows    <-chan []any
	current []any
}

func (p *programmeRows) Next() bool {
	row, ok := <-p.rows
	p.current = row
	return ok
}

func (p *programmeRows) Values() ([]any, error) { return p.current, nil }

func (p *programmeRows) Err() error { return nil }

// downloadGuide reads a guide and writes its channels and programmes under
// generation. It reports how many it wrote.
func (s *Service) downloadGuide(ctx context.Context, c addons.LiveCatalog, g addons.Guide, generation int, at time.Time) (int, int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	from, to := at.Add(-guidePast), at.AddDate(0, 0, guideAhead)
	rows := make(chan []any, 256)
	var channels [][]any
	var readErr error
	done := make(chan struct{})
	count := 0
	// Guides may give programmes for channels they do not describe: those
	// are channels without names, which identifiers still match.
	programmeChannels := map[string]bool{}
	seen := map[string]bool{}
	go func() {
		defer close(done)
		defer close(rows)
		readErr = s.readGuide(ctx, c, g.URL, xmltv.Options{Language: s.settings().Language, SpoolDir: s.guideDir},
			func(channel xmltv.Channel) error {
				if seen[channel.ID] {
					return nil
				}
				if len(channels) >= maxGuideChannels {
					return errTooManyProgrammes
				}
				seen[channel.ID] = true
				names := channel.Names
				if names == nil {
					names = []string{}
				}
				search := channel.ID
				for _, name := range names {
					search += "\n" + name
				}
				channels = append(channels, []any{g.ID, generation, channel.ID, names, channel.Icon, iptv.Fold(search)})
				return nil
			},
			func(p xmltv.Programme) error {
				if !p.Stop.After(from) || !p.Start.Before(to) {
					return nil
				}
				if count++; count > maxGuideProgrammes {
					return errTooManyProgrammes
				}
				if !seen[p.Channel] && !programmeChannels[p.Channel] {
					if len(programmeChannels) >= maxGuideChannels {
						return errTooManyProgrammes
					}
					programmeChannels[p.Channel] = true
				}
				select {
				case rows <- []any{g.ID, generation, p.Channel, p.Start, p.Stop, p.Title, p.SubTitle, p.Description, nonNilStrings(p.Categories),
					positive(p.Season), positive(p.Episode), p.Icon}:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
	}()
	source := &programmeRows{rows: rows}
	written, copyErr := s.db.CopyFrom(ctx, pgx.Identifier{"live_guide_programmes"}, []string{"guide_id", "generation", "xmltv_id", "starts_at",
		"ends_at", "title", "subtitle", "description", "categories", "season", "episode", "icon"}, source)
	if copyErr != nil {
		cancel()
		for range rows {
		}
	}
	<-done
	if readErr != nil {
		return 0, 0, readErr
	}
	if copyErr != nil {
		return 0, 0, copyErr
	}
	for id := range programmeChannels {
		if !seen[id] {
			channels = append(channels, []any{g.ID, generation, id, []string{}, "", iptv.Fold(id)})
		}
	}
	if _, err := s.db.CopyFrom(ctx, pgx.Identifier{"live_guide_channels"}, []string{"guide_id", "generation", "xmltv_id", "names", "icon", "search"},
		pgx.CopyFromRows(channels)); err != nil {
		return 0, 0, err
	}
	return len(channels), int(written), nil
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

// readGuide downloads a guide and reads it (see xmltv.Read). The address
// never appears in errors: it may embed credentials.
func (s *Service) readGuide(ctx context.Context, c addons.LiveCatalog, address string, options xmltv.Options,
	channel func(xmltv.Channel) error, programme func(xmltv.Programme) error) error {
	ctx, cancel := context.WithTimeout(ctx, guideTimeout)
	defer cancel()
	answered := time.AfterFunc(guideAnswer, cancel)
	response, err := s.client.Open(ctx, http.MethodGet, address, http.Header{"Accept": {"application/xml, text/xml, */*"}}, c.Confined)
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
// skip, which have Native EPG programmes: those of the guide channel each
// channel is mapped to. A channel two catalogs list takes the programmes
// of the first one's mapping; a channel has one programme from each
// start.
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
	rows, err := s.db.Query(ctx, `SELECT m.channel_id, m.addon_id, m.catalog_type, m.catalog_id, p.starts_at, p.ends_at, p.title, p.subtitle,
		p.description, p.categories, coalesce(p.season, 0), coalesce(p.episode, 0), p.icon
		FROM live_guide_maps m JOIN live_guides g ON g.id = m.guide_id
		JOIN live_guide_programmes p ON p.guide_id = g.id AND p.generation = g.generation AND p.xmltv_id = m.xmltv_id
		WHERE m.channel_id = ANY($1::uuid[]) AND p.ends_at > $2 AND p.starts_at < $3 ORDER BY p.starts_at`, ids, from, to)
	if err != nil {
		return nil, nil, err
	}
	type row struct {
		xmltv.Programme
		channel accounts.ID
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
	type start struct {
		channel accounts.ID
		at      int64
	}
	starts := map[start]bool{}
	for _, r := range found {
		if first[r.channel] != r.catalog || starts[start{r.channel, r.Start.UnixNano()}] {
			continue
		}
		starts[start{r.channel, r.Start.UnixNano()}] = true
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
