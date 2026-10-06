package iptv

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Codes of the reasons a list could not be fetched, which the admin app
// shows.
const (
	errorUnreachable    = "unreachable"
	errorPrivateNetwork = "private_network"
	errorTooLarge       = "too_large"
	errorMalformed      = "malformed"
	errorRateLimited    = "rate_limited"
)

// Backoff is how long after its failures-th failure in a row a list or a
// guide is tried again: 5 minutes, 15 minutes, then every hour, never later
// than interval (LiveTvRefreshHours), and not before after (what the
// provider asked, through Retry-After).
func Backoff(failures int, interval, after time.Duration) time.Duration {
	steps := []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour}
	wait := steps[min(max(failures, 1), len(steps))-1]
	return max(min(wait, interval), after)
}

// failureCode is the code the admin app shows for a failed download.
func failureCode(err error) string {
	switch {
	case errors.Is(err, stremio.ErrPrivateNetwork):
		return errorPrivateNetwork
	case errors.Is(err, ErrTooLarge):
		return errorTooLarge
	case errors.Is(err, ErrInvalidList), errors.Is(err, ErrLoginRefused):
		return errorMalformed
	case errors.Is(err, ErrRateLimited):
		return errorRateLimited
	default:
		return errorUnreachable
	}
}

// Service stores the IPTV sources of the server and of each user, their
// line-ups, and answers the library's catalog, meta and stream requests for
// them.
type Service struct {
	db       *pgxpool.Pool
	addons   *addons.Store
	client   *stremio.Client
	logger   *slog.Logger
	settings func() accounts.Settings
	now      func() time.Time
	flight   singleflight.Group
	// changed is told when a source's line-up changed (see OnChange).
	changed func(ctx context.Context, source accounts.ID)

	mu sync.Mutex
	// shown holds the channels each source shows, until its line-up
	// changes.
	shown map[accounts.ID][]stremio.Meta
	// lists holds the lists previews of new accounts downloaded, for
	// listCache, so that previewing again or adding the source does not
	// download the list again.
	lists map[string]cachedList

	// pacer spaces the requests to each provider host: lists and
	// details (see Pacer).
	pacer *Pacer
}

// listCache is how long the list a preview downloaded is kept, at most
// maxCachedLists of them.
const (
	listCache      = 5 * time.Minute
	maxCachedLists = 4
)

type cachedList struct {
	list downloaded
	at   time.Time
}

