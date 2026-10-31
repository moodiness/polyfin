package library

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Episode orders.
//
// A series' seasons and episodes come as its addon numbers them, as aired.
// As in Jellyfin, an administrator may show a series in another order, for
// every user: its DisplayOrder, which jellyfin-web's metadata editor sets
// by type ("dvd", "absolute", "storyArc"...). Jellyfin's TMDB provider
// reads the first of the series' TMDB episode groups of that type
// (TmdbClientManager.GetSeriesGroupAsync); Polyfin does the same with the
// server's TMDB key, and the admin app may choose a group by name, or put
// the specials last or hide them (see Overrides).
//
// The order is presentation only. The group's seasons replace the
// series' in its listings, under identifiers of their own, and each
// episode shows the group's season and number (Item.Shown); its
// identifier, its watch state, its tracking and the streams it plays keep
// the addon's season and number. An episode is matched with the group by
// TMDB's season and number: those its identifier names when it is TMDB's
// ("tmdb:1399:1:2"), else the addon's. As Jellyfin reads a group, its
// group at order N is season N, and its episode at order N is episode N+1.
// The group's season 0 holds the specials; the addon's specials it leaves
// out follow there, and its other episodes it leaves out show in a last
// season of their own, so that nothing is lost. While TMDB cannot tell
// the group, the series shows as aired.

// DisplayOrders maps the DisplayOrder values jellyfin-web offers that name
// a TMDB episode group type to it, as Jellyfin's TMDB provider reads them.
// The others ("alternate", "regional", "altdvd") are TVDB's: a series set
// to one shows as aired, as Jellyfin's TMDB provider reads it.
var displayOrders = map[string]int{
	"originalAirDate": tmdb.GroupOriginalAirDate,
	"absolute":        tmdb.GroupAbsolute,
	"dvd":             tmdb.GroupDVD,
	"digital":         tmdb.GroupDigital,
	"storyArc":        tmdb.GroupStoryArc,
	"production":      tmdb.GroupProduction,
	"tv":              tmdb.GroupTV,
}

// DisplayOrderOf returns the DisplayOrder value naming a TMDB episode
// group type, empty for an unknown type.
func DisplayOrderOf(groupType int) string {
	for order, t := range displayOrders {
		if t == groupType {
			return order
		}
	}
	return ""
}

// Where a series' specials show: first, as Jellyfin lists season 0, last,
// or not at all.
const (
	SpecialsFirst  = "first"
	SpecialsLast   = "last"
	SpecialsHidden = "hidden"
)

// orderWait bounds how long a listing waits for TMDB to tell a series'
// episode group: past it, the series shows as aired, and the group comes
// with TMDB's answer, which the TMDB client keeps.
const orderWait = 5 * time.Second

var (
	// ErrUnknownGroup reports an episode group TMDB does not give the
	// series.
	ErrUnknownGroup = errors.New("not an episode group of the series")
	// ErrInvalidSpecials reports a placement of the specials that is not
	// one of SpecialsFirst, SpecialsLast and SpecialsHidden.
	ErrInvalidSpecials = errors.New("invalid placement of the specials")
	// ErrGroupLookup reports that TMDB could not be asked for a series'
	// episode groups.
	ErrGroupLookup = errors.New("TMDB could not tell the series' episode groups")
)

// Place is where an episode shows in its series' order: its season, by
// item, name and number, and its number.
type Place struct {
	SeasonID   accounts.ID
	SeasonName string
	Season     int
	Episode    int
}

// ShownPlace returns where an episode shows: in its series' order, else
// as aired.
func (i Item) ShownPlace() Place {
	if i.Shown != nil {
		return *i.Shown
	}
	return Place{SeasonID: i.SeasonID, SeasonName: i.SeasonName, Season: i.ParentIndexNumber, Episode: i.IndexNumber}
}

// airedKey marks a context whose listings of episodes keep the addon's
// order (see InAiredOrder).
type airedKey struct{}

