package library

import (
	"errors"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// choicesEnv serves two movies from one addon, and a second addon listing
// streams for the first: each movie lists the same file, a file of its
// own, and the first movie a web page, a torrent and its own file twice;
// the second addon lists the first movie's shared file again, as a mirror.
func choicesEnv(t *testing.T) (e env, first, second accounts.ID) {
	t.Helper()
	e = newEnv(t)
	movies := titles("movie", 2)
	shared := stremio.StreamBehavior{Filename: "Shared.File.1080p.mkv", VideoSize: 4_000_000_000}
	own := func(name string) stremio.StreamBehavior {
		return stremio.StreamBehavior{Filename: name + ".2160p.mkv", VideoSize: 9_000_000_000}
	}
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "first", Name: "First", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
		metas:    map[string]stremio.Meta{"movie/" + movies[0].ID: movies[0], "movie/" + movies[1].ID: movies[1]},
		streams: map[string][]stremio.Stream{
			"movie/" + movies[0].ID: {
				{Name: "Shared", URL: "https://one.example/shared.mkv", BehaviorHints: shared},
				{Name: "Page", ExternalURL: "https://pages.example/movie"},
				{Name: "Torrent", InfoHash: "0123456789abcdef0123456789abcdef01234567"},
				{Name: "Own", URL: "https://one.example/own.mkv", BehaviorHints: own("First")},
				{Name: "Own again", URL: "https://one.example/own-again.mkv", BehaviorHints: own("First")},
			},
			"movie/" + movies[1].ID: {
				{Name: "Shared", URL: "https://one.example/shared-too.mkv", BehaviorHints: shared},
				{Name: "Own", URL: "https://one.example/second.mkv", BehaviorHints: own("Second")},
			},
		},
	})
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "second", Name: "Second", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "stream"}}},
		streams: map[string][]stremio.Stream{"movie/" + movies[0].ID: {
			{Name: "Shared elsewhere", URL: "https://two.example/token/shared.mkv", BehaviorHints: shared}}},
	})
	items := e.children(e.member, "Top", 0, 10).Items
	return e, items[0].ID, items[1].ID
}

// explainedSteps names each stream an explanation lists, with its step.
func explainedSteps(explained VersionsExplained) []string {
	var steps []string
	for _, source := range explained.Sources {
		for _, stream := range source.Streams {
			steps = append(steps, source.Name+": "+stream.Name+" "+stream.Step)
		}
	}
	return steps
}

// Each stream an addon listed is told with the step that left it out, or
// as a version: nothing to play, a torrent for a user who may not play
// them, the same file twice, another addon's copy as a mirror, and a file
// an administrator blocked.
func TestExplainVersionsTellsEachStreamsStep(t *testing.T) {
	t.Parallel()
	e, first, _ := choicesEnv(t)
	versions, err := e.service.Versions(t.Context(), e.member, first)
	if err != nil || !slices.Equal(versionNames(versions), []string{"Shared", "Own"}) {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	explained, err := e.service.ExplainVersions(t.Context(), e.member, first)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"First: Shared version", "First: Page nothing", "First: Torrent torrents", "First: Own version",
		"First: Own again duplicate", "Second: Shared elsewhere mirror"}
	if got := explainedSteps(explained); !slices.Equal(got, want) {
		t.Errorf("steps:\n%q\nwant\n%q", got, want)
	}
	if explained.Sources[0].State != ListKnown || explained.Pending != 0 || !slices.Equal(versionNames(explained.Versions), []string{"Shared", "Own"}) {
		t.Errorf("explained: %+v", explained)
	}
	// The duplicate and the mirror name the version they repeat.
	if own, again := explained.Sources[0].Streams[3], explained.Sources[0].Streams[4]; again.Version.ID != own.Version.ID {
		t.Errorf("the duplicate names %s, the version %s", again.Version.ID, own.Version.ID)
	}

	if _, err := e.service.BlockVersion(t.Context(), e.admin.ID, versions[1], false, "a fake"); err != nil {
		t.Fatal(err)
	}
	explained, err = e.service.ExplainVersions(t.Context(), e.member, first)
	if err != nil {
		t.Fatal(err)
	}
	want[3], want[4] = "First: Own blocked", "First: Own again blocked"
	if got := explainedSteps(explained); !slices.Equal(got, want) {
		t.Errorf("steps once blocked:\n%q\nwant\n%q", got, want)
	}
	if block := explained.Sources[0].Streams[3].Block; block.Reason != "a fake" || block.By != e.admin.ID || block.Everywhere() {
		t.Errorf("the block told: %+v", block)
	}
}