// fetchCached downloads the parts want names of an account, taking those
// a preview downloaded within listCache; keep caches what it downloads.
func (s *Service) fetchCached(ctx context.Context, account Account, confined bool, want parts, keep bool) (downloaded, error) {
	key := fmt.Sprint(account.Kind, "\x00", account.URL, "\x00", account.Server, "\x00", account.Username, "\x00", account.Password, "\x00", confined)
	s.mu.Lock()
	for k, list := range s.lists {
		if s.now().Sub(list.at) > listCache {
			delete(s.lists, k)
		}
	}
	cached, ok := s.lists[key]
	s.mu.Unlock()
	missing := want
	if ok {
		missing = want.missing(cached.list.got)
	}
	if !missing.any() {
		return cached.list, nil
	}
	fetched, err := fetch(ctx, s.requester(), account, confined, missing)
	if err != nil {
		return downloaded{}, err
	}
	if ok {
		fetched.merge(cached.list)
	}
	if !keep {
		return fetched, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.lists[key]; !ok && len(s.lists) >= maxCachedLists {
		oldest := ""
		for k, list := range s.lists {
			if oldest == "" || list.at.Before(s.lists[oldest].at) {
				oldest = k
			}
		}
		delete(s.lists, oldest)
	}
	s.lists[key] = cachedList{list: fetched, at: s.now()}
	return fetched, nil
}

// New returns the IPTV service. settings gives LiveTvRefreshHours, read
// whenever lists due are looked for.
func New(db *pgxpool.Pool, store *addons.Store, client *stremio.Client, logger *slog.Logger, settings func() accounts.Settings) *Service {
	return &Service{db: db, addons: store, client: client, logger: logger, settings: settings, now: time.Now,
		shown: map[accounts.ID][]stremio.Meta{}, lists: map[string]cachedList{}, pacer: Hosts}
}

// OnChange sets what is told, after the change, that a source's line-up
// changed its channels: a refresh, new options. The library maps the
// line-up's channels to its guides again. It is called before the service
// is used.
func (s *Service) OnChange(changed func(ctx context.Context, source accounts.ID)) {
	s.changed = changed
}

// NewSource is an IPTV source to add: its name, its account, the XMLTV
// guide of its catalog (empty for none), and its import options (nil for
// the defaults).
type NewSource struct {
	Name    string
	Account Account
	Guide   string
	Options *OptionsPatch
}

// OptionsPatch changes import options: nil fields keep their values.
type OptionsPatch struct {
	Categories   *string   `json:"categories"`
	Channels     *string   `json:"channels"`
	Excluded     *[]string `json:"excluded"`
	NewChannels  *bool     `json:"newChannels"`
	Numbering    *string   `json:"numbering"`
	LiveTv       *bool     `json:"liveTv"`
	Movies       *bool     `json:"movies"`
	Series       *bool     `json:"series"`
	VODExcluded  *[]string `json:"vodExcluded"`
	VODLibraries *string   `json:"vodLibraries"`
	Enrichment   *bool     `json:"enrichment"`
}

// apply returns options with patch applied, checked.
func (o Options) apply(patch *OptionsPatch) (Options, error) {
	if patch != nil {
		set := func(field *string, value *string) {
			if value != nil {
				*field = *value
			}
		}
		turn := func(field *bool, value *bool) {
			if value != nil {
				*field = *value
			}
		}
		set(&o.Categories, patch.Categories)
		set(&o.Channels, patch.Channels)
		set(&o.Numbering, patch.Numbering)
		set(&o.VODLibraries, patch.VODLibraries)
		if patch.Excluded != nil {
			o.Excluded = *patch.Excluded
		}
		if patch.VODExcluded != nil {
			o.VODExcluded = *patch.VODExcluded
		}
		turn(&o.NewChannels, patch.NewChannels)
		turn(&o.LiveTv, patch.LiveTv)
		turn(&o.Movies, patch.Movies)
		turn(&o.Series, patch.Series)
		turn(&o.Enrichment, patch.Enrichment)
	}
	if o.Excluded == nil {
		o.Excluded = []string{}
	}
	if o.VODExcluded == nil {
		o.VODExcluded = []string{}
	}
	return o, o.check()
}

// LineupCounts count a source's line-up: its categories, those enabled;
// its channels, those enabled, those apps show (enabled, in an enabled
// category, with an enabled stream); and those mapped to a guide channel
// or not.
type LineupCounts struct {
	Categories, EnabledCategories            int
	Channels, EnabledChannels, ShownChannels int
	Mapped, Unmapped                         int
}

// Source describes an IPTV source: its addon, how many entries its list
// has, how its last fetch went (FetchedAt is the last success, CheckedAt
// the last attempt, Error the code of the last failure; NextAt is when it
// is fetched again), its import options and its line-up's counts.
type Source struct {
	Addon     addons.Addon
	Channels  int
	CheckedAt *time.Time
	FetchedAt *time.Time
	NextAt    *time.Time
	Error     string
	// MaxConnections is how many streams the account may play at once,
	// as an Xtream server says; nil when unknown, 0 for no limit.
	MaxConnections *int
	Options        Options
	Lineup         LineupCounts
	VOD            VODCounts
}

// Add fetches an account's channel list and adds it to the scope as an
// IPTV source, its live TV catalog enabled, with its guide and its line-up
// made by its options. confined keeps the source on public addresses, for
// good: a source of a user's own scope is confined unless the user is an
// administrator.
func (s *Service) Add(ctx context.Context, scope addons.Scope, source NewSource, confined bool) (addons.Addon, error) {
	name, err := validName(source.Name)
	if err != nil {
		return addons.Addon{}, err
	}
	address, err := source.Account.address()
	if err != nil {
		return addons.Addon{}, err
	}
	guide := strings.TrimSpace(source.Guide)
	if guide != "" && !webAddress(guide) {
		return addons.Addon{}, addons.ErrInvalidGuideURL
	}
	options, err := DefaultOptions().apply(source.Options)
	if err != nil {
		return addons.Addon{}, err
	}
	list, err := s.fetchCached(ctx, accountOf(source.Account.Kind, address), confined, partsOf(options), false)
	if err != nil {
		return addons.Addon{}, err
	}
	at := s.now()
	addon, err := s.addons.Create(ctx, scope, source.Account.Kind, address,
		func(id accounts.ID) stremio.Manifest { return manifest(id, source.Account.Kind, name, options, nil) },
		func(tx pgx.Tx, addon addons.Addon) error {
			if _, err := tx.Exec(ctx, "INSERT INTO iptv_sources (addon_id, "+optionColumns+") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)",
				append([]any{addon.ID}, options.values()...)...); err != nil {
				return err
			}
			if guide != "" {
				if _, err := tx.Exec(ctx, `INSERT INTO live_guides (addon_id, catalog_type, catalog_id, position, url)
					SELECT addon_id, catalog_type, catalog_id, 1, $2 FROM libraries WHERE addon_id = $1 AND catalog_type = 'tv'`, addon.ID, guide); err != nil {
					return err
				}
			}
			if err := storeDownload(ctx, tx, addon.ID, list, at); err != nil {
				return err
			}
			return reconcile(ctx, tx, addon.ID, at)
		})
	if err != nil {
		return addons.Addon{}, err
	}
	s.forget(addon.ID)
	return addon, nil
}

// Changes are what Update changes of a source: its name, its account and
// its import options.
type Changes struct {
	Name    *string
	Account *Account
	Options *OptionsPatch
}

// Update changes a source of the scope. A new account is fetched first,
// and kept only if its list could be read. New options, or a new list,
// reconcile the line-up at once.
func (s *Service) Update(ctx context.Context, scope addons.Scope, id accounts.ID, changes Changes, confined bool) error {
	current, err := s.source(ctx, scope, id)
	if err != nil {
		return err
	}
	addon := current.Addon
	name, address := addon.Manifest.Name, addon.ManifestURL
	if changes.Name != nil {
		if name, err = validName(*changes.Name); err != nil {
			return err
		}
	}
	options, err := current.Options.apply(changes.Options)
	if err != nil {
		return err
	}
	var list downloaded
	if changes.Account != nil {
		account := *changes.Account
		account.Kind = addon.Kind
		// A password left out keeps the current one.
		if account.Kind == addons.KindXtream && strings.TrimSpace(account.Password) == "" {
			account.Password = accountOf(addon.Kind, addon.ManifestURL).Password
		}
		if address, err = account.address(); err != nil {
			return err
		}
		if list, err = fetch(ctx, s.requester(), accountOf(addon.Kind, address), confined, partsOf(options)); err != nil {
			return err
		}
	} else if addon.Kind == addons.KindXtream {
		// The parts of an Xtream account turned on are downloaded first;
		// those of an M3U playlist were stored with it.
		if need := partsOf(options).missing(partsOf(current.Options)); need.any() {
			if list, err = s.fetchCached(ctx, accountOf(addon.Kind, address), confined, need, false); err != nil {
				return err
			}
		}
	}
	at := s.now()
	_, err = s.addons.Update(ctx, scope, id, addon.Kind, address, renamed(addon.Manifest, name), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE iptv_sources SET ("+optionColumns+") = ($2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) WHERE addon_id = $1",
			append([]any{id}, options.values()...)...); err != nil {
			return err
		}
		if err := storeDownload(ctx, tx, id, list, at); err != nil {
			return err
		}
		if changes.Account != nil || changes.Options != nil {
			return reconcile(ctx, tx, id, at)
		}
		return nil
	})
	if err == nil && (changes.Account != nil || changes.Options != nil) {
		s.lineupChanged(ctx, id)
	}
	s.forget(id)
	return err
}