// InAiredOrder marks a context whose listings of a series' episodes list
// them as aired, whatever order the series shows in: those that number
// them as the addon does, as tracking does.
func InAiredOrder(ctx context.Context) context.Context {
	return context.WithValue(ctx, airedKey{}, true)
}

func inAiredOrder(ctx context.Context) bool {
	marked, _ := ctx.Value(airedKey{}).(bool)
	return marked
}

// groupSeasonKey is the key of season of an episode group's order.
func groupSeasonKey(seriesID, group string, season int) string {
	return "groupseason|" + seriesID + "|" + group + "|" + strconv.Itoa(season)
}

// shownSeason is a season as a series' order shows it: its identifier,
// number and name, and its episodes, by video, in order.
type shownSeason struct {
	id     accounts.ID
	number int
	name   string
	videos []string
}

// episodeLayout is how a series' order shows its seasons and episodes.
type episodeLayout struct {
	// group is the TMDB episode group, empty for the aired order.
	group string
	// seasons are listed in this order, the specials placed; hidden is
	// the season of the specials when they are hidden.
	seasons []shownSeason
	hidden  *shownSeason
	// places are where the episodes show, by video; nil as aired.
	places map[string]Place
}

// tmdbNumbers returns the season and number TMDB gives an episode: those
// its identifier names when it is TMDB's, else the addon's.
func tmdbNumbers(video stremio.Video) [2]int {
	if rest, ok := strings.CutPrefix(video.ID, "tmdb:"); ok {
		if parts := strings.Split(rest, ":"); len(parts) == 3 {
			season, seasonErr := strconv.Atoi(parts[1])
			episode, episodeErr := strconv.Atoi(parts[2])
			if seasonErr == nil && episodeErr == nil {
				return [2]int{season, episode}
			}
		}
	}
	return [2]int{int(video.Season), video.EpisodeNumber()}
}

// newLayout lays a series' videos (meta's) out: in group's order, of the
// series TMDB identifies as show, or as aired when group is nil; with the
// specials placed as specials tells.
func newLayout(meta stremio.Meta, group *tmdb.EpisodeGroup, show int, specials string, w words) *episodeLayout {
	aired := slices.Clone(meta.Videos)
	slices.SortStableFunc(aired, func(a, b stremio.Video) int {
		if a.Season != b.Season {
			return int(a.Season) - int(b.Season)
		}
		return a.EpisodeNumber() - b.EpisodeNumber()
	})
	l := &episodeLayout{}
	var seasons []*shownSeason
	at := func(number int) *shownSeason {
		if i := slices.IndexFunc(seasons, func(s *shownSeason) bool { return s.number == number }); i >= 0 {
			return seasons[i]
		}
		season := &shownSeason{number: number, name: w.seasonName(number)}
		seasons = append(seasons, season)
		return season
	}
	if group == nil {
		for _, video := range aired {
			season := at(int(video.Season))
			season.id = itemID(seasonKey(meta.ID, season.number))
			season.videos = append(season.videos, video.ID)
		}
	} else {
		l.group, l.places = group.ID, map[string]Place{}
		byNumber := map[[2]int]string{}
		for _, video := range aired {
			if numbers := tmdbNumbers(video); byNumber[numbers] == "" {
				byNumber[numbers] = video.ID
			}
		}
		// next is the number after the last placed in each season.
		next := map[*shownSeason]int{}
		place := func(season *shownSeason, video string, number int) {
			season.videos = append(season.videos, video)
			l.places[video] = Place{Season: season.number, Episode: number}
			next[season] = max(next[season], number+1)
		}
		groups := slices.Clone(group.Groups)
		slices.SortStableFunc(groups, func(a, b tmdb.EpisodeSeason) int { return a.Order - b.Order })
		last := 0
		for _, g := range groups {
			season := at(g.Order)
			if name := strings.TrimSpace(g.Name); name != "" && len(season.videos) == 0 {
				season.name = name
			}
			last = max(last, g.Order)
			episodes := slices.Clone(g.Episodes)
			slices.SortStableFunc(episodes, func(a, b tmdb.GroupEpisode) int { return a.Order - b.Order })
			for _, e := range episodes {
				if e.Show != 0 && show != 0 && e.Show != show {
					continue
				}
				if video := byNumber[[2]int{e.Season, e.Episode}]; video != "" {
					if _, placed := l.places[video]; !placed {
						place(season, video, e.Order+1)
					}
				}
			}
		}
		// The episodes the group leaves out: its specials, then a season
		// of the others, numbered on from the last of each.
		var others *shownSeason
		for _, video := range aired {
			if _, placed := l.places[video.ID]; placed {
				continue
			}
			season := others
			switch {
			case video.Season == 0:
				season = at(0)
			case others == nil:
				others = at(last + 1)
				others.name = w.otherEpisodes
				season = others
			}
			place(season, video.ID, max(next[season], 1))
		}
		seasons = slices.DeleteFunc(seasons, func(s *shownSeason) bool { return len(s.videos) == 0 })
		for _, s := range seasons {
			s.id = itemID(groupSeasonKey(meta.ID, group.ID, s.number))
			slices.SortStableFunc(s.videos, func(a, b string) int { return l.places[a].Episode - l.places[b].Episode })
			for _, video := range s.videos {
				p := l.places[video]
				p.SeasonID, p.SeasonName = s.id, s.name
				l.places[video] = p
			}
		}
	}
	slices.SortStableFunc(seasons, func(a, b *shownSeason) int { return a.number - b.number })
	for _, season := range seasons {
		switch {
		case season.number != 0 || specials != SpecialsLast && specials != SpecialsHidden:
			l.seasons = append(l.seasons, *season)
		case specials == SpecialsHidden:
			l.hidden = season
		}
	}
	if i := slices.IndexFunc(seasons, func(s *shownSeason) bool { return s.number == 0 }); i >= 0 && specials == SpecialsLast {
		l.seasons = append(l.seasons, *seasons[i])
	}
	return l
}

