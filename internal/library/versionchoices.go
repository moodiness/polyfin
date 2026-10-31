package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Administrators may pin a movie's or an episode's version, which every
// user's details and PlaybackInfo then list first while an addon lists it,
// and block a file, for one title or for every title: its versions are
// then listed nowhere, nor played by their identifiers. A pin names the
// title's version (see versionID), a block the file (see fileKey). Both
// are few, kept in the database and read once into memory, which every
// listing reads.

// VersionPin is the version an administrator pinned for a movie or an
// episode: its identifier, and its name, file name and size as it was
// listed, for when no addon lists it any more.
type VersionPin struct {
	Item, Version  accounts.ID
	Name, Filename string
	Size           int64
	// By is the administrator who pinned it, zero once their account is
	// gone.
	By accounts.ID
	At time.Time
}

// VersionBlock is a file an administrator blocked, for the title Item
// names, or for every title when Item is zero; Name, Filename and Size
// tell it as it was listed, and Reason why it was blocked.
type VersionBlock struct {
	ID, Item       accounts.ID
	Name, Filename string
	Size           int64
	Reason         string
	// By is the administrator who blocked it, zero once their account is
	// gone.
	By accounts.ID
	At time.Time
	// file is the file's key (see fileKey).
	file string
}

// Everywhere reports whether the block holds for every title.
func (b VersionBlock) Everywhere() bool { return b.Item == (accounts.ID{}) }

// MaxBlockReason bounds the reason of a block, in characters.
const MaxBlockReason = 200

// maxChoiceText bounds the name and file name a pin or a block keeps, in
// bytes, as the database does.
const maxChoiceText = 1000

// fileKey is the key of the file a stream is, by its identity (see
// streamIdentity): its name and size, else its link, which may carry a
// token, so that the database keeps neither.
func fileKey(identity string) string {
	sum := sha256.Sum256([]byte("polyfin:file:" + identity))
	return hex.EncodeToString(sum[:])
}

// choiceSet is what administrators chose for versions, as listings read
// it: pins by item, and blocks by item and file, the item zero for a
// block of every title.
type choiceSet struct {
	pins   map[accounts.ID]VersionPin
	blocks map[blockKey]VersionBlock
}

type blockKey struct {
	item accounts.ID
	file string
}

// blocked returns the block of a version's file, for its title or for
// every title, and whether there is one. Versions without a file's
// identity, a track's or a recording's, are never blocked.
func (c choiceSet) blocked(version Version) (VersionBlock, bool) {
	if len(c.blocks) == 0 || version.identity == "" {
		return VersionBlock{}, false
	}
	file := fileKey(version.identity)
	if block, ok := c.blocks[blockKey{version.Item, file}]; ok {
		return block, true
	}
	block, ok := c.blocks[blockKey{file: file}]
	return block, ok
}

// versionChoices holds the choices read from the database, nil until
// read; mu orders the changes, each read again whole afterwards.
type versionChoices struct {
	set atomic.Pointer[choiceSet]
	mu  sync.Mutex
}

// versionChoices returns what administrators chose for versions, reading
// it from the database the first time. A read that fails is logged, and
// tried again by the next listing: meanwhile, nothing is pinned or blocked.
func (s *Service) versionChoices(ctx context.Context) choiceSet {
	if set := s.choices.set.Load(); set != nil {
		return *set
	}
	set, err := s.readChoices(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("The pinned and blocked versions could not be read", "error", err)
		}
		return choiceSet{}
	}
	s.choices.set.CompareAndSwap(nil, &set)
	return *s.choices.set.Load()
}

// readChoices reads the pins and blocks from the database.
func (s *Service) readChoices(ctx context.Context) (choiceSet, error) {
	set := choiceSet{pins: map[accounts.ID]VersionPin{}, blocks: map[blockKey]VersionBlock{}}
	rows, err := s.db.Query(ctx, `SELECT item_id, version_id, name, file_name, size, pinned_by, pinned_at FROM version_pins`)
	if err != nil {
		return choiceSet{}, err
	}
	pins, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (VersionPin, error) {
		var pin VersionPin
		var by *accounts.ID
		err := row.Scan(&pin.Item, &pin.Version, &pin.Name, &pin.Filename, &pin.Size, &by, &pin.At)
		if by != nil {
			pin.By = *by
		}
		return pin, err
	})
	if err != nil {
		return choiceSet{}, err
	}
	for _, pin := range pins {
		set.pins[pin.Item] = pin
	}
	rows, err = s.db.Query(ctx, `SELECT id, item_id, file, name, file_name, size, reason, blocked_by, blocked_at FROM version_blocks`)
	if err != nil {
		return choiceSet{}, err
	}
	blocks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (VersionBlock, error) {
		var block VersionBlock
		var item, by *accounts.ID
		err := row.Scan(&block.ID, &item, &block.file, &block.Name, &block.Filename, &block.Size, &block.Reason, &by, &block.At)
		if item != nil {
			block.Item = *item
		}
		if by != nil {
			block.By = *by
		}
		return block, err
	})
	if err != nil {
		return choiceSet{}, err
	}
	for _, block := range blocks {
		set.blocks[blockKey{block.Item, block.file}] = block
	}
	return set, nil
}

