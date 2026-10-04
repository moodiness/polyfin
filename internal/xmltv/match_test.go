package xmltv

import (
	"slices"
	"testing"
)

type guideChannel struct {
	id     string
	names  []string
	titles int
}

// match adds channels, as "stremio ID=name" or a name, declares the guide's
// channels in order and returns what each channel takes.
func match(language string, channels []string, guide []guideChannel) []string {
	m := NewMatcher(language)
	for i, channel := range channels {
		id, name, ok := cutID(channel)
		if !ok {
			id, name = string(rune('a'+i)), channel
		}
		m.Add(id, "", name)
	}
	titles := map[string]int{}
	for _, g := range guide {
		m.Declare(Channel{ID: g.id, Names: g.names})
		titles[g.id] = g.titles
	}
	return m.Choose(func(id string) int { return titles[id] })
}

func cutID(channel string) (string, string, bool) {
	for i := range channel {
		if channel[i] == '=' {
			return channel[:i], channel[i+1:], true
		}
	}
	return "", "", false
}

func expect(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s: %q, want %q", what, got, want)
	}
}

// A name several countries share takes the guide of the server language's
// country, told by the guide channel's identifier or its display name's
// prefix; another country's guide is still better than none.
func TestGuideChannelsOfTheServerCountryWin(t *testing.T) {
	byID := []guideChannel{{"ZebMax.de", []string{"Zeb Max"}, 30}, {"ZebMax.fr", []string{"Zeb Max"}, 10}}
	expect(t, "by identifier", match("fr", []string{"Zeb Max"}, byID), "ZebMax.fr")
	byPrefix := []guideChannel{{"zm1", []string{"PL| Zeb Max"}, 30}, {"zm2", []string{"Zeb Max Polska", "FR| Zeb Max"}, 10}}
	expect(t, "by display-name prefix", match("fr", []string{"Zeb Max"}, byPrefix), "zm2")
	// An identifier from elsewhere does not hide a prefix of the country.
	elsewhere := []guideChannel{{"ZebMax.pl", []string{"PL| Zeb Max"}, 30}, {"ZebMax.mu", []string{"FR| Zeb Max"}, 10}}
	expect(t, "prefix beside an identifier", match("fr", []string{"Zeb Max"}, elsewhere), "ZebMax.mu")
	expect(t, "German server", match("de", []string{"Zeb Max"}, byID), "ZebMax.de")
	// English suggests no country: the guide with the most titles.
	expect(t, "English server", match("en", []string{"Zeb Max"}, byID), "ZebMax.de")
	expect(t, "only another country's", match("fr", []string{"Zeb Max"}, byID[:1]), "ZebMax.de")
	// The country comes before the tier: a provider names a channel with
	// quality tags in one country and plainly in another.
	tagged := []guideChannel{{"ZebMax.de", []string{"DE| Zeb Max"}, 30}, {"ZebMax.fr", []string{"FR| Zeb Max FHD"}, 10}}
	expect(t, "country before tier", match("fr", []string{"Zeb Max"}, tagged), "ZebMax.fr")
}

// A channel listed under a country prefix takes that country's guide,
// whatever the server language; a group prefix names no country.
func TestTheChannelsOwnCountryBeatsTheServerLanguage(t *testing.T) {
	guide := []guideChannel{{"ZebMax.fr", []string{"FR| Zeb Max"}, 10}, {"ZebMax.de", []string{"DE| Zeb Max"}, 10},
		{"ZebMax.uk", []string{"Zeb Max"}, 10}}
	expect(t, "own prefixes", match("fr", []string{"DE: Zeb Max", "[UK] Zeb Max", "KIDS| Zeb Max"}, guide),
		"ZebMax.de", "ZebMax.uk", "ZebMax.fr")
}

// Within a country, a name equal with its quality tags beats one equal
// without them, whatever their programmes.
func TestExactNamesBeatLooseOnes(t *testing.T) {
	guide := []guideChannel{{"ZebMaxHD.fr", []string{"FR| Zeb Max HD"}, 50}, {"ZebMax.fr", []string{"Zeb Max"}, 5},
		{"ZebMax4K.fr", []string{"Zeb Max 4K"}, 50}}
	expect(t, "exact", match("fr", []string{"Zeb Max", "Zeb Max 4K", "Zeb Max UHD"}, guide), "ZebMax.fr", "ZebMax4K.fr", "ZebMaxHD.fr")
}

// Among equal candidates, placeholders that repeat one title, or have no
// programme, lose to the guide with the most titles; then the first one
// declared wins.
func TestPlaceholderGuidesLose(t *testing.T) {
	guide := []guideChannel{{"empty.fr", []string{"FR| Zeb Max LQ"}, 0}, {"ZebMax4K.fr", []string{"FR| Zeb Max 4K"}, 1},
		{"ZebMaxFHD.fr", []string{"FR| Zeb Max FHD"}, 40}, {"ZebMaxSD.fr", []string{"FR| Zeb Max SD"}, 40}}
	expect(t, "placeholders", match("fr", []string{"Zeb Max ᴿᵂ"}, guide), "ZebMaxFHD.fr")
}

// Names escaped twice match once their entities are decoded.
func TestEscapedGuideNamesMatch(t *testing.T) {
	guide := []guideChannel{{"OrbeSeries.fr", []string{"Orbe S&eacute;ries"}, 10}, {"PifHercule.fr", []string{"PIF &amp; HERCULE"}, 10}}
	expect(t, "entities", match("fr", []string{"Orbe Séries", "Pif & Hercule"}, guide), "OrbeSeries.fr", "PifHercule.fr")
}

