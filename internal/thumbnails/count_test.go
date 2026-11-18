package thumbnails

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// centuries is a duration a file may state: 285 years.
const centuries = 9_000_000_000 * time.Second

// The thumbnails made from a whole file are padded with the last frame
// FFmpeg wrote up to the most a version has, and their tiles kept: one
// more is refused, whatever was written, and no more than that many are
// packed.
func TestFileThumbnailsStopAtTheMostAVersionHas(t *testing.T) {
	t.Parallel()
	const interval = accounts.MinTrickplayInterval * time.Second
	frame := thumbnail(8, 8, 100)

	// At the bound: one frame written stands for the others.
	thumbnails := &fileThumbnails{tiles: newTiler(maxThumbnails, interval), count: maxThumbnails}
	if err := thumbnails.add(0, frame); err != nil {
		t.Fatal(err)
	}
	info, tiles, err := thumbnails.finish()
	if err != nil || info.ThumbnailCount != maxThumbnails || len(tiles) != maxThumbnails/100 {
		t.Fatalf("%d thumbnails in %d tiles, %v: %d in %d expected", info.ThumbnailCount, len(tiles), err, maxThumbnails, maxThumbnails/100)
	}

	// One more: refused, and nothing is padded.
	thumbnails = &fileThumbnails{tiles: newTiler(maxThumbnails+1, interval), count: maxThumbnails + 1}
	if err := thumbnails.add(0, frame); err != nil {
		t.Fatal(err)
	}
	info, tiles, err = thumbnails.finish()
	if !errors.Is(err, errNoVideo) || len(tiles) != 0 || info.ThumbnailCount != 0 {
		t.Fatalf("%d thumbnails in %d tiles, %v: one more than a version has is not refused", info.ThumbnailCount, len(tiles), err)
	}
	if state, reason := outcome(err); state != StateCannot || reason != ReasonNoVideo || !lasting(err) {
		t.Errorf("the refusal is kept as %s, %s", state, reason)
	}
	if thumbnails.added != 1 || len(thumbnails.tiles.tiles) != 0 {
		t.Errorf("%d thumbnails packed in %d tiles before the refusal", thumbnails.added, len(thumbnails.tiles.tiles))
	}

	// 285 years of frames written: those past the bound are left out as
	// they come. All but the last 150 count as packed already.
	count := thumbnailCount(centuries, interval)
	thumbnails = &fileThumbnails{tiles: newTiler(count, interval), count: count, added: maxThumbnails - 150}
	for i := range 400 {
		if err := thumbnails.add(i, frame); err != nil {
			t.Fatal(err)
		}
	}
	if thumbnails.added != maxThumbnails || len(thumbnails.tiles.tiles) != 1 || thumbnails.tiles.placed != 50 {
		t.Errorf("%d thumbnails packed, %d of the last 400 written, of %d at most", thumbnails.added,
			100*len(thumbnails.tiles.tiles)+thumbnails.tiles.placed, maxThumbnails)
	}
	if _, _, err := thumbnails.finish(); !errors.Is(err, errNoVideo) {
		t.Errorf("%d thumbnails are not refused: %v", count, err)
	}
}

// A version whose analysis tells 285 years is asked no thumbnails, as one
// without a duration: its result tells that it cannot have them, nothing
// is read nor kept, and a local folder's file without an index is not
// read whole for them.
func TestAVersionTooLongGetsNoThumbnails(t *testing.T) {
	t.Parallel()
	ts := newTestService(t, "")
	ts.change(func(s *accounts.Settings) { s.ChapterImages = false })
	analysis := media.Analysis{Format: "mpegts", Duration: centuries, Size: 1 << 20,
		Streams: []media.Stream{{Index: 0, Type: "video", Codec: "mpeg2video", Width: 128, Height: 72}}}
	ts.mu.Lock()
	ts.analysis = analysis
	ts.mu.Unlock()
	version := library.Version{ID: accounts.ID{41}, Item: accounts.ID{42}, URL: "/media/home.ts", Addon: "Home", LocalFolder: true}
	ctx := t.Context()
	for _, seconds := range []int{accounts.MinTrickplayInterval, accounts.DefaultTrickplayInterval, accounts.MaxTrickplayInterval} {
		ts.change(func(s *accounts.Settings) { s.TrickplayInterval = seconds })
		want, err := ts.wanted(ctx, version.ID, analysis)
		if err != nil || want.trickplay || !want.tooLong {
			t.Fatalf("one thumbnail every %d s of %v: wanted %+v, %v", seconds, centuries, want, err)
		}
		// One more than a version has, as near the bound as can be.
		analysis.Duration = time.Duration(maxThumbnails*seconds)*time.Second + time.Nanosecond
		if want, err = ts.wanted(ctx, version.ID, analysis); err != nil || want.trickplay || !want.tooLong {
			t.Fatalf("one thumbnail every %d s of %v: wanted %+v, %v", seconds, analysis.Duration, want, err)
		}
		// One fewer: asked.
		analysis.Duration -= time.Nanosecond
		if want, err = ts.wanted(ctx, version.ID, analysis); err != nil || !want.trickplay || want.tooLong {
			t.Fatalf("one thumbnail every %d s of %v: wanted %+v, %v", seconds, analysis.Duration, want, err)
		}
		analysis.Duration = centuries
	}
	// The whole file is not read for a count past the bound, whoever asks.
	stream, _ := videoStream(analysis)
	if _, err := ts.decodeFile(ctx, version, analysis, stream, wanted{trickplay: true, width: 240, interval: 5 * time.Second}); !errors.Is(err, errNoVideo) {
		t.Errorf("a file of %v is read whole: %v", centuries, err)
	}
	ts.generate(ctx, job{version: version, title: "Home video", host: hostOf(version.URL)})
	if result := resultOf(t, ts, version.ID); result.State != StateCannot || result.Reason != ReasonNoVideo || result.Count != 0 {
		t.Errorf("result %+v: no video to make thumbnails of expected", result)
	}
	if manifest, err := ts.Manifest(ctx, version.Item); err != nil || len(manifest) != 0 {
		t.Errorf("thumbnails kept: %+v, %v", manifest, err)
	}
	// With thumbnails off, nothing is asked of it, as before.
	ts.change(func(s *accounts.Settings) { s.Trickplay, s.ChapterImages = false, true })
	if want, err := ts.wanted(ctx, version.ID, analysis); err != nil || want.trickplay || want.tooLong {
		t.Errorf("thumbnails off: wanted %+v, %v", want, err)
	}
}

