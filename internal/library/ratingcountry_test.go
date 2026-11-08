package library

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// A title shows the rating of the server's rating country: the addon's
// rating for that country first, then its local certification when it is
// one of that country's, then its certification.
func TestCertificationPicksTheRatingCountrysRating(t *testing.T) {
	t.Parallel()
	meta := func(extras stremio.Extras) stremio.Meta { return stremio.Meta{Extras: &extras} }
	for _, c := range []struct {
		name, country string
		extras        stremio.Extras
		want          string
	}{
		{"country's rating", "IT", stremio.Extras{Certification: "PG-13", Certifications: stremio.Certifications{"IT": "VM14", "US": "PG-13"}}, "VM14"},
		{"another country's rating", "US", stremio.Extras{Certification: "PG-13", Certifications: stremio.Certifications{"IT": "VM14", "US": "PG-13"}}, "PG-13"},
		{"local rating of the country", "IT", stremio.Extras{Certification: "R", CertificationLocal: "VM14"}, "VM14"},
		{"local rating of another country", "IT", stremio.Extras{Certification: "R", CertificationLocal: "PG-13"}, "R"},
		{"rating text", "US", stremio.Extras{CertificationLocal: "PG-13 - Teens 13 or older"}, "PG-13"},
		// Not an Italian rating, yet the only one given, as a local FSK 12
		// is in the US.
		{"rating text of another country", "IT", stremio.Extras{CertificationLocal: "PG-13 - Teens 13 or older"}, "PG-13"},
		{"unknown rating text", "US", stremio.Extras{CertificationLocal: "Rx - Hentai"}, ""},
		{"only a local rating", "US", stremio.Extras{CertificationLocal: "FSK 12"}, "FSK 12"},
		{"default country", "", stremio.Extras{Certification: "PG", Certifications: stremio.Certifications{"IT": "T"}}, "PG"},
		{"youngest rating", "IT", stremio.Extras{Certifications: stremio.Certifications{"IT": "T"}}, "T"},
	} {
		if got := certification(meta(c.extras), c.country); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if got := certification(stremio.Meta{}, "IT"); got != "" {
		t.Errorf("no extras: %q", got)
	}
}

// italianAddon serves movies rated in the US and in Italy: a teen movie
// rated VM18 in Italy, and one rated T, for all ages there and unrated in
// the US.
func italianAddon() *fakeAddon {
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "it", Name: "IT", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
		streams:  map[string][]stremio.Stream{},
	}
	for _, m := range []struct {
		id, name string
		extras   stremio.Extras
	}{
		{"tt1", "Strong", stremio.Extras{Certification: "PG-13", Certifications: stremio.Certifications{"US": "PG-13", "IT": "VM18"}}},
		{"tt2", "Gentle", stremio.Extras{Certifications: stremio.Certifications{"IT": "T"}}},
	} {
		row := stremio.Meta{ID: m.id, Type: "movie", Name: m.name}
		full := row
		full.Extras = &m.extras
		addon.catalogs["movie/top"] = append(addon.catalogs["movie/top"], row)
		addon.metas["movie/"+m.id] = full
		addon.streams["movie/"+m.id] = []stremio.Stream{{Name: "1080p", URL: "https://cdn.example/" + m.id}}
	}
	return addon
}

// Parental control judges titles by the ratings of the server's rating
// country, and the titles show them.
func TestParentalControlJudgesByTheRatingCountry(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), italianAddon())
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13), BlockUnrated: []string{"Movie"}})

	// In the US, the teen movie is allowed and the other is unrated.
	if got := names(e.children(child, "Top", 0, 10).Items); !slices.Equal(got, []string{"Strong"}) {
		t.Fatalf("US listing: %v", got)
	}

	e.setting(func(s *accounts.Settings) { s.RatingCountry = "IT" })
	if got := names(e.children(child, "Top", 0, 10).Items); !slices.Equal(got, []string{"Gentle"}) {
		t.Fatalf("Italian listing: %v", got)
	}
	for _, item := range e.children(e.member, "Top", 0, 10).Items {
		want := map[string]string{"Strong": "VM18", "Gentle": "T"}[item.Name]
		if full, err := e.service.Item(t.Context(), e.member, item.ID); err != nil || full.OfficialRating != want {
			t.Errorf("%s: %q %v, want %q", item.Name, full.OfficialRating, err, want)
		}
	}
}

