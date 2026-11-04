package library

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// PersonID identifies someone credited in titles by their name, the way
// credits without a TMDB identifier do.
func PersonID(name string) accounts.ID { return itemID(personKey(name)) }

// creditedID identifies someone credited in titles: by their TMDB
// identifier when the credit gives it, so that two people of the same name
// are two people, else by their name. Credits are kept so only while the
// server's TMDB key is saved (see creditedAs).
func creditedID(name, tmdb string) accounts.ID {
	if tmdb != "" {
		return itemID(personTMDBKey(tmdb))
	}
	return PersonID(name)
}

// mergedPerson is the record a person's upsert stores: the new one, with the
// credits of both, the stored photo when the new one has none, the TMDB
// identifier given before when the new one gives none, what TMDB told of
// the person (see persondetails.go), which credits never carry, and the
// photos found dead (see PhotoFailed) the person still shows: a new photo
// from the addon is not.
const mergedPerson = `excluded.data
	|| jsonb_build_object('credits', COALESCE(items.data->'credits', '{}'::jsonb) || COALESCE(excluded.data->'credits', '{}'::jsonb))
	|| CASE WHEN excluded.data->'person' ? 'image' OR NOT COALESCE(items.data->'person' ? 'image', false) THEN '{}'::jsonb
		ELSE jsonb_build_object('person', items.data->'person', 'confined', COALESCE(items.data->'confined', 'false'::jsonb)) END
	|| CASE WHEN excluded.data ? 'tmdbId' OR NOT items.data ? 'tmdbId' THEN '{}'::jsonb
		ELSE jsonb_build_object('tmdbId', items.data->'tmdbId') END
	|| CASE WHEN items.data ? 'tmdb' THEN jsonb_build_object('tmdb', items.data->'tmdb') ELSE '{}'::jsonb END
	|| COALESCE((SELECT jsonb_build_object('deadPhotos', jsonb_object_agg(dead.key, dead.value))
		FROM jsonb_each(items.data->'deadPhotos') AS dead
		WHERE dead.key IN (COALESCE(excluded.data->'person'->>'image', items.data->'person'->>'image'), items.data->'tmdb'->>'photo')
		HAVING count(*) > 0), '{}'::jsonb)`