// changeChoices runs change on the database, then reads the choices again
// for the listings, and tells the pages of item that follow its versions,
// unless item is zero.
func (s *Service) changeChoices(ctx context.Context, item accounts.ID, change func() error) error {
	s.choices.mu.Lock()
	defer s.choices.mu.Unlock()
	if err := change(); err != nil {
		return err
	}
	set, err := s.readChoices(ctx)
	if err != nil {
		// The next listing reads them.
		s.choices.set.Store(nil)
	} else {
		s.choices.set.Store(&set)
	}
	if item != (accounts.ID{}) {
		s.versionsChanged(item)
	}
	return err
}

// PinnedVersion returns the version pinned for item, and whether one is.
func (s *Service) PinnedVersion(ctx context.Context, item accounts.ID) (VersionPin, bool) {
	pin, ok := s.versionChoices(ctx).pins[item]
	return pin, ok
}

// PinVersion pins version for its title, in place of the one pinned
// before, for by, an administrator.
func (s *Service) PinVersion(ctx context.Context, by accounts.ID, version Version) error {
	return s.changeChoices(ctx, version.Item, func() error {
		_, err := s.db.Exec(ctx, `INSERT INTO version_pins (item_id, version_id, name, file_name, size, pinned_by, pinned_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (item_id) DO UPDATE SET version_id = excluded.version_id, name = excluded.name, file_name = excluded.file_name,
				size = excluded.size, pinned_by = excluded.pinned_by, pinned_at = excluded.pinned_at`,
			version.Item, version.ID, clipBytes(version.Name, maxChoiceText), clipBytes(version.Filename, maxChoiceText), max(version.Size, 0),
			by, s.now())
		return err
	})
}

// Unpin unpins the version pinned for item, if any.
func (s *Service) Unpin(ctx context.Context, item accounts.ID) error {
	return s.changeChoices(ctx, item, func() error {
		_, err := s.db.Exec(ctx, `DELETE FROM version_pins WHERE item_id = $1`, item)
		return err
	})
}

// ErrNotBlockable is a version whose file cannot be blocked: one without
// a file's identity, as a track's.
var ErrNotBlockable = errors.New("the version cannot be blocked")

// BlockVersion blocks version's file for its title, or for every title
// with everywhere set, for by, an administrator, and why: reason, at most
// MaxBlockReason characters. A file blocked again keeps its block, with
// the new reason.
func (s *Service) BlockVersion(ctx context.Context, by accounts.ID, version Version, everywhere bool, reason string) (VersionBlock, error) {
	if version.identity == "" {
		return VersionBlock{}, ErrNotBlockable
	}
	key := blockKey{file: fileKey(version.identity)}
	var item *accounts.ID
	if !everywhere {
		key.item, item = version.Item, &version.Item
	}
	err := s.changeChoices(ctx, version.Item, func() error {
		_, err := s.db.Exec(ctx, `INSERT INTO version_blocks (id, item_id, file, name, file_name, size, reason, blocked_by, blocked_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (item_id, file) DO UPDATE SET reason = excluded.reason, blocked_by = excluded.blocked_by, blocked_at = excluded.blocked_at`,
			item, key.file, clipBytes(version.Name, maxChoiceText), clipBytes(version.Filename, maxChoiceText),
			max(version.Size, 0), reason, by, s.now())
		return err
	})
	if err != nil {
		return VersionBlock{}, err
	}
	return s.versionChoices(ctx).blocks[key], nil
}