// season returns the season of the order with identifier id, the hidden
// specials' included.
func (l *episodeLayout) season(id accounts.ID) (shownSeason, bool) {
	if l.hidden != nil && l.hidden.id == id {
		return *l.hidden, true
	}
	i := slices.IndexFunc(l.seasons, func(s shownSeason) bool { return s.id == id })
	if i < 0 {
		return shownSeason{}, false
	}
	return l.seasons[i], true
}

// place sets where an episode shows in the order.
func (l *episodeLayout) place(episode *Item) {
	if p, ok := l.places[episode.StremioID]; ok {
		episode.Shown = &p
	}
}

// arrange returns a series' episodes, listed as aired, as the order lists
// them: in its order, where they show, without the hidden specials. When
// season is set, the episodes are those of that season, one of the
// order's, the hidden specials included, or one of the series' own as
// aired; found is false when there is no such season.
func (l *episodeLayout) arrange(episodes []Item, season *accounts.ID) (result []Item, found bool) {
	rank := map[string]int{}
	for _, s := range l.seasons {
		for _, video := range s.videos {
			rank[video] = len(rank)
		}
	}
	var own *shownSeason
	if season != nil {
		if s, ok := l.season(*season); ok {
			own = &s
			for i, video := range s.videos {
				rank[video] = i
			}
		}
	}
	found = season == nil || own != nil
	for _, episode := range episodes {
		_, listed := rank[episode.StremioID]
		switch {
		case own != nil && !slices.Contains(own.videos, episode.StremioID):
			continue
		case own == nil && season != nil && episode.SeasonID != *season:
			continue
		case own == nil && season != nil:
			found = true
		}
		if !listed && l.hidden != nil && slices.Contains(l.hidden.videos, episode.StremioID) {
			continue
		}
		l.place(&episode)
		result = append(result, episode)
	}
	// Episodes the order does not know, which a description read since
	// may add, come last.
	slices.SortStableFunc(result, func(a, b Item) int {
		ra, okA := rank[a.StremioID]
		rb, okB := rank[b.StremioID]
		switch {
		case okA && okB:
			return ra - rb
		case okA:
			return -1
		case okB:
			return 1
		}
		return 0
	})
	return result, found
}

