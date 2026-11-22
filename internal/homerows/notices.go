package homerows

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// tellEvery is how often the Teller looks for the rows that started
// showing to their users.
const tellEvery = time.Minute

// connectedWait is how long after an app connects what its user's apps
// still have to show is shown: an app just connected is still starting,
// and jellyfin-web closes a dialog as it leaves its sign-in page.
const connectedWait = 5 * time.Second

// Counts tells, for a row shown to chosen users, how many of them there
// are, how many were told of it, and how many saw it. They never say who.
type Counts struct {
	Users, Told, Seen int
}

// Counts returns the counts of each row shown to chosen users.
func (s *Store) Counts(ctx context.Context) (map[accounts.ID]Counts, error) {
	rows, _ := s.db.Query(ctx, `SELECT u.row_id, count(*), count(n.told_at), count(n.seen_at)
		FROM home_row_users u LEFT JOIN home_row_notices n ON n.row_id = u.row_id AND n.user_id = u.user_id
		GROUP BY u.row_id`)
	result := map[accounts.ID]Counts{}
	var row accounts.ID
	var c Counts
	_, err := pgx.ForEachRow(rows, []any{&row, &c.Users, &c.Told, &c.Seen}, func() error {
		result[row] = c
		return nil
	})
	return result, err
}

// Seen returns the rows shown to chosen users that user saw.
func (s *Store) Seen(ctx context.Context, user accounts.ID) (map[accounts.ID]bool, error) {
	rows, _ := s.db.Query(ctx, "SELECT row_id FROM home_row_notices WHERE user_id = $1 AND seen_at IS NOT NULL", user)
	seen := map[accounts.ID]bool{}
	var id accounts.ID
	_, err := pgx.ForEachRow(rows, []any{&id}, func() error {
		seen[id] = true
		return nil
	})
	return seen, err
}

// SetSeen records that user saw a row at now, unless they saw it before.
// A row that is no suggestion, or not shown to them, is ErrNotFound.
func (s *Store) SetSeen(ctx context.Context, user, id accounts.ID, now time.Time) error {
	r, err := s.Row(ctx, id)
	if err != nil {
		return err
	}
	if !r.Suggested() || !r.ShownTo(user) {
		return ErrNotFound
	}
	_, err = s.db.Exec(ctx, `INSERT INTO home_row_notices (row_id, user_id, seen_at) VALUES ($1, $2, $3)
		ON CONFLICT (row_id, user_id) DO UPDATE SET seen_at = coalesce(home_row_notices.seen_at, excluded.seen_at)`, id, user, now)
	return err
}

// claim records that user is told of a row at now, and reports whether
// they are to be: a user told before, or who saw the row already, is not.
// It is kept before anything is sent, so that a restart never tells twice.
func (s *Store) claim(ctx context.Context, row, user accounts.ID, now time.Time) (bool, error) {
	var claimed bool
	err := s.db.QueryRow(ctx, `INSERT INTO home_row_notices (row_id, user_id, told_at) VALUES ($1, $2, $3)
		ON CONFLICT (row_id, user_id) DO UPDATE SET told_at = excluded.told_at
		WHERE home_row_notices.told_at IS NULL AND home_row_notices.seen_at IS NULL
		RETURNING true`, row, user, now).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return claimed, err
}

// pendingApps lists the rows user was told of that none of their apps
// showed yet, and that they did not see.
func (s *Store) pendingApps(ctx context.Context, user accounts.ID) ([]accounts.ID, error) {
	rows, _ := s.db.Query(ctx, `SELECT row_id FROM home_row_notices
		WHERE user_id = $1 AND told_at IS NOT NULL AND apps_at IS NULL AND seen_at IS NULL`, user)
	return pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
}

// shownOnApps records that an app of user showed a row at now.
func (s *Store) shownOnApps(ctx context.Context, row, user accounts.ID, now time.Time) error {
	_, err := s.db.Exec(ctx, "UPDATE home_row_notices SET apps_at = $3 WHERE row_id = $1 AND user_id = $2 AND apps_at IS NULL", row, user, now)
	return err
}

// Suggestion is a row a user is told of, and the administrator who signs
// its note, the zero User when it has none.
type Suggestion struct {
	Row  Row
	From accounts.User
}