// interval is how long a list or guide fetched is kept: the settings'
// LiveTvRefreshHours.
func (s *Service) interval() time.Duration {
	return time.Duration(s.settings().LiveTvRefreshHours) * time.Hour
}

// Due reports whether a list or guide last tried at checked is due at now:
// at retry after a failure, else once interval passed.
func Due(checked, retry *time.Time, interval time.Duration, now time.Time) bool {
	switch {
	case retry != nil:
		return !now.Before(*retry)
	case checked != nil:
		return now.Sub(*checked) >= interval
	}
	return true
}

// lineupChanged forgets a source's shown channels and tells OnChange.
func (s *Service) lineupChanged(ctx context.Context, source accounts.ID) {
	s.forget(source)
	if s.changed != nil {
		s.changed(ctx, source)
	}
}

// Refresh fetches a source's list again now and reconciles its line-up.
// How it went is stored with the source; the error is only for an
// unknown source or the database.
func (s *Service) Refresh(ctx context.Context, scope addons.Scope, id accounts.ID, confined bool) error {
	current, err := s.source(ctx, scope, id)
	if err != nil {
		return err
	}
	return s.refresh(ctx, current.Addon, confined)
}

// RefreshDue fetches the lists not fetched for the settings'
// LiveTvRefreshHours, or whose failure's backoff ended (see Backoff), or
// every list when all is set, one after the other, of the enabled sources
// of every scope.
func (s *Service) RefreshDue(ctx context.Context, all bool) error {
	rows, err := s.db.Query(ctx, `SELECT a.id, a.kind, a.manifest_url, a.manifest, a.enabled, a.refreshed_at,
		a.owner_id IS NOT NULL AND NOT coalesce(u.is_administrator, false), i.checked_at, i.next_try_at
		FROM iptv_sources i JOIN addons a ON a.id = i.addon_id LEFT JOIN users u ON u.id = a.owner_id WHERE a.enabled`)
	if err != nil {
		return err
	}
	type due struct {
		addon    addons.Addon
		confined bool
	}
	var list []due
	var d due
	var manifestJSON []byte
	var checked, retry *time.Time
	if _, err := pgx.ForEachRow(rows, []any{&d.addon.ID, &d.addon.Kind, &d.addon.ManifestURL, &manifestJSON, &d.addon.Enabled,
		&d.addon.RefreshedAt, &d.confined, &checked, &retry}, func() error {
		if !all && !Due(checked, retry, s.interval(), s.now()) {
			return nil
		}
		d.addon.Manifest = stremio.Manifest{}
		if err := json.Unmarshal(manifestJSON, &d.addon.Manifest); err != nil {
			return err
		}
		list = append(list, d)
		return nil
	}); err != nil {
		return err
	}
	for _, d := range list {
		if err := s.refresh(ctx, d.addon, d.confined); err != nil {
			return err
		}
	}
	return nil
}

