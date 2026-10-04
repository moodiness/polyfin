package iptv

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Errors of line-up edits.
var (
	ErrInvalidCategoryName = errors.New("category names are 1 to 64 printable characters")
	ErrCategoryNotCustom   = errors.New("only custom categories can be deleted")
	ErrInvalidCategory     = errors.New("unknown category")
	ErrInvalidChannelName  = errors.New("channel names are 1 to 100 printable characters")
	ErrInvalidLogo         = errors.New("logos are http or https URLs of at most 4096 characters")
	ErrInvalidDescription  = errors.New("descriptions are at most 2000 characters")
	ErrInvalidNumber       = errors.New("channel numbers are from 1 to 99999")
	ErrInvalidMove         = errors.New("a channel moves before another one of its category")
	ErrInvalidStreamURL    = errors.New("stream addresses are http or https URLs of at most 4096 characters")
	ErrInvalidStreamLabel  = errors.New("stream labels are 1 to 32 printable characters")
	ErrStreamNotCustom     = errors.New("only custom streams can be removed")
	ErrInvalidStreams      = errors.New("the streams must list every stream of the channel once")
	ErrInvalidBulk         = errors.New("a bulk change takes exactly one selector")
	ErrInvalidOrder        = errors.New("the order must list every category once")
)

// Nullable is a value a change sets (Value) or resets to the provider's
// (Value nil).
type Nullable[T any] struct{ Value *T }

