package virtualtv

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// MaxChannels bounds the channels of the server: every guide listing
// reads them all.
const MaxChannels = 100

// ErrTooMany reports a server that holds MaxChannels channels already.
var ErrTooMany = errors.New("the server holds at most 100 channels of its own")

// Store keeps the channels, and the list of them in memory: the guide and
// playback read it all the time.
type Store struct {
	db *pgxpool.Pool

	mu sync.Mutex
	// all is every channel, in order, nil until read or once changed;
	// generation counts the changes, so that a list read while one was
	// made is not kept.
	all        []Channel
	generation int
}

// New returns a store of the channels in db.
func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

const columns = "id, name, coalesce(number, 0), enabled, source, entries, shuffle, seed, blocks, epoch, updated_at"

// storedSource, storedEntry and storedBlock are how sources, entries and
// blocks are written in the database, items by their identifiers.
type storedSource struct {
	Kind  string           `json:"kind"`
	Items []storedItemName `json:"items"`
}

type storedItemName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type storedEntry struct {
	Entry
	Item  string `json:"item"`
	Title string `json:"title"`
}

type storedBlock struct {
	Start   int           `json:"start"`
	End     int           `json:"end"`
	Source  storedSource  `json:"source"`
	Entries []storedEntry `json:"entries"`
	Shuffle bool          `json:"shuffle"`
}

func storeSource(s Source) storedSource {
	stored := storedSource{Kind: s.Kind, Items: make([]storedItemName, 0, len(s.Items))}
	for _, item := range s.Items {
		stored.Items = append(stored.Items, storedItemName{ID: item.ID.String(), Name: item.Name, Kind: item.Kind})
	}
	return stored
}

func (s storedSource) source() (Source, error) {
	source := Source{Kind: s.Kind}
	for _, item := range s.Items {
		id, err := accounts.ParseID(item.ID)
		if err != nil {
			return Source{}, err
		}
		source.Items = append(source.Items, SourceItem{ID: id, Name: item.Name, Kind: item.Kind})
	}
	return source, nil
}

func storeEntries(entries []Entry) []storedEntry {
	stored := make([]storedEntry, 0, len(entries))
	for _, e := range entries {
		stored = append(stored, storedEntry{Entry: e, Item: e.Item.String(), Title: e.Title.String()})
	}
	return stored
}

func entriesOf(stored []storedEntry) ([]Entry, error) {
	entries := make([]Entry, 0, len(stored))
	for _, s := range stored {
		e := s.Entry
		var err error
		if e.Item, err = accounts.ParseID(s.Item); err != nil {
			return nil, err
		}
		if e.Title, err = accounts.ParseID(s.Title); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// encoded is a channel's source, entries and blocks as written.
func encoded(c Channel) (source, entries, blocks []byte, err error) {
	if source, err = json.Marshal(storeSource(c.Source)); err != nil {
		return nil, nil, nil, err
	}
	if entries, err = json.Marshal(storeEntries(c.Entries)); err != nil {
		return nil, nil, nil, err
	}
	stored := make([]storedBlock, 0, len(c.Blocks))
	for _, b := range c.Blocks {
		stored = append(stored, storedBlock{Start: b.Start, End: b.End, Source: storeSource(b.Source), Entries: storeEntries(b.Entries), Shuffle: b.Shuffle})
	}
	blocks, err = json.Marshal(stored)
	return source, entries, blocks, err
}

// Channels lists every channel, in the order they were made. The list is
// shared: callers do not change it.
func (s *Store) Channels(ctx context.Context) ([]Channel, error) {
	s.mu.Lock()
	all, generation := s.all, s.generation
	s.mu.Unlock()
	if all != nil {
		return all, nil
	}
	all, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation == generation {
		s.all = all
	}
	return all, nil
}

// Channel returns a channel, ErrNotFound when there is none.
func (s *Store) Channel(ctx context.Context, id accounts.ID) (Channel, error) {
	all, err := s.Channels(ctx)
	if err != nil {
		return Channel{}, err
	}
	i := slices.IndexFunc(all, func(c Channel) bool { return c.ID == id })
	if i < 0 {
		return Channel{}, ErrNotFound
	}
	return all[i], nil
}

func (s *Store) load(ctx context.Context) ([]Channel, error) {
	rows, _ := s.db.Query(ctx, "SELECT "+columns+" FROM virtual_channels ORDER BY created_at, id")
	all := []Channel{}
	var c Channel
	var source, entries, blocks []byte
	_, err := pgx.ForEachRow(rows, []any{&c.ID, &c.Name, &c.Number, &c.Enabled, &source, &entries, &c.Shuffle, &c.Seed, &blocks, &c.Epoch, &c.UpdatedAt},
		func() error {
			decoded, err := decode(c, source, entries, blocks)
			if err != nil {
				return err
			}
			all = append(all, decoded)
			return nil
		})
	return all, err
}

func decode(c Channel, source, entries, blocks []byte) (Channel, error) {
	var storedSrc storedSource
	var storedEntries []storedEntry
	var storedBlocks []storedBlock
	if err := errors.Join(json.Unmarshal(source, &storedSrc), json.Unmarshal(entries, &storedEntries), json.Unmarshal(blocks, &storedBlocks)); err != nil {
		return Channel{}, err
	}
	var err error
	if c.Source, err = storedSrc.source(); err != nil {
		return Channel{}, err
	}
	if c.Entries, err = entriesOf(storedEntries); err != nil {
		return Channel{}, err
	}
	c.Blocks = nil
	for _, b := range storedBlocks {
		block := Block{Start: b.Start, End: b.End, Shuffle: b.Shuffle}
		if block.Source, err = b.Source.source(); err != nil {
			return Channel{}, err
		}
		if block.Entries, err = entriesOf(b.Entries); err != nil {
			return Channel{}, err
		}
		c.Blocks = append(c.Blocks, block)
	}
	return c, nil
}

// NewSeed is a seed for a channel's shuffle.
func NewSeed() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.LittleEndian.Uint64(b[:]) >> 1)
}