// refresh fetches a source's list, stores it and reconciles the line-up,
// once at a time for each source; a failure keeps the last list.
func (s *Service) refresh(ctx context.Context, addon addons.Addon, confined bool) error {
	_, err, _ := s.flight.Do(addon.ID.String(), func() (any, error) {
		at := s.now()
		options, err := loadOptions(ctx, s.db, addon.ID)
		if err != nil {
			return nil, err
		}
		list, err := fetch(ctx, s.requester(), accountOf(addon.Kind, addon.ManifestURL), confined, partsOf(options))
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			code := failureCode(err)
			var after time.Duration
			if limited, ok := errors.AsType[*RateLimitError](err); ok {
				after = limited.RetryAfter
			}
			var failures int
			if err := s.db.QueryRow(ctx, "SELECT failures FROM iptv_sources WHERE addon_id = $1", addon.ID).Scan(&failures); err != nil {
				return nil, err
			}
			next := at.Add(Backoff(failures+1, s.interval(), after))
			s.logger.Warn("An IPTV channel list could not be fetched", "source", addon.Manifest.Name, "reason", code, "retry", next.Sub(at).Round(time.Second))
			_, err := s.db.Exec(ctx, "UPDATE iptv_sources SET checked_at = $2, error = $3, failures = failures + 1, next_try_at = $4 WHERE addon_id = $1",
				addon.ID, at, code, next)
			return nil, err
		}
		err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			if err := storeDownload(ctx, tx, addon.ID, list, at); err != nil {
				return err
			}
			return reconcile(ctx, tx, addon.ID, at)
		})
		if err == nil {
			s.lineupChanged(ctx, addon.ID)
		}
		return nil, err
	})
	return err
}

