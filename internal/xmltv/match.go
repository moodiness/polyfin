package xmltv

import (
	"cmp"
	"slices"
	"strings"
)

// Matcher chooses, for each channel of a catalog, the guide channel that
// gives its programmes. Channels are added first, then the guide's channels
// declared as the guide is read (see Read); the programmes worth keeping
// are those of the guide channels it Wants, and Choose decides once the
// guide is read.
//
// A guide channel whose identifier is a channel's Stremio ID, or the
// channel's own guide identifier (an IPTV list's tvg-id or epg_channel_id),
// gives that channel its programmes. Lists often write that identifier in
// another case, or as "<channel>@<feed>" ("Name.fr@SD"): failing an exact
// match, guide channels are candidates whose identifier is the channel's
// ignoring case, then the part before its last "@", exactly and then
// ignoring case, each before the next, and before any name. Otherwise
// guide channels are candidates by name (see ParseName). Among candidates
// of the same kind, they are ranked by:
//   - country: one of the guide channel's countries (that of its
//     identifier, as in "Name.fr", and those of its display names'
//     prefixes, as in "FR| Name") is the channel's own prefix country, else
//     the country the server language suggests. It comes first: IPTV
//     guides name a channel plainly in one country and with quality tags
//     in another;
//   - tier: a display name equal to the channel's name once folded (Exact)
//     before one equal once quality tags are left out too (Loose), which
//     a placeholder such as "Name 4K" may share with "Name";
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
	// wanted counts, for each guide channel, the channels it is a
	// candidate of or matched by identifier; declared holds the guide
	// channels ranked already.
	wanted   map[string]int
	declared map[string]bool
}

type matchChannel struct {
	// country is the country whose guides the channel prefers, if any.
	country string
	// byID is the guide channel matched by the channel's Stremio ID or
	// guide identifier; otherwise candidates are the guide channels of the
	// best rank, in the order they were declared.
	byID       string
	rank       int
	candidates []string
}

// NewMatcher returns a matcher for a server in language (see
// LanguageCountry).
func NewMatcher(language string) *Matcher {
	return &Matcher{country: LanguageCountry(language), byID: map[string][]int{},
		exact: map[string][]int{}, loose: map[string][]int{}, folded: map[string][]int{}, base: map[string][]int{},
		foldedBase: map[string][]int{}, wanted: map[string]int{}, declared: map[string]bool{}}
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
	if parsed.Exact != "" {
		m.exact[parsed.Exact] = append(m.exact[parsed.Exact], i)
	}
	if parsed.Loose != "" {
		m.loose[parsed.Loose] = append(m.loose[parsed.Loose], i)
	}
}

// matchByID gives channel i the guide channel id, its Stremio ID or guide
// identifier, in place of its candidates.
func (m *Matcher) matchByID(i int, id string) {
	c := &m.channels[i]
	if c.byID != "" {
		return
	}
	for _, candidate := range c.candidates {
		m.wanted[candidate]--
	}
	c.byID, c.candidates = id, nil
	m.wanted[id]++
}

// Identifier kinds of candidates, above any name (see Declare): the guide
// identifier in another case, then before its "@", exactly and in another
// case.
const (
	byFoldedBase = 1
	byBase       = 2
	byFolded     = 3
)

// Declare ranks a guide channel against the channels whose guide
// identifiers or names it shares.
func (m *Matcher) Declare(channel Channel) {
	m.declared[channel.ID] = true
	for _, i := range m.byID[channel.ID] {
		m.matchByID(i, channel.ID)
	}
	countries := []string{IDCountry(channel.ID)}
	tiers := map[int]int{}
	for _, raw := range channel.Names {
		name := ParseName(raw)
		countries = append(countries, name.Country)
		for _, i := range m.loose[name.Loose] {
			tiers[i] = max(tiers[i], 1)
		}
		for _, i := range m.exact[name.Exact] {
			tiers[i] = 2
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
		if c.byID != "" {
			continue
		}
		// The identifier's kind first, then the country, then the tier.
		rank := kinds[i]*10 + tier
		if c.country != "" && slices.Contains(countries, c.country) {
			rank += 3
		}
		switch {
		case rank > c.rank:
			for _, candidate := range c.candidates {
				m.wanted[candidate]--
			}
			c.rank, c.candidates = rank, []string{channel.ID}
			m.wanted[channel.ID]++
		case rank == c.rank && !slices.Contains(c.candidates, channel.ID):
			c.candidates = append(c.candidates, channel.ID)
			m.wanted[channel.ID]++
		}
	}
}

// Wants reports whether the programmes of a guide channel may be chosen: it
// is a candidate of a channel, or a channel's Stremio ID or guide
// identifier, declared or not.
func (m *Matcher) Wants(id string) bool {
	if m.wanted[id] > 0 {
		return true
	}
	if channels, ok := m.byID[id]; ok {
		for _, i := range channels {
			m.matchByID(i, id)
		}
		return true
	}
	// A guide channel not declared yet may still match by identifier.
	if !m.declared[id] {
		m.Declare(Channel{ID: id})
	}
	return m.wanted[id] > 0
}

// Choose returns the guide channel of each channel, in the order they were
// added, "" for a channel without one. titles counts the distinct titles
// of a guide channel's programmes within the guide's window.
func (m *Matcher) Choose(titles func(id string) int) []string {
	counts := map[string]int{}
	count := func(id string) int {
		n, ok := counts[id]
		if !ok {
			n = titles(id)
			counts[id] = n
		}
		return n
	}
	chosen := make([]string, len(m.channels))
	for i, c := range m.channels {
		if c.byID != "" {
			chosen[i] = c.byID
			continue
		}
		best := -1
		for _, candidate := range c.candidates {
			if n := count(candidate); n > best {
				chosen[i], best = candidate, n
			}
		}
	}
	return chosen
}
