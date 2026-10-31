package jellyfin

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/logs"
	"github.com/moodiness/polyfin/internal/playback"
)

// The admin app's Versions page explains a movie's or an episode's
// versions for a user: every stream each addon listed, and the step of
// docs/playback.md, Which streams become versions, that left it out, or
// its place among the versions offered. It reads what details and
// PlaybackInfo read, through the same functions (see offered).

// What became of a version, beyond the steps the library tells (see
// library.StepVersion), as StreamExplanation tells it.
const (
	// StepOffered: the version is offered, at Place.
	StepOffered = "offered"
	// StepFailed: the version failed lately; Detail says why.
	StepFailed = "failed"
	// StepHidden: a rule hides the version, Rule (see versionRules.hiddenBy),
	// with Detail.
	StepHidden = "hidden"
	// StepGroup: the user's quality group leaves the version out.
	StepGroup = "group"
)

// VersionsExplanation explains a title's versions for a user.
type VersionsExplanation struct {
	// Sources are the addons and folders asked, in order, with what each
	// listed; Pending is how many addons are still asked.
	Sources []SourceExplanation
	Pending int
	// Criteria are those of Order versions by, in order, and Group the
	// user's quality group, a height, 0 for none. KeptAll tells that the
	// hide rules would hide every version, so none is hidden; Closest, that
	// no version fits the group, so the closest heights come first.
	Criteria []string
	Group    int
	KeptAll  bool
	Closest  bool
	// Pin is the version pinned for the title, nil for none; Blocks are
	// the blocks of the title's files, those of every title aside.
	Pin    *PinExplanation
	Blocks []library.VersionBlock
	// Decisions are the title's last plays, by any user (see
	// PlayDecisions).
	Decisions []PlayDecision
}

// PinExplanation is the version pinned for a title, and whether it is
// missing: no addon lists it now, so the order stays as it is.
type PinExplanation struct {
	library.VersionPin
	Missing bool
}

// SourceExplanation is an addon or a folder asked for a title's streams:
// see library.SourceExplained.
type SourceExplanation struct {
	Addon    accounts.ID
	Name     string
	Embedded bool
	State    string
	Streams  []StreamExplanation
}

// StreamExplanation is a stream an addon listed, and what became of it:
// Step, a library step (see library.StepNothing) or one of those above.
// Version is the version it makes, for StepDuplicate and StepMirror the
// one listed before it; Place its place among the versions offered, from
// 1; Pinned marks the version pinned; Unstarted, a torrent that did not
// start lately, which a play picking no version passes over. Labels and
// Height are what its addon's labels tell, Facts what the rules know of it
// (see versionFacts), Analyzed whether it was analyzed.
type StreamExplanation struct {
	Name      string
	Filename  string
	Size      int64
	Torrent   bool
	Version   accounts.ID
	Step      string
	Rule      string
	Detail    string
	Place     int
	Pinned    bool
	Unstarted bool
	Block     *library.VersionBlock
	Height    int
	Labels    library.Labels
	Facts     ExplainedFacts
	Analyzed  bool
}

// ExplainedFacts are what the version rules know of a version: its
// height, size and video codec, 0 or empty when unknown, its range, and
// its audio languages, with multi when its labels say it has several.
type ExplainedFacts struct {
	Height    int
	Size      int64
	Codec     string
	Range     library.DynamicRange
	Languages []string
	Multi     bool
}

