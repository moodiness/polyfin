package jellyfin

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// The version rules (Settings › Playback › Version order) hide some of a
// title's versions and order the others, before the user's quality group
// leaves out those taller than it. They read what Polyfin knows of each
// version without reading it: its analysis when there is one, else
// RemuxDB's description of its file, else its addon's labels (see
// library.ReadLabels). A version a rule knows nothing of comes after those
// it knows, and is never hidden by it. Merged channels keep their own
// order: only titles' versions follow the rules.

// versionRules are the settings' rules, with the audio language the user
// prefers, which the language criterion reads.
type versionRules struct {
	order []string
	// codecs is the order of the codec criterion, sources that of the
	// source criterion.
	codecs  []string
	sources []accounts.ID
	// larger hides the versions larger than so many bytes, 0 none.
	larger       int64
	hiddenCodecs []string
	// words are the words hiding versions, in lower case.
	words    []string
	language string
	// releases are the kinds of low-quality releases hidden (see
	// library.Labels.Releases).
	releases []string
}

// newVersionRules returns the rules settings set, without the user's
// preferred audio language.
func newVersionRules(settings accounts.Settings) versionRules {
	rules := versionRules{order: settings.VersionOrder, codecs: settings.VersionCodecOrder, sources: settings.VersionSourceOrder,
		larger: int64(settings.HideVersionsLargerGB) * 1e9, hiddenCodecs: settings.HiddenVersionCodecs, releases: settings.HiddenReleases}
	for _, word := range settings.HiddenVersionWords {
		rules.words = append(rules.words, strings.ToLower(word))
	}
	return rules
}

// rulesFor returns the rules settings set for user: with their preferred
// audio language when a criterion reads it.
func (h *Handler) rulesFor(ctx context.Context, user accounts.User) versionRules {
	rules := newVersionRules(h.Accounts.Settings())
	if slices.Contains(rules.order, "language") {
		rules.language = h.trackPreferences(ctx, user).audioLanguage
	}
	return rules
}

// key names what orders versions under the rules: the criteria, the
// order of the codecs, that of the sources and the language the user
// prefers. A page keeps the versions it lists in place only while it
// stays the same.
func (r versionRules) key() string {
	sources := make([]string, len(r.sources))
	for i, source := range r.sources {
		sources[i] = source.String()
	}
	return strings.Join(r.order, ",") + "|" + strings.Join(r.codecs, ",") + "|" + strings.Join(sources, ",") + "|" + r.language
}

// hides reports whether a rule may hide versions.
func (r versionRules) hides() bool {
	return r.larger > 0 || len(r.hiddenCodecs) > 0 || len(r.words) > 0 || len(r.releases) > 0
}

// elsewhere reports whether a version's labels name another title than
// its own: another episode, a season without it, or another movie's year
// (see library.OtherTitle). Such a version is always hidden, as a rule
// hides one, without a setting.
func elsewhere(version library.Version) bool {
	return version.Labels.Elsewhere != ""
}

// reordered reports whether the versions a user is offered may be ordered
// otherwise than by the addons, or some hidden: with the user's quality
// group or the version rules (see offered).
func (h *Handler) reordered(user accounts.User) bool {
	rules := newVersionRules(h.Accounts.Settings())
	return user.QualityGroup > 0 || len(rules.order) > 0 || rules.hides()
}

// hiddenVersion reports whether the hide rules hide a version among its
// title's versions, for a user: a version an app names by its own
// identifier, which it may have been shown before a rule hid it.
func (h *Handler) hiddenVersion(ctx context.Context, user accounts.User, version library.Version) bool {
	rules := newVersionRules(h.Accounts.Settings())
	if !rules.hides() && !elsewhere(version) || !rules.hidden(version, h.versionFacts(ctx, version)) {
		return false
	}
	versions, err := h.Library.Versions(ctx, user, version.Item)
	if err != nil {
		return false
	}
	// When the rules would hide every version, none is hidden.
	return slices.ContainsFunc(versions, func(other library.Version) bool {
		return !h.Playback.Failed(other.ID) && !rules.hidden(other, h.versionFacts(ctx, other))
	})
}

// versionFacts are what the rules know of a version: its height, size and
// video codec, 0 or empty when unknown, its range, its audio languages,
// by ISO 639-2/B code, and multi when its labels say it has several
// without naming them all; and source, the rank of its source for the
// source criterion (see sourceRanks).
type versionFacts struct {
	height    int
	size      int64
	codec     string
	hdr       library.DynamicRange
	languages []string
	multi     bool
	source    int
}

