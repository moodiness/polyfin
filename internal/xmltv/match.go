package xmltv

import (
	"cmp"
	"slices"
	"strings"
)

// Matcher chooses, for each channel of a catalog, the guide channel that
// gives its programmes, among the channels of the catalog's guides.
// Channels are added first, then each guide's channels declared, guide by
// guide in the guides' order; Choose decides.
//
// A guide channel whose identifier is a channel's Stremio ID, or the
// channel's own guide identifier (an IPTV list's tvg-id or epg_channel_id),
// gives that channel its programmes, the first guide's first. Lists often
// write that identifier in another case, or as "<channel>@<feed>"
// ("Name.fr@SD"): failing an exact match, guide channels are candidates
// whose identifier is the channel's ignoring case, then the part before its
// last "@", exactly and then ignoring case, each before the next, and
// before any name. Otherwise guide channels are candidates by name (see
// ParseName). Among candidates of the same kind, they are ranked by:
//   - country: one of the guide channel's countries (that of its
//     identifier, as in "Name.fr", and those of its display names'
//     prefixes, as in "FR| Name") is the channel's own prefix country, else
//     the country the server language suggests. It comes first: IPTV
//     guides name a channel plainly in one country and with quality tags
//     in another;
//   - tier: a display name equal to the channel's name once folded (Exact)
//     before one equal once quality tags are left out too (Loose), which
//     a placeholder such as "Name 4K" may share with "Name", before an
//     identifier that writes the channel's name ("ZebPlus1.fr" for
//     "Zeb+ 1", "ZebAndCo.fr" for "Zeb & Co", see IDName), which some
//     guides keep for a channel renamed since. A French public network written short, "F2" to "F5" before a
//     region, counts as written in full (see FrenchName);
//   - then the guide's order;
//   - then, among the candidates left, the one with the most distinct
//     programme titles in the guide's window, so that placeholders, which
//     repeat one title, lose to real guides;
//   - then the one declared first.
//
// Each channel takes one guide channel; several channels may share one.
type Matcher struct {
	country  string
	channels []matchChannel
	// byID, exact and loose index the channels by Stremio ID and guide
	// identifier, and by names; folded, base and foldedBase by their guide
	// identifier in lower case, before its last "@", and both.
	byID                     map[string][]int
	exact, loose             map[string][]int
	folded, base, foldedBase map[string][]int
}

// Match is a guide channel: the position of its guide among those
// declared (from 0) and its identifier.
type Match struct {
	Guide int
	ID    string
}

type matchChannel struct {
	// country is the country whose guides the channel prefers, if any.
	country string
	// byID is the guide channel matched by the channel's Stremio ID or
	// guide identifier; otherwise candidates are the guide channels of the
	// best rank, in the order they were declared.
	byID       *Match
	rank       int
	candidates []Match
}

// NewMatcher returns a matcher for a server in language (see
// LanguageCountry).
func NewMatcher(language string) *Matcher {
	return &Matcher{country: LanguageCountry(language), byID: map[string][]int{},
		exact: map[string][]int{}, loose: map[string][]int{}, folded: map[string][]int{}, base: map[string][]int{},
		foldedBase: map[string][]int{}}
}

// index adds channel i to the channels of key in index, once.
func index(m map[string][]int, key string, i int) {
	if key != "" && !slices.Contains(m[key], i) {
		m[key] = append(m[key], i)
	}
}

