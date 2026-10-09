package library

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/xmltv"
)

const (
	// GuideCheck is how often the lists and guides due are looked for
	// (see RefreshGuides): they are due after the settings'
	// LiveTvRefreshHours, or 5 minutes after a first failure (see
	// iptv.Backoff).
	GuideCheck = 5 * time.Minute
	// programSweep is how often past programmes kept are deleted.
	programSweep = time.Hour
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
	guideRateLimited    = "rate_limited"
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
// LiveTvRefreshHours, or whose failure's backoff ended (see iptv.Backoff),
// or every guide when all is set, one after the other: each costs its
// source a download. A catalog that fetched one maps its channels to its
// guides again. The past programmes kept are deleted then (see
// SweepPrograms). The task scheduler runs it every
// GuideCheck, and an administrator runs it with all.
func (s *Service) RefreshGuides(ctx context.Context, all bool) error {
	catalogs, err := s.addons.LiveCatalogs(ctx, true)
	if err != nil {
		return err
	}
	interval := time.Duration(s.settings().LiveTvRefreshHours) * time.Hour
	retries, err := s.guideRetries(ctx)
	if err != nil {
		return err
	}
	for _, c := range catalogs {
		due := slices.DeleteFunc(slices.Clone(c.Guides), func(g addons.Guide) bool {
			return !all && !iptv.Due(g.CheckedAt, retries[g.ID], interval, s.now())
		})
		if len(due) > 0 {
			if err := s.refreshCatalog(ctx, c, due); err != nil {
				return err
			}
		}
	}
	if !sweepDue(s.now()) {
		return nil
	}
	if deleted, err := s.SweepPrograms(ctx); err != nil {
		return err
	} else if deleted > 0 {
		s.logger.Info("Deleted past programmes", "programmes", deleted)
	}
	return nil
}

// guideRetries maps the guides whose last download failed to their next
// try, and remembers them for GuideRetry.
func (s *Service) guideRetries(ctx context.Context) (map[accounts.ID]*time.Time, error) {
	rows, err := s.db.Query(ctx, "SELECT id, next_try_at FROM live_guides WHERE next_try_at IS NOT NULL")
	if err != nil {
		return nil, err
	}
	retries := map[accounts.ID]*time.Time{}
	var id accounts.ID
	var at time.Time
	_, err = pgx.ForEachRow(rows, []any{&id, &at}, func() error {
		retries[id] = &at
		return nil
	})
	if err == nil {
		retriesMu.Lock()
		knownRetries = map[accounts.ID]time.Time{}
		for id, at := range retries {
			knownRetries[id] = *at
		}
		retriesMu.Unlock()
	}
	return retries, err
}

// lastSweep is when past programmes were last deleted.
var lastSweep struct {
	sync.Mutex
	at time.Time
}

// sweepDue reports whether past programmes are deleted now, at most every
// programSweep, and takes the turn.
func sweepDue(now time.Time) bool {
	lastSweep.Lock()
	defer lastSweep.Unlock()
	if now.Sub(lastSweep.at) < programSweep {
		return false
	}
	lastSweep.at = now
	return true
}

var (
	retriesMu    sync.Mutex
	knownRetries = map[accounts.ID]time.Time{}
)

// GuideRetry returns when a guide whose last download failed is tried
// again, as known since the guides were last looked for.
func GuideRetry(guide accounts.ID) (time.Time, bool) {
	retriesMu.Lock()
	defer retriesMu.Unlock()
	at, ok := knownRetries[guide]
	return at, ok
}

// rememberRetry records a guide's next try, the zero time after a success.
func rememberRetry(guide accounts.ID, at time.Time) {
	retriesMu.Lock()
	defer retriesMu.Unlock()
	if at.IsZero() {
		delete(knownRetries, guide)
	} else {
		knownRetries[guide] = at
	}
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
// for each guide: from guidePast before it, or as far back as the archive
// of the channels mapped to a guide channel reaches (see guideArchives).
// They are written under the guide's next generation as the guide is
// read, then made the current one, with the past programmes of the last
// download that such archives still keep and the new one no longer gives;
// a failure keeps the last download's and records why.
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
		archives, err := s.guideArchives(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		channels, programmes, err := s.downloadGuide(ctx, c, g, generation, at, archives)
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
		case errors.Is(err, iptv.ErrRateLimited):
			code = guideRateLimited
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
			var after time.Duration
			if limited, ok := errors.AsType[*iptv.RateLimitError](err); ok {
				after = limited.RetryAfter
			}
			var failures int
			if err := s.db.QueryRow(ctx, "SELECT failures FROM live_guides WHERE id = $1", g.ID).Scan(&failures); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			next := at.Add(iptv.Backoff(failures+1, time.Duration(s.settings().LiveTvRefreshHours)*time.Hour, after))
			_, err := s.db.Exec(ctx, "UPDATE live_guides SET checked_at = $2, error = $3, failures = failures + 1, next_try_at = $4 WHERE id = $1",
				g.ID, at, code, next)
			rememberRetry(g.ID, next)
			return nil, err
		}
		return nil, pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			kept, err := keepArchived(ctx, tx, g, generation, at, archives)
			if err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `UPDATE live_guides SET generation = $2, checked_at = $3, fetched_at = $3, error = '', channels = $4,
				programmes = $5, failures = 0, next_try_at = NULL WHERE id = $1`, g.ID, generation, at, channels, programmes+kept)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			rememberRetry(g.ID, time.Time{})
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
// generation: those from guidePast before at, and those of the guide
// channels archives names as far back as the days it gives. It reports
// how many it wrote.
func (s *Service) downloadGuide(ctx context.Context, c addons.LiveCatalog, g addons.Guide, generation int, at time.Time,
	archives map[string]time.Duration) (int, int, error) {
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
				if !p.Stop.After(from) && !p.Stop.After(at.Add(-archives[p.Channel])) || !p.Start.Before(to) {
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
// never appears in errors: it may embed credentials. A provider that asks
// to slow down fails it with an iptv.RateLimitError, retried after a
// backoff (see RefreshGuides).
func (s *Service) readGuide(ctx context.Context, c addons.LiveCatalog, address string, options xmltv.Options,
	channel func(xmltv.Channel) error, programme func(xmltv.Programme) error) error {
	ctx, cancel := context.WithTimeout(ctx, guideTimeout)
	defer cancel()
	// The guide of an IPTV source is mostly its provider's, whose host its
	// list was just asked of: it waits for the host's turn.
	if c.Addon.IPTV() {
		if err := iptv.Hosts.Wait(ctx, address); err != nil {
			return err
		}
	}
	answered := time.AfterFunc(guideAnswer, cancel)
	response, err := s.client.Open(ctx, http.MethodGet, address, http.Header{"Accept": {"application/xml, text/xml, */*"}}, c.Confined)
	answered.Stop()
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if limited := iptv.RateLimited(response); limited != nil {
		iptv.Hosts.Delay(address, cmp.Or(limited.RetryAfter, 2*iptv.RequestGap))
		return limited
	}
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
// TV catalogs give the channels that overlap [q.From, q.To) and that keep
// accepts, by start time then channel number, but those of skip, which
// have Native EPG programmes: those of the guide channel each channel is
// mapped to. A channel two catalogs list takes the programmes of the first
// one's mapping; a channel has one programme from each start. With a limit,
// the guide is read a window at a time from q.From until enough are kept.
// Every programme read is remembered by its identifier (see
// unsavedProgram).
func (s *Service) guidePrograms(ctx context.Context, v view, channels map[string]Item, skip map[string]bool, q GuideQuery,
	keep func(Item) bool) ([]Item, error) {
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
		return nil, nil
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
		return nil, nil
	}
	// Each channel's mapping: that of the first of its catalogs.
	rows, err := s.db.Query(ctx, `SELECT channel_id, addon_id, catalog_type, catalog_id, guide_id, xmltv_id FROM live_guide_maps
		WHERE channel_id = ANY($1::uuid[]) AND guide_id IS NOT NULL`, ids)
	if err != nil {
		return nil, err
	}
	type mapping struct {
		guide   accounts.ID
		xmltvID string
		catalog int
	}
	mapped := map[accounts.ID]mapping{}
	var channel accounts.ID
	var key addons.LibraryKey
	var m mapping
	if _, err := pgx.ForEachRow(rows, []any{&channel, &key.AddonID, &key.CatalogType, &key.CatalogID, &m.guide, &m.xmltvID}, func() error {
		index, ok := order[key]
		if current, seen := mapped[channel]; ok && (!seen || index < current.catalog) {
			m.catalog = index
			mapped[channel] = m
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if len(mapped) == 0 {
		return nil, nil
	}
	var channelIDs, guideIDs []accounts.ID
	var xmltvIDs []string
	var numbers []int
	for id, m := range mapped {
		number, _ := strconv.Atoi(byID[id].Number)
		channelIDs, guideIDs, xmltvIDs, numbers = append(channelIDs, id), append(guideIDs, m.guide), append(xmltvIDs, m.xmltvID), append(numbers, number)
	}
	var items []Item
	type start struct {
		channel accounts.ID
		at      int64
	}
	starts := map[start]bool{}
	read := func(from, to time.Time) error {
		rows, err := s.db.Query(ctx, `SELECT c.channel_id, p.guide_id, p.xmltv_id, p.starts_at, p.ends_at, p.title, p.subtitle, p.description,
			p.categories, coalesce(p.season, 0), coalesce(p.episode, 0), p.icon
			FROM unnest($1::uuid[], $2::uuid[], $3::text[], $4::int[]) AS c (channel_id, guide_id, xmltv_id, number)
			JOIN live_guides g ON g.id = c.guide_id
			JOIN live_guide_programmes p ON p.guide_id = g.id AND p.generation = g.generation AND p.xmltv_id = c.xmltv_id
			WHERE p.ends_at > $5 AND p.starts_at < $6 ORDER BY p.starts_at, c.number, c.channel_id`, channelIDs, guideIDs, xmltvIDs, numbers, from, to)
		if err != nil {
			return err
		}
		var p xmltv.Programme
		var channel, guide accounts.ID
		var xmltvID string
		_, err = pgx.ForEachRow(rows, []any{&channel, &guide, &xmltvID, &p.Start, &p.Stop, &p.Title, &p.SubTitle, &p.Description, &p.Categories,
			&p.Season, &p.Episode, &p.Icon}, func() error {
			if starts[start{channel, p.Start.UnixNano()}] || q.Limit > 0 && len(items) >= q.Limit {
				return nil
			}
			starts[start{channel, p.Start.UnixNano()}] = true
			video := guideVideo(p)
			program, ok := programItem(byID[channel], video, p.SubTitle)
			if !ok {
				return nil
			}
			rememberProgram(program.ID, programRef{channel: byID[channel].StremioID, guide: guide, xmltvID: xmltvID, start: p.Start})
			if keep(program) {
				items = append(items, program)
			}
			return nil
		})
		return err
	}
	if q.Limit <= 0 {
		return items, read(q.From, q.To)
	}
	// A limited listing reads 6 hours, then a day, then the rest, which
	// keeps each read's sort small.
	from := q.From
	for _, span := range []time.Duration{6 * time.Hour, 24 * time.Hour, q.To.Sub(q.From)} {
		to := q.From.Add(span)
		if to.After(q.To) {
			to = q.To
		}
		if !to.After(from) {
			continue
		}
		// A programme that started before from is read with the window it
		// started in, and its start keeps it from being listed twice.
		if err := read(from, to); err != nil || len(items) >= q.Limit || !to.Before(q.To) {
			return items, err
		}
		from = to
	}
	return items, nil
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