// versionFacts finds what is known of a version without reading it: each
// fact from its analysis when the analysis tells it, else from RemuxDB's
// description of its file, else from its addon's labels, as its height,
// its size given by the addon (videoSize), and the words of its labels
// (see library.ReadLabels).
func (h *Handler) versionFacts(ctx context.Context, version library.Version) versionFacts {
	var known []media.Analysis
	if analysis, ok := h.Playback.Analyzed(ctx, version.ID); ok {
		known = append(known, analysis)
	}
	if described, ok := h.RemuxDB.Described(version); ok {
		known = append(known, described)
	}
	var facts versionFacts
	ranged, spoken := false, false
	for _, analysis := range known {
		facts.height = cmp.Or(facts.height, videoHeight(analysis))
		if facts.size == 0 {
			facts.size = analysis.Size
		}
		if i := slices.IndexFunc(analysis.Streams, func(s media.Stream) bool { return s.Type == "video" && !s.AttachedPicture }); i >= 0 {
			video := analysis.Streams[i]
			if facts.codec == "" {
				facts.codec = video.Codec
			}
			if !ranged {
				facts.hdr, ranged = library.StandardRange, true
				if playback.HighRange(video) {
					facts.hdr = library.HighRange
				}
			}
		}
		if languages := audioLanguages(analysis); !spoken && len(languages) > 0 {
			facts.languages, spoken = languages, true
		}
	}
	labels := version.Labels
	facts.height = cmp.Or(facts.height, version.Height)
	facts.size = cmp.Or(facts.size, version.Size)
	facts.codec = cmp.Or(facts.codec, labels.Codec)
	if !ranged {
		facts.hdr = labels.Range
	}
	if !spoken {
		facts.languages, facts.multi = labels.Languages, labels.Multi
	}
	return facts
}

// audioLanguages are the languages of an analysis's audio tracks, by ISO
// 639-2/B code, those tagged with none left out.
func audioLanguages(analysis media.Analysis) []string {
	var languages []string
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" || undeterminedLanguage(stream.Language) {
			continue
		}
		if code := localization.LanguageCode(stream.Language); !slices.Contains(languages, code) {
			languages = append(languages, code)
		}
	}
	return languages
}

// hidden reports whether a rule hides a version: one naming another title
// (see elsewhere), or one known to be larger than the limit, or in a codec
// hidden, or whose labels, its name and description and its file name,
// hold a word hiding it, or tag it as a kind of low-quality release
// hidden.
func (r versionRules) hidden(version library.Version, facts versionFacts) bool {
	if elsewhere(version) {
		return true
	}
	if r.larger > 0 && facts.size > r.larger || facts.codec != "" && slices.Contains(r.hiddenCodecs, facts.codec) {
		return true
	}
	if slices.ContainsFunc(version.Labels.Releases, func(kind string) bool { return slices.Contains(r.releases, kind) }) {
		return true
	}
	if len(r.words) == 0 {
		return false
	}
	labels := strings.ToLower(version.Name + "\n" + version.Filename)
	return slices.ContainsFunc(r.words, func(word string) bool { return strings.Contains(labels, word) })
}

// compare orders two versions by the criteria, each grouping them and the
// next sorting within them; 0 when they tie on all of them.
func (r versionRules) compare(a, b versionFacts) int {
	for _, criterion := range r.order {
		if c := r.criterion(criterion, a, b); c != 0 {
			return c
		}
	}
	return 0
}

// criterion orders two versions by one criterion, those it knows nothing
// of last.
func (r versionRules) criterion(criterion string, a, b versionFacts) int {
	switch criterion {
	case "language":
		return cmp.Compare(r.languageRank(a), r.languageRank(b))
	case "resolution-high":
		return knownFirst(a.height, b.height, -1)
	case "resolution-low":
		return knownFirst(a.height, b.height, 1)
	case "size-large":
		return knownFirst(a.size, b.size, -1)
	case "size-small":
		return knownFirst(a.size, b.size, 1)
	case "codec":
		return cmp.Compare(r.codecRank(a.codec), r.codecRank(b.codec))
	case "hdr-first":
		return cmp.Compare(rangeRank(a.hdr, library.HighRange), rangeRank(b.hdr, library.HighRange))
	case "hdr-last":
		return cmp.Compare(rangeRank(a.hdr, library.StandardRange), rangeRank(b.hdr, library.StandardRange))
	case "source":
		return cmp.Compare(a.source, b.source)
	}
	return 0
}

// knownFirst compares two values, 0 being unknown, the known first, in
// the direction of sign: 1 for the lowest first, -1 for the highest.
func knownFirst[T int | int64](a, b T, sign int) int {
	switch {
	case a == 0 && b == 0:
		return 0
	case a == 0:
		return 1
	case b == 0:
		return -1
	}
	return sign * cmp.Compare(a, b)
}

// languageRank ranks a version for the user's preferred audio language:
// the versions known to carry it first, then those whose labels say they
// have several languages without naming it, then those of unknown
// language, then the others. Without a preference, all rank the same.
func (r versionRules) languageRank(facts versionFacts) int {
	switch {
	case r.language == "" || slices.ContainsFunc(facts.languages, func(code string) bool { return localization.SameLanguage(code, r.language) }):
		return 0
	case facts.multi:
		return 1
	case len(facts.languages) == 0:
		return 2
	}
	return 3
}