// owned checks that a source is one of the scope's IPTV sources and that
// its line-up exists.
func (s *Service) owned(ctx context.Context, scope addons.Scope, source accounts.ID) error {
	var found bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM addons a JOIN iptv_sources i ON i.addon_id = a.id
		WHERE a.id = $1 AND a.owner_id IS NOT DISTINCT FROM $2)`, source, scope.Owner).Scan(&found); err != nil {
		return err
	}
	if !found {
		return addons.ErrNotFound
	}
	return s.ensureLineup(ctx, source)
}

func printable(text string, longest int) bool {
	if text == "" || utf8.RuneCountInString(text) > longest {
		return false
	}
	for _, r := range text {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// likePattern matches folded text containing q.
func likePattern(q string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(Fold(strings.TrimSpace(q)))
	return "%" + escaped + "%"
}

// Category is a category of a line-up: shown name, provider name ("" for
// a custom one), position from 1, and how many channels it holds, enabled
// or not.
type Category struct {
	ID              accounts.ID
	Key             string
	Name            string
	ProviderName    string
	Custom          bool
	Enabled         bool
	Position        int
	Channels        int
	EnabledChannels int
}

const categoryQuery = `SELECT c.id, c.key, coalesce(c.name, c.provider_name), c.provider_name, c.custom, c.enabled, c.position,
	count(l.id), count(l.id) FILTER (WHERE l.enabled)
	FROM iptv_categories c LEFT JOIN iptv_lineup l ON l.addon_id = c.addon_id AND coalesce(l.moved_to, l.category_id) = c.id
	WHERE c.addon_id = $1`

func scanCategory(row pgx.CollectableRow) (Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.Key, &c.Name, &c.ProviderName, &c.Custom, &c.Enabled, &c.Position, &c.Channels, &c.EnabledChannels)
	return c, err
}

// Categories lists a line-up's categories in order, those whose name or
// key holds q when given.
func (s *Service) Categories(ctx context.Context, scope addons.Scope, source accounts.ID, q string) ([]Category, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return nil, err
	}
	filter, args := "", []any{source}
	if strings.TrimSpace(q) != "" {
		filter, args = ` AND (lower(coalesce(c.name, c.provider_name)) LIKE $2 OR lower(c.key) LIKE $2)`, append(args, likePattern(q))
	}
	rows, err := s.db.Query(ctx, categoryQuery+filter+" GROUP BY c.id ORDER BY c.position", args...)
	if err != nil {
		return nil, err
	}
	categories, err := pgx.CollectRows(rows, scanCategory)
	if categories == nil {
		categories = []Category{}
	}
	return categories, err
}

func (s *Service) category(ctx context.Context, source, id accounts.ID) (Category, error) {
	rows, err := s.db.Query(ctx, categoryQuery+" AND c.id = $2 GROUP BY c.id", source, id)
	if err != nil {
		return Category{}, err
	}
	c, err := pgx.CollectExactlyOneRow(rows, scanCategory)
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, addons.ErrNotFound
	}
	return c, err
}

// CreateCategory adds a custom category, enabled, after the others.
func (s *Service) CreateCategory(ctx context.Context, scope addons.Scope, source accounts.ID, name string) (Category, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return Category{}, err
	}
	name = strings.TrimSpace(name)
	if !printable(name, 64) {
		return Category{}, ErrInvalidCategoryName
	}
	// A custom category's key is u: and its identifier, as the admin API
	// writes identifiers.
	var id accounts.ID
	if err := s.db.QueryRow(ctx, `WITH new AS (SELECT gen_random_uuid() AS id)
		INSERT INTO iptv_categories (id, addon_id, key, name, position, custom)
		SELECT new.id, $1, 'u:' || replace(new.id::text, '-', ''), $2,
			(SELECT coalesce(max(position), 0) + 1 FROM iptv_categories WHERE addon_id = $1), true FROM new RETURNING id`,
		source, name).Scan(&id); err != nil {
		return Category{}, err
	}
	return s.category(ctx, source, id)
}

// CategoryChanges change a category: its name (nil Value goes back to the
// provider's) and whether it is enabled.
type CategoryChanges struct {
	Name    *Nullable[string]
	Enabled *bool
}

// UpdateCategory changes a category of a line-up.
func (s *Service) UpdateCategory(ctx context.Context, scope addons.Scope, source, id accounts.ID, changes CategoryChanges) (Category, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return Category{}, err
	}
	current, err := s.category(ctx, source, id)
	if err != nil {
		return Category{}, err
	}
	if changes.Name != nil {
		var name *string
		if changes.Name.Value != nil {
			trimmed := strings.TrimSpace(*changes.Name.Value)
			if !printable(trimmed, 64) {
				return Category{}, ErrInvalidCategoryName
			}
			name = &trimmed
		} else if current.Custom {
			return Category{}, ErrInvalidCategoryName
		}
		if _, err := s.db.Exec(ctx, "UPDATE iptv_categories SET name = $3 WHERE addon_id = $1 AND id = $2", source, id, name); err != nil {
			return Category{}, err
		}
	}
	if changes.Enabled != nil {
		if _, err := s.db.Exec(ctx, "UPDATE iptv_categories SET enabled = $3 WHERE addon_id = $1 AND id = $2", source, id, *changes.Enabled); err != nil {
			return Category{}, err
		}
	}
	s.forget(source)
	return s.category(ctx, source, id)
}

// DeleteCategory deletes a custom category; its channels go back to their
// provider category.
func (s *Service) DeleteCategory(ctx context.Context, scope addons.Scope, source, id accounts.ID) error {
	if err := s.owned(ctx, scope, source); err != nil {
		return err
	}
	current, err := s.category(ctx, source, id)
	if err != nil {
		return err
	}
	if !current.Custom {
		return ErrCategoryNotCustom
	}
	if _, err := s.db.Exec(ctx, "DELETE FROM iptv_categories WHERE addon_id = $1 AND id = $2", source, id); err != nil {
		return err
	}
	s.forget(source)
	return nil
}

// OrderCategories sets the order of a line-up's categories; ids must list
// each of them once.
func (s *Service) OrderCategories(ctx context.Context, scope addons.Scope, source accounts.ID, ids []accounts.ID) error {
	if err := s.owned(ctx, scope, source); err != nil {
		return err
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM iptv_categories WHERE addon_id = $1`, source).Scan(&count); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE iptv_categories c SET position = o.n FROM unnest($2::uuid[]) WITH ORDINALITY AS o(id, n)
			WHERE c.addon_id = $1 AND c.id = o.id`, source, ids)
		if err != nil {
			return err
		}
		distinct := slices.Clone(ids)
		slices.SortFunc(distinct, func(a, b accounts.ID) int { return strings.Compare(a.String(), b.String()) })
		if len(slices.Compact(distinct)) != len(ids) || int(tag.RowsAffected()) != count || len(ids) != count {
			return ErrInvalidOrder
		}
		return nil
	})
	if err == nil {
		s.forget(source)
	}
	return err
}

// BulkCategories enables or disables the categories ids, every category
// when ids is nil. It reports how many changed.
func (s *Service) BulkCategories(ctx context.Context, scope addons.Scope, source accounts.ID, enabled bool, ids []accounts.ID) (int, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return 0, err
	}
	tag, err := s.db.Exec(ctx, `UPDATE iptv_categories SET enabled = $2 WHERE addon_id = $1 AND enabled <> $2
		AND ($3::uuid[] IS NULL OR id = ANY($3))`, source, enabled, ids)
	s.forget(source)
	return int(tag.RowsAffected()), err
}

// Mapping is the guide channel a channel takes: a guide and its channel,
// both nil for a manual "no guide".
type Mapping struct {
	Guide            *accounts.ID
	GuideChannel     *string
	GuideChannelName string
	Manual           bool
}

// Stream is a stream of a line-up channel; Address is a custom stream's,
// redacted, "" for a provider stream.
type Stream struct {
	ID      string
	Label   string
	Enabled bool
	Custom  bool
	Address string
}

// Channel is a channel of a line-up as the administrator edits it.
type Channel struct {
	ID               accounts.ID
	Name             string
	ProviderName     string
	Renamed          bool
	Logo             string
	ProviderLogo     string
	Description      string
	Category         accounts.ID
	CategoryName     string
	ProviderCategory accounts.ID
	Moved            bool
	Enabled          bool
	Shown            bool
	Number           *int
	ProviderNumber   *int
	FixedNumber      *int
	GuideID          string
	Mapping          *Mapping
	Streams          []Stream

	key string
}

// ChannelFilter narrows a channel listing.
type ChannelFilter struct {
	Category               *accounts.ID
	Enabled, Shown, Mapped *bool
	Q                      string
	Offset, Limit          int
}

const channelQuery = `SELECT l.id, l.item_id, coalesce(l.name, l.provider_name), l.provider_name, l.name IS NOT NULL,
	coalesce(l.logo, l.provider_logo), l.provider_logo, l.description, c.id, coalesce(c.name, c.provider_name), l.category_id,
	l.moved_to IS NOT NULL, l.enabled, ` + shownSQL + `,
	coalesce(l.number, CASE WHEN i.numbering = 'provider' THEN l.provider_number END), l.provider_number, l.number, l.guide_id,
	m.manual, m.guide_id, m.xmltv_id, coalesce(gc.names[1], ''), count(*) OVER ()
	` + channelJoins + `
	LEFT JOIN live_guide_maps m ON m.addon_id = l.addon_id AND m.catalog_type = 'tv' AND m.catalog_id = '` + catalogID + `' AND m.channel_id = l.item_id
	LEFT JOIN live_guides g ON g.id = m.guide_id
	LEFT JOIN live_guide_channels gc ON gc.guide_id = m.guide_id AND gc.generation = g.generation AND gc.xmltv_id = m.xmltv_id
	WHERE l.addon_id = $1`

func (s *Service) channels(ctx context.Context, source accounts.ID, filter string, args []any, offset, limit int) (int, []Channel, error) {
	args = append([]any{source}, args...)
	query := channelQuery + filter + " ORDER BY c.position, l.sort, l.id"
	if limit > 0 {
		query += " OFFSET " + strconv.Itoa(offset) + " LIMIT " + strconv.Itoa(limit)
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return 0, nil, err
	}
	total := 0
	channels, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Channel, error) {
		var c Channel
		var manual *bool
		var guide *accounts.ID
		var xmltvID *string
		var mappedName string
		if err := row.Scan(&c.key, &c.ID, &c.Name, &c.ProviderName, &c.Renamed, &c.Logo, &c.ProviderLogo, &c.Description, &c.Category,
			&c.CategoryName, &c.ProviderCategory, &c.Moved, &c.Enabled, &c.Shown, &c.Number, &c.ProviderNumber, &c.FixedNumber, &c.GuideID,
			&manual, &guide, &xmltvID, &mappedName, &total); err != nil {
			return Channel{}, err
		}
		if manual != nil {
			c.Mapping = &Mapping{Guide: guide, GuideChannel: xmltvID, GuideChannelName: mappedName, Manual: *manual}
		}
		return c, nil
	})
	if err != nil || len(channels) == 0 {
		return total, []Channel{}, err
	}
	keys := make([]string, len(channels))
	index := map[string]int{}
	for i, c := range channels {
		keys[i], index[c.key] = c.key, i
		channels[i].Streams = []Stream{}
	}
	rows, err = s.db.Query(ctx, `SELECT s.channel_id, s.key, s.label, s.enabled, s.custom_url FROM iptv_streams s
		WHERE s.addon_id = $1 AND s.channel_id = ANY($2) ORDER BY s.channel_id, `+streamOrder, source, keys)
	if err != nil {
		return 0, nil, err
	}
	var channel string
	var st Stream
	var custom *string
	_, err = pgx.ForEachRow(rows, []any{&channel, &st.ID, &st.Label, &st.Enabled, &custom}, func() error {
		stream := st
		stream.Custom = custom != nil
		if custom != nil {
			stream.Address = Redact(*custom)
		}
		c := &channels[index[channel]]
		c.Streams = append(c.Streams, stream)
		return nil
	})
	return total, channels, err
}

// ListChannels lists a line-up's channels in order, narrowed by filter:
// its category, whether enabled, shown or mapped, and q in the shown name,
// the provider name or the guide identifier. It reports how many match.
func (s *Service) ListChannels(ctx context.Context, scope addons.Scope, source accounts.ID, f ChannelFilter) (int, []Channel, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return 0, nil, err
	}
	filter, args := channelFilter(f.Category, f.Enabled, f.Shown, f.Mapped, f.Q)
	return s.channels(ctx, source, filter, args, max(f.Offset, 0), cmp.Or(f.Limit, 100))
}

// channelFilter builds the conditions of a listing or bulk change, its
// arguments from $2.
func channelFilter(category *accounts.ID, enabled, shown, mapped *bool, q string) (string, []any) {
	var conditions []string
	var args []any
	arg := func(value any) string {
		args = append(args, value)
		return "$" + strconv.Itoa(len(args)+1)
	}
	if category != nil {
		conditions = append(conditions, "c.id = "+arg(*category))
	}
	if enabled != nil {
		conditions = append(conditions, "l.enabled = "+arg(*enabled))
	}
	if shown != nil {
		conditions = append(conditions, "("+shownSQL+") = "+arg(*shown))
	}
	if mapped != nil {
		conditions = append(conditions, `EXISTS (SELECT 1 FROM live_guide_maps mm WHERE mm.addon_id = l.addon_id AND mm.catalog_type = 'tv'
			AND mm.catalog_id = '`+catalogID+`' AND mm.channel_id = l.item_id AND mm.guide_id IS NOT NULL) = `+arg(*mapped))
	}
	if strings.TrimSpace(q) != "" {
		conditions = append(conditions, "l.search LIKE "+arg(likePattern(q)))
	}
	if len(conditions) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(conditions, " AND "), args
}

// Channel describes a channel of a line-up by its item identifier.
func (s *Service) Channel(ctx context.Context, scope addons.Scope, source, id accounts.ID) (Channel, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return Channel{}, err
	}
	return s.channel(ctx, source, id)
}

func (s *Service) channel(ctx context.Context, source, id accounts.ID) (Channel, error) {
	_, channels, err := s.channels(ctx, source, " AND l.item_id = $2", []any{id}, 0, 0)
	if err != nil {
		return Channel{}, err
	}
	if len(channels) == 0 {
		return Channel{}, addons.ErrNotFound
	}
	return channels[0], nil
}

// ChannelChanges change a channel: whether it is enabled, its name and
// logo (nil Values go back to the provider's), its description, its
// category (nil Value goes back to its provider category) and its fixed
// number (nil Value: none).
type ChannelChanges struct {
	Enabled     *bool
	Name        *Nullable[string]
	Logo        *Nullable[string]
	Description *string
	Category    *Nullable[accounts.ID]
	Number      *Nullable[int]
}

// UpdateChannel changes a channel of a line-up.
func (s *Service) UpdateChannel(ctx context.Context, scope addons.Scope, source, id accounts.ID, changes ChannelChanges) (Channel, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return Channel{}, err
	}
	current, err := s.channel(ctx, source, id)
	if err != nil {
		return Channel{}, err
	}
	var sets []string
	args := []any{source, current.key}
	set := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
	}
	if changes.Enabled != nil {
		set("enabled", *changes.Enabled)
	}
	name := current.Name
	if changes.Name != nil {
		var value *string
		name = current.ProviderName
		if changes.Name.Value != nil {
			trimmed := strings.TrimSpace(*changes.Name.Value)
			if !printable(trimmed, 100) {
				return Channel{}, ErrInvalidChannelName
			}
			value, name = &trimmed, trimmed
		}
		set("name", value)
		set("search", Fold(name+"\n"+current.ProviderName+"\n"+current.GuideID))
	}
	if changes.Logo != nil {
		var value *string
		if changes.Logo.Value != nil {
			logo := strings.TrimSpace(*changes.Logo.Value)
			if !webAddress(logo) {
				return Channel{}, ErrInvalidLogo
			}
			value = &logo
		}
		set("logo", value)
	}
	if changes.Description != nil {
		description := strings.TrimSpace(*changes.Description)
		if utf8.RuneCountInString(description) > 2000 {
			return Channel{}, ErrInvalidDescription
		}
		set("description", description)
	}
	if changes.Category != nil {
		var value *accounts.ID
		if changes.Category.Value != nil && *changes.Category.Value != current.ProviderCategory {
			if _, err := s.category(ctx, source, *changes.Category.Value); errors.Is(err, addons.ErrNotFound) {
				return Channel{}, ErrInvalidCategory
			} else if err != nil {
				return Channel{}, err
			}
			value = changes.Category.Value
		}
		set("moved_to", value)
	}
	if changes.Number != nil {
		if changes.Number.Value != nil && (*changes.Number.Value < 1 || *changes.Number.Value > 99999) {
			return Channel{}, ErrInvalidNumber
		}
		set("number", changes.Number.Value)
	}
	if len(sets) > 0 {
		if _, err := s.db.Exec(ctx, "UPDATE iptv_lineup SET "+strings.Join(sets, ", ")+" WHERE addon_id = $1 AND id = $2", args...); err != nil {
			return Channel{}, err
		}
		s.forget(source)
	}
	return s.channel(ctx, source, id)
}

// MoveChannel puts a channel before another one of its category, or last
// in it when before is nil.
func (s *Service) MoveChannel(ctx context.Context, scope addons.Scope, source, id accounts.ID, before *accounts.ID) error {
	if err := s.owned(ctx, scope, source); err != nil {
		return err
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var key string
		var category accounts.ID
		err := tx.QueryRow(ctx, `SELECT id, coalesce(moved_to, category_id) FROM iptv_lineup WHERE addon_id = $1 AND item_id = $2 FOR UPDATE`,
			source, id).Scan(&key, &category)
		if errors.Is(err, pgx.ErrNoRows) {
			return addons.ErrNotFound
		}
		if err != nil {
			return err
		}
		// The category's channels but this one, in order.
		rows, err := tx.Query(ctx, `SELECT id, item_id, sort FROM iptv_lineup WHERE addon_id = $1 AND coalesce(moved_to, category_id) = $2
			AND id <> $3 ORDER BY sort, id`, source, category, key)
		if err != nil {
			return err
		}
		type placed struct {
			key  string
			item accounts.ID
			sort float64
		}
		var others []placed
		var p placed
		if _, err := pgx.ForEachRow(rows, []any{&p.key, &p.item, &p.sort}, func() error { others = append(others, p); return nil }); err != nil {
			return err
		}
		at := len(others)
		if before != nil {
			at = slices.IndexFunc(others, func(o placed) bool { return o.item == *before })
			if at < 0 {
				return ErrInvalidMove
			}
		}
		sortAt := func() (float64, bool) {
			switch {
			case len(others) == 0:
				return 1, true
			case at == len(others):
				return others[at-1].sort + 1, true
			case at == 0:
				return others[0].sort - 1, true
			}
			low, high := others[at-1].sort, others[at].sort
			middle := low + (high-low)/2
			return middle, middle > low && middle < high
		}
		sort, ok := sortAt()
		if !ok {
			// Too close to tell apart: the category is numbered again.
			keys := make([]string, len(others))
			for i, o := range others {
				keys[i] = o.key
				others[i].sort = float64(i + 1)
			}
			if _, err := tx.Exec(ctx, `UPDATE iptv_lineup l SET sort = o.n, sort_set = true FROM unnest($2::text[]) WITH ORDINALITY AS o(id, n)
				WHERE l.addon_id = $1 AND l.id = o.id`, source, keys); err != nil {
				return err
			}
			sort, _ = sortAt()
		}
		_, err = tx.Exec(ctx, "UPDATE iptv_lineup SET sort = $3, sort_set = true WHERE addon_id = $1 AND id = $2", source, key, sort)
		return err
	})
	if err == nil {
		s.forget(source)
	}
	return err
}

// Bulk selects the channels a bulk change applies to: the channels ids,
// those of a category, or those whose names hold Q (at least 2
// characters), optionally in a category. Exactly one selector is given.
type Bulk struct {
	Enabled    bool
	DryRun     bool
	IDs        []accounts.ID
	Category   *accounts.ID
	Q          string
	InCategory *accounts.ID
}

// BulkChannels enables or disables the channels a bulk change selects, or,
// for a dry run, only counts them. It reports how many matched and how
// many changed.
func (s *Service) BulkChannels(ctx context.Context, scope addons.Scope, source accounts.ID, b Bulk) (int, int, error) {
	selectors := 0
	if b.IDs != nil {
		selectors++
	}
	if b.Category != nil {
		selectors++
	}
	q := strings.TrimSpace(b.Q)
	if q != "" {
		selectors++
	}
	if selectors != 1 || len(b.IDs) > 5000 || q != "" && utf8.RuneCountInString(q) < 2 || b.InCategory != nil && q == "" {
		return 0, 0, ErrInvalidBulk
	}
	if err := s.owned(ctx, scope, source); err != nil {
		return 0, 0, err
	}
	category := b.Category
	if b.InCategory != nil {
		category = b.InCategory
	}
	filter, args := channelFilter(category, nil, nil, nil, q)
	if b.IDs != nil {
		args = append(args, b.IDs)
		filter += " AND l.item_id = ANY($" + strconv.Itoa(len(args)+1) + ")"
	}
	from := " FROM iptv_lineup l JOIN iptv_categories c ON c.id = coalesce(l.moved_to, l.category_id) WHERE l.addon_id = $1" + filter
	var matched int
	if err := s.db.QueryRow(ctx, "SELECT count(*)"+from, append([]any{source}, args...)...).Scan(&matched); err != nil || b.DryRun {
		return matched, 0, err
	}
	args = append([]any{source}, args...)
	args = append(args, b.Enabled)
	tag, err := s.db.Exec(ctx, `UPDATE iptv_lineup u SET enabled = $`+strconv.Itoa(len(args))+` WHERE u.addon_id = $1 AND u.enabled <> $`+strconv.Itoa(len(args))+
		` AND u.id IN (SELECT l.id`+from+`)`, args...)
	s.forget(source)
	return matched, int(tag.RowsAffected()), err
}

// StreamSetting is a stream's place and enabled flag, in SetStreams.
type StreamSetting struct {
	ID      string
	Enabled bool
}

// SetStreams sets the order and the enabled flags of a channel's streams;
// streams must list each of them once.
func (s *Service) SetStreams(ctx context.Context, scope addons.Scope, source, id accounts.ID, streams []StreamSetting) (Channel, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return Channel{}, err
	}
	current, err := s.channel(ctx, source, id)
	if err != nil {
		return Channel{}, err
	}
	keys, enabled := make([]string, len(streams)), make([]bool, len(streams))
	known := map[string]bool{}
	for _, st := range current.Streams {
		known[st.ID] = true
	}
	for i, st := range streams {
		if !known[st.ID] {
			return Channel{}, ErrInvalidStreams
		}
		delete(known, st.ID)
		keys[i], enabled[i] = st.ID, st.Enabled
	}
	if len(known) > 0 || len(streams) != len(current.Streams) {
		return Channel{}, ErrInvalidStreams
	}
	if _, err := s.db.Exec(ctx, `UPDATE iptv_streams s SET sort = o.n, enabled = o.e
		FROM unnest($3::text[], $4::bool[]) WITH ORDINALITY AS o(k, e, n) WHERE s.addon_id = $1 AND s.channel_id = $2 AND s.key = o.k`,
		source, current.key, keys, enabled); err != nil {
		return Channel{}, err
	}
	s.forget(source)
	return s.channel(ctx, source, id)
}

// AddStream adds a custom stream to a channel, last. confined refuses an
// address on a local network.
func (s *Service) AddStream(ctx context.Context, scope addons.Scope, source, id accounts.ID, address, label string, confined bool) (Channel, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return Channel{}, err
	}
	current, err := s.channel(ctx, source, id)
	if err != nil {
		return Channel{}, err
	}
	address = strings.TrimSpace(address)
	if !webAddress(address) {
		return Channel{}, ErrInvalidStreamURL
	}
	label = cmp.Or(strings.TrimSpace(label), "Custom")
	if !printable(label, 32) {
		return Channel{}, ErrInvalidStreamLabel
	}
	if confined {
		parsed, _ := url.Parse(address)
		if err := stremio.CheckPublic(ctx, parsed.Hostname()); err != nil {
			return Channel{}, err
		}
	}
	// A custom stream follows the provider's, whose ranks are their places,
	// and the custom ones before it; an order the administrator set puts
	// it after every stream too.
	if _, err := s.db.Exec(ctx, `INSERT INTO iptv_streams (addon_id, key, channel_id, label, rank, sort, custom_url)
		SELECT $1, $2, $3, $4, 1000000 + count(*) FILTER (WHERE custom_url IS NOT NULL), CASE WHEN count(sort) > 0 THEN max(sort) + 1 END, $5
		FROM iptv_streams WHERE addon_id = $1 AND channel_id = $3`,
		source, "u:"+randomKey(), current.key, label, address); err != nil {
		return Channel{}, err
	}
	s.forget(source)
	return s.channel(ctx, source, id)
}

// DeleteStream removes a custom stream of a channel.
func (s *Service) DeleteStream(ctx context.Context, scope addons.Scope, source, id accounts.ID, stream string) (Channel, error) {
	if err := s.owned(ctx, scope, source); err != nil {
		return Channel{}, err
	}
	current, err := s.channel(ctx, source, id)
	if err != nil {
		return Channel{}, err
	}
	index := slices.IndexFunc(current.Streams, func(st Stream) bool { return st.ID == stream })
	if index < 0 {
		return Channel{}, addons.ErrNotFound
	}
	if !current.Streams[index].Custom {
		return Channel{}, ErrStreamNotCustom
	}
	if _, err := s.db.Exec(ctx, "DELETE FROM iptv_streams WHERE addon_id = $1 AND key = $2", source, stream); err != nil {
		return Channel{}, err
	}
	s.forget(source)
	return s.channel(ctx, source, id)
}

// PreviewCategory is a category a list would import: its preview key (an
// exclusion key), its name, how many entries it has, and whether the
// source excludes it.
type PreviewCategory struct {
	Key      string
	Name     string
	Channels int
	Excluded bool
}

// Preview kinds.
const (
	PreviewGroups    = "group"
	PreviewCountries = "country"
)

// preview groups entries by group or by country, those whose name or key
// holds q, in import order.
func preview(entries []storedEntry, by, q string, excluded []string) []PreviewCategory {
	var keys []string
	categories := map[string]*PreviewCategory{}
	country := countries(entries)
	for i, e := range entries {
		key, name := groupKey(e.Group), e.Group
		if by == PreviewCountries {
			key, name = countryKey(country[i]), country[i]
		}
		c := categories[key]
		if c == nil {
			c = &PreviewCategory{Key: key, Name: name, Excluded: slices.Contains(excluded, key)}
			categories[key] = c
			keys = append(keys, key)
		}
		c.Channels++
	}
	if by == PreviewCountries {
		slices.SortStableFunc(keys, func(a, b string) int {
			if (a == countryKey(otherCountry)) != (b == countryKey(otherCountry)) {
				if a == countryKey(otherCountry) {
					return 1
				}
				return -1
			}
			return strings.Compare(a, b)
		})
	}
	needle := Fold(strings.TrimSpace(q))
	result := []PreviewCategory{}
	for _, key := range keys {
		c := categories[key]
		if needle == "" || strings.Contains(Fold(c.Name), needle) || strings.Contains(Fold(c.Key), needle) {
			result = append(result, *c)
		}
	}
	return result
}

// Preview groups a source's stored list by group or by country, without
// downloading it. It reports how many entries the list has.
func (s *Service) Preview(ctx context.Context, scope addons.Scope, source accounts.ID, by, q string) (int, []PreviewCategory, error) {
	if by != PreviewGroups && by != PreviewCountries {
		return 0, nil, ErrInvalidOptions
	}
	if err := s.owned(ctx, scope, source); err != nil {
		return 0, nil, err
	}
	entries, err := loadEntries(ctx, s.db, source)
	if err != nil {
		return 0, nil, err
	}
	options, err := loadOptions(ctx, s.db, source)
	if err != nil {
		return 0, nil, err
	}
	return len(entries), preview(entries, by, q, options.Excluded), nil
}

// PreviewAccount downloads an account's list and groups it by group or by
// country, storing nothing but, for a few minutes, the list in memory:
// previewing it again or adding the source takes it.
func (s *Service) PreviewAccount(ctx context.Context, account Account, by, q string, confined bool) (int, []PreviewCategory, error) {
	if by != PreviewGroups && by != PreviewCountries {
		return 0, nil, ErrInvalidOptions
	}
	address, err := account.address()
	if err != nil {
		return 0, nil, err
	}
	fetched, err := s.fetchCached(ctx, accountOf(account.Kind, address), confined, true)
	if err != nil {
		return 0, nil, err
	}
	entries := make([]storedEntry, len(fetched))
	for i, e := range fetched {
		entries[i] = storedEntry{Name: e.Name, Group: e.Group}
	}
	return len(entries), preview(entries, by, q, nil), nil
}
