package library

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// PersonID identifies someone credited in titles by their name, the way
// titles' credits do.
func PersonID(name string) accounts.ID { return itemID(personKey(name)) }

// mergedPerson is the record a person's upsert stores: the new one, with the
// credits of both, and the stored photo when the new one has none.
const mergedPerson = `excluded.data
	|| jsonb_build_object('credits', COALESCE(items.data->'credits', '{}'::jsonb) || COALESCE(excluded.data->'credits', '{}'::jsonb))
	|| CASE WHEN excluded.data->'person' ? 'image' OR NOT COALESCE(items.data->'person' ? 'image', false) THEN '{}'::jsonb
		ELSE jsonb_build_object('person', items.data->'person', 'confined', COALESCE(items.data->'confined', 'false'::jsonb)) END`

// saveCredits records the people a title credits, so that apps can open
// them by identifier, and so that a person lists the titles Polyfin knows
// them in. Credits accumulate across titles; the merge happens in the
// upsert, so that titles described at once do not lose each other's.
func (s *Service) saveCredits(ctx context.Context, title accounts.ID, credits []Person, confined bool) error {
	if len(credits) == 0 {
		return nil
	}
	byID := map[accounts.ID]*record{}
	var people []*record
	for _, person := range credits {
		r, ok := byID[person.ID]
		if !ok {
			r = &record{ID: person.ID, Key: personKey(person.Name), Kind: KindPerson, Person: &Person{Name: person.Name},
				Credits: map[string][]string{}, Confined: confined}
			byID[person.ID] = r
			people = append(people, r)
		}
		if r.Person.Image == "" {
			r.Person.Image = person.Image
		}
		if types := r.Credits[title.String()]; !slices.Contains(types, person.Type) {
			r.Credits[title.String()] = append(types, person.Type)
		}
	}
	// Upserts lock the rows they reach in order: titles crediting the same
	// people in another order, described together, must not wait on each
	// other.
	slices.SortFunc(people, func(a, b *record) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	ids := make([]accounts.ID, 0, len(people))
	keys := make([]string, 0, len(people))
	kinds := make([]string, 0, len(people))
	data := make([]string, 0, len(people))
	for _, r := range people {
		encoded, err := json.Marshal(r)
		if err != nil {
			return err
		}
		ids, keys, kinds, data = append(ids, r.ID), append(keys, r.Key), append(kinds, string(r.Kind)), append(data, string(encoded))
	}
	_, err := s.db.Exec(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[], $4::jsonb[])
		ON CONFLICT (id) DO UPDATE SET data = `+mergedPerson+`, updated_at = now()
		WHERE items.data IS DISTINCT FROM `+mergedPerson, ids, keys, kinds, data)
	return err
}

// searchable reports whether a catalog looks a term up: it takes a search
// property, and any other property it requires offers options.
func searchable(catalog stremio.Catalog) bool {
	search := false
	for _, extra := range catalog.Extra {
		if extra.Name == "search" {
			search = true
		} else if extra.IsRequired && len(extra.Options) == 0 {
			return false
		}
	}
	return search
}

// searchesPeople reports whether a catalog looks titles up by the name of
// someone credited in them. The Stremio protocol has no word for this:
// metadata addons name such catalogs after people, as AIOMetadata's
// "People Search" catalogs (people_search.people_search_movie and
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
// reaches. Jellyfin knows people through the items a user can access;
// likewise, a person credited only in titles of other users' addons is not
// found.
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
	if len(titles) == 0 {
		return nil, ErrNotFound
	}
	return titles, nil
}

// PersonTitles lists titles of the given kinds a person is credited in:
// first those the people-search catalogs of the user's addons find for the
// person's name, at least count of them when the catalogs have that many,
// then the other titles of the user's addons Polyfin knows the person in.
// A people search can miss a title, or find someone else of the same name:
// the known titles keep the credits the user saw. Page.More tells that the
// catalogs have more titles. A person whose people search fails still lists
// their known titles.
func (s *Service) PersonTitles(ctx context.Context, user accounts.User, person accounts.ID, kinds []Kind, count int) (Page, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return Page{}, err
	}
	r, err := s.load(ctx, person)
	if err != nil {
		return Page{}, err
	}
	if r.Kind != KindPerson {
		return Page{}, ErrNotFound
	}
	credited, err := s.credited(ctx, v, r)
	if err != nil {
		return Page{}, err
	}
	var sources []source
	for _, entry := range v.addons {
		for _, catalog := range entry.addon.Manifest.Catalogs {
			if kind, ok := titleKind(catalog.Type); ok && slices.Contains(kinds, kind) && searchesPeople(catalog) {
				sources = append(sources, source{addon: entry, catalog: catalog, search: r.Person.Name})
			}
		}
	}
	var items []Item
	var records []record
	seen := map[accounts.ID]bool{}
	more := false
	if len(sources) > 0 && count > 0 {
		titles, total, err := s.merged(ctx, sources, 0, count)
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
			seen[item.ID] = true
			// Like a search result, the title keeps the folder it was last
			// listed in.
			rec.Parent = nil
			items, records = append(items, item), append(records, rec)
		}
	}
	for _, item := range s.creditedTitles(v, credited, kinds) {
		if !seen[item.ID] {
			items = append(items, item)
		}
	}
	total := len(items)
	if more {
		total++
	}
	return Page{Items: items, Total: total, More: more}, s.save(ctx, records)
}

// creditedTitles describes, by name, the titles of the given kinds among a
// person's credited ones (see person), from what was stored when they were
// listed, with the complete description when it is still cached:
// describing each title anew would cost a request per title. A title keeps
// its folder only when it came from one of the user's addons.
func (s *Service) creditedTitles(v view, credited []record, kinds []Kind) []Item {
	var items []Item
	for _, r := range credited {
		if !slices.Contains(kinds, r.Kind) {
			continue
		}
		meta := *r.Meta
		if full, ok := s.cachedMeta(r); ok {
			meta = full
		}
		item := Item{ID: r.ID, Kind: r.Kind, Available: true}
		if _, own := v.addon(deref(r.Addon)); own {
			item.ParentID = deref(r.Parent)
		}
		fromMeta(&item, meta)
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b Item) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
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

// peopleFilter selects the people matching $1 to $12 (see People): those
// credited in a title the user reaches.
const peopleFilter = `FROM items AS person WHERE person.kind = 'person'
	AND ($1 = '' OR strpos(lower(person.data->'person'->>'name'), lower($1)) > 0)
	AND ($2 = '' OR starts_with(lower(person.data->'person'->>'name'), lower($2)))
	AND ($3 = '' OR lower(person.data->'person'->>'name') < lower($3))
	AND ($4 = '' OR lower(person.data->'person'->>'name') >= lower($4))
	AND EXISTS (
		SELECT 1 FROM jsonb_each(person.data->'credits') AS credit(item, kinds)
			JOIN items AS title ON title.id = credit.item::uuid
			CROSS JOIN jsonb_array_elements_text(credit.kinds) AS credited(kind)
		WHERE (cardinality($5::text[]) = 0 OR lower(credited.kind) = ANY($5::text[])) AND NOT lower(credited.kind) = ANY($6::text[])
			AND ` + reachesTitle + `)
	AND (NOT $7 OR person.id = ANY($8::uuid[]))
	AND NOT person.id = ANY($9::uuid[])`

// People lists, by name, the people credited in the titles Polyfin knows
// that the user reaches, and how many match the query. Jellyfin likewise
// knows people through the items a user can access, whatever their
// library.
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
		q.Restricted, ids(q.Only), ids(q.Excluded), addonIDs, types, prefixes}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) "+peopleFilter, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	var limit *int
	if q.Limit >= 0 {
		limit = &q.Limit
	}
	rows, err := s.db.Query(ctx, "SELECT person.id, person.data "+peopleFilter+
		" ORDER BY lower(person.data->'person'->>'name'), person.id OFFSET $13 LIMIT $14", append(args, max(q.Start, 0), limit)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var people []Item
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
			people = append(people, Item{ID: id, Kind: KindPerson, Name: r.Person.Name, Images: Images{Primary: r.Person.Image}})
		}
	}
	return people, total, rows.Err()
}
