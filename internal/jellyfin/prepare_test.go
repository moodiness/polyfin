package jellyfin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
)

// fakeProbe is an ffprobe that counts its runs and, once released, answers
// a recorded analysis two hours long, which no test title takes for a
// stand-in.
type fakeProbe struct{ path string }

func newFakeProbe(t *testing.T, released bool) fakeProbe {
	t.Helper()
	f := fakeProbe{path: filepath.Join(t.TempDir(), "ffprobe")}
	data, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-aac-mp4.json"))
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	output["format"].(map[string]any)["duration"] = "7200.000000"
	data, _ = json.Marshal(output)
	script := "#!/bin/sh\necho run >> \"$0.runs\"\nwhile [ ! -e \"$0.released\" ]; do sleep 0.01; done\nexec cat \"$0.json\"\n"
	if err := os.WriteFile(f.path+".json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if released {
		f.release(t)
	}
	// A run still waiting ends with the test.
	t.Cleanup(func() { f.release(t) })
	return f
}

func (f fakeProbe) runs() int {
	data, _ := os.ReadFile(f.path + ".runs")
	return bytes.Count(data, []byte("\n"))
}

func (f fakeProbe) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(f.path+".released", nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// sized makes ffprobe's answer give size, that of the file a test title
// serves, so that reading the file's index finds its end.
func (f fakeProbe) sized(t *testing.T, size int64) {
	t.Helper()
	data, err := os.ReadFile(f.path + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	output["format"].(map[string]any)["size"] = strconv.FormatInt(size, 10)
	data, _ = json.Marshal(output)
	if err := os.WriteFile(f.path+".json", data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// settle waits for the preparations under way to end. They are admitted
// before the request that triggers them is answered, so once settled, what
// a request started is done.
func (s testServer) settle(t *testing.T) {
	t.Helper()
	eventually(t, "preparations to end", func() bool {
		p := s.handler.preparations
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.running == 0
	})
}

// eventually waits for done to hold.
func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s: timed out", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOpeningATitlePreparesItsFirstVersion(t *testing.T) {
	probe := newFakeProbe(t, false)
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	t.Cleanup(func() { p.settle(t) })
	analyzed := func() bool {
		_, ok := p.handler.Playback.Analyzed(t.Context(), p.versions[0].ID)
		return ok
	}
	details := "/Users/" + p.user.ID.String() + "/Items/" + p.movie

	if status := p.get(t, details, p.token, nil); status != http.StatusOK {
		t.Fatalf("details: %d", status)
	}
	p.settle(t)
	if probe.runs() != 0 || analyzed() {
		t.Fatalf("switched off, opening the details analyzed: %d runs", probe.runs())
	}

	p.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	// The answer comes while ffprobe is held: details never wait for it.
	if status := p.get(t, details, p.token, nil); status != http.StatusOK {
		t.Fatalf("details: %d", status)
	}
	eventually(t, "ffprobe to run", func() bool { return probe.runs() == 1 })
	if analyzed() {
		t.Fatal("analyzed before ffprobe answered")
	}
	// The first version serves a real Matroska file, whose index the
	// preparation reads once the analysis is in.
	info, err := os.Stat(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	probe.sized(t, info.Size())
	probe.release(t)
	eventually(t, "the first version to be analyzed", analyzed)
	p.settle(t)
	if _, ok := p.handler.Playback.Analyzed(t.Context(), p.versions[1].ID); !ok || probe.runs() != 1 {
		t.Errorf("runs %d, the other version keeps its analysis: %v", probe.runs(), ok)
	}
	// What an HLS play reads besides is kept too, so its first PlaybackInfo
	// reads nothing from the source.
	var indexed int
	if err := p.pool.QueryRow(t.Context(), "SELECT count(*) FROM media_keyframes WHERE version_id = $1", p.versions[0].ID).Scan(&indexed); err != nil || indexed != 1 {
		t.Errorf("the first version's keyframe index was not kept: %d %v", indexed, err)
	}
}

func TestNextEpisodeIsPreparedNearTheEnd(t *testing.T) {
	probe := newFakeProbe(t, true)
	tr := trackingOn(t, newProbingServer(t, 10, probe.path))
	t.Cleanup(func() { tr.settle(t) })
	if _, err := tr.addons.Install(t.Context(), addons.Shared(), episodeStreams(t), false); err != nil {
		t.Fatal(err)
	}
	member, err := tr.store.Authenticate(t.Context(), "member", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	next, _ := accounts.ParseID(tr.episodes[1])
	// The first episode lasts 45 minutes.
	progress := func(position time.Duration) {
		t.Helper()
		tr.report(t, "/Sessions/Playing/Progress", map[string]any{"ItemId": tr.episodes[0], "PositionTicks": int64(position / 100), "PlayMethod": "DirectPlay"})
		tr.settle(t)
	}
	listed := func() []library.Version {
		versions, _ := tr.library.CachedVersions(t.Context(), member, next)
		return versions
	}
	started := func() int {
		p := tr.handler.preparations
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.starts[member.ID])
	}

	progress(40 * time.Minute)
	if len(listed()) != 0 || probe.runs() != 0 || started() != 0 {
		t.Fatalf("switched off: %d versions listed, %d runs", len(listed()), probe.runs())
	}
	tr.setting(t, func(s *accounts.Settings) { s.PrepareAhead = true })
	progress(30 * time.Minute)
	if len(listed()) != 0 || probe.runs() != 0 || started() != 0 {
		t.Fatalf("15 minutes left: %d versions listed, %d runs", len(listed()), probe.runs())
	}

	progress(37 * time.Minute)
	versions := listed()
	if len(versions) != 2 {
		t.Fatalf("8 minutes left: next episode's versions %+v", versions)
	}
	if _, ok := tr.handler.subtitleFiles.Get(next); !ok {
		t.Error("the next episode's subtitles were not listed")
	}
	if _, ok := tr.handler.Playback.Analyzed(t.Context(), versions[0].ID); !ok || probe.runs() != 1 {
		t.Errorf("first version analyzed %v, %d runs", ok, probe.runs())
	}
	if _, ok := tr.handler.Playback.Analyzed(t.Context(), versions[1].ID); ok {
		t.Error("the second version was analyzed")
	}

	progress(38 * time.Minute)
	if started() != 1 || probe.runs() != 1 {
		t.Errorf("a repeat report prepared again: %d preparations, %d runs", started(), probe.runs())
	}
}

func TestNextEpisodeLeadFollowsTheVersionListLife(t *testing.T) {
	for minutes, want := range map[int]time.Duration{
		accounts.DefaultVersionListMinutes: 9 * time.Minute,
		accounts.MaxVersionListMinutes:     9 * time.Minute,
		5:                                  4 * time.Minute,
		2:                                  time.Minute,
		accounts.MinVersionListMinutes:     time.Minute,
	} {
		if got := nextEpisodeLead(accounts.Settings{VersionListMinutes: minutes}); got != want {
			t.Errorf("lists kept %d minutes: lead %v, want %v", minutes, got, want)
		}
	}

	probe := newFakeProbe(t, true)
	tr := trackingOn(t, newProbingServer(t, 10, probe.path))
	t.Cleanup(func() { tr.settle(t) })
	if _, err := tr.addons.Install(t.Context(), addons.Shared(), episodeStreams(t), false); err != nil {
		t.Fatal(err)
	}
	member, err := tr.store.Authenticate(t.Context(), "member", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	next, _ := accounts.ParseID(tr.episodes[1])
	tr.setting(t, func(s *accounts.Settings) { s.PrepareAhead, s.VersionListMinutes = true, 5 })
	// The first episode lasts 45 minutes; with lists kept 5 minutes, the
	// next one is prepared 4 minutes before its end, not 9.
	progress := func(position time.Duration) {
		t.Helper()
		tr.report(t, "/Sessions/Playing/Progress", map[string]any{"ItemId": tr.episodes[0], "PositionTicks": int64(position / 100), "PlayMethod": "DirectPlay"})
		tr.settle(t)
	}
	listed := func() int {
		versions, _ := tr.library.CachedVersions(t.Context(), member, next)
		return len(versions)
	}
	progress(37 * time.Minute)
	if listed() != 0 || probe.runs() != 0 {
		t.Fatalf("8 minutes left: %d versions listed, %d runs", listed(), probe.runs())
	}
	progress(41*time.Minute + 30*time.Second)
	if listed() != 2 || probe.runs() != 1 {
		t.Errorf("3m30 left: %d versions listed, %d runs", listed(), probe.runs())
	}
}

func TestEpisodeAfter(t *testing.T) {
	episode := func(id byte, season int, available bool) library.Item {
		return library.Item{ID: accounts.ID{id}, Kind: library.KindEpisode, ParentIndexNumber: season, Available: available}
	}
	// As Library.Episodes orders them: specials first, then each season.
	episodes := []library.Item{episode(1, 0, true), episode(2, 0, true), episode(3, 1, true), episode(4, 1, true),
		episode(5, 2, true), episode(6, 2, false)}
	for _, tc := range []struct {
		current byte
		want    byte // 0 for none
	}{
		{3, 4}, // next in the season
		{4, 5}, // first of the next season
		{5, 0}, // the next is not out
		{6, 0}, // the last
		{1, 0}, // specials are left out, as Next Up leaves them out
		{9, 0}, // not of the series
	} {
		got, ok := episodeAfter(episodes, accounts.ID{tc.current})
		if tc.want == 0 && ok || tc.want != 0 && (!ok || got.ID != accounts.ID{tc.want}) {
			t.Errorf("after %d: %v %v, want %d", tc.current, got.ID[0], ok, tc.want)
		}
	}
}

func TestPreparationsAreBounded(t *testing.T) {
	p := newPreparations()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return now }
	alice, bob, carol := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}
	title := func(n byte) accounts.ID { return accounts.ID{0xff, n} }

	// Two run at once over the server; a third is dropped, not queued.
	if p.start(preparationKey{user: alice, title: title(1)}) != nil || p.start(preparationKey{user: bob, title: title(1)}) != nil {
		t.Fatal("the first two preparations were refused")
	}
	if err := p.start(preparationKey{user: carol, title: title(1)}); err != errTooMany {
		t.Errorf("a third at once: %v", err)
	}
	p.finish()
	if err := p.start(preparationKey{user: carol, title: title(1)}); err != nil {
		t.Errorf("once one ended: %v", err)
	}
	p.finish()
	p.finish()

	// Six start per user in a minute.
	for n := byte(2); n <= 6; n++ {
		if err := p.start(preparationKey{user: alice, title: title(n)}); err != nil {
			t.Fatalf("preparation %d of the minute: %v", n, err)
		}
		p.finish()
	}
	if err := p.start(preparationKey{user: alice, title: title(7)}); err != errTooMany {
		t.Errorf("a seventh in the minute: %v", err)
	}
	if err := p.start(preparationKey{user: bob, title: title(7)}); err != nil {
		t.Errorf("another user in the same minute: %v", err)
	}
	p.finish()
	now = now.Add(time.Minute)
	if err := p.start(preparationKey{user: alice, title: title(7)}); err != nil {
		t.Errorf("a minute later: %v", err)
	}
	p.finish()

	// A title is prepared once per user in ten minutes, the next episode
	// after a title apart from the title.
	if err := p.start(preparationKey{user: alice, title: title(1)}); err != errPrepared {
		t.Errorf("the same title again: %v", err)
	}
	if err := p.start(preparationKey{user: alice, title: title(1), next: true}); err != nil {
		t.Errorf("the episode after it: %v", err)
	}
	p.finish()
	if p.claim(preparationKey{user: alice, title: title(2)}) || !p.claim(preparationKey{user: alice, title: title(8)}) {
		t.Error("claims ignore what was prepared")
	}
	now = now.Add(preparedFor)
	if err := p.start(preparationKey{user: alice, title: title(1)}); err != nil {
		t.Errorf("ten minutes later: %v", err)
	}
	p.finish()
}
