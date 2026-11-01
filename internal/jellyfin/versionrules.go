package jellyfin

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

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

// versionRules are the settings' rules, with the audio language and the
// region of it the user prefers, which the language criterion reads.
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
	// region is the regional variant of language preferred, empty for
	// none (see trackPreferences.audioRegion).
	region string
	// releases are the kinds of low-quality releases hidden (see
	// library.Labels.Releases).
	releases []string
	// editions is the order of the edition criterion; perMinute, the size
	// in bytes a minute the size-per-minute criterion aims at, 0 for the
	// largest.
	editions  []string
	perMinute int64
	// labels are the label rules, in the settings' order.
	labels []labelRule
}

// labelRule is a label rule with its word in lower case, or its regular
// expression compiled.
type labelRule struct {
	accounts.VersionLabelRule
	word    string
	pattern *regexp.Regexp
}

// labelPatterns are the label rules' regular expressions compiled, by
// their text: the rules are read for every list of versions, and change
// rarely.
var labelPatterns sync.Map

// maxLabelBytes bounds the labels a label rule reads, as
// accounts.CompileLabelRule bounds its expression: a version's name and
// file name, cut beyond.
const maxLabelBytes = 4096

// newVersionRules returns the rules settings set, without the user's
// preferred audio language. Scoring label rules order the versions by
// their score after the criteria when the labels criterion is not among
// them.
func newVersionRules(settings accounts.Settings) versionRules {
	rules := versionRules{order: settings.VersionOrder, codecs: settings.VersionCodecOrder, sources: settings.VersionSourceOrder,
		larger: int64(settings.HideVersionsLargerGB) * 1e9, hiddenCodecs: settings.HiddenVersionCodecs, releases: settings.HiddenReleases,
		editions: settings.VersionEditionOrder, perMinute: int64(settings.VersionMBPerMinute) * 1e6}
	for _, word := range settings.HiddenVersionWords {
		rules.words = append(rules.words, strings.ToLower(word))
	}
	scored := false
	for _, setting := range settings.VersionLabelRules {
		rule := labelRule{VersionLabelRule: setting, word: strings.ToLower(setting.Text)}
		if setting.Regex {
			compiled, ok := labelPatterns.Load(setting.Text)
			if !ok {
				pattern, err := accounts.CompileLabelRule(setting.Text)
				if err != nil {
					continue
				}
				compiled, _ = labelPatterns.LoadOrStore(setting.Text, pattern)
			}
			rule.pattern = compiled.(*regexp.Regexp)
		}
		rules.labels = append(rules.labels, rule)
		scored = scored || !setting.Hide
	}
	if scored && !slices.Contains(rules.order, "labels") {
		rules.order = append(slices.Clone(rules.order), "labels")
	}
	return rules
}

// rulesFor returns the rules settings set for user: with their preferred
// audio language and region when a criterion reads them.
func (h *Handler) rulesFor(ctx context.Context, user accounts.User) versionRules {
	rules := newVersionRules(h.Accounts.Settings())
	if slices.Contains(rules.order, "language") {
		preferences := h.trackPreferences(ctx, user)
		rules.language, rules.region = preferences.audioLanguage, preferences.audioRegion
	}
	return rules
}

// key names what orders versions under the rules: the criteria, the
// order of the codecs, that of the sources, the language and region the
// user prefers, the order of the editions, the size a minute aimed at and
// the label rules.
func (r versionRules) key() string {
	sources := make([]string, len(r.sources))
	for i, source := range r.sources {
		sources[i] = source.String()
	}
	labels := make([]string, len(r.labels))
	for i, rule := range r.labels {
		labels[i] = fmt.Sprintf("%t:%d:%t:%s", rule.Regex, rule.Score, rule.Hide, rule.Text)
	}
	return strings.Join(r.order, ",") + "|" + strings.Join(r.codecs, ",") + "|" + strings.Join(sources, ",") + "|" + r.language +
		"|" + r.region + "|" + strings.Join(r.editions, ",") + "|" + strconv.FormatInt(r.perMinute, 10) + "|" + strings.Join(labels, "\n")
}