// Unblock lifts the block id names; ErrNotFound when there is none.
func (s *Service) Unblock(ctx context.Context, id accounts.ID) error {
	var item *accounts.ID
	err := s.changeChoices(ctx, accounts.ID{}, func() error {
		return s.db.QueryRow(ctx, `DELETE FROM version_blocks WHERE id = $1 RETURNING item_id`, id).Scan(&item)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err == nil && item != nil {
		s.versionsChanged(*item)
	}
	return err
}

// VersionBlocks lists the blocks of item's files, or, item zero, the
// blocks of every title, the latest first.
func (s *Service) VersionBlocks(ctx context.Context, item accounts.ID) []VersionBlock {
	var blocks []VersionBlock
	for key, block := range s.versionChoices(ctx).blocks {
		if key.item == item {
			blocks = append(blocks, block)
		}
	}
	slices.SortFunc(blocks, func(a, b VersionBlock) int { return b.At.Compare(a.At) })
	return blocks
}

// clipBytes shortens text to at most n bytes, without cutting a
// character.
func clipBytes(text string, n int) string {
	if len(text) <= n {
		return text
	}
	for n > 0 && text[n]&0xc0 == 0x80 {
		n--
	}
	return text[:n]
}

// The states of an addon's list for a title, as SourceExplained tells
// them.
const (
	// ListKnown: its list is known, and not asked again.
	ListKnown = "known"
	// ListStale: its list expired; it is listed while the addon is asked
	// again.
	ListStale = "stale"
	// ListAsked: its list is not known; the addon is asked.
	ListAsked = "asked"
)

// VersionsExplained tells what became of every stream the addons listed
// for a movie or an episode, for a user: the streams of each source, in
// the order they come, and the versions they make, as Versions lists them.
// Pending is how many addons are still asked.
type VersionsExplained struct {
	Sources  []SourceExplained
	Versions []Version
	Pending  int
}

// SourceExplained is one addon or folder asked for a title's streams, and
// what it listed; Embedded marks the streams the title's description
// embeds for the video (see embed). State tells whether its list is known
// (see ListKnown).
type SourceExplained struct {
	Addon    accounts.ID
	Name     string
	Embedded bool
	State    string
	Streams  []StreamExplained
}

// StreamExplained is a stream as an addon listed it, and what became of
// it, Step (see StepVersion): Version is the version it makes, zero for a
// stream with nothing to play, and Block the block of its file for
// StepBlocked.
type StreamExplained struct {
	Name     string
	Filename string
	Size     int64
	Torrent  bool
	Version  Version
	Step     string
	Block    VersionBlock
}

// ExplainVersions lists the versions of a movie, a home video or an
// episode for a user, as VersionsNow does, telling what became of every
// stream each addon listed. It asks the addons whose lists are not known,
// or are stale, in the background, as item details do. ErrNotFound is
// returned for an item the user cannot reach, or of another kind.
func (s *Service) ExplainVersions(ctx context.Context, user accounts.User, id accounts.ID) (VersionsExplained, error) {
	t, v, err := s.target(ctx, user, id)
	if err != nil {
		return VersionsExplained{}, err
	}
	if t.kind != KindMovie && t.kind != KindVideo && t.kind != KindEpisode {
		return VersionsExplained{}, ErrNotFound
	}
	serving := s.streamServing(ctx, v, t)
	if len(t.embedded) > 0 {
		serving = slices.DeleteFunc(serving, func(source streamSource) bool { return source.addon.ID == t.embeddedBy.addon.ID })
	}
	tracked := s.tracked()
	tracked.titles.Put(listedTitle{t.metaType, t.id}, t.item)
	for _, source := range serving {
		if source.id != t.id || source.contentType != t.metaType {
			tracked.titles.Put(listedTitle{source.contentType, source.id}, t.item)
		}
	}
	lists, unknown := s.gatherStreams(ctx, t, serving, askLater)
	var explained VersionsExplained
	var embedded *SourceExplained
	if len(t.embedded) > 0 {
		embedded = &SourceExplained{Addon: t.embeddedBy.addon.ID, Name: t.embeddedBy.addon.Manifest.Name, Embedded: true, State: ListKnown}
	}
	explained.Sources = make([]SourceExplained, len(serving))
	for i, source := range serving {
		state := ListKnown
		if slices.ContainsFunc(unknown, func(other streamSource) bool { return other.key() == source.key() }) {
			state = ListAsked
			if lists[i] != nil {
				state = ListStale
			}
		}
		explained.Sources[i] = SourceExplained{Addon: source.addon.ID, Name: source.addon.Manifest.Name, State: state}
	}
	choices := s.versionChoices(ctx)
	note := func(source int, stream stremio.Stream, version Version, step string) {
		listed := StreamExplained{Name: versionName(stream), Filename: stream.BehaviorHints.Filename, Size: int64(stream.BehaviorHints.VideoSize),
			Torrent: stream.Torrent(), Version: version, Step: step}
		if step == StepBlocked {
			listed.Block, _ = choices.blocked(version)
		}
		if source < 0 {
			embedded.Streams = append(embedded.Streams, listed)
			return
		}
		explained.Sources[source].Streams = append(explained.Sources[source].Streams, listed)
	}
	explained.Versions = s.buildVersions(t, serving, lists, s.settings().TorrentsAllowed(user), choices, note)
	if embedded != nil {
		explained.Sources = slices.Insert(explained.Sources, 0, *embedded)
	}
	s.askStreams(ctx, user, t, unknown)
	explained.Pending = s.pendingOf(user.ID, t, serving)
	return explained, nil
}