// storeDownload stores the parts a download brought: its list's entries,
// an Xtream account's movies and series.
func storeDownload(ctx context.Context, tx pgx.Tx, source accounts.ID, list downloaded, at time.Time) error {
	if list.got.live {
		if err := storeList(ctx, tx, source, list.entries, at); err != nil {
			return err
		}
	}
	if list.got.any() {
		if _, err := tx.Exec(ctx, "UPDATE iptv_sources SET checked_at = $2, fetched_at = $2, error = '', failures = 0, next_try_at = NULL WHERE addon_id = $1", source, at); err != nil {
			return err
		}
	}
	if !list.xtream {
		return nil
	}
	if list.connections != nil {
		if _, err := tx.Exec(ctx, "UPDATE iptv_sources SET max_connections = $2 WHERE addon_id = $1", source, *list.connections); err != nil {
			return err
		}
	}
	if list.got.movies {
		if err := storeTitles(ctx, tx, source, typeMovie, list.movies, nil); err != nil {
			return err
		}
	}
	if list.got.series {
		if err := storeTitles(ctx, tx, source, typeSeries, list.series, nil); err != nil {
			return err
		}
	}
	return nil
}

// storeList replaces a source's stored list with a list fetched at at.
func storeList(ctx context.Context, tx pgx.Tx, source accounts.ID, entries []Entry, at time.Time) error {
	if _, err := tx.Exec(ctx, "DELETE FROM iptv_entries WHERE addon_id = $1", source); err != nil {
		return err
	}
	keys := map[string]bool{}
	rows := make([][]any, 0, len(entries))
	for _, e := range entries {
		key := entryKey(e)
		for n := 2; keys[key]; n++ {
			key = entryKey(e) + "-" + strconv.Itoa(n)
		}
		keys[key] = true
		headers := e.Headers
		if headers == nil {
			headers = map[string]string{}
		}
		var number *int
		if e.Number > 0 {
			number = &e.Number
		}
		kind := cmp.Or(e.Kind, kindLive)
		var season, episode *int
		if kind == KindEpisode {
			season, episode = &e.Season, &e.Episode
		}
		rows = append(rows, []any{source, key, len(rows) + 1, e.Name, number, e.Logo, e.Group, e.GuideID, e.URL, headers, kind, e.Series, season, episode})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"iptv_entries"},
		[]string{"addon_id", "key", "position", "name", "number", "logo", "group_title", "guide_id", "url", "headers", "kind", "series_name", "season", "episode"},
		pgx.CopyFromRows(rows)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE iptv_sources SET checked_at = $2, fetched_at = $2, error = '', failures = 0, next_try_at = NULL WHERE addon_id = $1", source, at)
	return err
}

