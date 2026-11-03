package playback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/ocr"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// finnishFiles lists the files of names beside a local video, in Finnish.
func finnishFiles(t *testing.T, p pictureSetup, names ...string) []library.ExternalSubtitle {
	t.Helper()
	files := sideFiles(t, p, names...)
	for i := range files {
		files[i].Language = "fin"
	}
	return files
}

// reads are the languages p's reader read pictures in.
func (p pictureSetup) reads() []string {
	p.reader.mu.Lock()
	defer p.reader.mu.Unlock()
	return slices.Clone(p.reader.reads)
}

// waitFetches waits for the downloads started by themselves to end, and
// for the reads they queue.
func (p pictureSetup) waitFetches(t *testing.T) {
	t.Helper()
	p.waitReads(t)
	deadline := time.Now().Add(time.Minute)
	for {
		p.pictures.mu.Lock()
		idle := len(p.pictures.fetching) == 0
		p.pictures.mu.Unlock()
		if idle {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the downloads did not end")
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.waitReads(t)
}

// A language's data downloaded from Health, from a server serving it with
// the checksum listed, is kept in the folder the reader reads: the
// language is no longer missing, and the track waiting for it is read.
// A file of another checksum is not kept, and Health tells why.
func TestADownloadedLanguageIsNoLongerMissingAndItsTracksAreRead(t *testing.T) {
	t.Parallel()
	data := []byte("Finnish model")
	served := data
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write(served)
	}))
	t.Cleanup(server.Close)
	sum := sha256.Sum256(data)
	tessdata := filepath.Join(t.TempDir(), "tessdata")
	downloader := &ocr.Downloader{Dir: tessdata, URL: server.URL,
		Files: map[string]ocr.DataFile{"fin": {Language: "fin", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}}
	disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
	p := newPictureService(t, migrated.New(t), &fakeReader{languages: []string{"eng"}, dir: tessdata}, disk, "ffmpeg")
	p.FetchLanguagesWith(downloader)
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
	files := finnishFiles(t, p, "fin.sup")
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitReads(t)
	if missing := p.MissingLanguages(t.Context()); !slices.Equal(missing, []MissingLanguage{{Language: "fin", Downloadable: true}}) {
		t.Fatalf("missing languages: %+v", missing)
	}
	mu.Lock()
	served = []byte("Finnish model, altered")
	mu.Unlock()
	if err := p.DownloadLanguage(t.Context(), "fin"); !errors.Is(err, ocr.ErrChecksum) {
		t.Fatalf("a file of another checksum: %v", err)
	}
	if entries, _ := os.ReadDir(tessdata); len(entries) != 0 {
		t.Errorf("kept %v", entries)
	}
	if missing := p.MissingLanguages(t.Context()); len(missing) != 1 || missing[0].Error != ocr.ErrChecksum.Error() {
		t.Errorf("missing languages after a refused file: %+v", missing)
	}
	mu.Lock()
	served = data
	mu.Unlock()
	if err := p.DownloadLanguage(t.Context(), "fin"); err != nil {
		t.Fatal(err)
	}
	if missing := p.MissingLanguages(t.Context()); len(missing) != 0 {
		t.Errorf("missing languages once downloaded: %+v", missing)
	}
	p.waitReads(t)
	if reads := p.reads(); !slices.Equal(reads, []string{"fin"}) {
		t.Errorf("read in %v", reads)
	}
}

// fakeFetcher downloads the data of Finnish into dir, failing with fail
// when set. With hold set, each download waits for it to close.
type fakeFetcher struct {
	dir  string
	hold chan struct{}

	mu    sync.Mutex
	fail  error
	calls int
}

func (f *fakeFetcher) Downloadable(language string) bool { return language == "fin" }

func (f *fakeFetcher) Download(_ context.Context, language string) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.hold != nil {
		<-f.hold
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	return os.WriteFile(filepath.Join(f.dir, language+".traineddata"), nil, 0o600)
}