// Add adds the next channel of the catalog, with its guide identifier if
// it has one; Choose answers in the order channels were added. Several
// channels may share a guide identifier.
func (m *Matcher) Add(stremioID, guideID, name string) {
	i := len(m.channels)
	parsed := ParseName(name)
	m.channels = append(m.channels, matchChannel{country: cmp.Or(parsed.Country, m.country)})
	index(m.byID, stremioID, i)
	index(m.byID, guideID, i)
	if guideID != "" {
		index(m.folded, strings.ToLower(guideID), i)
		if at := strings.LastIndexByte(guideID, '@'); at > 0 {
			index(m.base, guideID[:at], i)
			index(m.foldedBase, strings.ToLower(guideID[:at]), i)
		}
	}
	names := []Name{parsed}
	if full := FrenchName(name); full != "" && m.channels[i].country == "fr" {
		names = append(names, ParseName(full))
	}
	// Identifiers write "&" as "And" ("ZebAndCo.fr" for "Zeb & Co").
	if strings.Contains(name, "&") {
		names = append(names, ParseName(strings.ReplaceAll(name, "&", " and ")))
	}
	for _, n := range names {
		index(m.exact, n.Exact, i)
		index(m.loose, n.Loose, i)
	}
}

// Identifier kinds of candidates, above any name (see Declare): the guide
// identifier in another case, then before its "@", exactly and in another
// case.
const (
	byFoldedBase = 1
	byBase       = 2
	byFolded     = 3
)

// Name tiers of candidates (see Declare), and what the country adds,
// above any tier.
const (
	byIDName     = 1
	byLooseName  = 2
	byExactName  = 3
	countryBonus = 4
)

// Declare ranks a channel of the guide at position guide against the
// channels whose guide identifiers or names it shares. Guides are declared
// in their order, and a guide's channels in its order.
func (m *Matcher) Declare(guide int, channel Channel) {
	match := Match{Guide: guide, ID: channel.ID}
	for _, i := range m.byID[channel.ID] {
		if c := &m.channels[i]; c.byID == nil {
			c.byID, c.candidates = &match, nil
		}
	}
	countries := []string{IDCountry(channel.ID)}
	tiers := map[int]int{}
	for _, i := range m.exact[IDName(channel.ID)] {
		tiers[i] = byIDName
	}
	for _, raw := range channel.Names {
		name := ParseName(raw)
		countries = append(countries, name.Country)
		for _, i := range m.loose[name.Loose] {
			tiers[i] = max(tiers[i], byLooseName)
		}
		for _, i := range m.exact[name.Exact] {
			tiers[i] = byExactName
		}
	}
	// An identifier's kind outranks every name; the country and the name's
	// tier rank candidates of the same kind.
	kinds := map[int]int{}
	lower := strings.ToLower(channel.ID)
	for kind, channels := range map[int][]int{byFoldedBase: m.foldedBase[lower], byBase: m.base[channel.ID], byFolded: m.folded[lower]} {
		for _, i := range channels {
			kinds[i] = max(kinds[i], kind)
		}
	}
	for i := range kinds {
		if _, ok := tiers[i]; !ok {
			tiers[i] = 0
		}
	}
	for i, tier := range tiers {
		c := &m.channels[i]
		if c.byID != nil {
			continue
		}
		// The identifier's kind first, then the country, then the tier.
		rank := kinds[i]*10 + tier
		if c.country != "" && slices.Contains(countries, c.country) {
			rank += countryBonus
		}
		switch {
		case rank > c.rank:
			c.rank, c.candidates = rank, []Match{match}
		case rank == c.rank && !slices.Contains(c.candidates, match):
			c.candidates = append(c.candidates, match)
		}
	}
}

// Choose returns the guide channel of each channel, in the order they were
// added, nil for a channel without one. titles counts the distinct titles
// of a guide channel's programmes within the guide's window.
func (m *Matcher) Choose(titles func(Match) int) []*Match {
	counts := map[Match]int{}
	count := func(match Match) int {
		n, ok := counts[match]
		if !ok {
			n = titles(match)
			counts[match] = n
		}
		return n
	}
	chosen := make([]*Match, len(m.channels))
	for i, c := range m.channels {
		if c.byID != nil {
			chosen[i] = c.byID
			continue
		}
		for j := range c.candidates {
			candidate := c.candidates[j]
			best := chosen[i]
			if best == nil || candidate.Guide < best.Guide || candidate.Guide == best.Guide && count(candidate) > count(*best) {
				chosen[i] = &c.candidates[j]
			}
		}
	}
	return chosen
}