// entryKey identifies an entry within its source: by the provider's own
// identifier, else by its name, group and guide identifier, which keep an
// entry's identity when its address, holding credentials, changes.
func entryKey(e Entry) string {
	if e.ID != "" {
		return e.ID
	}
	sum := sha256.Sum256([]byte(e.Name + "\x00" + e.Group + "\x00" + e.GuideID))
	return hex.EncodeToString(sum[:8])
}

// ensureLineup reconciles a source's line-up once, for a source added
// before line-ups existed.
func (s *Service) ensureLineup(ctx context.Context, source accounts.ID) error {
	var pending bool
	err := s.db.QueryRow(ctx, "SELECT lineup_at IS NULL FROM iptv_sources WHERE addon_id = $1", source).Scan(&pending)
	if errors.Is(err, pgx.ErrNoRows) {
		return addons.ErrNotFound
	}
	if err != nil || !pending {
		return err
	}
	_, err, _ = s.flight.Do("lineup "+source.String(), func() (any, error) {
		return nil, pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error { return reconcile(ctx, tx, source, s.now()) })
	})
	if err == nil {
		s.lineupChanged(ctx, source)
	}
	return err
}

// Source describes one of the scope's IPTV sources.
func (s *Service) Source(ctx context.Context, scope addons.Scope, id accounts.ID) (Source, error) {
	return s.source(ctx, scope, id)
}

// shownSQL selects, joined as l with its category as c, the channels apps
// show: enabled, in an enabled category, with an enabled stream.
const shownSQL = `l.enabled AND c.enabled AND EXISTS (SELECT 1 FROM iptv_streams s WHERE s.addon_id = l.addon_id AND s.channel_id = l.id AND s.enabled)`