// Create stores a new channel and returns it, with its identifier.
func (s *Store) Create(ctx context.Context, c Channel) (Channel, error) {
	c.Name = strings.TrimSpace(c.Name)
	if err := c.Validate(); err != nil {
		return Channel{}, err
	}
	source, entries, blocks, err := encoded(c)
	if err != nil {
		return Channel{}, err
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Channels are counted one at a time.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('virtual_channels'))"); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM virtual_channels").Scan(&count); err != nil {
			return err
		}
		if count >= MaxChannels {
			return ErrTooMany
		}
		return tx.QueryRow(ctx, `INSERT INTO virtual_channels (name, number, enabled, source, entries, shuffle, seed, blocks, epoch)
			VALUES ($1, nullif($2, 0), $3, $4, $5, $6, $7, $8, $9) RETURNING id, updated_at`,
			c.Name, c.Number, c.Enabled, source, entries, c.Shuffle, c.Seed, blocks, c.Epoch).Scan(&c.ID, &c.UpdatedAt)
	})
	s.changed()
	if err != nil {
		return Channel{}, err
	}
	return c, nil
}

// Update replaces a channel and returns it.
func (s *Store) Update(ctx context.Context, c Channel) (Channel, error) {
	c.Name = strings.TrimSpace(c.Name)
	if err := c.Validate(); err != nil {
		return Channel{}, err
	}
	source, entries, blocks, err := encoded(c)
	if err != nil {
		return Channel{}, err
	}
	err = s.db.QueryRow(ctx, `UPDATE virtual_channels SET name = $2, number = nullif($3, 0), enabled = $4, source = $5, entries = $6,
		shuffle = $7, seed = $8, blocks = $9, epoch = $10, updated_at = now() WHERE id = $1 RETURNING updated_at`,
		c.ID, c.Name, c.Number, c.Enabled, source, entries, c.Shuffle, c.Seed, blocks, c.Epoch).Scan(&c.UpdatedAt)
	s.changed()
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return c, err
}

// Delete deletes a channel.
func (s *Store) Delete(ctx context.Context, id accounts.ID) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM virtual_channels WHERE id = $1", id)
	s.changed()
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// changed forgets the list kept, read again on next use.
func (s *Store) changed() {
	s.mu.Lock()
	s.all = nil
	s.generation++
	s.mu.Unlock()
}

// Now is the time a channel saved now starts its schedule at: today's
// midnight in loc, so that the guide of the day is full.
func Now(loc *time.Location) time.Time { return DayStart(time.Now(), loc) }