// ExplainVersions explains a movie's, a home video's or an episode's
// versions for user, as their details list them (see
// library.Service.ExplainVersions), asking the addons whose lists are not
// known in the background.
func (h *Handler) ExplainVersions(ctx context.Context, user accounts.User, id accounts.ID) (VersionsExplanation, error) {
	listed, err := h.Library.ExplainVersions(ctx, user, id)
	if err != nil {
		return VersionsExplanation{}, err
	}
	versions := listed.Versions
	h.RemuxDB.Describe(ctx, versions)
	rules := h.rulesFor(ctx, user)
	explanation := VersionsExplanation{Pending: listed.Pending, Criteria: slices.Clone(rules.order), Group: user.QualityGroup,
		Blocks: h.Library.VersionBlocks(ctx, id), Decisions: h.PlayDecisions(id)}
	offered := h.offered(ctx, user, slices.Clone(versions))
	place := make(map[accounts.ID]int, len(offered))
	for i, version := range offered {
		place[version.ID] = i + 1
	}
	// fates are why each version listed is not offered, as offered and
	// ordered leave it out.
	type fate struct{ step, rule, detail string }
	fates := map[accounts.ID]fate{}
	var kept []library.Version
	for _, version := range versions {
		if h.Playback.Failed(version.ID) && !h.didNotStart(version) {
			detail := ""
			if failure := h.Playback.Failure(version.ID); failure != nil {
				detail = logs.Redact(failure.Error())
			}
			fates[version.ID] = fate{step: StepFailed, detail: detail}
			continue
		}
		kept = append(kept, version)
	}
	facts := make(map[accounts.ID]versionFacts, len(versions))
	for _, version := range versions {
		facts[version.ID] = h.versionFacts(ctx, version)
	}
	if h.reordered(user) || slices.ContainsFunc(kept, elsewhere) {
		if rules.hides() || slices.ContainsFunc(kept, elsewhere) {
			hidden := map[accounts.ID]fate{}
			for _, version := range kept {
				if rule, detail := rules.hiddenBy(version, facts[version.ID]); rule != "" {
					hidden[version.ID] = fate{step: StepHidden, rule: rule, detail: detail}
				}
			}
			if len(hidden) > 0 && len(hidden) == len(kept) {
				explanation.KeptAll = true
			} else {
				for id, f := range hidden {
					fates[id] = f
				}
				kept = slices.DeleteFunc(kept, func(v library.Version) bool { _, ok := hidden[v.ID]; return ok })
			}
		}
		if user.QualityGroup > 0 {
			fits := slices.ContainsFunc(kept, func(v library.Version) bool { return user.FitsGroup(facts[v.ID].height) })
			explanation.Closest = len(kept) > 0 && !fits
			for _, version := range kept {
				if fits && !user.FitsGroup(facts[version.ID].height) {
					fates[version.ID] = fate{step: StepGroup}
				}
			}
		}
	}
	pin, pinned := h.Library.PinnedVersion(ctx, id)
	if pinned {
		explanation.Pin = &PinExplanation{VersionPin: pin,
			Missing: !slices.ContainsFunc(versions, func(v library.Version) bool { return v.ID == pin.Version })}
	}
	explain := func(stream library.StreamExplained) StreamExplanation {
		explained := StreamExplanation{Name: stream.Name, Filename: stream.Filename, Size: stream.Size, Torrent: stream.Torrent,
			Version: stream.Version.ID, Step: stream.Step, Height: stream.Version.Height, Labels: stream.Version.Labels}
		if stream.Step == library.StepBlocked {
			block := stream.Block
			explained.Block = &block
		}
		if stream.Version.ID == (accounts.ID{}) {
			return explained
		}
		known := facts[stream.Version.ID]
		if stream.Step != library.StepVersion {
			known = h.versionFacts(ctx, stream.Version)
		}
		explained.Facts = ExplainedFacts{Height: known.height, Size: known.size, Codec: known.codec, Range: known.hdr,
			Languages: known.languages, Multi: known.multi}
		_, explained.Analyzed = h.Playback.Analyzed(ctx, stream.Version.ID)
		explained.Pinned = pinned && stream.Version.ID == pin.Version && stream.Step == library.StepVersion
		if stream.Step != library.StepVersion {
			return explained
		}
		explained.Unstarted = h.didNotStart(stream.Version)
		if n, ok := place[stream.Version.ID]; ok {
			explained.Step, explained.Place = StepOffered, n
			return explained
		}
		f, ok := fates[stream.Version.ID]
		if !ok {
			// The rules changed since they were read: it is hidden all
			// the same.
			f = fate{step: StepHidden}
		}
		explained.Step, explained.Rule, explained.Detail = f.step, f.rule, f.detail
		return explained
	}
	explanation.Sources = make([]SourceExplanation, len(listed.Sources))
	for i, source := range listed.Sources {
		streams := make([]StreamExplanation, len(source.Streams))
		for j, stream := range source.Streams {
			streams[j] = explain(stream)
		}
		explanation.Sources[i] = SourceExplanation{Addon: source.Addon, Name: source.Name, Embedded: source.Embedded, State: source.State,
			Streams: streams}
	}
	return explanation, nil
}