func (s *Service) source(ctx context.Context, scope addons.Scope, id accounts.ID) (Source, error) {
	list, err := s.addons.Addons(ctx, scope)
	if err != nil {
		return Source{}, err
	}
	index := slices.IndexFunc(list, func(a addons.Addon) bool { return a.ID == id && a.IPTV() })
	if index < 0 {
		return Source{}, addons.ErrNotFound
	}
	if err := s.ensureLineup(ctx, id); err != nil {
		return Source{}, err
	}
	source := Source{Addon: list[index]}
	o := &source.Options
	var retry *time.Time
	err = s.db.QueryRow(ctx, `SELECT checked_at, fetched_at, error, next_try_at, max_connections, `+optionColumns+`,
		(SELECT count(*) FROM iptv_entries WHERE addon_id = $1) FROM iptv_sources WHERE addon_id = $1`, id).
		Scan(append(append([]any{&source.CheckedAt, &source.FetchedAt, &source.Error, &retry, &source.MaxConnections}, o.fields()...), &source.Channels)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, addons.ErrNotFound
	}
	if err != nil {
		return Source{}, err
	}
	if retry != nil {
		source.NextAt = retry
	} else if source.CheckedAt != nil {
		next := source.CheckedAt.Add(s.interval())
		source.NextAt = &next
	}
	n := &source.Lineup
	err = s.db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM iptv_categories WHERE addon_id = $1),
		(SELECT count(*) FROM iptv_categories WHERE addon_id = $1 AND enabled),
		count(*), count(*) FILTER (WHERE l.enabled), count(*) FILTER (WHERE `+shownSQL+`),
		count(*) FILTER (WHERE m.guide_id IS NOT NULL)
		FROM iptv_lineup l JOIN iptv_categories c ON c.id = coalesce(l.moved_to, l.category_id)
		LEFT JOIN live_guide_maps m ON m.addon_id = l.addon_id AND m.catalog_type = 'tv' AND m.catalog_id = $2 AND m.channel_id = l.item_id
		WHERE l.addon_id = $1`, id, catalogID).Scan(&n.Categories, &n.EnabledCategories, &n.Channels, &n.EnabledChannels, &n.ShownChannels, &n.Mapped)
	n.Unmapped = n.Channels - n.Mapped
	if err != nil {
		return Source{}, err
	}
	source.VOD, err = s.vodCounts(ctx, id)
	return source, err
}

// forget drops what is remembered of a source's channels.
func (s *Service) forget(source accounts.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.shown, source)
}

// channelColumns are what a channel's meta is made of, from l joined with
// its source as i and its category as c.
const channelColumns = `l.id, coalesce(l.name, l.provider_name), coalesce(l.logo, l.provider_logo), l.description,
	coalesce(c.name, c.provider_name), coalesce(l.number, CASE WHEN i.numbering = 'provider' THEN l.provider_number END, 0), l.guide_id`

const channelJoins = `FROM iptv_lineup l JOIN iptv_sources i ON i.addon_id = l.addon_id
	JOIN iptv_categories c ON c.id = coalesce(l.moved_to, l.category_id)`

func scanMeta(source accounts.ID, row pgx.Row) (stremio.Meta, error) {
	var id, name, logo, description, category, guide string
	var number int
	if err := row.Scan(&id, &name, &logo, &description, &category, &number, &guide); err != nil {
		return stremio.Meta{}, err
	}
	meta := stremio.Meta{ID: prefix(source) + id, Type: "tv", Name: name, Logo: logo, Description: description, ChannelNumber: number, GuideID: guide}
	if category != "" {
		meta.Genres = stremio.Names{category}
	}
	return meta, nil
}

// Channels lists the channels a source shows, in its line-up's order
// (category, then channel), as its live TV catalog's entries: their
// category is their genre, their number the fixed one, else the
// provider's unless the source numbers by place.
func (s *Service) Channels(ctx context.Context, source accounts.ID) ([]stremio.Meta, error) {
	s.mu.Lock()
	metas, ok := s.shown[source]
	s.mu.Unlock()
	if ok {
		return metas, nil
	}
	if err := s.ensureLineup(ctx, source); err != nil {
		return nil, err
	}
	result, err, _ := s.flight.Do("channels "+source.String(), func() (any, error) {
		rows, err := s.db.Query(ctx, "SELECT "+channelColumns+" "+channelJoins+" WHERE l.addon_id = $1 AND i.live_tv AND "+shownSQL+
			" ORDER BY c.position, l.sort, l.id", source)
		if err != nil {
			return nil, err
		}
		metas, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stremio.Meta, error) { return scanMeta(source, row) })
		if err != nil {
			return nil, err
		}
		if metas == nil {
			metas = []stremio.Meta{}
		}
		s.mu.Lock()
		s.shown[source] = metas
		s.mu.Unlock()
		return metas, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]stremio.Meta), nil
}

// MappingChannels lists every channel of a source's line-up, shown or not,
// in line-up order, for mapping them to guide channels.
func (s *Service) MappingChannels(ctx context.Context, source accounts.ID) ([]stremio.Meta, error) {
	if err := s.ensureLineup(ctx, source); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, "SELECT "+channelColumns+" "+channelJoins+" WHERE l.addon_id = $1 ORDER BY c.position, l.sort, l.id", source)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (stremio.Meta, error) { return scanMeta(source, row) })
}

// Logo finds the logo a line-up channel shows, by its item identifier,
// shown or not, so that the admin app shows every channel's: the
// administrator's, else the provider's. confined tells a source of a
// user's own scope, kept on public addresses unless the user is an
// administrator. A channel no line-up has is stremio.ErrNotFound.
func (s *Service) Logo(ctx context.Context, item accounts.ID) (string, bool, error) {
	var logo string
	var confined bool
	err := s.db.QueryRow(ctx, `SELECT coalesce(l.logo, l.provider_logo), a.owner_id IS NOT NULL AND NOT coalesce(u.is_administrator, false)
		FROM iptv_lineup l JOIN addons a ON a.id = l.addon_id LEFT JOIN users u ON u.id = a.owner_id WHERE l.item_id = $1`, item).Scan(&logo, &confined)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, stremio.ErrNotFound
	}
	return logo, confined, err
}

// Meta describes a channel, a movie or a series of a source by its
// Stremio identifier, while it shows: the others play no more than they
// are listed (see titleMeta for movies and series).
func (s *Service) Meta(ctx context.Context, source accounts.ID, id string) (stremio.Meta, error) {
	key, ok := strings.CutPrefix(id, prefix(source))
	if !ok {
		return stremio.Meta{}, stremio.ErrNotFound
	}
	if movie, ok := strings.CutPrefix(key, "vod:"); ok {
		return s.titleMeta(ctx, source, typeMovie, movie)
	}
	if series, ok := strings.CutPrefix(key, "series:"); ok {
		return s.titleMeta(ctx, source, typeSeries, series)
	}
	meta, err := scanMeta(source, s.db.QueryRow(ctx, "SELECT "+channelColumns+" "+channelJoins+" WHERE l.addon_id = $1 AND l.id = $2 AND i.live_tv AND "+shownSQL,
		source, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return stremio.Meta{}, stremio.ErrNotFound
	}
	return meta, err
}

// streamOrder orders a channel's streams s: the administrator's order,
// then the provider's (best quality first).
const streamOrder = `(s.sort IS NULL), s.sort, s.rank, s.key`

// Streams lists the enabled streams of a channel that shows, best first:
// its entries' addresses, requested with the headers their list gives, and
// its custom streams. Each is labelled by its quality. Streams that failed
// last (see ReportStream) come after the others, and those found dead or
// silent are left out until their backoff ends (see hidden).
func (s *Service) Streams(ctx context.Context, source accounts.ID, id string) ([]stremio.Stream, error) {
	if key, ok := strings.CutPrefix(id, prefix(source)); ok && (strings.HasPrefix(key, "vod:") || strings.HasPrefix(key, "ep:")) {
		return s.vodStreams(ctx, source, key)
	}
	meta, err := s.Meta(ctx, source, id)
	if errors.Is(err, stremio.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("IPTV channel: %w", err)
	}
	rows, err := s.db.Query(ctx, `SELECT s.label, coalesce(s.custom_url, e.url), coalesce(e.headers, '{}'), coalesce(s.health_url, ''),
		s.failure, s.failed_at, s.failures FROM iptv_streams s
		LEFT JOIN iptv_entries e ON e.addon_id = s.addon_id AND e.key = s.key
		WHERE s.addon_id = $1 AND s.channel_id = $2 AND s.enabled AND (s.custom_url IS NOT NULL OR e.url IS NOT NULL)
		ORDER BY `+streamOrder, source, strings.TrimPrefix(id, prefix(source)))
	if err != nil {
		return nil, err
	}
	var healthy, failing []stremio.Stream
	now := s.now()
	var label, address, checked string
	var headers map[string]string
	var h health
	if _, err := pgx.ForEachRow(rows, []any{&label, &address, &headers, &checked, &h.failure, &h.failedAt, &h.failures}, func() error {
		stream := stremio.Stream{Name: meta.Name, Description: label, URL: address}
		if len(headers) > 0 {
			stream.BehaviorHints.ProxyHeaders = &stremio.ProxyHeaders{Request: maps.Clone(headers)}
		}
		switch {
		case checked != addressHash(address) || h.failure == nil:
			healthy = append(healthy, stream)
		case !h.hidden(now):
			failing = append(failing, stream)
		}
		// pgx decodes JSON into the map given: each row starts empty.
		headers = nil
		return nil
	}); err != nil {
		return nil, err
	}
	return append(healthy, failing...), nil
}