// playedFile is forced.mkv wholly held by the source cache, as a play of
// all of it leaves it, with its analysis.
func playedFile(t *testing.T) (*testService, library.Version, media.Analysis) {
	t.Helper()
	ffmpeg := testFFmpeg(t)
	data := fixture(t, "forced.mkv")
	origin := newHost(t, data)
	ts := newTestService(t, ffmpeg)
	cache, err := source.New(t.TempDir(), func() source.Policy { return source.Policy{Limit: 1 << 30} }, httpOpener{origin.client}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	ts.cache = cache
	version := library.Version{ID: accounts.ID{51}, Item: accounts.ID{52}, URL: origin.url, Addon: "Test"}
	src := cache.Open(version.ID, source.Location{URL: version.URL}, nil)
	t.Cleanup(src.Release)
	if _, err := src.ReadAt(t.Context(), make([]byte, len(data)), 0); err != nil {
		t.Fatal(err)
	}
	analysis := forcedAnalysis
	analysis.Size = int64(len(data))
	return ts, version, analysis
}

// During a play, a version gets thumbnails from what the source cache
// holds while its duration gives no more than a version has: one more,
// and none is made, though its keyframes are held. It is left before
// anything is read of it, the set it has included: a context already
// over fails no read.
func TestThumbnailsDuringAPlayStopAtTheMostAVersionHas(t *testing.T) {
	t.Parallel()
	ts, version, analysis := playedFile(t)
	ctx := t.Context()
	over, cancel := context.WithCancel(ctx)
	cancel()
	for _, duration := range []time.Duration{longestVersion + time.Nanosecond, centuries} {
		analysis.Duration = duration
		if added, err := ts.extend(over, version, analysis, false); added != 0 || err != nil {
			t.Fatalf("%v gave %d thumbnails, %v: nothing was to be read", duration, added, err)
		}
		if _, found, err := ts.trickplaySet(ctx, version.ID, 240); found || err != nil {
			t.Fatalf("thumbnails kept for %v: %v", duration, err)
		}
	}
	// The same file with its own duration has them.
	analysis.Duration = forcedAnalysis.Duration
	if added, err := ts.extend(ctx, version, analysis, false); added != 4 || err != nil {
		t.Fatalf("%v gave %d thumbnails, %v: 4 expected", analysis.Duration, added, err)
	}
}

// During a play, a pass keeps a budget of decoded keyframes: it stops at
// the keyframe that passes it and keeps the thumbnails read so far, and
// the next passes make the following ones, to the last.
func TestAPassDuringAPlayKeepsABudgetOfKeyframes(t *testing.T) {
	t.Parallel()
	ts, version, analysis := playedFile(t)
	ctx := t.Context()
	// A keyframe's thumbnail takes 86,400 bytes: each pass stops at its
	// second.
	ts.mu.Lock()
	ts.frameBytes = 86_400 + 1
	ts.mu.Unlock()
	for pass := 1; pass <= 2; pass++ {
		if added, err := ts.extend(ctx, version, analysis, false); added != 2 || err != nil {
			t.Fatalf("pass %d added %d thumbnails, %v: two keyframes kept, two thumbnails, expected", pass, added, err)
		}
		if info, found, err := ts.trickplaySet(ctx, version.ID, 240); !found || err != nil || info.ThumbnailCount != 2*pass {
			t.Fatalf("after pass %d: %+v, %v", pass, info, err)
		}
	}
	if added, err := ts.extend(ctx, version, analysis, false); added != 0 || err != nil {
		t.Fatalf("a set complete got %d thumbnails more, %v", added, err)
	}
}

// After a play, the decoded keyframes kept for a set weigh no more than
// a budget: the keyframe that passes it gives the set up, the version
// keeps "cannot", "no video", and nothing of the set. Within the budget,
// every thumbnail is made.
func TestASetsKeyframesWeighNoMoreThanItsBudget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		budget int64
		made   int
	}{
		// A thumbnail of forced.mkv, 240 by 240, takes 86,400 bytes, and
		// its 15 thumbnails show 11 keyframes.
		{"past the budget", 3*86_400 - 1, 0},
		{"within the budget", 11 * 86_400, 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts, file := localFile(t)
			ts.mu.Lock()
			ts.frameBytes = tc.budget
			ts.mu.Unlock()
			ctx := t.Context()
			ts.generate(ctx, job{version: file, title: "Home video", host: hostOf(file.URL)})
			manifest, err := ts.Manifest(ctx, file.Item)
			if got := manifest[file.ID][240].ThumbnailCount; err != nil || got != tc.made {
				t.Fatalf("%d thumbnails, %v: %d expected", got, err, tc.made)
			}
			result := resultOf(t, ts, file.ID)
			if tc.made == 0 && (result.State != StateCannot || result.Reason != ReasonNoVideo) {
				t.Errorf("result %+v: the set given up expected", result)
			}
			if tc.made > 0 && (result.State != StateMade || result.Count != tc.made) {
				t.Errorf("result %+v: %d made expected", result, tc.made)
			}
		})
	}
}