// adminVersion finds a version of a movie or an episode a user is offered,
// for an administrator's action on it; ErrNotFound otherwise, and for the
// title's own identifier, which names no version.
func (h *Handler) adminVersion(ctx context.Context, user accounts.User, item, id accounts.ID) (library.Version, error) {
	if id == item {
		return library.Version{}, library.ErrNotFound
	}
	return h.Library.Version(ctx, user, item, id)
}

// PinVersion pins a version of a movie or an episode, one user is
// offered, for every user: by, an administrator, pins it.
func (h *Handler) PinVersion(ctx context.Context, by, user accounts.User, item, id accounts.ID) error {
	version, err := h.adminVersion(ctx, user, item, id)
	if err != nil {
		return err
	}
	return h.Library.PinVersion(ctx, by.ID, version)
}

// UnpinVersion unpins the version pinned for a title, if any.
func (h *Handler) UnpinVersion(ctx context.Context, item accounts.ID) error {
	return h.Library.Unpin(ctx, item)
}

// BlockVersion blocks the file of a version one user is offered, for its
// title or, everywhere set, for every title: by, an administrator, blocks
// it, for reason.
func (h *Handler) BlockVersion(ctx context.Context, by, user accounts.User, item, id accounts.ID, everywhere bool, reason string) (library.VersionBlock, error) {
	version, err := h.adminVersion(ctx, user, item, id)
	if err != nil {
		return library.VersionBlock{}, err
	}
	return h.Library.BlockVersion(ctx, by.ID, version, everywhere, reason)
}

// UnblockVersion lifts a block (see library.Service.Unblock).
func (h *Handler) UnblockVersion(ctx context.Context, id accounts.ID) error {
	return h.Library.Unblock(ctx, id)
}

// VersionBlocks lists the blocks of every title, the latest first.
func (h *Handler) VersionBlocks(ctx context.Context) []library.VersionBlock {
	return h.Library.VersionBlocks(ctx, accounts.ID{})
}

// ErrTorrentAnalysis is an analysis asked for a torrent's version, which
// only a play reads.
var ErrTorrentAnalysis = errors.New("a torrent is read by its plays only")

// adminAnalysisWait bounds how long an administrator's analysis waits, on
// top of the settings' analysis timeout, for the source to be held.
const adminAnalysisWait = 10 * time.Second

// AnalyzeVersion analyzes a version a user is offered now, its failure
// forgotten, in the background of the plays. failure tells, without
// addresses, what kept it from being analyzed, empty when it was; err is
// for a version not found, or a torrent's.
func (h *Handler) AnalyzeVersion(ctx context.Context, user accounts.User, item, id accounts.ID) (failure string, err error) {
	version, err := h.adminVersion(ctx, user, item, id)
	if err != nil {
		return "", err
	}
	if version.Torrent != nil {
		return "", ErrTorrentAnalysis
	}
	h.Playback.RetryVersion(version)
	timeout := time.Duration(h.Accounts.Settings().AnalysisTimeout)*time.Second + adminAnalysisWait
	ctx, cancel := context.WithTimeout(playback.Background(ctx), timeout)
	defer cancel()
	if _, err := h.Playback.Analyze(ctx, version); err != nil {
		return logs.Redact(err.Error()), nil
	}
	return "", nil
}

// AskVersionsAgain asks a user's addons again for a title's streams, as
// Versions/Search does for the user (see searchVersions), at most once
// every searchInterval for each administrator's title; it returns how long
// to wait otherwise.
func (h *Handler) AskVersionsAgain(ctx context.Context, by, user accounts.User, item accounts.ID) (time.Duration, error) {
	if allowed, wait := h.searches.allow(by.ID, item, h.now()); !allowed {
		return wait, nil
	}
	return 0, h.Library.AskAgain(ctx, user, item)
}