// orderKey names what orders a user's versions of a title (see ordered):
// the rules (see versionRules.key), the user's quality group, Play local
// files first, and the app the order is made for (see appOrder). A page
// keeps the versions it lists in place only while it stays the same.
func orderKey(rules versionRules, user accounts.User, app *appOrder) string {
	return rules.key() + "|" + strconv.Itoa(user.QualityGroup) + "|" + strconv.FormatBool(user.LocalFirst) + "|" + app.key()
}

// pageKey is the orderKey of the versions a user's app is offered.
func (h *Handler) pageKey(ctx context.Context, user accounts.User) string {
	return orderKey(h.rulesFor(ctx, user), user, h.appOrderFor(ctx, user))
}

// hides reports whether a rule may hide versions.
func (r versionRules) hides() bool {
	return r.larger > 0 || len(r.hiddenCodecs) > 0 || len(r.words) > 0 || len(r.releases) > 0 ||
		slices.ContainsFunc(r.labels, func(rule labelRule) bool { return rule.Hide })
}

// labelText is what the label rules read of a version: its labels, its
// name and description and its file name, cut at maxLabelBytes.
func labelText(version library.Version) string {
	text := version.Name + "\n" + version.Filename
	if len(text) > maxLabelBytes {
		text = strings.ToValidUTF8(text[:maxLabelBytes], "")
	}
	return text
}

// matches reports whether a label rule matches a version's labels (see
// labelText), lower being them in lower case.
func (rule labelRule) matches(text, lower string) bool {
	if rule.pattern != nil {
		return rule.pattern.MatchString(text)
	}
	return !rule.Regex && strings.Contains(lower, rule.word)
}

// labelScore is a version's score under the label rules, the sum of the
// scores of those matching it, with the text of every rule matching it,
// scoring or hiding, in the settings' order.
func (r versionRules) labelScore(version library.Version) (score int, matched []string) {
	if len(r.labels) == 0 {
		return 0, nil
	}
	text := labelText(version)
	lower := strings.ToLower(text)
	for _, rule := range r.labels {
		if rule.matches(text, lower) {
			score += rule.Score
			matched = append(matched, rule.Text)
		}
	}
	return score, matched
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
// group, Play local files first, the version rules, or an order for the
// app asking (see appOrder, and offered).
func (h *Handler) reordered(ctx context.Context, user accounts.User) bool {
	rules := newVersionRules(h.Accounts.Settings())
	return user.QualityGroup > 0 || user.LocalFirst || len(rules.order) > 0 || rules.hides() || h.appOrderFor(ctx, user) != nil
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
// without naming them all; variants, its audio languages with their
// regions, each a variant of localization.AudioVariants where its region
// is known, else its language's code; and source, the rank of its source
// for the source criterion (see sourceRanks). tier is its source tier, of
// library.Tiers, audio its best audio format, edition its edition, of
// accounts.VersionEditions, empty, zero or empty when unknown, and
// perMinute its size in bytes a minute of its runtime, 0 when either is
// unknown; score is its score under the label rules (see labelScore).
type versionFacts struct {
	height    int
	size      int64
	codec     string
	hdr       library.DynamicRange
	languages []string
	multi     bool
	variants  []string
	source    int
	tier      string
	audio     library.AudioFormat
	edition   string
	perMinute int64
	score     int
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
	ranged, spoken, heard := false, false, false
	var runtime time.Duration
	for _, analysis := range known {
		if runtime == 0 {
			runtime = analysis.Duration
		}
		if audio, ok := analysisAudio(analysis); ok && !heard {
			facts.audio, heard = audio, true
		}
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
		if languages, variants := audioLanguages(analysis); !spoken && len(languages) > 0 {
			facts.languages, facts.variants, spoken = languages, variants, true
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
		facts.variants = slices.Clone(labels.Regions)
		for _, code := range labels.Languages {
			if !slices.ContainsFunc(labels.Regions, func(variant string) bool { return localization.VariantLanguage(variant) == code }) {
				facts.variants = append(facts.variants, code)
			}
		}
	}
	facts.tier, facts.edition = labels.Tier, labels.Edition
	if !heard {
		facts.audio = labels.Audio
	}
	runtime = cmp.Or(runtime, version.Runtime)
	if facts.size > 0 && runtime >= time.Minute {
		facts.perMinute = int64(float64(facts.size) / runtime.Minutes())
	}
	return facts
}

// audioLanguages are the languages of an analysis's audio tracks, by ISO
// 639-2/B code, those tagged with none left out, and their variants (see
// versionFacts): each track's of localization.AudioVariants, read from its
// tag and its title, else its language's code.
func audioLanguages(analysis media.Analysis) (languages, variants []string) {
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" || undeterminedLanguage(stream.Language) {
			continue
		}
		code := localization.LanguageCode(stream.Language)
		if !slices.Contains(languages, code) {
			languages = append(languages, code)
		}
		if variant := cmp.Or(localization.AudioVariant(stream.Language, stream.Title), code); !slices.Contains(variants, variant) {
			variants = append(variants, variant)
		}
	}
	return languages, variants
}

// analysisAudio is the best audio format of an analysis's audio tracks
// (see library.AudioFormat.Better), false when it has none: lossless for
// Dolby TrueHD, FLAC, ALAC, PCM and DTS-HD Master Audio, object based for
// Dolby Atmos and DTS:X, which ffprobe names in the track's profile.
func analysisAudio(analysis media.Analysis) (library.AudioFormat, bool) {
	var best library.AudioFormat
	found := false
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" {
			continue
		}
		profile := strings.ToLower(stream.Profile)
		format := library.AudioFormat{
			Lossless: slices.Contains([]string{"truehd", "mlp", "flac", "alac"}, stream.Codec) || strings.HasPrefix(stream.Codec, "pcm_") ||
				stream.Codec == "dts" && strings.Contains(profile, "dts-hd ma"),
			Object:   strings.Contains(profile, "atmos") || strings.Contains(profile, "dts:x"),
			Channels: stream.Channels,
		}
		if !found || format.Better(best) {
			best, found = format, true
		}
	}
	return best, found
}