// seasonItem describes a season of an episode group's order, of series,
// described by meta, as the user sees them; videos are meta's, by
// identifier.
func (l *episodeLayout) seasonItem(season shownSeason, series Item, meta stremio.Meta, videos map[string]stremio.Video, now time.Time) Item {
	item := Item{
		ID:           season.id,
		Kind:         KindSeason,
		Name:         season.name,
		ParentID:     series.ID,
		SeriesID:     series.ID,
		SeriesName:   series.Name,
		SeriesPoster: series.Images.Primary,
		IndexNumber:  season.number,
		Available:    true,
		Images:       Images{Primary: series.Images.Primary, Backdrop: series.Images.Backdrop},
		Contents:     &Contents{Children: len(season.videos)},
		StremioType:  meta.Type,
		StremioID:    meta.ID,
		EpisodeGroup: l.group,
	}
	for _, id := range season.videos {
		video, ok := videos[id]
		if !ok {
			continue
		}
		item.Contents.Videos = append(item.Contents.Videos, itemID(episodeKey(video.ID)))
		date := parseDate(video.Released)
		if date != nil && (item.PremiereDate == nil || date.Before(*item.PremiereDate)) {
			item.PremiereDate, item.ProductionYear = date, date.Year()
		}
		if !released(video, now) {
			continue
		}
		item.Contents.Released++
		item.Contents.Runtime += parseRuntime(string(video.Runtime))
		if date != nil && (item.Contents.LastReleased == nil || date.After(*item.Contents.LastReleased)) {
			item.Contents.LastReleased = date
		}
	}
	return item
}

// seasonItems returns a series' seasons as the order lists them, aired
// being those the addon gives.
func (l *episodeLayout) seasonItems(aired []Item, series Item, meta stremio.Meta, now time.Time) []Item {
	result := make([]Item, 0, len(l.seasons))
	videos := videosByID(meta)
	for _, season := range l.seasons {
		if l.group != "" {
			result = append(result, l.seasonItem(season, series, meta, videos, now))
		} else if i := slices.IndexFunc(aired, func(item Item) bool { return item.ID == season.id }); i >= 0 {
			result = append(result, aired[i])
		}
	}
	return result
}

// videosByID maps the videos of a description to their identifiers.
func videosByID(meta stremio.Meta) map[string]stremio.Video {
	videos := make(map[string]stremio.Video, len(meta.Videos))
	for _, video := range meta.Videos {
		videos[video.ID] = video
	}
	return videos
}

// records returns the records of the seasons of an episode group's
// order, of the series seriesID, whose description is meta, so that their
// identifiers open them.
func (l *episodeLayout) records(seriesID accounts.ID, meta string) []record {
	if l.group == "" {
		return nil
	}
	seasons := l.seasons
	if l.hidden != nil {
		seasons = append(slices.Clip(seasons), *l.hidden)
	}
	records := make([]record, 0, len(seasons))
	for _, season := range seasons {
		records = append(records, record{ID: season.id, Key: groupSeasonKey(meta, l.group, season.number), Kind: KindSeason,
			Parent: &seriesID, SeriesID: meta, Season: season.number, Group: l.group})
	}
	return records
}

// layoutOf returns how the series seriesID, described by meta, shows its
// seasons and episodes to the user of w, nil when it shows them as aired
// with the specials first. A series whose episode group TMDB cannot tell
// shows as aired.
func (s *Service) layoutOf(ctx context.Context, seriesID accounts.ID, meta stremio.Meta, w words) *episodeLayout {
	o := s.Overrides(seriesID)
	specials := valueOf(o.Specials)
	var group *tmdb.EpisodeGroup
	show := 0
	if groupType, ok := displayOrders[valueOf(o.DisplayOrder)]; ok {
		group, show = s.episodeGroup(ctx, meta, groupType, valueOf(o.EpisodeGroup))
	}
	if group == nil && specials != SpecialsLast && specials != SpecialsHidden {
		return nil
	}
	return newLayout(meta, group, show, specials, w)
}