// saveCredits records the people a title credits, so that apps can open
// them by identifier, and so that a person lists the titles Polyfin knows
// them in. A title's credits replace those recorded for it before: people
// it no longer credits lose it, as in Jellyfin, where a refreshed item's
// people replace the ones it had. Credits accumulate across titles; the
// merge happens in the upsert, so that titles described at once do not
// lose each other's. A person's record is the one of their TMDB
// identifier when the credit gives it while the server's TMDB key is
// saved, else the one of their name, or the one it became (see
// creditedAs).
func (s *Service) saveCredits(ctx context.Context, title accounts.ID, credits []Person, confined bool) error {
	credits, err := s.creditedAs(ctx, title, credits)
	if err != nil {
		return err
	}
	byID := map[accounts.ID]*record{}
	var people []*record
	for _, person := range credits {
		r, ok := byID[person.ID]
		if !ok {
			key := personKey(person.Name)
			if person.ID != PersonID(person.Name) {
				key = personTMDBKey(person.TMDB)
			}
			r = &record{ID: person.ID, Key: key, Kind: KindPerson, Person: &Person{Name: person.Name},
				Credits: map[string][]string{}, Confined: confined}
			byID[person.ID] = r
			people = append(people, r)
		}
		if r.Person.Image == "" {
			r.Person.Image = person.Image
		}
		if r.TMDBID == "" {
			r.TMDBID = person.TMDB
		}
		if types := r.Credits[title.String()]; !slices.Contains(types, person.Type) {
			r.Credits[title.String()] = append(types, person.Type)
		}
	}
	// Upserts lock the rows they reach in order: titles crediting the same
	// people in another order, described together, must not wait on each
	// other.
	slices.SortFunc(people, func(a, b *record) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	if len(people) > 0 {
		if err := s.upsert(ctx, people, mergedPerson); err != nil {
			return err
		}
	}
	// The people no longer credited lose the title, in a statement of its
	// own that locks them in the same order.
	listed := make([]accounts.ID, len(people))
	for i, r := range people {
		listed[i] = r.ID
	}
	_, err = s.db.Exec(ctx, `WITH dropped AS (SELECT id FROM items
			WHERE kind = 'person' AND data->'credits' ? $1 AND NOT id = ANY($2::uuid[]) ORDER BY id FOR UPDATE)
		UPDATE items SET data = jsonb_set(items.data, '{credits}', (items.data->'credits') - $1::text), updated_at = now()
		FROM dropped WHERE items.id = dropped.id`, title.String(), listed)
	return err
}

// People kept by name, and by TMDB identifier.
//
// People used to be kept by name only, and credits without a TMDB
// identifier still are, as are all credits while the server has no TMDB
// key: without TMDB to tell who is who, a name is one person, as before
// (and in Jellyfin), whatever identifiers addons give. A person kept by
// name becomes the person of a TMDB
// identifier once a credit tells who they are (see creditedAs): their
// record is merged into that identifier's (see polyfin_merged_person in
// the migrations, which also moved the people addons gave an identifier),
// and the name's record only points to it (SameAs), so that its
// identifier, and credits by that name alone, still reach them. The merge
// keeps their photo, what TMDB told of them and their credits, and moves
// the users' marks, an administrator's edits and uploaded images.

// creditedAs returns credits with each person's identifier: their TMDB
// identifier's when the credit gives it, else their name's, or the record
// the name's became (SameAs). A name no one is kept by yet comes to point
// to the first person of that name a credit gives the TMDB identifier of,
// as a name credited alone, or opened by name, likely means the person
// already known. A credit of a TMDB identifier first merges
// into that identifier's record the person kept by the same name when the
// name's record is that person: of that identifier, or of no known one
// while it credits this very title, the addon's credits the TMDB ones
// replace. A name's record known as someone else stays: they are
// namesakes.
func (s *Service) creditedAs(ctx context.Context, title accounts.ID, credits []Person) ([]Person, error) {
	byTMDB := s.tmdb.Enabled()
	names := make([]accounts.ID, 0, len(credits))
	for _, person := range credits {
		names = append(names, PersonID(person.Name))
	}
	records, err := s.loadAll(ctx, names)
	if err != nil {
		return nil, err
	}
	byName := make(map[accounts.ID]record, len(records))
	for _, r := range records {
		if r.Kind == KindPerson {
			byName[r.ID] = r
		}
	}
	credits = slices.Clone(credits)
	merged := map[accounts.ID]bool{}
	var unnamed []Person
	for i, person := range credits {
		name := PersonID(person.Name)
		r, ok := byName[name]
		switch {
		case person.TMDB == "" || !byTMDB:
			credits[i].ID = name
			if same, moved := r.sameAs(); ok && moved && r.TMDBID != "" {
				credits[i].ID, credits[i].TMDB = same, r.TMDBID
			}
		default:
			credits[i].ID = creditedID(person.Name, person.TMDB)
			if !ok {
				unnamed = append(unnamed, credits[i])
				continue
			}
			if _, moved := r.sameAs(); moved || merged[name] {
				continue
			}
			identity := r.identity()
			if identity == person.TMDB || identity == "" && len(r.Credits[title.String()]) > 0 {
				if err := s.mergePerson(ctx, name, person.TMDB); err != nil {
					return nil, err
				}
				merged[name] = true
			}
		}
	}
	// A name no one is kept by yet points to the first person of that name
	// credited by their TMDB identifier, so that it reaches them.
	if len(unnamed) > 0 {
		var ids []accounts.ID
		var keys, data []string
		for _, person := range unnamed {
			encoded, err := json.Marshal(record{Kind: KindPerson, Person: &Person{Name: person.Name}, SameAs: person.ID.String(), TMDBID: person.TMDB})
			if err != nil {
				return nil, err
			}
			ids, keys, data = append(ids, PersonID(person.Name)), append(keys, personKey(person.Name)), append(data, string(encoded))
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO items (id, key, kind, data)
			SELECT id, key, 'person', data FROM unnest($1::uuid[], $2::text[], $3::jsonb[]) AS named(id, key, data) ORDER BY id
			ON CONFLICT DO NOTHING`, ids, keys, data); err != nil {
			return nil, err
		}
	}
	return credits, nil
}

// sameAs is the record a person kept by name became (SameAs), false for
// one that became none.
func (r record) sameAs() (accounts.ID, bool) {
	if r.SameAs == "" {
		return accounts.ID{}, false
	}
	id, err := accounts.ParseID(r.SameAs)
	return id, err == nil
}

// identity is the TMDB identifier of a person kept by name: the one an
// addon gave, else the one TMDB found them as; empty when neither is
// known.
func (r record) identity() string {
	switch {
	case r.TMDBID != "":
		return r.TMDBID
	case r.TMDB != nil && r.TMDB.ID > 0:
		return strconv.Itoa(r.TMDB.ID)
	}
	return ""
}

// mergePerson merges the person kept by name as source into the record of
// the TMDB identifier tmdb (see polyfin_merged_person), and has source
// point to it, with the users' marks, an administrator's edits and
// uploaded images; nothing when source already points somewhere.
func (s *Service) mergePerson(ctx context.Context, source accounts.ID, tmdb string) error {
	target := itemID(personTMDBKey(tmdb))
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		first, second := source, target
		if bytes.Compare(first[:], second[:]) > 0 {
			first, second = second, first
		}
		if _, err := tx.Exec(ctx, `SELECT 1 FROM items WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, []accounts.ID{first, second}); err != nil {
			return err
		}
		var data []byte
		err := tx.QueryRow(ctx, `SELECT data FROM items WHERE id = $1 AND kind = 'person' AND NOT data ? 'sameAs'`, source).Scan(&data)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE items SET data = polyfin_merged_person(data, $2::jsonb, $3), updated_at = now()
			WHERE id = $1`, target, data, tmdb)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO items (id, key, kind, data) VALUES ($1, $2, 'person', polyfin_merged_person(NULL, $3::jsonb, $4))`,
				target, personTMDBKey(tmdb), data, tmdb); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE items SET data = jsonb_build_object('kind', 'person', 'person', jsonb_build_object('name', data->'person'->'name'),
				'sameAs', $2::text, 'tmdbId', $3::text), updated_at = now() WHERE id = $1`, source, target.String(), tmdb); err != nil {
			return err
		}
		for _, moved := range []string{
			`UPDATE user_data SET item_id = $2 WHERE item_id = $1
				AND NOT EXISTS (SELECT 1 FROM user_data AS kept WHERE kept.user_id = user_data.user_id AND kept.item_id = $2)`,
			`UPDATE item_overrides SET item_id = $2 WHERE item_id = $1 AND NOT EXISTS (SELECT 1 FROM item_overrides WHERE item_id = $2)`,
			`UPDATE item_images SET item_id = $2 WHERE item_id = $1
				AND NOT EXISTS (SELECT 1 FROM item_images AS kept WHERE kept.item_id = $2 AND kept.image_type = item_images.image_type)`,
		} {
			if _, err := tx.Exec(ctx, moved, source, target); err != nil {
				return err
			}
		}
		return nil
	})
}

// searchable reports whether a catalog looks a term up: it takes a search
// property, and requires no other, as Stremio apps search only the
// catalogs a search alone can ask.
func searchable(catalog stremio.Catalog) bool {
	search := false
	for _, extra := range catalog.Extra {
		if extra.Name == "search" {
			search = true
		} else if extra.IsRequired {
			return false
		}
	}
	return search
}

// searchesPeople reports whether a catalog looks titles up by the name of
// someone credited in them. The Stremio protocol has no word for this:
// metadata addons name such catalogs after people, such as "People Search"
// catalogs (people_search.people_search_movie and
// people_search.people_search_series).
func searchesPeople(catalog stremio.Catalog) bool {
	if !searchable(catalog) {
		return false
	}
	for _, text := range []string{catalog.ID, catalog.Name} {
		text = strings.ToLower(text)
		if strings.Contains(text, "people") || strings.Contains(text, "person") {
			return true
		}
	}
	return false
}

// reaches reports whether a user's view reaches a title Polyfin recorded:
// one of their addons listed it, or one of their addons describes titles
// of its type and identifier. See also reachesTitle, which must agree.
func (v view) reaches(r record) bool {
	if r.Addon != nil {
		if _, ok := v.addon(*r.Addon); ok {
			return true
		}
	}
	return r.Meta != nil && slices.ContainsFunc(v.addons, func(entry installed) bool {
		return entry.addon.Manifest.Serves("meta", r.Meta.Type, r.Meta.ID)
	})
}

// reach is view.reaches as query parameters (see reachesTitle): the addons
// of the view, encoded as records store them, and the types and identifier
// prefixes their meta resources serve, an empty prefix serving any
// identifier.
func (v view) reach() (addonIDs, types, prefixes []string, err error) {
	addonIDs, types, prefixes = []string{}, []string{}, []string{}
	for _, entry := range v.addons {
		encoded, err := json.Marshal(entry.addon.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		addonIDs = append(addonIDs, string(encoded))
		manifest := entry.addon.Manifest
		for _, resource := range manifest.Resources {
			if resource.Name != "meta" {
				continue
			}
			served, idPrefixes := resource.Types, resource.IDPrefixes
			if len(served) == 0 {
				served = manifest.Types
			}
			if len(idPrefixes) == 0 {
				idPrefixes = manifest.IDPrefixes
			}
			if len(idPrefixes) == 0 {
				idPrefixes = []string{""}
			}
			for _, kind := range served {
				for _, prefix := range idPrefixes {
					types, prefixes = append(types, kind), append(prefixes, prefix)
				}
			}
		}
	}
	return addonIDs, types, prefixes, nil
}

// reachesTitle is view.reaches on the row title of the items table, with
// the parameters of view.reach at $10, $11 and $12.
const reachesTitle = `(title.data->'addon' = ANY($10::jsonb[]) OR EXISTS (
	SELECT 1 FROM unnest($11::text[], $12::text[]) AS rule(type, prefix)
	WHERE title.data->'meta'->>'type' = rule.type AND starts_with(title.data->'meta'->>'id', rule.prefix)))`

// credited lists the titles a person is credited in that the user's view
// reaches and their parental control lets them see, but the movies
// waiting for their digital release. Jellyfin knows people through the
// items a user can access; likewise, a person credited only in titles of
// other users' addons, or in titles hidden from the user, is not found.
func (s *Service) credited(ctx context.Context, v view, person record) ([]record, error) {
	if person.Person == nil {
		return nil, ErrNotFound
	}
	ids := make([]accounts.ID, 0, len(person.Credits))
	for raw := range person.Credits {
		if id, err := accounts.ParseID(raw); err == nil {
			ids = append(ids, id)
		}
	}
	titles, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	titles = slices.DeleteFunc(titles, func(title record) bool { return title.Meta == nil || !v.reaches(title) })
	titles, _ = s.visible(ctx, v, titles)
	titles, _ = s.released(ctx, v, titles)
	if len(titles) == 0 {
		return nil, ErrNotFound
	}
	return titles, nil
}

// PersonTitles lists titles of the given kinds a person is credited in:
// first those the people-search catalogs of the user's addons find for the
// person's name, at least count of them when the catalogs have that many,
// then the other titles of the user's addons Polyfin knows the person in,
// those TMDB credits them in included (see tmdbCredited), then the titles
// TMDB knows them best for that the page does not list yet, beyond the
// libraries (see knownFor).
// A people search can miss a title, or find someone else of the same name:
// the known titles keep the credits the user saw. A people search may also
// find a known title under another addon's identifier, such as "tmdb:…"
// for the "tt…" a library lists: it is listed once, as the known title,
// which keeps its folder. Page.More tells that the catalogs have more
// titles. A person whose people search fails still lists their known
// titles.
func (s *Service) PersonTitles(ctx context.Context, user accounts.User, person accounts.ID, kinds []Kind, count int) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	r, err := s.person(ctx, person)
	if err != nil {
		return Page{}, err
	}
	if r.Kind != KindPerson {
		return Page{}, ErrNotFound
	}
	credited, err := s.credited(ctx, v, r)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Page{}, err
	}
	tmdbCredits := s.personTMDBCredits(ctx, r)
	for _, title := range s.tmdbCredited(ctx, v, tmdbCredits) {
		if !slices.ContainsFunc(credited, func(known record) bool { return known.ID == title.ID }) {
			credited = append(credited, title)
		}
	}
	// The titles no catalog listed, as those of the titles a person is known
	// for (see knownFor), are no titles of the libraries: they are listed
	// with those.
	unlisted := slices.DeleteFunc(slices.Clone(credited), func(title record) bool { return title.Addon != nil })
	credited = slices.DeleteFunc(credited, func(title record) bool { return title.Addon == nil })
	best := s.knownFor(ctx, v, tmdbCredits, credited, unlisted)
	if err := s.saveNew(ctx, best); err != nil {
		return Page{}, err
	}
	if len(credited) == 0 && len(best) == 0 {
		return Page{}, ErrNotFound
	}
	// A title's people are its facts: an addon given a language tells
	// none of them, unless no addon without a language searches people
	// (see languages.go).
	sources := standInCatalogs(v, kinds, false, func(entry installed, catalog stremio.Catalog) (source, bool) {
		return source{addon: entry, catalog: catalog, search: r.Person.Name}, searchesPeople(catalog)
	})
	known := s.creditedTitles(ctx, v, credited, kinds)
	bestItems := s.titleItems(ctx, v, best, kinds)
	// The titles listed by their identifiers: by IMDb identifier, and by
	// kind and TMDB identifier.
	listed := map[string]accounts.ID{}
	for _, item := range slices.Concat(known, bestItems) {
		for _, key := range listedKeys(item) {
			if _, ok := listed[key]; !ok {
				listed[key] = item.ID
			}
		}
	}
	var items []Item
	var records []record
	seen := map[accounts.ID]bool{}
	more := false
	if len(sources) > 0 && count > 0 {
		titles, total, err := s.merged(ctx, v, sources, 0, count)
		if ctx.Err() != nil {
			return Page{}, ctx.Err()
		}
		if err != nil {
			s.logger.Warn("A people search failed", "catalog", sources[0].catalog.ID, "error", err)
		}
		more = total > len(titles)
		for _, title := range titles {
			item, rec, err := title.title(accounts.ID{})
			if err != nil || seen[item.ID] {
				continue
			}
			if slices.ContainsFunc(listedKeys(item), func(key string) bool { id, ok := listed[key]; return ok && id != item.ID }) {
				continue
			}
			seen[item.ID] = true
			// Like a search result, the title keeps the folder it was last
			// listed in.
			rec.Parent = nil
			items, records = append(items, item), append(records, rec)
		}
	}
	for _, item := range known {
		if !seen[item.ID] {
			seen[item.ID] = true
			items = append(items, item)
		}
	}
	for _, item := range bestItems {
		if !seen[item.ID] && !slices.ContainsFunc(listedKeys(item), func(key string) bool { return listed[key] != item.ID }) {
			seen[item.ID] = true
			items = append(items, item)
		}
	}
	total := len(items)
	if more {
		total++
	}
	s.translate(ctx, v, items)
	return Page{Items: s.overridden(items), Total: total, More: more}, s.save(ctx, records)
}

// listedKeys are the keys a title is listed by in PersonTitles: its IMDb
// identifier, and its kind with its TMDB identifier, as the identifiers
// of one title may differ among addons.
func listedKeys(item Item) []string {
	var keys []string
	if imdb := item.ProviderIDs["Imdb"]; imdb != "" {
		keys = append(keys, "imdb|"+imdb)
	}
	if tmdb := item.ProviderIDs["Tmdb"]; tmdb != "" {
		keys = append(keys, string(item.Kind)+"|tmdb|"+tmdb)
	}
	return keys
}

// creditedTitles describes, by name, the titles of the given kinds among a
// person's credited ones (see person), as titleItems does.
func (s *Service) creditedTitles(ctx context.Context, v view, credited []record, kinds []Kind) []Item {
	items := s.titleItems(ctx, v, credited, kinds)
	slices.SortFunc(items, func(a, b Item) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return items
}

// titleItems describes the titles of the given kinds among records, in
// order, from what was stored when they were listed, with the complete
// description when it is still cached (see describedMeta): describing each
// title anew would cost a request per title. A title keeps its folder
// only when it came from one of the user's addons. The titles the user hid
// are left out.
func (s *Service) titleItems(ctx context.Context, v view, records []record, kinds []Kind) []Item {
	var items []Item
	for _, r := range records {
		if !slices.Contains(kinds, r.Kind) || v.hidden[r.ID] {
			continue
		}
		meta := *r.Meta
		if full, _, ok := s.describedMeta(ctx, v, r, false, inMemory); ok {
			meta = full
		}
		item := Item{ID: r.ID, Kind: r.Kind, Available: true}
		if _, own := v.addon(deref(r.Addon)); own {
			item.ParentID = deref(r.Parent)
		}
		fromMeta(&item, meta)
		items = append(items, item)
	}
	return items
}

// PeopleQuery narrows the people Polyfin knows. Names compare without
// regard to case.
type PeopleQuery struct {
	// NameContains, NameStartsWith keep names containing, or starting
	// with, the text; NameBefore keeps names sorting before it, NameFrom
	// names sorting at or after it. Empty ones keep every name.
	NameContains, NameStartsWith, NameBefore, NameFrom string
	// A person is kept when one of their credits in the titles the user
	// reaches ("Actor", "Director", "Writer") is one of Types, if set, and
	// not one of ExcludedTypes.
	Types, ExcludedTypes []string
	// Restricted keeps only the people of Only; Excluded leaves people out.
	Restricted     bool
	Only, Excluded []accounts.ID
	// Start skips people; a negative Limit lists all the others.
	Start, Limit int
}

// peopleFilter selects the people matching $1 to $13 (see People): those
// credited in a title the user reaches, among the titles of $13 when it is
// set. Like Jellyfin 12.2, it keeps only the people People can list, so
// that the count it reports is theirs.
const peopleFilter = `FROM items AS person WHERE person.kind = 'person' AND jsonb_typeof(person.data->'person') = 'object'
	AND ($1 = '' OR strpos(lower(person.data->'person'->>'name'), lower($1)) > 0)
	AND ($2 = '' OR starts_with(lower(person.data->'person'->>'name'), lower($2)))
	AND ($3 = '' OR lower(person.data->'person'->>'name') < lower($3))
	AND ($4 = '' OR lower(person.data->'person'->>'name') >= lower($4))
	AND EXISTS (
		SELECT 1 FROM jsonb_each(person.data->'credits') AS credit(item, kinds)
			JOIN items AS title ON title.id = credit.item::uuid
			CROSS JOIN jsonb_array_elements_text(credit.kinds) AS credited(kind)
		WHERE (cardinality($5::text[]) = 0 OR lower(credited.kind) = ANY($5::text[])) AND NOT lower(credited.kind) = ANY($6::text[])
			AND ($13::uuid[] IS NULL OR title.id = ANY($13::uuid[]))
			AND ` + reachesTitle + `)
	AND (NOT $7 OR person.id = ANY($8::uuid[]))
	AND NOT person.id = ANY($9::uuid[])`

// People lists, by name, the people credited in the titles Polyfin knows
// that the user reaches, and how many match the query. Jellyfin likewise
// knows people through the items a user can access, whatever their
// library; titles the user's parental control hides credit no one.
func (s *Service) People(ctx context.Context, user accounts.User, q PeopleQuery) ([]Item, int, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	addonIDs, types, prefixes, err := v.reach()
	if err != nil {
		return nil, 0, err
	}
	lower := func(values []string) []string {
		result := make([]string, 0, len(values))
		for _, value := range values {
			result = append(result, strings.ToLower(value))
		}
		return result
	}
	ids := func(values []accounts.ID) []accounts.ID {
		if values == nil {
			return []accounts.ID{}
		}
		return values
	}
	args := []any{q.NameContains, q.NameStartsWith, q.NameBefore, q.NameFrom, lower(q.Types), lower(q.ExcludedTypes),
		q.Restricted, ids(q.Only), ids(q.Excluded), addonIDs, types, prefixes, []accounts.ID(nil)}
	if v.restricted() {
		visible, err := s.visibleCredits(ctx, v, args)
		if err != nil {
			return nil, 0, err
		}
		args[len(args)-1] = visible
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) "+peopleFilter, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	var limit *int
	if q.Limit >= 0 {
		limit = &q.Limit
	}
	rows, err := s.db.Query(ctx, "SELECT person.id, person.data "+peopleFilter+
		" ORDER BY lower(person.data->'person'->>'name'), person.id OFFSET $14 LIMIT $15", append(args, max(q.Start, 0), limit)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var people []Item
	now := s.now()
	for rows.Next() {
		var id accounts.ID
		var r record
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, 0, err
		}
		if r.Person != nil {
			people = append(people, Item{ID: id, Kind: KindPerson, Name: r.Person.Name, Images: Images{Primary: r.personPhoto(now)}})
		}
	}
	return s.overridden(people), total, rows.Err()
}

// visibleCredits lists the titles crediting the people a query matches
// (see People, whose arguments args are) that the user's parental control
// lets them see.
func (s *Service) visibleCredits(ctx context.Context, v view, args []any) ([]accounts.ID, error) {
	rows, err := s.db.Query(ctx, "SELECT DISTINCT credit.item::uuid FROM items AS person, jsonb_each(person.data->'credits') AS credit(item, kinds) WHERE person.id IN (SELECT person.id "+peopleFilter+")", args...)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return nil, err
	}
	titles, err := s.loadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	titles = slices.DeleteFunc(titles, func(title record) bool { return title.Meta == nil || !v.reaches(title) })
	titles, _ = s.visible(ctx, v, titles)
	visible := make([]accounts.ID, 0, len(titles))
	for _, title := range titles {
		visible = append(visible, title.ID)
	}
	return visible, nil
}