// hidden reports whether a rule hides a version (see hiddenBy).
func (r versionRules) hidden(version library.Version, facts versionFacts) bool {
	rule, _ := r.hiddenBy(version, facts)
	return rule != ""
}

// hiddenBy tells the rule hiding a version, and what of the version it
// reads, empty when none hides it: "elsewhere", the marker naming another
// title (see elsewhere); "larger", the limit in GB, for a version known
// to be larger; "codec", its codec, when hidden; "release", the kind of
// low-quality release its labels tag it as, when hidden; "words", a word
// hiding it, in lower case, that its labels, its name and description and
// its file name, hold.
func (r versionRules) hiddenBy(version library.Version, facts versionFacts) (rule, detail string) {
	if elsewhere(version) {
		return "elsewhere", version.Labels.Elsewhere
	}
	if r.larger > 0 && facts.size > r.larger {
		return "larger", strconv.FormatInt(r.larger/1e9, 10)
	}
	if facts.codec != "" && slices.Contains(r.hiddenCodecs, facts.codec) {
		return "codec", facts.codec
	}
	if i := slices.IndexFunc(version.Labels.Releases, func(kind string) bool { return slices.Contains(r.releases, kind) }); i >= 0 {
		return "release", version.Labels.Releases[i]
	}
	labels := strings.ToLower(version.Name + "\n" + version.Filename)
	if i := slices.IndexFunc(r.words, func(word string) bool { return strings.Contains(labels, word) }); i >= 0 {
		return "words", r.words[i]
	}
	// "label", the text of the first label rule hiding it that matches its
	// labels: the label rules come after the others.
	if slices.ContainsFunc(r.labels, func(label labelRule) bool { return label.Hide }) {
		text := labelText(version)
		lower := strings.ToLower(text)
		for _, label := range r.labels {
			if label.Hide && label.matches(text, lower) {
				return "label", label.Text
			}
		}
	}
	return "", ""
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
	case "size-per-minute":
		if r.perMinute == 0 {
			return knownFirst(a.perMinute, b.perMinute, -1)
		}
		return knownFirst(r.perMinuteDistance(a.perMinute), r.perMinuteDistance(b.perMinute), 1)
	case "tier":
		return cmp.Compare(tierRank(a.tier), tierRank(b.tier))
	case "audio":
		return audioOrder(a.audio, b.audio)
	case "edition":
		return cmp.Compare(r.editionRank(a.edition), r.editionRank(b.edition))
	case "labels":
		return cmp.Compare(b.score, a.score)
	}
	return 0
}