// Notifier tells users of suggestions; notifications.Service is one.
// TitlesSuggested tells user's own targets, and returns the title and the
// message their apps may show too; SuggestionWords returns them alone.
type Notifier interface {
	TitlesSuggested(user accounts.User, s Suggestion) (title, message string)
	SuggestionWords(s Suggestion) (title, message string)
}

// Apps shows a message on the user's Jellyfin apps that accept messages,
// and returns how many did; jellyfin.Handler is one.
type Apps interface {
	ShowMessage(ctx context.Context, user accounts.ID, header, text string) int
}

// Users finds accounts; accounts.Store is one.
type Users interface {
	User(ctx context.Context, id accounts.ID) (accounts.User, error)
}

// TellerConfig are the dependencies of a Teller.
type TellerConfig struct {
	Store    *Store
	Users    Users
	Notifier Notifier
	// Apps shows the suggestions on the users' apps; nil shows none.
	Apps   Apps
	Logger *slog.Logger
	// Ready is closed once the Notifier can tell: Run tells nothing
	// before. Nil is ready.
	Ready <-chan struct{}
	// Sees reports whether a row shown to user lists them anything: a
	// Playlist row, whether they may see its playlist. Nil sees every row.
	Sees func(ctx context.Context, user accounts.User, row Row) (bool, error)
	// Now is the clock; nil is the system's.
	Now func() time.Time
}

// Teller tells each user of a row that tells them (Row.Notify) once, as it
// starts showing to them: on save when it shows now, at its schedule's
// start, or when they are added to its users. A user who turned the row
// off, whose account may not sign in now, or who may not see what it
// lists, is told once they may. One who saw the row first is not told.
// Their own targets are told, and their apps that accept messages show it:
// those open then, else the first to connect while the row shows, until
// one shows it or the user sees the row.
type Teller struct {
	TellerConfig
	wake chan struct{}
	mu   sync.Mutex
	// connected are the users an app of whom connected since the last
	// look at what their apps still have to show.
	connected map[accounts.ID]bool
}

// NewTeller returns a Teller; Run starts it.
func NewTeller(c TellerConfig) *Teller {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return &Teller{TellerConfig: c, wake: make(chan struct{}, 1), connected: map[accounts.ID]bool{}}
}

// Wake has the Teller look again soon, as after a row was saved.
func (t *Teller) Wake() {
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// Connected tells the Teller that an app of user connected, or said it
// accepts messages: what their apps still have to show is shown a few
// seconds later (connectedWait).
func (t *Teller) Connected(user accounts.ID) {
	time.AfterFunc(connectedWait, func() {
		t.mu.Lock()
		t.connected[user] = true
		t.mu.Unlock()
		t.Wake()
	})
}

// Run looks every minute, and when woken, once Ready, until ctx ends.
func (t *Teller) Run(ctx context.Context) {
	if t.Ready != nil {
		select {
		case <-ctx.Done():
			return
		case <-t.Ready:
		}
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.wake:
		case <-timer.C:
			timer.Reset(tellEvery)
		}
		t.Check(ctx)
		t.mu.Lock()
		users := t.connected
		t.connected = map[accounts.ID]bool{}
		t.mu.Unlock()
		for user := range users {
			t.ShowPending(ctx, user)
		}
	}
}

