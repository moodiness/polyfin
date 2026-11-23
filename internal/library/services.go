package library

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Streaming services from TMDB.
//
// TMDB's details of a title, read with its credits (see titlecredits.go),
// also tell which streaming services offer it, country by country. Polyfin
// keeps those of the server's rating country, the US when it has none
// (see view.country), that offer it by subscription, free, or free with
// ads: a service that only rents or sells it is left out, since a user
// subscribed to it would still pay for the title. They are kept with the
// title's credits, and their TMDB identifiers and logos apart, by
// identifier (tmdb_services). Another rating country makes them due: until
// they are read again for it, a title shows none.
//
// Jellyfin has no field for them. A title carries them as its tags, which
// Jellyfin apps show and filter by, unless an administrator edited its
// tags, which win (see Overrides). jellyfin-web links each tag of a title
// to a listing of the titles of that tag, which lists, as a studio's page
// does (see ServiceTitles):
//   - the titles of the user's catalogs for an option of its name, as a
//     genre's page does (see Narrowed): some addons offer catalogs of a
//     service's titles through their genre filter;
//   - then the titles whose kept details name it, or whose tags an
//     administrator set to it, that the user reaches, keptMost at most;
//   - then, while the settings hold the server's TMDB key, the most popular
//     movies and series TMDB's discover lists for it in the country, as a
//     studio's (see discoveredTitles).
//
// What it costs: nothing of its own to read them, which come in the same
// request as the credits. A title's tags facet reads the kept credits,
// from memory as far as they are remembered (see titleCreditsMemory). A
// service's page asks TMDB's discover studioDiscoverPages requests a kind.

// serviceCountry is the country whose streaming services are kept: the
// rating country, in upper case, the US when there is none.
func serviceCountry(ratingCountry string) string {
	return cmp.Or(strings.ToUpper(strings.TrimSpace(ratingCountry)), "US")
}

// serviceNames returns the names of services, in order; never nil.
func serviceNames(services []tmdb.StreamingService) []string {
	names := []string{}
	for _, s := range services {
		if !slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(name, s.Name) }) {
			names = append(names, s.Name)
		}
	}
	return names
}