func valueOf(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

// episodeGroup returns the TMDB episode group a series described by meta
// shows in, and its TMDB identifier: the group chosen, else the first of
// groupType, as Jellyfin takes it. It is nil when there is no TMDB key, no
// such group, or TMDB did not tell within orderWait.
func (s *Service) episodeGroup(ctx context.Context, meta stremio.Meta, groupType int, chosen string) (*tmdb.EpisodeGroup, int) {
	if !s.tmdb.Enabled() {
		return nil, 0
	}
	type found struct {
		group tmdb.EpisodeGroup
		show  int
		err   error
	}
	done := make(chan found, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		var f found
		show, groups, err := s.seriesGroups(ctx, meta)
		if err == nil {
			f.show = show
			i := slices.IndexFunc(groups, func(g tmdb.GroupSummary) bool { return g.ID == chosen })
			if i < 0 {
				i = slices.IndexFunc(groups, func(g tmdb.GroupSummary) bool { return g.Type == groupType })
			}
			if i < 0 {
				err = ErrUnknownGroup
			} else {
				f.group, err = s.tmdb.EpisodeGroupOf(ctx, groups[i].ID)
			}
		}
		f.err = err
		done <- f
	}()
	wait := time.NewTimer(orderWait)
	defer wait.Stop()
	select {
	case f := <-done:
		switch {
		case f.err == nil:
			return &f.group, f.show
		case !errors.Is(f.err, tmdb.ErrNotFound) && !errors.Is(f.err, ErrUnknownGroup) && !errors.Is(f.err, tmdb.ErrNoKey):
			s.logger.Warn("A series' episode order could not be read from TMDB: it shows as aired", "series", meta.ID, "error", f.err)
		}
	case <-wait.C:
	case <-ctx.Done():
	}
	return nil, 0
}

// seriesGroups returns the TMDB identifier of a series described by meta,
// by the one it gives, else by its IMDb one, and its episode groups.
func (s *Service) seriesGroups(ctx context.Context, meta stremio.Meta) (int, []tmdb.GroupSummary, error) {
	ids := providerIDs(meta)
	id := ids["Tmdb"]
	if id == "" {
		if ids["Imdb"] == "" {
			return 0, nil, tmdb.ErrNotFound
		}
		var err error
		if id, err = s.tmdb.FindByIMDb(ctx, tmdb.TV, ids["Imdb"]); err != nil {
			return 0, nil, err
		}
	}
	show, err := strconv.Atoi(id)
	if err != nil {
		return 0, nil, tmdb.ErrNotFound
	}
	groups, err := s.tmdb.EpisodeGroups(ctx, id)
	return show, groups, err
}

// EpisodeOrder is a series' order of episodes as the admin app shows and
// chooses it.
type EpisodeOrder struct {
	// DisplayOrder is the order as Jellyfin names it, empty as aired; Group
	// the TMDB episode group an administrator chose, empty for the first
	// of its type; Specials where the specials show.
	DisplayOrder string
	Group        string
	Specials     string
	// Groups are the series' TMDB episode groups, and Lookup why there are
	// none when TMDB could not tell: "no_key" without a TMDB key, "unknown"
	// for a series TMDB does not know, "failed" when it could not be
	// asked.
	Groups []tmdb.GroupSummary
	Lookup string
}

// orderOf returns the order the overrides o set.
func orderOf(o Overrides) EpisodeOrder {
	return EpisodeOrder{DisplayOrder: valueOf(o.DisplayOrder), Group: valueOf(o.EpisodeGroup),
		Specials: cmp.Or(valueOf(o.Specials), SpecialsFirst)}
}

// orderedSeries returns the series the user sees as id, and its
// description; ErrNotFound for another item.
func (s *Service) orderedSeries(ctx context.Context, user accounts.User, id accounts.ID) (Item, stremio.Meta, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Item{}, stremio.Meta{}, err
	}
	series, _, meta, err := s.series(ctx, v, id)
	if err != nil {
		return Item{}, stremio.Meta{}, err
	}
	return s.overriddenItem(series), meta, nil
}