// Check tells the users of each row that tells them, shown to them now,
// who were not told yet.
func (t *Teller) Check(ctx context.Context) {
	now := t.Now()
	rows, err := t.Store.Rows(ctx)
	if err != nil {
		t.warn(ctx, "The home rows could not be read to tell their users", err)
		return
	}
	var off map[accounts.ID]map[accounts.ID]bool
	var settled map[[2]accounts.ID]bool
	for _, row := range rows {
		if !row.Notify || !row.Suggested() || !row.ShowsAt(now) {
			continue
		}
		if off == nil {
			if off, err = t.Store.allOff(ctx); err == nil {
				settled, err = t.Store.settled(ctx)
			}
			if err != nil {
				t.warn(ctx, "The suggestions told could not be read", err)
				return
			}
		}
		var suggestion *Suggestion
		for _, id := range row.Users {
			if off[id][row.ID] || settled[[2]accounts.ID{row.ID, id}] {
				continue
			}
			user, err := t.Users.User(ctx, id)
			if err != nil {
				if !errors.Is(err, accounts.ErrNotFound) {
					t.warn(ctx, "A home row's user could not be read", err)
				}
				continue
			}
			if user.Refused(now) {
				continue
			}
			if t.Sees != nil {
				sees, err := t.Sees(ctx, user, row)
				if err != nil {
					t.warn(ctx, "Whether a user sees a home row could not be told", err)
				}
				if err != nil || !sees {
					continue
				}
			}
			if suggestion == nil {
				suggestion = &Suggestion{Row: row, From: t.from(ctx, row)}
			}
			claimed, err := t.Store.claim(ctx, row.ID, id, now)
			if err != nil {
				t.warn(ctx, "A home row's telling could not be recorded", err)
				return
			}
			if !claimed {
				continue
			}
			title, message := t.Notifier.TitlesSuggested(user, *suggestion)
			t.show(ctx, row.ID, id, title, message, now)
		}
	}
}

// ShowPending shows on user's apps the rows they were told of that no app
// showed yet, while each still shows to them and they did not see it.
func (t *Teller) ShowPending(ctx context.Context, user accounts.ID) {
	if t.Apps == nil {
		return
	}
	pending, err := t.Store.pendingApps(ctx, user)
	if err != nil || len(pending) == 0 {
		if err != nil {
			t.warn(ctx, "The suggestions for a user's apps could not be read", err)
		}
		return
	}
	now := t.Now()
	shown, err := t.Store.Shown(ctx, user, now)
	if err != nil {
		t.warn(ctx, "The suggestions for a user's apps could not be read", err)
		return
	}
	for _, row := range shown {
		if !row.Notify || !slices.Contains(pending, row.ID) {
			continue
		}
		title, message := t.Notifier.SuggestionWords(Suggestion{Row: row, From: t.from(ctx, row)})
		t.show(ctx, row.ID, user, title, message, now)
	}
}

// show shows a suggestion on user's apps, and records that one did.
func (t *Teller) show(ctx context.Context, row, user accounts.ID, title, message string, now time.Time) {
	if t.Apps == nil || t.Apps.ShowMessage(ctx, user, title, message) == 0 {
		return
	}
	if err := t.Store.shownOnApps(ctx, row, user, now); err != nil {
		t.warn(ctx, "A suggestion shown on an app could not be recorded", err)
	}
}

// from is the administrator who signs row's note, the zero User when none
// does or they cannot be read.
func (t *Teller) from(ctx context.Context, row Row) accounts.User {
	if row.Note == "" || row.NoteBy == (accounts.ID{}) {
		return accounts.User{}
	}
	user, err := t.Users.User(ctx, row.NoteBy)
	if err != nil {
		return accounts.User{}
	}
	return user
}

func (t *Teller) warn(ctx context.Context, message string, err error) {
	if ctx.Err() == nil {
		t.Logger.Warn(message, "error", err)
	}
}

// allOff returns the rows each user turned off.
func (s *Store) allOff(ctx context.Context) (map[accounts.ID]map[accounts.ID]bool, error) {
	rows, _ := s.db.Query(ctx, "SELECT user_id, row_id FROM home_rows_off")
	off := map[accounts.ID]map[accounts.ID]bool{}
	var user, row accounts.ID
	_, err := pgx.ForEachRow(rows, []any{&user, &row}, func() error {
		if off[user] == nil {
			off[user] = map[accounts.ID]bool{}
		}
		off[user][row] = true
		return nil
	})
	return off, err
}

// settled returns the rows and users, as [row, user], whose user was told
// of the row already or saw it: none of them is told again.
func (s *Store) settled(ctx context.Context) (map[[2]accounts.ID]bool, error) {
	rows, _ := s.db.Query(ctx, "SELECT row_id, user_id FROM home_row_notices WHERE told_at IS NOT NULL OR seen_at IS NOT NULL")
	settled := map[[2]accounts.ID]bool{}
	var row, user accounts.ID
	_, err := pgx.ForEachRow(rows, []any{&row, &user}, func() error {
		settled[[2]accounts.ID{row, user}] = true
		return nil
	})
	return settled, err
}
