package appimport

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// run is an import running.
type run struct {
	s *Service
	// status is the import's, which s.mu guards.
	status   *Status
	by, user accounts.User
	entries  []Entry
	addons   []Addon
}

// update changes the status of the import.
func (r *run) update(change func(*Status)) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	change(r.status)
}

// run merges the entries into the user's data, then installs the addons
// chosen, and ends the import. An import that fails installs no addon.
func (r *run) run(ctx context.Context) {
	err := r.merge(ctx)
	if err == nil {
		r.install(ctx)
	}
	r.update(func(s *Status) {
		s.State = StateDone
		if err != nil {
			s.State, s.Problem = StateFailed, ProblemInternal
			for i := range s.Addons {
				s.Addons[i].Problem = AddonInternal
			}
		}
		s.EndedAt = new(r.s.now().UTC())
	})
	if err != nil && ctx.Err() == nil {
		r.s.logger.Warn("An app's watch data could not be imported", "app", r.status.App, "user_id", r.user.ID.String(), "error", err)
	}
}

// Identified reports whether ref has an identifier Polyfin finds titles
// by.
func Identified(ref library.TitleRef) bool {
	return ref.IMDb != "" || ref.TMDB > 0 || ref.TVDB > 0 || ref.Anime != (library.AnimeRef{})
}

// mark is a played mark an import adds: at the latest date the app gives,
// nil when it gives none, and the most plays it counts. counts tells
// whether marking it counts in the import's result (see counts).
type mark struct {
	item   userdata.Item
	at     *time.Time
	plays  int
	counts bool
}

// point is a resume point an import sets.
type point struct {
	item   userdata.Item
	counts bool
	userdata.ImportedResume
}

// counts reports whether t, among the targets found of one title, counts
// in the import's result: the title an anime catalog lists under the
// title's anime entry is the one also found by its usual identifiers,
// counted once, unless it was found that way only.
func counts(found []library.TitleTarget, t library.TitleTarget) bool {
	return !t.Anime || !slices.ContainsFunc(found, func(o library.TitleTarget) bool { return !o.Anime })
}

// merge finds the titles of the entries and adds them to the user's data:
// played marks, then resume points of the titles still unplayed, then
// favorites. It reports the entries it did not find.
func (r *run) merge(ctx context.Context) error {
	refs := make([]library.TitleRef, 0, len(r.entries))
	// of is the index in refs of each entry's title, -1 for one without
	// identifiers.
	of := make([]int, len(r.entries))
	for i, e := range r.entries {
		of[i] = -1
		if Identified(e.Title) {
			of[i] = len(refs)
			refs = append(refs, e.Title)
		}
	}
	targets := make([][]library.TitleTarget, 0, len(refs))
	for start := 0; start < len(refs); start += batch {
		found, err := r.s.titles.Resolve(ctx, refs[start:min(start+batch, len(refs))])
		if err != nil {
			return err
		}
		targets = append(targets, found...)
	}

	var noMatch []Unmatched
	marks := map[accounts.ID]*mark{}
	points := map[accounts.ID]*point{}
	favorites := map[accounts.ID]userdata.Item{}
	var markOrder, pointOrder, favoriteOrder []accounts.ID
	for i, e := range r.entries {
		if of[i] < 0 {
			noMatch = append(noMatch, Unmatched{Label: e.Label, Reason: ReasonOtherIdentifier})
			continue
		}
		found := targets[of[i]]
		if len(found) == 0 {
			noMatch = append(noMatch, Unmatched{Label: e.Label, Reason: ReasonNotFound})
			continue
		}
		for _, t := range found {
			item := userdata.Item{ID: t.ID, Series: t.Series, Season: t.Season}
			if e.Played {
				m := marks[t.ID]
				if m == nil {
					m = &mark{item: item}
					marks[t.ID] = m
					markOrder = append(markOrder, t.ID)
				}
				m.counts = m.counts || counts(found, t)
				if e.PlayedAt != nil && (m.at == nil || e.PlayedAt.After(*m.at)) {
					m.at = e.PlayedAt
				}
				m.plays = max(m.plays, e.Plays)
			}
			if e.Resume != nil && (points[t.ID] == nil || e.Resume.At.After(points[t.ID].At)) {
				if points[t.ID] == nil {
					pointOrder = append(pointOrder, t.ID)
				}
				runtime := e.Resume.Runtime
				if runtime <= 0 {
					runtime = t.Runtime
				}
				points[t.ID] = &point{item: item, counts: counts(found, t),
					ImportedResume: userdata.ImportedResume{Position: e.Resume.Position, Runtime: runtime, At: e.Resume.At}}
			}
			if _, listed := favorites[t.ID]; e.Favorite && !listed {
				favorites[t.ID] = item
				favoriteOrder = append(favoriteOrder, t.ID)
			}
		}
	}
	r.update(func(s *Status) {
		s.UnmatchedCount = len(noMatch)
		s.Unmatched = noMatch[:min(len(noMatch), maxUnmatched)]
	})

	settings := r.s.settings()
	thresholds := userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}
	if err := r.change(ctx, markOrder, func(id accounts.ID) userdata.Item { return marks[id].item },
		func(id accounts.ID, d *userdata.Data, s *Status) {
			m := marks[id]
			if d.ImportPlayed(m.at) && m.counts {
				s.Played++
			}
			// Polyfin counted its own plays: the larger count stands.
			d.PlayCount = max(d.PlayCount, m.plays)
		}); err != nil {
		return err
	}
	if err := r.change(ctx, pointOrder, func(id accounts.ID) userdata.Item { return points[id].item },
		func(id accounts.ID, d *userdata.Data, s *Status) {
			p := points[id]
			switch d.ImportResume(p.ImportedResume, thresholds) {
			case userdata.ResumePlayed:
				if p.counts {
					s.Played++
				}
			case userdata.ResumeSet:
				if p.counts {
					s.Resumed++
				}
			}
		}); err != nil {
		return err
	}
	return r.change(ctx, favoriteOrder, func(id accounts.ID) userdata.Item { return favorites[id] },
		func(_ accounts.ID, d *userdata.Data, s *Status) {
			if !d.Favorite {
				d.Favorite = true
				s.Favorites++
			}
		})
}

