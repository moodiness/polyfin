package iptv

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"
)

// Import options (see Options).
const (
	CategoriesOriginal = "original"
	CategoriesCountry  = "country"
	ChannelsOriginal   = "original"
	ChannelsMerged     = "merged"
	NumberingProvider  = "provider"
	NumberingSequence  = "sequential"
	LibrariesByType    = "type"
	LibrariesByGroup   = "category"
)

// ErrInvalidOptions reports import options out of their values.
var ErrInvalidOptions = errors.New("invalid import options")

// Options are what a source imports and how. Its list's live channels,
// when LiveTv is set, become its line-up: provider groups or one category
// per country; one channel per entry, or the entries of the same name in a
// category merged into one channel; the group (g:<title>) and country
// (c:<code>) keys not imported; whether channels appearing on a refresh
// arrive enabled; and whether a channel without a fixed number takes the
// provider's or its place among the user's channels. Movies and Series
// import the provider's movies and series, but for the categories
// VODExcluded lists (movie:<name>, series:<name>), in libraries by type or
// by category (VODLibraries); Enrichment describes titles with a provider
// TMDB or IMDb id through the server's metadata addons.
type Options struct {
	Categories   string
	Channels     string
	Excluded     []string
	NewChannels  bool
	Numbering    string
	LiveTv       bool
	Movies       bool
	Series       bool
	VODExcluded  []string
	VODLibraries string
	Enrichment   bool
}

// DefaultOptions are today's: live channels only, in provider groups, one
// channel per entry, nothing excluded, new channels enabled, provider
// numbers; were movies or series turned on, libraries by type, described
// through metadata addons.
func DefaultOptions() Options {
	return Options{Categories: CategoriesOriginal, Channels: ChannelsOriginal, Excluded: []string{}, NewChannels: true, Numbering: NumberingProvider,
		LiveTv: true, VODExcluded: []string{}, VODLibraries: LibrariesByType, Enrichment: true}
}

// check validates options, removing duplicate excluded keys.
func (o *Options) check() error {
	if o.Categories != CategoriesOriginal && o.Categories != CategoriesCountry || o.Channels != ChannelsOriginal && o.Channels != ChannelsMerged ||
		o.Numbering != NumberingProvider && o.Numbering != NumberingSequence || o.VODLibraries != LibrariesByType && o.VODLibraries != LibrariesByGroup ||
		!o.LiveTv && !o.Movies && !o.Series {
		return ErrInvalidOptions
	}
	var err error
	if o.Excluded, err = checkKeys(o.Excluded, "g:", "c:"); err != nil {
		return err
	}
	o.VODExcluded, err = checkKeys(o.VODExcluded, typeMovie+":", typeSeries+":")
	return err
}

// checkKeys checks excluded keys, which start with one of prefixes,
// removing duplicates.
func checkKeys(keys []string, prefixes ...string) ([]string, error) {
	if len(keys) > 5000 {
		return nil, ErrInvalidOptions
	}
	kept := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		if len(key) > 300 || !slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(key, p) }) {
			return nil, ErrInvalidOptions
		}
		if !seen[key] {
			seen[key] = true
			kept = append(kept, key)
		}
	}
	return kept, nil
}

// storedEntry is an entry of a source's stored list.
type storedEntry struct {
	Key, Name, Logo, Group, GuideID string
	Number                          int
	Kind                            string
}

func groupKey(group string) string { return "g:" + group }

func countryKey(country string) string { return "c:" + country }

// countries finds the country of each entry: its group's, else its own
// name's, else otherCountry.
func countries(entries []storedEntry) []string {
	byGroup := map[string]string{}
	result := make([]string, len(entries))
	for i, e := range entries {
		country, ok := byGroup[e.Group]
		if !ok {
			country = detectCountry(e.Group)
			byGroup[e.Group] = country
		}
		result[i] = cmp.Or(country, detectCountry(e.Name), otherCountry)
	}
	return result
}

// derivedCategory, derivedChannel and derivedStream are a line-up as a
// list and options make it, before the administrator's edits.
type derivedCategory struct {
	key, name string
}

type derivedStream struct {
	entry string
	label string
	rank  int
}

type derivedChannel struct {
	key, category       string
	name, logo, guideID string
	number, position    int
	streams             []derivedStream
	// id is the permanent identifier the channel takes when it is new.
	id string
}

// derive makes a line-up of a list: categories in the provider's order
// (the order groups first appear) or by country name with OTHER last, and
// their channels in the list's order. Merged channels take the first
// entry's logo and number, the first guide identifier given, a cleaned
// name, and their streams best quality first.
func derive(entries []storedEntry, o Options) ([]derivedCategory, []derivedChannel) {
	excluded := map[string]bool{}
	for _, key := range o.Excluded {
		excluded[key] = true
	}
	country := countries(entries)
	var categories []derivedCategory
	seen := map[string]int{}
	category := make([]string, len(entries))
	for i, e := range entries {
		if excluded[groupKey(e.Group)] || excluded[countryKey(country[i])] {
			continue
		}
		key, name := groupKey(e.Group), e.Group
		if o.Categories == CategoriesCountry {
			key, name = countryKey(country[i]), country[i]
		}
		category[i] = key
		if _, ok := seen[key]; !ok {
			seen[key] = len(categories)
			categories = append(categories, derivedCategory{key: key, name: name})
		}
	}
	if o.Categories == CategoriesCountry {
		slices.SortStableFunc(categories, func(a, b derivedCategory) int {
			if (a.name == otherCountry) != (b.name == otherCountry) {
				if a.name == otherCountry {
					return 1
				}
				return -1
			}
			return strings.Compare(a.name, b.name)
		})
	}
	var channels []derivedChannel
	merged := map[string]int{}
	for i, e := range entries {
		if category[i] == "" {
			continue
		}
		label, rank := streamQuality(e.Name)
		if o.Channels == ChannelsOriginal {
			channels = append(channels, derivedChannel{key: "s:" + e.Key, id: e.Key, category: category[i], name: e.Name, logo: e.Logo,
				guideID: e.GuideID, number: e.Number, position: i + 1, streams: []derivedStream{{entry: e.Key, label: cmp.Or(label, "Live"), rank: rank}}})
			continue
		}
		name := mergeKey(e.Name)
		if name == "" {
			name = "\x00" + e.Key
		}
		sum := sha256.Sum256([]byte(category[i] + "\n" + name))
		digest := hex.EncodeToString(sum[:8])
		key := "m:" + digest
		index, ok := merged[key]
		if !ok {
			index = len(channels)
			merged[key] = index
			channels = append(channels, derivedChannel{key: key, id: "m" + digest, category: category[i], name: cleanName(e.Name),
				logo: e.Logo, number: e.Number, position: i + 1})
		}
		c := &channels[index]
		if c.guideID == "" {
			c.guideID = e.GuideID
		}
		if c.logo == "" {
			c.logo = e.Logo
		}
		c.streams = append(c.streams, derivedStream{entry: e.Key, label: label, rank: rank})
	}
	if o.Channels == ChannelsMerged {
		for i := range channels {
			streams := channels[i].streams
			slices.SortStableFunc(streams, func(a, b derivedStream) int { return a.rank - b.rank })
			for j := range streams {
				if streams[j].label == "" {
					streams[j].label = "Source " + strconv.Itoa(j+1)
				}
			}
		}
	}
	return categories, channels
}