func (f *fakeFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// With the setting on, the first tracks needing a language Tesseract
// lacks download its data once, then are read.
func TestMissingLanguagesAreDownloadedOnceWithTheSetting(t *testing.T) {
	t.Parallel()
	tessdata := t.TempDir()
	disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
	p := newPictureService(t, migrated.New(t), &fakeReader{languages: []string{"eng"}, dir: tessdata}, disk, "ffmpeg")
	fetcher := &fakeFetcher{dir: tessdata, hold: make(chan struct{})}
	p.FetchLanguagesWith(fetcher)
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
	// Off, nothing is downloaded.
	off := finnishFiles(t, p, "off.sup")
	p.queueTracks(t.Context(), version, videoAnalysis, off, -1)
	p.waitReads(t)
	if calls := fetcher.count(); calls != 0 {
		t.Fatalf("%d downloads with the setting off", calls)
	}
	p.set(func(s *accounts.Settings) { s.SubtitleOCRDownload = true })
	// Two tracks in Finnish are both left unread while the one download
	// they start is held.
	files := finnishFiles(t, p, "a.sup", "b.sup")
	p.queueTracks(t.Context(), library.Version{ID: accounts.ID{2}, URL: p.base + "other.mkv"}, videoAnalysis, files, -1)
	p.waitReads(t)
	if calls := fetcher.count(); calls != 1 {
		t.Fatalf("%d downloads", calls)
	}
	close(fetcher.hold)
	p.waitFetches(t)
	// The tracks waiting are read, the one queued before the setting too.
	if reads := p.reads(); !slices.Equal(reads, []string{"fin", "fin", "fin"}) {
		t.Errorf("read in %v", reads)
	}
	if calls := fetcher.count(); calls != 1 {
		t.Errorf("%d downloads", calls)
	}
}

// A failed download is told in Health and not tried again by itself
// within the hour, but is after it.
func TestFailedDownloadsAreRetriedHourly(t *testing.T) {
	t.Parallel()
	tessdata := t.TempDir()
	disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
	p := newPictureService(t, migrated.New(t), &fakeReader{languages: []string{"eng"}, dir: tessdata}, disk, "ffmpeg")
	p.set(func(s *accounts.Settings) { s.SubtitleOCRDownload = true })
	fetcher := &fakeFetcher{dir: tessdata, fail: errors.New("the server answered 503 Service Unavailable")}
	p.FetchLanguagesWith(fetcher)
	var clock sync.Mutex
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	p.pictures.mu.Lock()
	p.pictures.now = func() time.Time {
		clock.Lock()
		defer clock.Unlock()
		return now
	}
	p.pictures.mu.Unlock()
	version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
	files := finnishFiles(t, p, "a.sup")
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitFetches(t)
	if missing := p.MissingLanguages(t.Context()); len(missing) != 1 || missing[0].Error != "the server answered 503 Service Unavailable" {
		t.Fatalf("missing languages: %+v", missing)
	}
	// Played again 59 minutes later, the track starts no download.
	clock.Lock()
	now = now.Add(59 * time.Minute)
	clock.Unlock()
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitFetches(t)
	if calls := fetcher.count(); calls != 1 {
		t.Fatalf("%d downloads within the hour", calls)
	}
	// After the hour, it does, and the track is read.
	clock.Lock()
	now = now.Add(2 * time.Minute)
	clock.Unlock()
	fetcher.mu.Lock()
	fetcher.fail = nil
	fetcher.mu.Unlock()
	p.queueTracks(t.Context(), version, videoAnalysis, files, -1)
	p.waitFetches(t)
	if calls := fetcher.count(); calls != 2 {
		t.Errorf("%d downloads after the hour", calls)
	}
	if reads := p.reads(); !slices.Equal(reads, []string{"fin"}) {
		t.Errorf("read in %v", reads)
	}
	if missing := p.MissingLanguages(t.Context()); len(missing) != 0 {
		t.Errorf("missing languages once downloaded: %+v", missing)
	}
}

// await waits for c to tell, failing t when it does not within a minute,
// as waitReads.
func await(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	select {
	case <-c:
	case <-ctx.Done():
		t.Fatalf("%s did not happen", what)
	}
}

// A track whose read found its language missing just before its data
// arrived, too late to wait for it, is read: with the setting on, it
// starts no second download, and with it off, it is read without Health
// being looked at.
func TestATrackCheckedJustBeforeItsDataArrivesIsRead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		download bool
	}{{"setting on", true}, {"setting off", false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tessdata := t.TempDir()
			disk := &localDisk{dir: t.TempDir(), states: map[string]string{}}
			// The read is paused once it has Tesseract's languages,
			// without Finnish, until resumed, by the end of the test at
			// the latest.
			checked, resume := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			var lists atomic.Int32
			reader := &fakeReader{languages: []string{"eng"}, dir: tessdata, listed: func([]string) {
				if lists.Add(1) == 1 {
					close(checked)
					<-resume
				}
			}}
			p := newPictureService(t, migrated.New(t), reader, disk, "ffmpeg")
			t.Cleanup(release)
			p.set(func(s *accounts.Settings) { s.SubtitleOCRDownload = tc.download })
			fetcher := &fakeFetcher{dir: tessdata}
			p.FetchLanguagesWith(fetcher)
			version := library.Version{ID: accounts.ID{1}, URL: p.base + "video.mkv"}
			p.queueTracks(t.Context(), version, videoAnalysis, finnishFiles(t, p, "a.sup"), -1)
			await(t, checked, "the track's check of its language")
			// The data arrives from Health meanwhile.
			if err := p.DownloadLanguage(t.Context(), "fin"); err != nil {
				t.Fatal(err)
			}
			release()
			p.waitFetches(t)
			if reads := p.reads(); !slices.Equal(reads, []string{"fin"}) {
				t.Errorf("read in %v", reads)
			}
			if calls := fetcher.count(); calls != 1 {
				t.Errorf("%d downloads", calls)
			}
		})
	}
}