// serviceIDsOf returns the TMDB identifiers of services, in order; never
// nil.
func serviceIDsOf(services []tmdb.StreamingService) []int {
	ids := []int{}
	for _, s := range services {
		if !slices.Contains(ids, s.ID) {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// servicesIn returns the streaming services of a title's kept details in
// country, none when they were read for another.
func (k keptCredits) servicesIn(country string) []string {
	if k.serviceCountry != country {
		return nil
	}
	return k.services
}

// keepServices keeps the services a title's details name, with their
// names and logos.
func (s *Service) keepServices(ctx context.Context, services []tmdb.StreamingService, now time.Time) error {
	if len(services) == 0 {
		return nil
	}
	ids, names, logos := make([]int, len(services)), make([]string, len(services)), make([]string, len(services))
	for i, service := range services {
		ids[i], names[i], logos[i] = service.ID, service.Name, service.LogoPath
	}
	_, err := s.db.Exec(ctx, `INSERT INTO tmdb_services (tmdb_id, name, logo_path, seen_at)
		SELECT tmdb_id, name, logo_path, $4 FROM unnest($1::int[], $2::text[], $3::text[]) AS s (tmdb_id, name, logo_path)
		ON CONFLICT (tmdb_id) DO UPDATE SET name = excluded.name, logo_path = excluded.logo_path, seen_at = excluded.seen_at`,
		ids, names, logos, now)
	return err
}

// serviceIDsNamed returns the TMDB identifiers of the services named any
// of names, whatever their case, lowest first.
func (s *Service) serviceIDsNamed(ctx context.Context, names []string) ([]int, error) {
	rows, err := s.db.Query(ctx, `SELECT tmdb_id FROM tmdb_services
		WHERE lower(name) = ANY(ARRAY(SELECT lower(btrim(n)) FROM unnest($1::text[]) AS n)) ORDER BY tmdb_id`, names)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int])
}

// TagsShown returns the tags each of items shows: those an administrator
// set, else the streaming services of its kept details in the server's
// rating country (see titlesFromTMDB), read at once.
func (s *Service) TagsShown(ctx context.Context, items []Item) [][]string {
	shown := make([][]string, len(items))
	set := s.overridesNow()
	keys := make([]creditsKey, len(items))
	valid := make([]bool, len(items))
	var asked []creditsKey
	for i, item := range items {
		if o, edited := set.fields[item.ID]; edited && o.Tags != nil {
			shown[i] = o.Tags
			continue
		}
		if len(item.Tags) > 0 {
			shown[i] = item.Tags
			continue
		}
		if keys[i], valid[i] = titleCreditsKey(item.Kind, item.ProviderIDs); valid[i] && s.lookups.Enabled() {
			asked = append(asked, keys[i])
		}
	}
	if len(asked) == 0 {
		return shown
	}
	kept, err := s.keptTitleCredits(ctx, asked)
	if err != nil {
		return shown
	}
	country := s.creditsAsk().country
	for i, item := range items {
		if k, ok := kept[keys[i]]; valid[i] && ok && k.describes(item.ID) {
			shown[i] = k.servicesIn(country)
		}
	}
	return shown
}

// keptServices returns the streaming services of items in the server's
// rating country, from TMDB only, for smart lists' service rules, by
// their place in items: known is set for those whose services are kept
// for the country, for titles TMDB does not know, which have none, and
// for those that cannot be known, without TMDB key or identifier, which
// have none too. The others' credits are queued in the background, with
// the passes' (see creditsLookup), at most smartLookups of them, so that
// the list grows as they come.
func (s *Service) keptServices(ctx context.Context, items []Item) (services [][]string, known []bool, err error) {
	services, known = make([][]string, len(items)), make([]bool, len(items))
	enabled := s.lookups.Enabled()
	keys := make([]creditsKey, len(items))
	valid := make([]bool, len(items))
	var asked []creditsKey
	for i, item := range items {
		if keys[i], valid[i] = titleCreditsKey(item.Kind, item.ProviderIDs); !valid[i] || !enabled {
			known[i] = true
			continue
		}
		asked = append(asked, keys[i])
	}
	if len(asked) == 0 {
		return services, known, nil
	}
	kept, err := s.keptTitleCredits(ctx, asked)
	if err != nil {
		return nil, nil, err
	}
	country, now := s.creditsAsk().country, s.now()
	var due []tmdb.Lookup
	for i, item := range items {
		if !valid[i] || known[i] {
			continue
		}
		k, ok := kept[keys[i]]
		switch {
		case ok && !k.found:
			known[i] = true
		case ok && k.serviceCountry == country:
			services[i], known[i] = k.services, true
		}
		if (!ok || now.Sub(k.asked) >= titleCreditsFresh || k.found && k.serviceCountry != country) && !keptOnly(ctx) &&
			len(due) < smartLookups {
			due = append(due, s.creditsLookup(keys[i], item.ID))
		}
	}
	s.lookups.Add(due, false)
	return services, known, nil
}

// ServiceTitles lists the titles of the given kinds of the streaming
// services named names (see above), [start, start+count): those of the
// user's catalogs for a genre option matches matches, as Narrowed lists
// them, then the titles whose kept details or edited tags name them, then
// those of TMDB's discover in the server's rating country.
func (s *Service) ServiceTitles(ctx context.Context, user accounts.User, names []string, matches func(option string) bool, kinds []Kind,
	start, count int) (Page, error) {
	return s.namedTitles(ctx, user, matches, kinds, start, count, func(v view, listed []Item) []Item {
		return s.serviceExtras(ctx, v, names, kinds, listed)
	})
}

// serviceExtras returns the titles of the given kinds of the services
// named names that listed, the catalogs' titles, do not hold: those whose
// kept details or edited tags name them, then those of TMDB's discover.
func (s *Service) serviceExtras(ctx context.Context, v view, names []string, kinds []Kind, listed []Item) []Item {
	names = slices.DeleteFunc(slices.Clone(names), func(name string) bool { return strings.TrimSpace(name) == "" })
	if len(names) == 0 || !slices.Contains(kinds, KindMovie) && !slices.Contains(kinds, KindSeries) {
		return nil
	}
	ids, err := s.serviceIDsNamed(ctx, names)
	if err != nil {
		s.logger.Debug("A streaming service's identifiers could not be read", "error", err)
	}
	country := serviceCountry(v.country)
	shown := newShownTitles(listed)
	kept := s.serviceKept(ctx, v, names, ids, country, kinds)
	for t := range s.listedTMDB(ctx, v, kept) {
		shown.tmdb[t] = true
	}
	extras := shown.byName(s.titleItems(ctx, v, kept, kinds))
	discovered := s.serviceDiscovered(ctx, v, ids, country, kinds, shown.tmdb)
	for _, item := range s.titleItems(ctx, v, discovered, kinds) {
		if shown.add(item) {
			extras = append(extras, item)
		}
	}
	s.translate(ctx, v, extras)
	return s.overridden(extras)
}

// serviceKept returns the records of the titles of the given kinds whose
// kept details name one of the services ids in country, or whose tags an
// administrator set to one of names, as keptTitles keeps them. An
// administrator's tags win over TMDB's services.
func (s *Service) serviceKept(ctx context.Context, v view, names []string, ids []int, country string, kinds []Kind) []record {
	var titles []accounts.ID
	for id, o := range s.overridesNow().fields {
		if o.Tags != nil && namedAny(o.Tags, names) {
			titles = append(titles, id)
		}
	}
	if len(ids) > 0 {
		rows, err := s.db.Query(ctx, `SELECT titles FROM title_credits
			WHERE tmdb_id IS NOT NULL AND service_country = $1 AND service_ids && $2::int[]
			ORDER BY asked_at DESC LIMIT $3`, country, ids, keptMost)
		if err == nil {
			var listed []accounts.ID
			_, err = pgx.ForEachRow(rows, []any{&listed}, func() error {
				titles = append(titles, listed...)
				return nil
			})
		}
		if err != nil {
			s.logger.Debug("The titles of a streaming service could not be read", "error", err)
		}
	}
	return s.keptTitles(ctx, v, titles, kinds, func(o Overrides) bool { return o.Tags != nil && !namedAny(o.Tags, names) })
}

// serviceDiscovered returns the records of the titles of the given kinds
// TMDB's discover lists for the services ids in country that the user may
// open and see, but those of shown (see discoveredTitles); none without a
// TMDB key.
func (s *Service) serviceDiscovered(ctx context.Context, v view, ids []int, country string, kinds []Kind,
	shown map[tmdbTitle]bool) []record {
	if !s.tmdb.Enabled() || len(ids) == 0 {
		return nil
	}
	var asked []tmdb.Kind
	if slices.Contains(kinds, KindMovie) {
		asked = append(asked, tmdb.Movie)
	}
	if slices.Contains(kinds, KindSeries) {
		asked = append(asked, tmdb.TV)
	}
	discover := func(ctx context.Context, kind tmdb.Kind, page int, today string) ([]tmdb.Discovered, error) {
		return s.tmdb.DiscoverServices(ctx, kind, ids, country, page, today)
	}
	return s.discoveredTitles(ctx, v, asked, shown, discover)
}