// codecRank ranks a video codec in the order of the codec criterion: the
// codecs it does not list after those it lists, unknown last.
func (r versionRules) codecRank(codec string) int {
	if codec == "" {
		return len(r.codecs) + 1
	}
	if i := slices.Index(r.codecs, codec); i >= 0 {
		return i
	}
	return len(r.codecs)
}

// sourceRanks ranks the sources of a title's versions, listed in the
// addons' order, for the source criterion: the sources the rules list
// first, in their order, then the others in the order their versions come
// in, by the first place of their versions. A source installed since the
// list was saved, and a user's own addon, which come after the server's,
// thus come after the sources listed.
func (r versionRules) sourceRanks(versions []library.Version) map[accounts.ID]int {
	ranks := make(map[accounts.ID]int, len(r.sources))
	for i, source := range r.sources {
		ranks[source] = i
	}
	next := len(r.sources)
	for _, version := range versions {
		if _, ok := ranks[version.Origin.Addon]; !ok {
			ranks[version.Origin.Addon] = next
			next++
		}
	}
	return ranks
}

// rangeRank ranks a range: first first, then the other known, unknown
// last.
func rangeRank(hdr, first library.DynamicRange) int {
	switch hdr {
	case first:
		return 0
	case library.UnknownRange:
		return 2
	}
	return 1
}

// ordered applies the rules and the user's quality group to a title's
// versions, those that failed lately already left out: the hide rules
// first, keeping every version when they would hide them all; then the
// quality group, leaving out the versions taller than it while one fits
// or has an unknown height, else keeping them all, the closest to the
// group first; then the criteria, ties keeping the addons' order. With
// criteria, a page of the title open keeps the versions it shows in place
// (see keptInPlace).
func (h *Handler) ordered(ctx context.Context, user accounts.User, versions []library.Version) []library.Version {
	rules := h.rulesFor(ctx, user)
	var sources map[accounts.ID]int
	if slices.Contains(rules.order, "source") {
		sources = rules.sourceRanks(versions)
	}
	facts := make(map[accounts.ID]versionFacts, len(versions))
	for _, version := range versions {
		known := h.versionFacts(ctx, version)
		known.source = sources[version.Origin.Addon]
		facts[version.ID] = known
	}
	kept := versions
	if rules.hides() || slices.ContainsFunc(versions, elsewhere) {
		shown := slices.DeleteFunc(slices.Clone(versions), func(v library.Version) bool { return rules.hidden(v, facts[v.ID]) })
		if len(shown) > 0 {
			kept = shown
		}
	}
	closest := false
	if user.QualityGroup > 0 {
		fitting := slices.DeleteFunc(slices.Clone(kept), func(v library.Version) bool { return !user.FitsGroup(facts[v.ID].height) })
		if len(fitting) > 0 {
			kept = fitting
		} else {
			// Every height is known: an unknown one fits.
			closest = true
		}
	}
	place := make(map[accounts.ID]int, len(kept))
	for i, version := range kept {
		place[version.ID] = i
	}
	compare := func(a, b library.Version) int {
		fa, fb := facts[a.ID], facts[b.ID]
		if closest {
			if c := cmp.Compare(fa.height, fb.height); c != 0 {
				return c
			}
		}
		if c := rules.compare(fa, fb); c != 0 {
			return c
		}
		return cmp.Compare(place[a.ID], place[b.ID])
	}
	slices.SortFunc(kept, compare)
	if len(rules.order) > 0 && len(kept) > 0 {
		if shown := h.watched.shown(user.ID, kept[0].Item, rules.key()); len(shown) > 0 {
			kept = keptInPlace(kept, shown, compare)
		}
	}
	return kept
}

// keptInPlace keeps the versions of shown, those an open page lists, in
// the order the page lists them, among sorted, a title's versions in the
// rules' order: the others take their places among them by compare, as
// they would among versions in the rules' order. The rules order versions
// by what is known of them, which grows as they are analyzed: a version
// whose analysis tells more than its labels would otherwise move on the
// page.
func keptInPlace(sorted []library.Version, shown []accounts.ID, compare func(a, b library.Version) int) []library.Version {
	at := make(map[accounts.ID]int, len(shown))
	for i, id := range shown {
		at[id] = i
	}
	var listed, coming []library.Version
	for _, version := range sorted {
		if _, ok := at[version.ID]; ok {
			listed = append(listed, version)
		} else {
			coming = append(coming, version)
		}
	}
	slices.SortFunc(listed, func(a, b library.Version) int { return cmp.Compare(at[a.ID], at[b.ID]) })
	merged := make([]library.Version, 0, len(sorted))
	for len(listed) > 0 && len(coming) > 0 {
		if compare(coming[0], listed[0]) < 0 {
			merged, coming = append(merged, coming[0]), coming[1:]
		} else {
			merged, listed = append(merged, listed[0]), listed[1:]
		}
	}
	merged = append(merged, listed...)
	return append(merged, coming...)
}
