package silo

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// errUnknownNumber reports a number no identifier was given.
var errUnknownNumber = errors.New("unknown number")

// maxCached bounds the numbers kept in memory: past it, they are read
// again from the database.
const maxCached = 100_000

// numbers gives libraries and versions the numbers Silo's apps know them
// by, which must be positive 32-bit integers: each identifier gets one the
// first time an answer names it, kept in the silo_numbers table.
type numbers struct {
	db       *pgxpool.Pool
	mu       sync.Mutex
	byID     map[accounts.ID]int32
	byNumber map[int32]accounts.ID
	// trips counts the round trips to the database, which the tests read.
	trips atomic.Int64
}

func newNumbers(db *pgxpool.Pool) *numbers {
	return &numbers{db: db, byID: map[accounts.ID]int32{}, byNumber: map[int32]accounts.ID{}}
}

func (n *numbers) keep(id accounts.ID, number int32) {
	if len(n.byID) >= maxCached {
		clear(n.byID)
		clear(n.byNumber)
	}
	n.byID[id], n.byNumber[number] = number, id
}

// of returns the numbers of ids, giving one to those that have none.
func (n *numbers) of(ctx context.Context, ids ...accounts.ID) (map[accounts.ID]int32, error) {
	found := make(map[accounts.ID]int32, len(ids))
	var missing []accounts.ID
	seen := make(map[accounts.ID]bool, len(ids))
	n.mu.Lock()
	for _, id := range ids {
		if number, ok := n.byID[id]; ok {
			found[id] = number
		} else if !seen[id] {
			seen[id] = true
			missing = append(missing, id)
		}
	}
	n.mu.Unlock()
	if len(missing) == 0 {
		return found, nil
	}
	read := func(rows pgx.Rows, err error) error {
		n.trips.Add(1)
		if err != nil {
			return err
		}
		var id accounts.ID
		var number int32
		_, err = pgx.ForEachRow(rows, []any{&id, &number}, func() error {
			found[id] = number
			n.mu.Lock()
			n.keep(id, number)
			n.mu.Unlock()
			return nil
		})
		return err
	}
	if err := read(n.db.Query(ctx, "SELECT id, number FROM silo_numbers WHERE id = ANY($1)", missing)); err != nil {
		return nil, err
	}
	missing = slices.DeleteFunc(missing, func(id accounts.ID) bool { _, ok := found[id]; return ok })
	if len(missing) == 0 {
		return found, nil
	}
	// The rows are written in the order of their keys, so that writers at
	// once never deadlock; one written meanwhile by another is read after.
	slices.SortFunc(missing, func(a, b accounts.ID) int { return slices.Compare(a[:], b[:]) })
	if err := read(n.db.Query(ctx, `INSERT INTO silo_numbers (id) SELECT id FROM unnest($1::uuid[]) AS ids (id) ORDER BY id
		ON CONFLICT (id) DO NOTHING RETURNING id, number`, missing)); err != nil {
		return nil, err
	}
	missing = slices.DeleteFunc(missing, func(id accounts.ID) bool { _, ok := found[id]; return ok })
	if len(missing) > 0 {
		if err := read(n.db.Query(ctx, "SELECT id, number FROM silo_numbers WHERE id = ANY($1)", missing)); err != nil {
			return nil, err
		}
	}
	return found, nil
}

// number is the number of id, given now if it has none.
func (n *numbers) number(ctx context.Context, id accounts.ID) (string, error) {
	found, err := n.of(ctx, id)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(int64(found[id]), 10), nil
}

// id is the identifier a number written by the apps was given;
// errUnknownNumber for none.
func (n *numbers) id(ctx context.Context, text string) (accounts.ID, error) {
	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil || value <= 0 || strconv.FormatInt(value, 10) != text {
		return accounts.ID{}, errUnknownNumber
	}
	number := int32(value)
	n.mu.Lock()
	id, ok := n.byNumber[number]
	n.mu.Unlock()
	if ok {
		return id, nil
	}
	err = n.db.QueryRow(ctx, "SELECT id FROM silo_numbers WHERE number = $1", number).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return accounts.ID{}, errUnknownNumber
	}
	if err != nil {
		return accounts.ID{}, err
	}
	n.mu.Lock()
	n.keep(id, number)
	n.mu.Unlock()
	return id, nil
}
