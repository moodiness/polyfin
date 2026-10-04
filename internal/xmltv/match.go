package xmltv

import (
	"cmp"
	"slices"
)

// Matcher chooses, for each channel of a catalog, the guide channel that
// gives its programmes. Channels are added first, then the guide's channels
// declared as the guide is read (see Read); the programmes worth keeping
// are those of the guide channels it Wants, and Choose decides once the
// guide is read.
//
// A guide channel whose identifier is a channel's Stremio ID, or the
// channel's own guide identifier (an IPTV list's tvg-id or epg_channel_id),
// gives that channel its programmes. Otherwise guide channels are
// candidates by name (see ParseName), ranked by:
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
	// identifier, and by names.
	byID         map[string][]int
	exact, loose map[string][]int
	// wanted counts, for each guide channel, the channels it is a
	// candidate of or matched by identifier.
	wanted map[string]int
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
		exact: map[string][]int{}, loose: map[string][]int{}, wanted: map[string]int{}}
}

// Add adds the next channel of the catalog, with its guide identifier if
// it has one; Choose answers in the order channels were added. Several
// channels may share a guide identifier.
func (m *Matcher) Add(stremioID, guideID, name string) {
	i := len(m.channels)
	parsed := ParseName(name)
	m.channels = append(m.channels, matchChannel{country: cmp.Or(parsed.Country, m.country)})
	for _, id := range []string{stremioID, guideID} {
		if id != "" && !slices.Contains(m.byID[id], i) {
			m.byID[id] = append(m.byID[id], i)
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

// Declare ranks a guide channel against the channels whose names it
// shares.
func (m *Matcher) Declare(channel Channel) {
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
	for i, tier := range tiers {
		c := &m.channels[i]
		if c.byID != "" {
			continue
		}
		// The country first, then the tier.
		rank := tier
		if c.country != "" && slices.Contains(countries, c.country) {
			rank += 2
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
	return false
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