// EpisodeOrderOf returns the series the user sees as id, and its order of
// episodes with the episode groups TMDB gives it.
func (s *Service) EpisodeOrderOf(ctx context.Context, user accounts.User, id accounts.ID) (Item, EpisodeOrder, error) {
	series, meta, err := s.orderedSeries(ctx, user, id)
	if err != nil {
		return Item{}, EpisodeOrder{}, err
	}
	order := orderOf(s.Overrides(id))
	if !s.tmdb.Enabled() {
		order.Lookup = "no_key"
		return series, order, nil
	}
	_, order.Groups, err = s.seriesGroups(ctx, meta)
	switch {
	case errors.Is(err, tmdb.ErrNotFound):
		order.Lookup = "unknown"
	case err != nil:
		s.logger.Warn("The episode groups of a series could not be read from TMDB", "series", meta.ID, "error", err)
		order.Lookup = "failed"
	}
	return series, order, nil
}

// SetEpisodeOrder shows the series the user sees as id in the TMDB
// episode group group, as aired when group is empty, with its specials
// where specials tells (SpecialsFirst when empty). Its DisplayOrder
// becomes the group's type, which Jellyfin's metadata editor shows.
func (s *Service) SetEpisodeOrder(ctx context.Context, user accounts.User, id accounts.ID, group, specials string) error {
	if !slices.Contains([]string{"", SpecialsFirst, SpecialsLast, SpecialsHidden}, specials) {
		return ErrInvalidSpecials
	}
	_, meta, err := s.orderedSeries(ctx, user, id)
	if err != nil {
		return err
	}
	order := ""
	if group != "" {
		if !s.tmdb.Enabled() {
			return tmdb.ErrNoKey
		}
		_, groups, err := s.seriesGroups(ctx, meta)
		if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
			return fmt.Errorf("%w: %w", ErrGroupLookup, err)
		}
		i := slices.IndexFunc(groups, func(g tmdb.GroupSummary) bool { return g.ID == group })
		if i >= 0 {
			order = DisplayOrderOf(groups[i].Type)
		}
		if order == "" {
			return ErrUnknownGroup
		}
	}
	optional := func(value string) *string {
		if value == "" || value == SpecialsFirst {
			return nil
		}
		return &value
	}
	return s.changeOwnOverrides(ctx, id, func(o Overrides) Overrides {
		o.DisplayOrder, o.EpisodeGroup, o.Specials = optional(order), optional(group), optional(specials)
		return o
	})
}

// OrderedSeries is a series shown in an order of its own, as the admin
// app lists them.
type OrderedSeries struct {
	Series Item
	Order  EpisodeOrder
}

// EpisodeOrders lists the series the user sees that show in an order of
// their own, or with their specials elsewhere than first, by name.
func (s *Service) EpisodeOrders(ctx context.Context, user accounts.User) ([]OrderedSeries, error) {
	set := s.overridesNow()
	var ids []accounts.ID
	for id, o := range set.fields {
		if o.DisplayOrder != nil || o.Specials != nil {
			ids = append(ids, id)
		}
	}
	items, err := s.ItemsOf(ctx, user, ids, []Kind{KindSeries})
	if err != nil {
		return nil, err
	}
	result := make([]OrderedSeries, 0, len(items))
	for _, item := range items {
		result = append(result, OrderedSeries{Series: item, Order: orderOf(set.fields[item.ID])})
	}
	slices.SortStableFunc(result, func(a, b OrderedSeries) int {
		return strings.Compare(strings.ToLower(a.Series.Name), strings.ToLower(b.Series.Name))
	})
	return result, nil
}
