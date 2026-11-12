package library

import (
	"cmp"
	"context"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Studios' pages and logos.
//
// A title's studios are the networks and companies TMDB's details name
// (see titlecredits.go), or those an administrator set. Every read of a
// title's details also keeps each network and company TMDB names, by its
// TMDB identifier, with its name and logo (tmdb_studios), and their
// identifiers with the title's credits. A studio is a name, whatever its
// letter case: networks and companies sharing a name are one studio. Its
// logo is that of its network of the lowest TMDB identifier that has one,
// else that of such a company; Polyfin draws it on a plate of its own
// (see StudioArtwork), so that apps never reach TMDB.
//
// A studio's page lists (see StudioTitles):
//   - the titles the user's catalogs list for an option of its name, as a
//     genre's page does (see Narrowed);
//   - then the titles whose kept details name it that the user reaches,
//     keptMost at most, by name;
//   - then, while the settings hold the server's TMDB key, the most popular
//     movies and series of its networks and companies on TMDB's discover,
//     studioDiscoverMost of each at most, from studioDiscoverPages pages
//     of each: movies by their companies, series by their networks, else
//     by their companies. Titles for adults, not out yet or of no date are
//     left out. They are items of the user's metadata addons, as the
//     titles a person is known for are (see tmdbTitleRecords): a title no
//     addon describes is left out. The page waits studioWait at most for
//     them and for the IMDb identifiers they need; those that come later
//     are listed the next time.
//
// Each title is listed once. Parental control, blocked genres, the user's
// hidden titles and the movies waiting for their digital release apply as
// on every listing.
//
// What it costs: TMDB's discover is asked studioDiscoverPages requests a
// kind, kept in memory a day (see tmdb.Client), and one request for the
// IMDb identifier of each title the user's addons describe by IMDb
// identifier only, kept in the database (see knownfor.go).
//
// The studios listed (see Studios) are those of the first CreditsPassMost
// titles of each library of movies or series the user reaches, those
// whose details are kept, as their pages show them.

const (
	// studioDiscoverPages is how many pages of TMDB's discover a studio's
	// page reads of each kind, and studioDiscoverMost how many titles of
	// each kind it lists from them at most.
	studioDiscoverPages = 2
	studioDiscoverMost  = studioDiscoverPages * tmdb.DiscoverPage
	// studioWait bounds how long a studio's page waits for TMDB's discover
	// and the IMDb identifiers of its titles.
	studioWait = knownForWait
	// studioLogoSize is the width of TMDB's logos Polyfin draws on its
	// plates: TMDB serves every logo as a PNG at it.
	studioLogoSize = "w500"
	// studioNamesMost bounds the names StudioNames returns.
	studioNamesMost = 50_000
)

// studioIDs returns the TMDB identifiers of networks or companies, each
// once; never nil.
func studioIDs(list []tmdb.Studio) []int {
	ids := []int{}
	for _, s := range list {
		if s.ID > 0 && !slices.Contains(ids, s.ID) {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// keepStudios keeps the networks and companies a title's details name,
// with their names and logos.
func (s *Service) keepStudios(ctx context.Context, details tmdb.TitleDetails, now time.Time) error {
	var kinds, names, logos []string
	var ids []int
	for _, list := range []struct {
		kind    string
		studios []tmdb.Studio
	}{{"network", details.Networks}, {"company", details.Companies}} {
		for _, studio := range list.studios {
			if studio.ID > 0 {
				kinds, ids, names, logos = append(kinds, list.kind), append(ids, studio.ID), append(names, studio.Name), append(logos, studio.LogoPath)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `INSERT INTO tmdb_studios (kind, tmdb_id, name, logo_path, seen_at)
		SELECT kind, tmdb_id, name, logo_path, $5 FROM unnest($1::text[], $2::int[], $3::text[], $4::text[]) AS s (kind, tmdb_id, name, logo_path)
		ON CONFLICT (kind, tmdb_id) DO UPDATE SET name = excluded.name, logo_path = excluded.logo_path, seen_at = excluded.seen_at`,
		kinds, ids, names, logos, now)
	return err
}

// studioRefs are the networks and companies of a studio, by their TMDB
// identifiers, lowest first, and the path of its logo, empty for none.
type studioRefs struct {
	networks, companies []int
	logo                string
}

// studioRefsOf returns the networks and companies named any of names,
// whatever their case.
func (s *Service) studioRefsOf(ctx context.Context, names []string) (studioRefs, error) {
	var refs studioRefs
	rows, err := s.db.Query(ctx, `SELECT kind, tmdb_id, logo_path FROM tmdb_studios
		WHERE lower(name) = ANY(ARRAY(SELECT lower(btrim(n)) FROM unnest($1::text[]) AS n))
		ORDER BY kind = 'network' DESC, tmdb_id`, names)
	if err != nil {
		return refs, err
	}
	var kind, logo string
	var id int
	_, err = pgx.ForEachRow(rows, []any{&kind, &id, &logo}, func() error {
		if kind == "network" {
			refs.networks = append(refs.networks, id)
		} else {
			refs.companies = append(refs.companies, id)
		}
		if refs.logo == "" {
			refs.logo = logo
		}
		return nil
	})
	return refs, err
}

// StudioNames returns the names of the studios TMDB's details named, each
// once whatever its case, the most recently named first, studioNamesMost
// at most.
func (s *Service) StudioNames(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT name FROM (
			SELECT DISTINCT ON (lower(name)) name, seen_at FROM tmdb_studios ORDER BY lower(name), seen_at DESC) AS named
		ORDER BY seen_at DESC LIMIT $1`, studioNamesMost)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// StudioLogo returns the path of the logo of the studio named name on
// TMDB's image server, empty when it has none (see above).
func (s *Service) StudioLogo(ctx context.Context, name string) (string, error) {
	refs, err := s.studioRefsOf(ctx, []string{name})
	return refs.logo, err
}

// studioImage is the image Polyfin draws of a studio's logo (see
// GeneratedImage): wide for its Thumb image, square for its Primary one,
// with a narrow margin when narrow.
func (s *Service) studioImage(logo string, wide, narrow bool) GeneratedImage {
	return GeneratedImage{Wide: wide, Logo: s.tmdb.ImageURL(logo, studioLogoSize), NarrowMargin: narrow}
}

// studioImageWide tells the shape of a studio's image of imageType, false
// for a type it has none of.
func studioImageWide(imageType string) (wide, ok bool) {
	switch strings.ToLower(imageType) {
	case "thumb":
		return true, true
	case "primary":
		return false, true
	}
	return false, false
}

// StudioImageTags returns the tags of the Thumb and Primary images of a
// studio of the logo logo (see StudioLogo), none without one.
func (s *Service) StudioImageTags(logo string) map[string]string {
	tags := map[string]string{}
	if logo == "" || s.tmdb == nil {
		return tags
	}
	for _, imageType := range []string{"Thumb", "Primary"} {
		wide, _ := studioImageWide(imageType)
		tags[imageType] = ImageTag(s.studioImage(logo, wide, false).URL())
	}
	return tags
}

// StudioArtwork returns the image of type imageType of the studio named
// name: its logo drawn on a plate, wide for Thumb, square for Primary,
// with a narrow margin when narrow (see GeneratedImage). ErrNotFound
// reports a studio of no logo, or another type.
func (s *Service) StudioArtwork(ctx context.Context, name, imageType string, narrow bool) (Artwork, error) {
	wide, ok := studioImageWide(imageType)
	if !ok || s.tmdb == nil {
		return Artwork{}, ErrNotFound
	}
	logo, err := s.StudioLogo(ctx, name)
	if err != nil {
		return Artwork{}, err
	}
	if logo == "" {
		return Artwork{}, ErrNotFound
	}
	g := s.studioImage(logo, wide, narrow)
	return g.artwork(), nil
}

// studioURL is the URL standing for the image of a studio's logo, as
// GeneratedImage.URL gives it. A plate of the narrow margin has its own;
// the others' stay as they were, so are their tags.
func studioURL(g GeneratedImage) string {
	shape := "square"
	if g.Wide {
		shape = "wide"
	}
	values := url.Values{"v": {generatedVersion}, "shape": {shape}, "logo": {g.Logo}}
	if g.NarrowMargin {
		values.Set("margin", "narrow")
	}
	return generatedScheme + "logo?" + values.Encode()
}

// studiosShown returns the studios each of items shows on its page: those
// an administrator set, else those of its kept details (see
// titlesFromTMDB), read at once.
func (s *Service) studiosShown(ctx context.Context, items []Item) [][]string {
	shown := make([][]string, len(items))
	set := s.overridesNow()
	keys := make([]creditsKey, len(items))
	valid := make([]bool, len(items))
	var asked []creditsKey
	for i, item := range items {
		if o, edited := set.fields[item.ID]; edited && o.Studios != nil {
			shown[i] = o.Studios
			continue
		}
		if keys[i], valid[i] = titleCreditsKey(item.Kind, item.ProviderIDs); valid[i] {
			asked = append(asked, keys[i])
		}
	}
	kept, err := s.keptTitleCredits(ctx, asked)
	if err != nil {
		return shown
	}
	for i, item := range items {
		if k, ok := kept[keys[i]]; valid[i] && ok && k.describes(item.ID) {
			shown[i] = k.studios(item.Kind)
		}
	}
	return shown
}

// Studio is a studio of the titles a user reaches (see Studios): its name,
// how many of them are movies and series, and the path of its logo on
// TMDB's image server, empty for none.
type Studio struct {
	Name           string
	Movies, Series int
	Logo           string
}

// Studios lists the studios of the first CreditsPassMost titles of each
// library of movies or series the user reaches, of parent only when it is
// not zero, as their pages show them, by name, each once whatever its
// case. A parent that is no such library lists none.
func (s *Service) Studios(ctx context.Context, user accounts.User, parent accounts.ID) ([]Studio, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, err
	}
	titles := WithKinds(ctx, func(kind Kind) bool { return kind == KindMovie || kind == KindSeries })
	var items []Item
	for _, l := range v.libraries {
		if l.item.CollectionType != "movies" && l.item.CollectionType != "tvshows" || parent != (accounts.ID{}) && l.item.ID != parent {
			continue
		}
		for start := 0; start < CreditsPassMost; {
			page, err := s.Children(titles, user, l.item.ID, start, min(movieIDsPage, CreditsPassMost-start), "")
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				s.logger.Debug("A library's titles could not be listed for its studios", "library", l.item.Name, "error", err)
				break
			}
			for _, item := range page.Items {
				if item.Kind == KindMovie || item.Kind == KindSeries {
					items = append(items, item)
				}
			}
			start += len(page.Items)
			if len(page.Items) == 0 || !page.More && start >= page.Total {
				break
			}
		}
	}
	byName := map[string]*Studio{}
	var studios []*Studio
	seen := map[accounts.ID]bool{}
	for i, names := range s.studiosShown(ctx, items) {
		item := items[i]
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		counted := map[string]bool{}
		for _, name := range names {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" || counted[key] {
				continue
			}
			counted[key] = true
			studio := byName[key]
			if studio == nil {
				studio = &Studio{Name: strings.TrimSpace(name)}
				byName[key] = studio
				studios = append(studios, studio)
			}
			if item.Kind == KindMovie {
				studio.Movies++
			} else {
				studio.Series++
			}
		}
	}
	result := make([]Studio, 0, len(studios))
	names := make([]string, 0, len(studios))
	for _, studio := range studios {
		names = append(names, studio.Name)
	}
	logos, err := s.studioLogos(ctx, names)
	if err != nil {
		return nil, err
	}
	for _, studio := range studios {
		studio.Logo = logos[strings.ToLower(studio.Name)]
		result = append(result, *studio)
	}
	slices.SortFunc(result, func(a, b Studio) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Name, b.Name))
	})
	return result, nil
}

// studioLogos returns the paths of the logos of the studios named names,
// by their name in lower case, those of none left out.
func (s *Service) studioLogos(ctx context.Context, names []string) (map[string]string, error) {
	logos := map[string]string{}
	if len(names) == 0 {
		return logos, nil
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (lower(name)) name, logo_path FROM tmdb_studios
		WHERE logo_path <> '' AND lower(name) = ANY(ARRAY(SELECT lower(n) FROM unnest($1::text[]) AS n))
		ORDER BY lower(name), kind = 'network' DESC, tmdb_id`, names)
	if err != nil {
		return nil, err
	}
	var name, logo string
	_, err = pgx.ForEachRow(rows, []any{&name, &logo}, func() error {
		logos[strings.ToLower(name)] = logo
		return nil
	})
	return logos, err
}

// StudioTitles lists the titles of the given kinds of the studios named
// names (see above), [start, start+count): those of the user's catalogs
// for a genre option matches matches, as Narrowed lists them, then the
// titles whose kept details name them, then those of TMDB's discover (see
// namedTitles).
func (s *Service) StudioTitles(ctx context.Context, user accounts.User, names []string, matches func(option string) bool, kinds []Kind,
	start, count int) (Page, error) {
	return s.namedTitles(ctx, user, matches, kinds, start, count, func(v view, listed []Item) []Item {
		return s.studioExtras(ctx, v, names, kinds, listed)
	})
}

// studioExtras returns the titles of the given kinds of the studios named
// names that listed, the catalogs' titles, do not hold: those whose kept
// details name them, then those of TMDB's discover (see above).
func (s *Service) studioExtras(ctx context.Context, v view, names []string, kinds []Kind, listed []Item) []Item {
	names = slices.DeleteFunc(slices.Clone(names), func(name string) bool { return strings.TrimSpace(name) == "" })
	if len(names) == 0 || !slices.Contains(kinds, KindMovie) && !slices.Contains(kinds, KindSeries) {
		return nil
	}
	refs, err := s.studioRefsOf(ctx, names)
	if err != nil {
		s.logger.Debug("A studio's networks and companies could not be read", "error", err)
	}
	shown := newShownTitles(listed)
	kept := s.studioKept(ctx, v, names, refs, kinds)
	// The kept titles' TMDB identifiers, so that discover does not repeat
	// them.
	for t := range s.listedTMDB(ctx, v, kept) {
		shown.tmdb[t] = true
	}
	extras := shown.byName(s.titleItems(ctx, v, kept, kinds))
	discovered := s.studioDiscovered(ctx, v, refs, kinds, shown.tmdb)
	for _, item := range s.titleItems(ctx, v, discovered, kinds) {
		if shown.add(item) {
			extras = append(extras, item)
		}
	}
	s.translate(ctx, v, extras)
	return s.overridden(extras)
}

// studioKept returns the records of the titles of the given kinds whose
// kept details name the studio of refs, or whose studios an administrator
// set to one of names, as keptTitles keeps them. An administrator's
// studios win over TMDB's.
func (s *Service) studioKept(ctx context.Context, v view, names []string, refs studioRefs, kinds []Kind) []record {
	var ids []accounts.ID
	for id, o := range s.overridesNow().fields {
		if o.Studios != nil && namedAny(o.Studios, names) {
			ids = append(ids, id)
		}
	}
	if len(refs.networks) > 0 || len(refs.companies) > 0 {
		rows, err := s.db.Query(ctx, `SELECT titles FROM title_credits
			WHERE tmdb_id IS NOT NULL AND tagline IS NOT NULL AND network_ids IS NOT NULL AND company_ids IS NOT NULL
				AND (kind = 'tv' AND network_ids && $1::int[] OR company_ids && $2::int[])
			ORDER BY asked_at DESC LIMIT $3`, refs.networks, refs.companies, keptMost)
		if err == nil {
			var titles []accounts.ID
			_, err = pgx.ForEachRow(rows, []any{&titles}, func() error {
				ids = append(ids, titles...)
				return nil
			})
		}
		if err != nil {
			s.logger.Debug("The titles of a studio could not be read", "error", err)
		}
	}
	return s.keptTitles(ctx, v, ids, kinds, func(o Overrides) bool { return o.Studios != nil && !namedAny(o.Studios, names) })
}

// studioDiscovered returns the records of the titles of the given kinds
// TMDB's discover lists for the networks and companies of refs that the
// user may open and see (see above), but those of shown, the most popular
// first; none without a TMDB key.
func (s *Service) studioDiscovered(ctx context.Context, v view, refs studioRefs, kinds []Kind, shown map[tmdbTitle]bool) []record {
	if !s.tmdb.Enabled() || len(refs.networks) == 0 && len(refs.companies) == 0 {
		return nil
	}
	deadline := time.Now().Add(studioWait)
	asking, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	today := s.now().UTC().Format(time.DateOnly)
	type asked struct {
		kind tmdb.Kind
		page int
	}
	var pages []asked
	if slices.Contains(kinds, KindMovie) && len(refs.companies) > 0 {
		for page := 1; page <= studioDiscoverPages; page++ {
			pages = append(pages, asked{tmdb.Movie, page})
		}
	}
	if slices.Contains(kinds, KindSeries) {
		for page := 1; page <= studioDiscoverPages; page++ {
			pages = append(pages, asked{tmdb.TV, page})
		}
	}
	found := make([][]tmdb.Discovered, len(pages))
	var wg sync.WaitGroup
	for i, p := range pages {
		wg.Go(func() {
			var err error
			if found[i], err = s.tmdb.Discover(asking, p.kind, refs.networks, refs.companies, p.page, today); err != nil && ctx.Err() == nil {
				s.logger.Debug("TMDB's discover could not be read", "kind", p.kind, "page", p.page, "error", err)
			}
		})
	}
	wg.Wait()
	var credits []personCredit
	count := map[tmdb.Kind]int{}
	for i, list := range found {
		for _, d := range list {
			c := personCredit{Kind: string(pages[i].kind), ID: d.ID, Name: d.Name, Date: d.Date, Poster: d.PosterPath, Votes: d.Votes,
				Popularity: d.Popularity, Aside: d.Adult}
			if c.Aside || c.Date == "" || c.Date > today || count[pages[i].kind] >= studioDiscoverMost ||
				slices.ContainsFunc(credits, func(other personCredit) bool { return other.title() == c.title() }) {
				continue
			}
			count[pages[i].kind]++
			if !shown[c.title()] {
				credits = append(credits, c)
			}
		}
	}
	slices.SortStableFunc(credits, func(a, b personCredit) int { return cmp.Compare(b.Popularity, a.Popularity) })
	var records []record
	for _, r := range s.tmdbTitleRecords(ctx, v, credits, deadline) {
		if r != nil && !v.hidden[r.ID] && !slices.ContainsFunc(records, func(p record) bool { return p.ID == r.ID }) {
			records = append(records, *r)
		}
	}
	records, _ = s.visible(ctx, v, records)
	records, _ = s.released(ctx, v, records)
	if err := s.saveNew(ctx, records); err != nil {
		s.logger.Debug("The titles of a studio could not be recorded", "error", err)
	}
	return records
}