// A file blocked for a title is listed nowhere for it, nor played by its
// identifier, while the other titles keep their copies; blocked for every
// title, it leaves them all. A block lifted brings it back.
func TestABlockHoldsForItsTitleOrEveryTitle(t *testing.T) {
	t.Parallel()
	e, first, second := choicesEnv(t)
	versions, err := e.service.Versions(t.Context(), e.member, first)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	others, err := e.service.Versions(t.Context(), e.member, second)
	if err != nil || len(others) != 2 {
		t.Fatalf("the second movie's versions: %q %v", versionNames(others), err)
	}
	block, err := e.service.BlockVersion(t.Context(), e.admin.ID, versions[0], false, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := e.service.Versions(t.Context(), e.member, first); !slices.Equal(versionNames(got), []string{"Own"}) {
		t.Errorf("blocked for the title: %q", versionNames(got))
	}
	if _, err := e.service.Version(t.Context(), e.member, first, versions[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the version blocked, by its identifier: %v", err)
	}
	if got, _ := e.service.Versions(t.Context(), e.member, second); !slices.Equal(versionNames(got), []string{"Shared", "Own"}) {
		t.Errorf("the other title, the block for the first: %q", versionNames(got))
	}

	if _, err := e.service.BlockVersion(t.Context(), e.admin.ID, versions[0], true, "first reason"); err != nil {
		t.Fatal(err)
	}
	// Blocked again, the file keeps its block, with the new reason.
	everywhere, err := e.service.BlockVersion(t.Context(), e.admin.ID, versions[0], true, "fake everywhere")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := e.service.Versions(t.Context(), e.member, second); !slices.Equal(versionNames(got), []string{"Own"}) {
		t.Errorf("the other title, blocked everywhere: %q", versionNames(got))
	}
	if blocks := e.service.VersionBlocks(t.Context(), accounts.ID{}); len(blocks) != 1 || blocks[0].ID != everywhere.ID ||
		blocks[0].Reason != "fake everywhere" || blocks[0].Name != "Shared" || blocks[0].Size != 4_000_000_000 {
		t.Errorf("the blocks of every title: %+v", blocks)
	}
	if blocks := e.service.VersionBlocks(t.Context(), first); len(blocks) != 1 || blocks[0].ID != block.ID {
		t.Errorf("the title's blocks: %+v", blocks)
	}

	for _, id := range []accounts.ID{block.ID, everywhere.ID} {
		if err := e.service.Unblock(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.service.Unblock(t.Context(), block.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a block lifted twice: %v", err)
	}
	for _, movie := range []accounts.ID{first, second} {
		if got, _ := e.service.Versions(t.Context(), e.member, movie); !slices.Equal(versionNames(got), []string{"Shared", "Own"}) {
			t.Errorf("unblocked: %q", versionNames(got))
		}
	}
	// What was read stays read: a new service reads the database.
	if _, err := e.service.BlockVersion(t.Context(), e.admin.ID, versions[1], false, ""); err != nil {
		t.Fatal(err)
	}
	fresh := New(e.service.db, e.addons, e.service.client, e.service.logger, e.users.Settings)
	fresh.followUpDelays, fresh.readAhead = nil, false
	if got, _ := fresh.Versions(t.Context(), e.member, first); !slices.Equal(versionNames(got), []string{"Shared"}) {
		t.Errorf("after a restart: %q", versionNames(got))
	}
}

// A pin is kept for its title, with the version's name, file and size,
// read again after a restart, and replaced by the next.
func TestAPinIsKeptForItsTitle(t *testing.T) {
	t.Parallel()
	e, first, second := choicesEnv(t)
	versions, err := e.service.Versions(t.Context(), e.member, first)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %q %v", versionNames(versions), err)
	}
	if _, ok := e.service.PinnedVersion(t.Context(), first); ok {
		t.Fatal("pinned before any pin")
	}
	for _, version := range []Version{versions[0], versions[1]} {
		if err := e.service.PinVersion(t.Context(), e.admin.ID, version); err != nil {
			t.Fatal(err)
		}
	}
	fresh := New(e.service.db, e.addons, e.service.client, e.service.logger, e.users.Settings)
	pin, ok := fresh.PinnedVersion(t.Context(), first)
	if !ok || pin.Version != versions[1].ID || pin.Name != "Own" || pin.Filename != "First.2160p.mkv" || pin.Size != 9_000_000_000 ||
		pin.By != e.admin.ID {
		t.Errorf("pin: %+v %v", pin, ok)
	}
	if _, ok := fresh.PinnedVersion(t.Context(), second); ok {
		t.Error("the other title is pinned")
	}
	if err := e.service.Unpin(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.service.PinnedVersion(t.Context(), first); ok {
		t.Error("pinned once unpinned")
	}
}
