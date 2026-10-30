package taste

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

const (
	// BuildInterval is how often the profiles are built again.
	BuildInterval = time.Hour
	// maxTitles bounds the titles a profile is built from: those the user
	// did something with last.
	maxTitles = 300
)

// Titles describes the titles a user reaches (see library.Service.ItemsOf).
type Titles interface {
	ItemsOf(ctx context.Context, user accounts.User, ids []accounts.ID, kinds []library.Kind) ([]library.Item, error)
}

// Service builds, keeps and gives the users' profiles. Profiles are kept
// in PostgreSQL, and in memory once read.
type Service struct {
	db     *pgxpool.Pool
	users  *accounts.Store
	data   *userdata.Store
	titles Titles
	logger *slog.Logger
	now    func() time.Time

	mu    sync.Mutex
	saved map[accounts.ID]saved
}

// saved is a user's row: their profile, and whether their titles' details
// tell why a title is for them.
type saved struct {
	profile  Profile
	taglines bool
}

// New returns a service keeping profiles in db, built from the data of
// the users of users, of the titles that titles describes.
func New(db *pgxpool.Pool, users *accounts.Store, data *userdata.Store, titles Titles, logger *slog.Logger) *Service {
	return &Service{db: db, users: users, data: data, titles: titles, logger: logger, now: time.Now, saved: map[accounts.ID]saved{}}
}

// SetClock makes the service read the time from now.
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// load returns a user's row, read once from the database: a user without
// one has no profile and shows taglines.
func (s *Service) load(ctx context.Context, user accounts.ID) (saved, error) {
	s.mu.Lock()
	row, ok := s.saved[user]
	s.mu.Unlock()
	if ok {
		return row, nil
	}
	var profile []byte
	row.taglines = true
	err := s.db.QueryRow(ctx, "SELECT profile, taglines FROM taste_profiles WHERE user_id = $1", user).Scan(&profile, &row.taglines)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return saved{}, err
	}
	if profile != nil {
		if err := json.Unmarshal(profile, &row.profile); err != nil {
			return saved{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// A profile built meanwhile is newer.
	if newer, ok := s.saved[user]; ok {
		return newer, nil
	}
	s.saved[user] = row
	return row, nil
}

// Profile returns the user's profile: the zero Profile until one is
// built.
func (s *Service) Profile(ctx context.Context, user accounts.ID) (Profile, error) {
	row, err := s.load(ctx, user)
	return row.profile, err
}

// Taglines reports whether the user's titles' details tell why a title is
// for them (see Profile.Match): they do unless the user turned them off.
func (s *Service) Taglines(ctx context.Context, user accounts.ID) (bool, error) {
	row, err := s.load(ctx, user)
	return row.taglines, err
}

// SetTaglines turns the user's taglines on or off.
func (s *Service) SetTaglines(ctx context.Context, user accounts.ID, on bool) error {
	if _, err := s.db.Exec(ctx, `INSERT INTO taste_profiles (user_id, taglines) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET taglines = excluded.taglines`, user, on); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// The profile is read again with the setting.
	delete(s.saved, user)
	return nil
}

// BuildAll builds every user's profile again, one user after the other.
// A user whose profile fails is logged and left as it was.
func (s *Service) BuildAll(ctx context.Context) error {
	users, err := s.users.Users(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		if _, err := s.Build(ctx, user); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.logger.Warn("A taste profile could not be built", "user", user.ID.String(), "error", err)
		}
	}
	return nil
}

// Build builds the user's profile again from what they did with the
// movies and series they reach, and keeps it.
func (s *Service) Build(ctx context.Context, user accounts.User) (Profile, error) {
	evidence, err := s.evidence(ctx, user)
	if err != nil {
		return Profile{}, err
	}
	profile := Build(evidence, s.now().UTC())
	encoded, err := json.Marshal(profile)
	if err != nil {
		return Profile{}, err
	}
	var taglines bool
	if err := s.db.QueryRow(ctx, `INSERT INTO taste_profiles (user_id, profile, built_at) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET profile = excluded.profile, built_at = excluded.built_at
		RETURNING taglines`, user.ID, encoded, profile.Built).Scan(&taglines); err != nil {
		return Profile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved[user.ID] = saved{profile: profile, taglines: taglines}
	return profile, nil
}

// evidence is what the user did with each movie and series, the latest
// maxTitles of them, each described: from its data, its episodes' data
// for a series, and the titles hidden. The titles the user no longer
// reaches are left out.
func (s *Service) evidence(ctx context.Context, user accounts.User) ([]Evidence, error) {
	entries, err := s.data.Touched(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	hidden, err := s.data.Hidden(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	activities := map[accounts.ID]*Activity{}
	var order []accounts.ID
	of := func(id accounts.ID) *Activity {
		a := activities[id]
		if a == nil {
			a = &Activity{}
			activities[id] = a
			order = append(order, id)
		}
		return a
	}
	later := func(a *Activity, at *time.Time) {
		if at != nil && at.After(a.At) {
			a.At = *at
		}
	}
	// The episodes of each series finished, and whether one was left
	// further than early without being finished.
	type episodes struct {
		finished int
		further  bool
	}
	series := map[accounts.ID]*episodes{}
	for _, e := range entries {
		if e.Series != (accounts.ID{}) {
			later(of(e.Series), e.LastPlayed)
			counts := series[e.Series]
			if counts == nil {
				counts = &episodes{}
				series[e.Series] = counts
			}
			if e.Played {
				counts.finished++
			} else if e.LastPlayed != nil && !Early(e.Position, e.Runtime) {
				counts.further = true
			}
			continue
		}
		a := of(e.Item)
		later(a, e.LastPlayed)
		a.Favorite, a.Rating = e.Favorite, e.Rating
		if e.Played {
			a.Finished = max(e.PlayCount, 1)
		} else if e.LastPlayed != nil {
			a.Early = Early(e.Position, e.Runtime)
		}
	}
	// A series counts its episodes finished, or was abandoned early when
	// every episode started was.
	for id, counts := range series {
		a := activities[id]
		a.Series = true
		a.Finished = max(a.Finished, counts.finished)
		a.Early = a.Finished == 0 && !counts.further
	}
	for _, h := range hidden {
		a := of(h.ID)
		a.Hidden = true
		later(a, &h.At)
	}
	// The latest first, those never played after them.
	slices.SortStableFunc(order, func(a, b accounts.ID) int { return activities[b].At.Compare(activities[a].At) })
	order = order[:min(maxTitles, len(order))]
	items, err := s.titles.ItemsOf(ctx, user, order, []library.Kind{library.KindMovie, library.KindSeries})
	if err != nil {
		return nil, err
	}
	evidence := make([]Evidence, 0, len(items))
	for _, item := range items {
		a, ok := activities[item.ID]
		if !ok {
			continue
		}
		a.Series = item.Kind == library.KindSeries
		evidence = append(evidence, Evidence{Title: TitleOf(item), Activity: *a})
	}
	// Oldest first, in an order of their own, whatever order the titles
	// were described in.
	slices.SortStableFunc(evidence, func(a, b Evidence) int { return cmp.Compare(a.Activity.At.UnixNano(), b.Activity.At.UnixNano()) })
	return evidence, nil
}
