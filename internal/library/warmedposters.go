package library

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
)

// The titles whose posters are warmed.
//
// Once Warm read the libraries' first pages, it tells the function
// OnFirstPagesRead set, which warms the posters of the titles WarmedTitles
// lists for each user: those of the first page of the libraries the user
// sees, as the user sees them. A user's own libraries count only once the
// user opened them, for openedFor: the server's libraries serve every
// user, a user's own ones serve one.

const (
	// openedFor is how long a user's own library counts as opened.
	openedFor = 7 * 24 * time.Hour
	// openedLibraries bounds the libraries remembered as opened.
	openedLibraries = 10000
)

// warmedPosters holds what told the reads of first pages, and the users'
// own libraries opened lately, by library.
type warmedPosters struct {
	read   atomic.Pointer[func(context.Context)]
	opened *cache.Cache[accounts.ID, struct{}]
}

// OnFirstPagesRead has read called once Warm read the libraries' first
// pages, on Warm's goroutine: Warm ends once read returns.
func (s *Service) OnFirstPagesRead(read func(ctx context.Context)) {
	s.warmed.read.Store(&read)
}

// firstPagesRead tells that the libraries' first pages were read.
func (s *Service) firstPagesRead(ctx context.Context) {
	if read := s.warmed.read.Load(); read != nil {
		(*read)(ctx)
	}
}

// openingKey marks the context of a listing an app asked for (see
// WithOpening).
type openingKey struct{}

// WithOpening marks a context whose listings an app asked for: a listing
// of one of the user's own libraries under it opens that library, whose
// posters are then warmed (see WarmedTitles).
func WithOpening(ctx context.Context) context.Context {
	return context.WithValue(ctx, openingKey{}, true)
}

func opening(ctx context.Context) bool {
	opened, _ := ctx.Value(openingKey{}).(bool)
	return opened
}

// openedLibrary remembers that a user's own library was opened; the
// server's libraries need not be.
func (s *Service) openedLibrary(l library) {
	if !l.addon.shared {
		s.warmed.opened.Put(l.item.ID, struct{}{})
	}
}

// WarmedTitles lists the first count titles of the first page of each of
// the user's libraries whose posters are warmed, as the user sees them:
// their parental control, blocked genres and hidden titles leave titles
// out, and the server's libraries hidden from them are not theirs. Those
// are the libraries but music ones, the user's own only once opened (see
// WithOpening). It reads the pages kept, asking no addon, and waits for
// no rating: titles whose rating or genres are not known yet are left
// out.
func (s *Service) WarmedTitles(ctx context.Context, user accounts.User, count int) ([]accounts.ID, error) {
	v, err := s.buildView(ctx, user)
	if err != nil {
		return nil, err
	}
	v = v.withoutWait()
	ctx = withKeptOnly(ctx)
	var titles []accounts.ID
	for _, l := range v.libraries {
		if l.addon.addon.HasMusic() {
			continue
		}
		if _, opened := s.warmed.opened.Get(l.item.ID); !l.addon.shared && !opened {
			continue
		}
		page, err := s.libraryChildren(ctx, v, l, 0, count, "", false)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			titles = append(titles, item.ID)
		}
	}
	return titles, nil
}