// The tiles of a set weigh no more than a budget, whatever their count:
// the tile that passes it is refused as FFmpeg's frames come, or as the
// last frame is padded to the count, and no tile of the set is kept. A
// set that weighs the budget exactly is whole.
func TestASetsTilesWeighNoMoreThanItsBudget(t *testing.T) {
	t.Parallel()
	const interval = accounts.MinTrickplayInterval * time.Second
	const count = 1000
	frame := thumbnail(8, 8, 100)
	// Every tile of this frame alone weighs the same.
	one := newTiler(100, interval)
	for range 100 {
		if err := one.add(frame); err != nil {
			t.Fatal(err)
		}
	}
	tile := int64(len(one.tiles[0]))
	fill := func(limit int64, written int) (*fileThumbnails, error) {
		thumbnails := &fileThumbnails{tiles: newTiler(count, interval), count: count}
		thumbnails.tiles.limit = limit
		for i := range written {
			if err := thumbnails.add(i, frame); err != nil {
				return thumbnails, fmt.Errorf("frame %d: %w", i, err)
			}
		}
		return thumbnails, nil
	}

	// Written by FFmpeg: the third tile passes a budget of two and a half.
	thumbnails, err := fill(2*tile+tile/2, count)
	if !errors.Is(err, errNoVideo) || thumbnails.added != 300 || len(thumbnails.tiles.tiles) != 0 {
		t.Fatalf("%d thumbnails written, %d tiles kept, %v: refused at the 300th expected", thumbnails.added, len(thumbnails.tiles.tiles), err)
	}
	if state, reason := outcome(err); state != StateCannot || reason != ReasonNoVideo || !lasting(err) {
		t.Errorf("the refusal is kept as %s, %s", state, reason)
	}

	// Padded: 150 frames written are within it, and the padding is not.
	thumbnails, err = fill(2*tile+tile/2, 150)
	if err != nil {
		t.Fatal(err)
	}
	info, tiles, err := thumbnails.finish()
	if !errors.Is(err, errNoVideo) || len(tiles) != 0 || info.ThumbnailCount != 0 || thumbnails.added != 299 || len(thumbnails.tiles.tiles) != 0 {
		t.Fatalf("%d thumbnails in %d tiles, %d packed, %v: the padding is not refused at the third tile", info.ThumbnailCount, len(tiles), thumbnails.added, err)
	}

	// The budget exactly: whole.
	thumbnails, err = fill(count/100*tile, 150)
	if err != nil {
		t.Fatal(err)
	}
	if info, tiles, err = thumbnails.finish(); err != nil || info.ThumbnailCount != count || len(tiles) != count/100 {
		t.Fatalf("%d thumbnails in %d tiles, %v: %d in %d expected", info.ThumbnailCount, len(tiles), err, count, count/100)
	}
	// And what a set may weigh when nothing lowers it.
	if maxTileBytes != 256<<20 || maxFrameBytes != 2<<30 || thumbnails.tiles.weight != count/100*tile || one.limit != 0 {
		t.Errorf("a set keeps %d bytes of tiles and %d of keyframes at most; this one weighs %d", maxTileBytes, maxFrameBytes, thumbnails.tiles.weight)
	}
}