// A rating kept for another country is stale: it is asked again, kept for
// the server's country, and refreshing ratings asks for those first.
func TestRatingsOfAnotherCountryAreAskedAgain(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	rating, genres, now := "PG", []string{}, e.service.now()
	meta := stremio.Meta{ID: "tt1", Type: "movie", Name: "Kept", Extras: &stremio.Extras{Certification: "PG",
		Certifications: stremio.Certifications{"IT": "T"}}}
	r := record{ID: itemID(titleKey(KindMovie, "tt1")), Key: titleKey(KindMovie, "tt1"), Kind: KindMovie, Meta: &meta,
		Rating: &rating, Genres: &genres, RatedAt: &now}
	if _, stale := e.service.knownTraits(ctx, view{}, r); stale {
		t.Error("a fresh US rating is stale in the US")
	}
	if _, stale := e.service.knownTraits(ctx, view{country: "IT"}, r); !stale {
		t.Error("a US rating is not stale in Italy")
	}

	e.service.learnTraits(ctx, r, meta, now, "IT")
	kept, err := e.service.load(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if kept.RatedFor != "IT" || kept.Rating == nil || *kept.Rating != "T" {
		t.Fatalf("kept for Italy: %q %v", kept.RatedFor, kept.Rating)
	}
	if _, stale := e.service.knownTraits(ctx, view{country: "IT"}, kept); stale {
		t.Error("an Italian rating is stale in Italy")
	}
}

// Refreshing ratings asks again for those kept for another country, fresh
// as they are.
func TestRefreshRatingsAsksForAnotherCountrysRatings(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.install(addons.Shared(), italianAddon())
	child := e.restrict("child", accounts.ParentalControl{MaxRating: new(13)})
	e.children(child, "Top", 0, 10)
	id := itemID(titleKey(KindMovie, "tt1"))
	if r, err := e.service.load(t.Context(), id); err != nil || r.Rating == nil || r.ratedFor() != "US" {
		t.Fatalf("rating kept in the US: %+v %v", r, err)
	}
	if asked, err := e.service.RefreshRatings(t.Context()); err != nil || asked != 0 {
		t.Fatalf("fresh ratings asked again: %d %v", asked, err)
	}

	e.setting(func(s *accounts.Settings) { s.RatingCountry = "IT" })
	if asked, err := e.service.RefreshRatings(t.Context()); err != nil || asked == 0 {
		t.Fatalf("ratings of another country not asked again: %d %v", asked, err)
	}
	if r, err := e.service.load(t.Context(), id); err != nil || r.RatedFor != "IT" || r.Rating == nil || *r.Rating != "VM18" {
		t.Errorf("rating kept in Italy: %+v %v", r, err)
	}
}

// A virtual channel is judged by its entries' ratings as the server's
// rating country rates them.
func TestVirtualChannelsAreJudgedByTheRatingCountry(t *testing.T) {
	t.Parallel()
	v := view{parental: accounts.ParentalControl{MaxRating: new(13)}, country: "IT"}
	channel := func(rating string) virtualtv.Channel {
		return virtualtv.Channel{Enabled: true, Entries: []virtualtv.Entry{{Kind: virtualtv.EntryMovie, Runtime: time.Hour, Rating: rating}}}
	}
	if v.allowsVirtual(channel("VM18"), &overrideSet{}) {
		t.Error("a channel airing a VM18 movie is allowed up to 13")
	}
	if !v.allowsVirtual(channel("T"), &overrideSet{}) {
		t.Error("a channel airing a T movie is not allowed up to 13")
	}
}

// A virtual channel judges and shows its titles' ratings as administrators
// set them: a series' custom rating covers its episodes, and an entry whose
// title has no edit keeps the rating it was saved with.
func TestVirtualChannelsFollowTheEditsOfTheirTitles(t *testing.T) {
	t.Parallel()
	v := view{parental: accounts.ParentalControl{MaxRating: new(13)}, country: "US"}
	movie, series := accounts.ID{1}, accounts.ID{2}
	episode := virtualtv.Entry{Title: series, Kind: virtualtv.EntryEpisode, Runtime: time.Hour, Rating: "TV-PG", Genres: []string{"Drama"}}
	channel := virtualtv.Channel{Enabled: true, Entries: []virtualtv.Entry{
		{Title: movie, Kind: virtualtv.EntryMovie, Runtime: time.Hour, Rating: "PG"}, episode}}
	if !v.allowsVirtual(channel, &overrideSet{}) {
		t.Fatal("a channel airing PG and TV-PG titles is not allowed up to 13")
	}
	custom := "TV-MA"
	set := &overrideSet{fields: map[accounts.ID]Overrides{series: {CustomRating: &custom}}}
	if v.allowsVirtual(channel, set) {
		t.Error("a channel airing a series rated TV-MA by its custom rating is allowed up to 13")
	}
	airing := virtualtv.Airing{Entry: episode, Start: time.Unix(0, 0), End: time.Unix(3600, 0)}
	if p := virtualProgram(set, Item{}, airing); p.OfficialRating != "TV-PG" || p.CustomRating != "TV-MA" || p.ParentalRating() != "TV-MA" {
		t.Errorf("the episode's programme: %q %q", p.OfficialRating, p.CustomRating)
	}
	official := "G"
	set = &overrideSet{fields: map[accounts.ID]Overrides{movie: {OfficialRating: &official, Genres: []string{"Family"}}}}
	if p := virtualProgram(set, Item{}, airing); p.OfficialRating != "TV-PG" || p.CustomRating != "" {
		t.Errorf("an episode whose series has no edit: %q %q", p.OfficialRating, p.CustomRating)
	}
	airing.Entry = channel.Entries[0]
	if p := virtualProgram(set, Item{}, airing); p.OfficialRating != "G" || p.CustomRating != "" || !slices.Equal(p.Genres, []string{"Family", "Movie"}) {
		t.Errorf("the movie's programme: %q %q %q", p.OfficialRating, p.CustomRating, p.Genres)
	}
}