// perMinuteDistance is how far a size a minute is from the one aimed at,
// plus one so that 0 stays unknown: 0 for an unknown size.
func (r versionRules) perMinuteDistance(perMinute int64) int64 {
	if perMinute == 0 {
		return 0
	}
	return max(perMinute-r.perMinute, r.perMinute-perMinute) + 1
}

// tierRank ranks a source tier in the order of library.Tiers, unknown
// last.
func tierRank(tier string) int {
	if i := slices.Index(library.Tiers, tier); i >= 0 {
		return i
	}
	return len(library.Tiers)
}

// audioOrder orders two audio formats for the audio criterion: lossless
// and object based first (see library.AudioFormat.Rank), then those with
// more channels, unknown last.
func audioOrder(a, b library.AudioFormat) int {
	var unknown library.AudioFormat
	switch {
	case a == unknown && b == unknown:
		return 0
	case a == unknown:
		return 1
	case b == unknown:
		return -1
	}
	if c := cmp.Compare(b.Rank(), a.Rank()); c != 0 {
		return c
	}
	return knownFirst(a.Channels, b.Channels, -1)
}

// editionRank ranks an edition in the order of the edition criterion; a
// version whose labels name none counts as the theatrical cut, which
// releases do not name.
func (r versionRules) editionRank(edition string) int {
	if i := slices.Index(r.editions, cmp.Or(edition, "theatrical")); i >= 0 {
		return i
	}
	return len(r.editions)
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
// With a preferred region, the versions carrying the language rank among
// themselves: those carrying it in that region first, then those carrying
// it with no known region, then those carrying it only in other regions.
func (r versionRules) languageRank(facts versionFacts) int {
	switch {
	case r.language == "":
		return 0
	case slices.ContainsFunc(facts.languages, func(code string) bool { return localization.SameLanguage(code, r.language) }):
		switch {
		case r.region == "" || slices.Contains(facts.variants, r.region):
			return 0
		case slices.Contains(facts.variants, localization.LanguageCode(r.language)):
			return 1
		}
		return 2
	case facts.multi:
		return 3
	case len(facts.languages) == 0:
		return 4
	}
	return 5
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
// versions, those of item that failed lately already left out: the hide
// rules first, keeping every version when they would hide them all; then
// the quality group, leaving out the versions taller than it while one
// fits or has an unknown height, else keeping them all, the closest to the
// group first; then the criteria, ties keeping the addons' order. Then, for
// the app asking, the versions it plays without conversion come first (see
// appOrder); last, Play local files first puts the local files before the
// others for the users given it (see localfirst.go), each side keeping
// the order above. With criteria or an order for the app, a page of the
// title open keeps the versions it shows in place (see keptInPlace).
func (h *Handler) ordered(ctx context.Context, user accounts.User, item library.Item, versions []library.Version) []library.Version {
	rules := h.rulesFor(ctx, user)
	var sources map[accounts.ID]int
	if slices.Contains(rules.order, "source") {
		sources = rules.sourceRanks(versions)
	}
	facts := make(map[accounts.ID]versionFacts, len(versions))
	for _, version := range versions {
		known := h.versionFacts(ctx, version)
		known.source = sources[version.Origin.Addon]
		known.score, _ = rules.labelScore(version)
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
	app := h.appOrderFor(ctx, user)
	var unconverted map[accounts.ID]bool
	if app != nil {
		unconverted = h.playsUnconverted(ctx, app, user, item, kept)
	}
	place := make(map[accounts.ID]int, len(kept))
	for i, version := range kept {
		place[version.ID] = i
	}
	compare := func(a, b library.Version) int {
		if user.LocalFirst {
			if c := cmp.Compare(remoteRank(a), remoteRank(b)); c != 0 {
				return c
			}
		}
		if c := cmp.Compare(convertedRank(unconverted[a.ID]), convertedRank(unconverted[b.ID])); c != 0 {
			return c
		}
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
	if (len(rules.order) > 0 || app != nil) && len(kept) > 0 {
		if shown := h.watched.shown(user.ID, kept[0].Item, orderKey(rules, user, app)); len(shown) > 0 {
			kept = keptInPlace(kept, shown, compare)
		}
	}
	return kept
}

// convertedRank is 0 for a version the app plays without conversion, 1
// for another (see appOrder).
func convertedRank(unconverted bool) int {
	if unconverted {
		return 0
	}
	return 1
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