// change applies change to the data the user keeps of the items ids name,
// batch by batch, counting what it did in the status once each batch is
// saved.
func (r *run) change(ctx context.Context, ids []accounts.ID, itemOf func(accounts.ID) userdata.Item,
	change func(accounts.ID, *userdata.Data, *Status)) error {
	for start := 0; start < len(ids); start += batch {
		chunk := ids[start:min(start+batch, len(ids))]
		items := make([]userdata.Item, 0, len(chunk))
		for _, id := range chunk {
			items = append(items, itemOf(id))
		}
		var counted Status
		if _, err := r.s.userData.ChangeEach(ctx, r.user.ID, items, func(item userdata.Item, d *userdata.Data) {
			change(item.ID, d, &counted)
		}); err != nil {
			return err
		}
		r.update(func(s *Status) {
			s.Played += counted.Played
			s.Resumed += counted.Resumed
			s.Favorites += counted.Favorites
		})
	}
	return nil
}

// install installs the addons chosen as the user's own, one after the
// other, while the user may add their own. Only an administrator's import
// reaches addresses on local networks.
func (r *run) install(ctx context.Context) {
	for i, a := range r.addons {
		problem := AddonNotAllowed
		if r.s.settings().PersonalAddonsAllowed(r.user) {
			installed, err := r.s.addons.Install(ctx, addons.Personal(r.user.ID), a.ManifestURL, !r.by.IsAdministrator)
			if ctx.Err() != nil {
				return
			}
			problem = addonProblem(err)
			if err == nil {
				r.s.activity.AddonInstalled(ctx, r.user, installed.Manifest.Name, installed.Manifest.Version, false)
			} else if problem == AddonInternal {
				// Not the error, which may hold the address.
				r.s.logger.Warn("An app's addon could not be installed", "app", r.status.App, "user_id", r.user.ID.String())
			}
		}
		r.update(func(s *Status) {
			s.Addons[i].Installed, s.Addons[i].Problem = problem == "", problem
		})
	}
}

// addonProblem names why installing an addon failed, empty when it did
// not.
func addonProblem(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, addons.ErrExists):
		return AddonExists
	case errors.Is(err, stremio.ErrInvalidManifestURL):
		return AddonInvalidAddress
	case errors.Is(err, stremio.ErrPrivateNetwork):
		return AddonPrivateNetwork
	case errors.Is(err, stremio.ErrUnreachable):
		return AddonUnreachable
	case errors.Is(err, stremio.ErrInvalidManifest):
		return AddonInvalid
	}
	return AddonInternal
}
