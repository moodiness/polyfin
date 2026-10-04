package iptv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
)

// Service stores the IPTV sources of the server and of each user, and
// answers the library's catalog, meta and stream requests for them.
type Service struct {
	db       *pgxpool.Pool
	addons   *addons.Store
	client   *stremio.Client
	logger   *slog.Logger
	settings func() accounts.Settings
	now      func() time.Time
	flight   singleflight.Group

	mu sync.Mutex
	// shown holds the channels each source shows, until its list or its
	// groups change.
	shown map[accounts.ID][]stremio.Meta
}

// New returns the IPTV service. settings gives LiveTvRefreshHours, read
// whenever lists due are looked for.
func New(db *pgxpool.Pool, store *addons.Store, client *stremio.Client, logger *slog.Logger, settings func() accounts.Settings) *Service {
	return &Service{db: db, addons: store, client: client, logger: logger, settings: settings, now: time.Now,
		shown: map[accounts.ID][]stremio.Meta{}}
}

// NewSource is an IPTV source to add: its name, its account, and the
// XMLTV guide of its catalog, empty for none.
type NewSource struct {
	Name    string
	Account Account
	Guide   string
}

// Group is a group of a source's list, with how many channels it has.
type Group struct {
	Name     string
	Channels int
}

// Source describes an IPTV source: its addon, its list's groups and the
// groups shown (nil for all of them, those added later included), and how
// its last fetch went: FetchedAt is the last success, CheckedAt the last
// attempt, Error the code of the last failure; NextAt is when it is
// fetched again.
type Source struct {
	Addon     addons.Addon
	Groups    []Group
	Included  []string
	Channels  int
	CheckedAt *time.Time
	FetchedAt *time.Time
	NextAt    *time.Time
	Error     string
}

// Add fetches an account's channel list and adds it to the scope as an
// IPTV source, its live TV catalog enabled, with its guide. confined keeps
// the source on public addresses, for good: a source of a user's own
// scope is confined unless the user is an administrator.
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
	entries, err := fetch(ctx, s.client, accountOf(source.Account.Kind, address), confined)
	if err != nil {
		return addons.Addon{}, err
	}
	at := s.now()
	addon, err := s.addons.Create(ctx, scope, source.Account.Kind, address,
		func(id accounts.ID) stremio.Manifest { return manifest(id, source.Account.Kind, name) },
		func(tx pgx.Tx, addon addons.Addon) error {
			if _, err := tx.Exec(ctx, "INSERT INTO iptv_sources (addon_id) VALUES ($1)", addon.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE libraries SET guide_url = $2 WHERE addon_id = $1", addon.ID, guide); err != nil {
				return err
			}
			return storeList(ctx, tx, addon.ID, entries, at)
		})
	if err != nil {
		return addons.Addon{}, err
	}
	s.forget(addon.ID)
	return addon, nil
}

// Changes are what Update changes of a source: its name, its account, and
// the groups shown: every group with AllGroups, else Groups when set.
type Changes struct {
	Name      *string
	Account   *Account
	Groups    []string
	AllGroups bool
}

// Update changes a source of the scope. A new account is fetched first,
// and kept only if its list could be read.
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
	var entries []Entry
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
		if entries, err = fetch(ctx, s.client, accountOf(addon.Kind, address), confined); err != nil {
			return err
		}
	}
	at := s.now()
	_, err = s.addons.Update(ctx, scope, id, addon.Kind, address, manifest(id, addon.Kind, name), func(tx pgx.Tx) error {
		if changes.AllGroups || changes.Groups != nil {
			var included []string
			if !changes.AllGroups {
				included = changes.Groups
			}
			if _, err := tx.Exec(ctx, "UPDATE iptv_sources SET included_groups = $2 WHERE addon_id = $1", id, included); err != nil {
				return err
			}
		}
		if changes.Account != nil {
			return storeList(ctx, tx, id, entries, at)
		}
		return nil
	})
	s.forget(id)
	return err
}

// Refresh fetches a source's list again now. How it went is stored with
// the source; the error is only for an unknown source or the database.
func (s *Service) Refresh(ctx context.Context, scope addons.Scope, id accounts.ID, confined bool) error {
	current, err := s.source(ctx, scope, id)
	if err != nil {
		return err
	}
	return s.refresh(ctx, current.Addon, confined)
}