// A guide channel named by a channel's Stremio ID gives it its programmes,
// whatever the names, declared or not; several channels may share a
// guide channel.
func TestStremioIDsMatchFirst(t *testing.T) {
	guide := []guideChannel{{"ZebMax.fr", []string{"Zeb Max"}, 50}, {"zeb:max", []string{"Something Else"}, 2}}
	expect(t, "declared", match("fr", []string{"zeb:max=Zeb Max", "x=Zeb Max", "y=ZEB MAX ᴿᵂ"}, guide), "zeb:max", "ZebMax.fr", "ZebMax.fr")

	m := NewMatcher("fr")
	m.Add("zeb:max", "", "Zeb Max")
	m.Declare(Channel{ID: "ZebMax.fr", Names: []string{"Zeb Max"}})
	if !m.Wants("zeb:max") || m.Wants("ZebMax.fr") {
		t.Error("an undeclared guide channel named by a Stremio ID does not replace the candidates")
	}
	expect(t, "undeclared", m.Choose(func(string) int { return 1 }), "zeb:max")
}

// An IPTV channel's own guide identifier matches a guide channel exactly,
// before any name; channels sharing it share the guide channel, and those
// without one keep the name rules.
func TestGuideIdentifiersMatchBeforeNames(t *testing.T) {
	m := NewMatcher("fr")
	m.Add("polyfin-iptv:1", "Orbe.zz", "Zeb Max")
	m.Add("polyfin-iptv:2", "Orbe.zz", "Zeb Max ᴿᵂ")
	m.Add("polyfin-iptv:3", "", "Zeb Max")
	m.Add("polyfin-iptv:4", "Missing.zz", "Zeb Max")
	m.Declare(Channel{ID: "ZebMax.fr", Names: []string{"Zeb Max"}})
	m.Declare(Channel{ID: "Orbe.zz", Names: []string{"Orbe"}})
	expect(t, "guide identifiers", m.Choose(func(id string) int { return map[string]int{"ZebMax.fr": 50, "Orbe.zz": 1}[id] }),
		"Orbe.zz", "Orbe.zz", "ZebMax.fr", "ZebMax.fr")
}

// Lists write guide identifiers in another case, or with a feed after an
// "@": the exact identifier wins, then the same in another case, then the
// part before the "@", exactly and then in another case, all before names;
// an "@" identifier whose part before it is unknown falls back to names.
func TestGuideIdentifiersWithFeedsAndCase(t *testing.T) {
	m := NewMatcher("fr")
	m.Add("polyfin-iptv:1", "Zeb.zz@HD", "Zeb One")
	m.Add("polyfin-iptv:2", "ORBE.ZZ@SD", "Orbe")
	m.Add("polyfin-iptv:3", "quill.ZZ", "Quill")
	m.Add("polyfin-iptv:4", "Gone.zz@HD", "Lumo")
	m.Add("polyfin-iptv:5", "Pif.zz@HD", "Pif")
	m.Add("polyfin-iptv:6", "Tac.zz@SD", "Tac")
	for _, g := range []Channel{
		{ID: "ZebOther.zz", Names: []string{"Zeb One"}},
		{ID: "zeb.zz", Names: []string{"Elsewhere"}},
		{ID: "Zeb.zz", Names: []string{"Elsewhere"}},
		{ID: "orbe.zz"},
		{ID: "Quill.zz"},
		{ID: "Lumo.zz", Names: []string{"Lumo"}},
		{ID: "pif.zz"},
		{ID: "Pif.zz@hd"},
		{ID: "Tac.zz"},
		{ID: "Tac.zz@SD"},
	} {
		m.Declare(g)
	}
	expect(t, "feeds and case", m.Choose(func(string) int { return 1 }),
		"Zeb.zz", "orbe.zz", "Quill.zz", "Lumo.zz", "Pif.zz@hd", "Tac.zz@SD")
	// A programme before its channel is declared still finds the channel
	// by the part before the "@".
	late := NewMatcher("fr")
	late.Add("polyfin-iptv:1", "Zeb.zz@HD", "Zeb One")
	if !late.Wants("zeb.zz") || late.Wants("Other.zz") {
		t.Error("an undeclared guide channel by its identifier without the feed")
	}
	expect(t, "undeclared", late.Choose(func(string) int { return 1 }), "zeb.zz")
}

// Only the programmes of the best-ranked candidates are kept while the
// guide is read.
func TestOnlyTheBestCandidatesAreWanted(t *testing.T) {
	m := NewMatcher("fr")
	m.Add("a", "", "Zeb Max")
	m.Add("b", "", "Orbe")
	m.Declare(Channel{ID: "ZebMax.de", Names: []string{"Zeb Max"}})
	m.Declare(Channel{ID: "ZebMax.fr", Names: []string{"FR| Zeb Max HD"}})
	m.Declare(Channel{ID: "ZebMax.be", Names: []string{"Zeb Max"}})
	m.Declare(Channel{ID: "Orbe.fr", Names: []string{"Orbe"}})
	m.Declare(Channel{ID: "Other.fr", Names: []string{"Other"}})
	for id, want := range map[string]bool{"ZebMax.de": false, "ZebMax.fr": true, "ZebMax.be": false, "Orbe.fr": true, "Other.fr": false} {
		if got := m.Wants(id); got != want {
			t.Errorf("wants %s: %v", id, got)
		}
	}
}