// RefreshDue fetches the lists not fetched for the settings'
// LiveTvRefreshHours, or every list when all is set, one after the other,
// of the enabled sources of every scope.
func (s *Service) RefreshDue(ctx context.Context, all bool) error {
	rows, err := s.db.Query(ctx, `SELECT a.id, a.kind, a.manifest_url, a.manifest, a.enabled, a.refreshed_at,
		a.owner_id IS NOT NULL AND NOT coalesce(u.is_administrator, false), i.checked_at
		FROM iptv_sources i JOIN addons a ON a.id = i.addon_id LEFT JOIN users u ON u.id = a.owner_id WHERE a.enabled`)
	if err != nil {
		return err
	}
	type due struct {
		addon    addons.Addon
		confined bool
	}
	var list []due
	interval := time.Duration(s.settings().LiveTvRefreshHours) * time.Hour
	var d due
	var manifestJSON []byte
	var checked *time.Time
	if _, err := pgx.ForEachRow(rows, []any{&d.addon.ID, &d.addon.Kind, &d.addon.ManifestURL, &manifestJSON, &d.addon.Enabled,
		&d.addon.RefreshedAt, &d.confined, &checked}, func() error {
		if !all && checked != nil && s.now().Sub(*checked) < interval {
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

// refresh fetches a source's list and stores it, once at a time for each
// source; a failure keeps the channels of the last success.
func (s *Service) refresh(ctx context.Context, addon addons.Addon, confined bool) error {
	_, err, _ := s.flight.Do(addon.ID.String(), func() (any, error) {
		at := s.now()
		entries, err := fetch(ctx, s.client, accountOf(addon.Kind, addon.ManifestURL), confined)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var code string
		switch {
		case err == nil:
		case errors.Is(err, stremio.ErrPrivateNetwork):
			code = errorPrivateNetwork
		case errors.Is(err, ErrTooLarge):
			code = errorTooLarge
		case errors.Is(err, ErrInvalidList), errors.Is(err, ErrLoginRefused):
			code = errorMalformed
		default:
			code = errorUnreachable
		}
		if code != "" {
			s.logger.Warn("An IPTV channel list could not be fetched", "source", addon.Manifest.Name, "reason", code)
			_, err := s.db.Exec(ctx, "UPDATE iptv_sources SET checked_at = $2, error = $3 WHERE addon_id = $1", addon.ID, at, code)
			return nil, err
		}
		err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error { return storeList(ctx, tx, addon.ID, entries, at) })
		s.forget(addon.ID)
		return nil, err
	})
	return err
}

// storeList replaces a source's channels with a list fetched at at.
func storeList(ctx context.Context, tx pgx.Tx, source accounts.ID, entries []Entry, at time.Time) error {
	if _, err := tx.Exec(ctx, "DELETE FROM iptv_channels WHERE addon_id = $1", source); err != nil {
		return err
	}
	var groups []string
	keys := map[string]bool{}
	rows := make([][]any, 0, len(entries))
	for _, e := range entries {
		if !slices.Contains(groups, e.Group) {
			groups = append(groups, e.Group)
		}
		key := channelKey(e)
		for n := 2; keys[key]; n++ {
			key = channelKey(e) + "-" + strconv.Itoa(n)
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
		rows = append(rows, []any{source, key, len(rows) + 1, e.Name, number, e.Logo, e.Group, e.GuideID, e.URL, headers})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"iptv_channels"},
		[]string{"addon_id", "key", "position", "name", "number", "logo", "group_title", "guide_id", "url", "headers"},
		pgx.CopyFromRows(rows)); err != nil {
		return err
	}
	if groups == nil {
		groups = []string{}
	}
	_, err := tx.Exec(ctx, "UPDATE iptv_sources SET groups = $2, checked_at = $3, fetched_at = $3, error = '' WHERE addon_id = $1",
		source, groups, at)
	return err
}

// channelKey identifies a channel within its source: by the provider's own
// identifier, else by its name, group and guide identifier, which keep a
// channel's identity when its address, holding credentials, changes.
func channelKey(e Entry) string {
	if e.ID != "" {
		return e.ID
	}
	sum := sha256.Sum256([]byte(e.Name + "\x00" + e.Group + "\x00" + e.GuideID))
	return hex.EncodeToString(sum[:8])
}

// Source describes one of the scope's IPTV sources.
func (s *Service) Source(ctx context.Context, scope addons.Scope, id accounts.ID) (Source, error) {
	return s.source(ctx, scope, id)
}

func (s *Service) source(ctx context.Context, scope addons.Scope, id accounts.ID) (Source, error) {
	list, err := s.addons.Addons(ctx, scope)
	if err != nil {
		return Source{}, err
	}
	index := slices.IndexFunc(list, func(a addons.Addon) bool { return a.ID == id && a.IPTV() })
	if index < 0 {
		return Source{}, addons.ErrNotFound
	}
	source := Source{Addon: list[index]}
	var order []string
	err = s.db.QueryRow(ctx, "SELECT groups, included_groups, checked_at, fetched_at, error FROM iptv_sources WHERE addon_id = $1", id).
		Scan(&order, &source.Included, &source.CheckedAt, &source.FetchedAt, &source.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, addons.ErrNotFound
	}
	if err != nil {
		return Source{}, err
	}
	if source.CheckedAt != nil {
		next := source.CheckedAt.Add(time.Duration(s.settings().LiveTvRefreshHours) * time.Hour)
		source.NextAt = &next
	}
	counts := map[string]int{}
	rows, err := s.db.Query(ctx, "SELECT group_title, count(*) FROM iptv_channels WHERE addon_id = $1 GROUP BY group_title", id)
	if err != nil {
		return Source{}, err
	}
	var group string
	var count int
	if _, err := pgx.ForEachRow(rows, []any{&group, &count}, func() error {
		counts[group] = count
		source.Channels += count
		return nil
	}); err != nil {
		return Source{}, err
	}
	source.Groups = make([]Group, 0, len(order))
	for _, name := range order {
		source.Groups = append(source.Groups, Group{Name: name, Channels: counts[name]})
	}
	return source, nil
}

// forget drops what is remembered of a source's channels.
func (s *Service) forget(source accounts.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.shown, source)
}

// Channels lists the channels a source shows, those of its groups shown,
// in its list's order, as its live TV catalog's entries.
func (s *Service) Channels(ctx context.Context, source accounts.ID) ([]stremio.Meta, error) {
	s.mu.Lock()
	metas, ok := s.shown[source]
	s.mu.Unlock()
	if ok {
		return metas, nil
	}
	result, err, _ := s.flight.Do("channels "+source.String(), func() (any, error) {
		rows, err := s.db.Query(ctx, `SELECT c.key, c.name, coalesce(c.number, 0), c.logo, c.group_title, c.guide_id
			FROM iptv_channels c JOIN iptv_sources i ON i.addon_id = c.addon_id
			WHERE c.addon_id = $1 AND (i.included_groups IS NULL OR c.group_title = ANY(i.included_groups)) ORDER BY c.position`, source)
		if err != nil {
			return nil, err
		}
		metas := []stremio.Meta{}
		var key, name, logo, group, guide string
		var number int
		if _, err := pgx.ForEachRow(rows, []any{&key, &name, &number, &logo, &group, &guide}, func() error {
			metas = append(metas, channelMeta(source, key, name, number, logo, group, guide))
			return nil
		}); err != nil {
			return nil, err
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

func channelMeta(source accounts.ID, key, name string, number int, logo, group, guide string) stremio.Meta {
	meta := stremio.Meta{ID: prefix(source) + key, Type: "tv", Name: name, Logo: logo, ChannelNumber: number, GuideID: guide}
	if group != "" {
		meta.Genres = stremio.Names{group}
	}
	return meta
}

// channel returns a channel of a source by its Stremio identifier, if its
// group is shown: the others play no more than they are listed.
func (s *Service) channel(ctx context.Context, source accounts.ID, id string) (stremio.Meta, string, map[string]string, error) {
	key, ok := strings.CutPrefix(id, prefix(source))
	if !ok {
		return stremio.Meta{}, "", nil, stremio.ErrNotFound
	}
	var name, logo, group, guide, address string
	var number int
	var headers map[string]string
	err := s.db.QueryRow(ctx, `SELECT c.name, coalesce(c.number, 0), c.logo, c.group_title, c.guide_id, c.url, c.headers
		FROM iptv_channels c JOIN iptv_sources i ON i.addon_id = c.addon_id
		WHERE c.addon_id = $1 AND c.key = $2 AND (i.included_groups IS NULL OR c.group_title = ANY(i.included_groups))`, source, key).
		Scan(&name, &number, &logo, &group, &guide, &address, &headers)
	if errors.Is(err, pgx.ErrNoRows) {
		return stremio.Meta{}, "", nil, stremio.ErrNotFound
	}
	if err != nil {
		return stremio.Meta{}, "", nil, err
	}
	return channelMeta(source, key, name, number, logo, group, guide), address, headers, nil
}

// Meta describes a channel of a source.
func (s *Service) Meta(ctx context.Context, source accounts.ID, id string) (stremio.Meta, error) {
	meta, _, _, err := s.channel(ctx, source, id)
	return meta, err
}

// Streams lists a channel's stream: its address, requested with the
// headers its list gives.
func (s *Service) Streams(ctx context.Context, source accounts.ID, id string) ([]stremio.Stream, error) {
	meta, address, headers, err := s.channel(ctx, source, id)
	if errors.Is(err, stremio.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("IPTV channel: %w", err)
	}
	stream := stremio.Stream{Name: meta.Name, URL: address}
	if len(headers) > 0 {
		stream.BehaviorHints.ProxyHeaders = &stremio.ProxyHeaders{Request: headers}
	}
	return []stremio.Stream{stream}, nil
}
