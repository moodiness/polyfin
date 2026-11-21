package jellyfin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// changingArt is an artwork server whose pictures change behind the same
// address. It gives an ETag with each picture when validators is set,
// and answers 304 to a request that names it, counting the requests, the
// 304s and the picture bytes it sent.
type changingArt struct {
	*httptest.Server
	validators bool
	mu         sync.Mutex
	pictures   map[string][]byte
	requests   int
	unchanged  int
	sent       int
}

func newChangingArt(t *testing.T, validators bool) *changingArt {
	a := &changingArt{validators: validators, pictures: map[string][]byte{}}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.requests++
		picture, ok := a.pictures[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if a.validators {
			sum := sha256.Sum256(picture)
			etag := `"` + hex.EncodeToString(sum[:8]) + `"`
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				a.unchanged++
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		w.Header().Set("Content-Type", "image/jpeg")
		a.sent += len(picture)
		_, _ = w.Write(picture)
	}))
	t.Cleanup(a.Close)
	return a
}

// show has the server answer picture at path.
func (a *changingArt) show(path string, picture []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pictures[path] = picture
}

// counts returns the requests, the 304s and the picture bytes sent.
func (a *changingArt) counts() (requests, unchanged, sent int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.requests, a.unchanged, a.sent
}

// imageHandlerIn is clockedImageHandler keeping its images in dir.
func imageHandlerIn(dir string, clock *artClock) *Handler {
	h := &Handler{Options: Options{Stremio: stremio.NewClient("test"), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	h.images.now = clock.Now
	h.images.open(dir, h.Logger)
	return h
}

// ask asks h for the image at target as an app does, and waits for the
// server asked in the background, if it was.
func ask(t *testing.T, h *Handler, target string) artwork {
	t.Helper()
	image := askNow(t, h, target)
	h.images.refreshes.Wait()
	return image
}

// askNow is ask, not waiting for the server asked in the background.
func askNow(t *testing.T, h *Handler, target string) artwork {
	t.Helper()
	image, err := h.fetchArtwork(t.Context(), target, false, false)
	if err != nil {
		t.Error(err)
	}
	return image
}

// A picture that changes behind its address is served as kept until the
// max age, then asked of its server once: the new picture gets a tag of
// its own, which a restart keeps, and resized copies of their own.
func TestChangedArtworkGetsANewTagAfterTheMaxAge(t *testing.T) {
	t.Parallel()
	for _, validators := range []bool{true, false} {
		t.Run(fmt.Sprint("validators=", validators), func(t *testing.T) {
			t.Parallel()
			art := newChangingArt(t, validators)
			before, after := jpegOf(t, 40, 60), jpegOf(t, 60, 90)
			art.show("/rated.jpg", before)
			target := art.URL + "/rated.jpg"
			clock := &artClock{now: time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)}
			dir := t.TempDir()
			h := imageHandlerIn(dir, clock)

			ask(t, h, target)
			tag := library.ImageTag(target)
			resized := resizedKey(target, 240)
			art.show("/rated.jpg", after)
			clock.advance(imageMaxAge - time.Second)
			if image := ask(t, h, target); !bytes.Equal(image.body, before) {
				t.Error("the picture kept was not served within the max age")
			}
			if requests, _, _ := art.counts(); requests != 1 {
				t.Errorf("within the max age: %d requests", requests)
			}

			clock.advance(time.Second)
			// The request past the max age gets the picture kept at once.
			if image := ask(t, h, target); !bytes.Equal(image.body, before) {
				t.Error("the request past the max age waited for the new picture")
			}
			if requests, _, _ := art.counts(); requests != 2 {
				t.Errorf("past the max age: %d requests", requests)
			}
			if library.ImageTag(target) == tag {
				t.Fatal("the changed picture kept its tag")
			}
			if resizedKey(target, 240) == resized {
				t.Error("the changed picture shares its resized copies with the one before")
			}
			if image := ask(t, h, target); !bytes.Equal(image.body, after) {
				t.Error("the new picture is not served")
			}
			if requests, _, _ := art.counts(); requests != 2 {
				t.Errorf("once checked: %d requests", requests)
			}

			// A restart gives the same tag, read from the disk.
			changed := library.ImageTag(target)
			library.StampImage(target, "")
			imageHandlerIn(dir, clock)
			if library.ImageTag(target) != changed {
				t.Error("a restart lost the tag of the changed picture")
			}
		})
	}
}

// A picture that did not change keeps its tag: its server, given its
// ETag, answers 304 without the picture; one that gives none sends it
// again.
func TestUnchangedArtworkKeepsItsTag(t *testing.T) {
	t.Parallel()
	for _, validators := range []bool{true, false} {
		t.Run(fmt.Sprint("validators=", validators), func(t *testing.T) {
			t.Parallel()
			art := newChangingArt(t, validators)
			picture := jpegOf(t, 40, 60)
			art.show("/poster.jpg", picture)
			target := art.URL + "/poster.jpg"
			clock := &artClock{now: time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)}
			dir := t.TempDir()
			h := imageHandlerIn(dir, clock)
			ask(t, h, target)
			tag := library.ImageTag(target)

			clock.advance(imageMaxAge)
			// Apps asking at once ask the server once.
			var wg sync.WaitGroup
			for range 5 {
				wg.Go(func() { askNow(t, h, target) })
			}
			wg.Wait()
			h.images.refreshes.Wait()
			requests, unchanged, sent := art.counts()
			if requests != 2 {
				t.Errorf("%d requests", requests)
			}
			if validators && (unchanged != 1 || sent != len(picture)) {
				t.Errorf("with an ETag: %d answers 304, %d bytes sent, want 1 and %d", unchanged, sent, len(picture))
			}
			if !validators && sent != 2*len(picture) {
				t.Errorf("without an ETag: %d bytes sent, want %d", sent, 2*len(picture))
			}
			if library.ImageTag(target) != tag {
				t.Error("an unchanged picture got a new tag")
			}

			// Once checked, it is served for another max age, after a
			// restart too.
			clock.advance(imageMaxAge - time.Second)
			if image := ask(t, imageHandlerIn(dir, clock), target); !bytes.Equal(image.body, picture) {
				t.Error("the picture kept was not served after a restart")
			}
			if requests, _, _ := art.counts(); requests != 2 {
				t.Errorf("after a restart: %d requests", requests)
			}
		})
	}
}

// homeScreenDay asks a home screen's images every half hour for a day,
// as an app with no cache of its own does, a fifth of them changing at
// noon, and returns what their server answered: requests, 304s and
// picture bytes, and how many of the images got a new tag.
func homeScreenDay(t *testing.T, art *changingArt, h *Handler, clock *artClock, images int) (requests, unchanged, sent, retagged int) {
	pictures := [2][]byte{jpegOf(t, 40, 60), jpegOf(t, 60, 90)}
	tags := make([]string, images)
	for i := range images {
		art.show(fmt.Sprintf("/%d.jpg", i), pictures[0])
		tags[i] = library.ImageTag(fmt.Sprintf("%s/%d.jpg", art.URL, i))
	}
	for half := 0; half <= 48; half++ {
		if half == 24 {
			for i := 0; i < images; i += 5 {
				art.show(fmt.Sprintf("/%d.jpg", i), pictures[1])
			}
		}
		for i := range images {
			ask(t, h, fmt.Sprintf("%s/%d.jpg", art.URL, i))
		}
		clock.advance(30 * time.Minute)
	}
	for i := range images {
		if library.ImageTag(fmt.Sprintf("%s/%d.jpg", art.URL, i)) != tags[i] {
			retagged++
		}
	}
	requests, unchanged, sent = art.counts()
	return requests, unchanged, sent, retagged
}

// An app that keeps an image and asks whether it changed, answered 304
// by Polyfin, still asks for it: past the max age its server is asked
// too, once, even after a restart, when the image was read from disk.
func TestArtworkKeptByAnAppIsChecked(t *testing.T) {
	t.Parallel()
	art := newChangingArt(t, true)
	art.show("/kept.jpg", jpegOf(t, 40, 60))
	target := art.URL + "/kept.jpg"
	clock := &artClock{now: time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)}
	dir := t.TempDir()
	ask(t, imageHandlerIn(dir, clock), target)
	h := imageHandlerIn(dir, clock)
	h.refreshKept(t.Context(), target, false)
	h.images.refreshes.Wait()
	if requests, _, _ := art.counts(); requests != 1 {
		t.Errorf("within the max age: %d requests", requests)
	}
	clock.advance(imageMaxAge)
	for range 2 {
		h.refreshKept(t.Context(), target, false)
		h.images.refreshes.Wait()
	}
	if requests, unchanged, _ := art.counts(); requests != 2 || unchanged != 1 {
		t.Errorf("past the max age: %d requests, %d answered 304", requests, unchanged)
	}
}

// An image kept before check times were is not asked about at its first
// request, which gives it a check time: it is asked about a max age
// later, once.
func TestArtworkKeptBeforeChecksIsCheckedADayLater(t *testing.T) {
	t.Parallel()
	art := newChangingArt(t, true)
	picture := jpegOf(t, 40, 60)
	art.show("/old.jpg", picture)
	target := art.URL + "/old.jpg"
	dir := t.TempDir()
	if _, err := writeImage(dir, fileName(target), artwork{body: picture, contentType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	clock := &artClock{now: time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)}
	h := imageHandlerIn(dir, clock)
	ask(t, h, target)
	clock.advance(imageMaxAge - time.Second)
	ask(t, imageHandlerIn(dir, clock), target)
	if requests, _, _ := art.counts(); requests != 0 {
		t.Errorf("within a max age of the first request: %d requests", requests)
	}
	clock.advance(time.Second)
	ask(t, h, target)
	ask(t, h, target)
	if requests, _, sent := art.counts(); requests != 1 || sent != len(picture) {
		t.Errorf("past it: %d requests, %d bytes", requests, sent)
	}
}

// A home screen of 60 images asked every half hour for a day costs each
// image one download, then one request once the max age passed: 304 for
// those that did not change, the picture for those that did, which alone
// get a new tag.
func TestArtworkTrafficOverADay(t *testing.T) {
	t.Parallel()
	art := newChangingArt(t, true)
	clock := &artClock{now: time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)}
	h := imageHandlerIn(t.TempDir(), clock)
	requests, unchanged, sent, retagged := homeScreenDay(t, art, h, clock, 60)
	want := 60*len(jpegOf(t, 40, 60)) + 12*len(jpegOf(t, 60, 90))
	t.Logf("%d requests, %d answered 304, %d bytes, %d new tags", requests, unchanged, sent, retagged)
	if requests != 120 || unchanged != 48 || sent != want || retagged != 12 {
		t.Errorf("%d requests, %d answered 304, %d bytes, %d new tags; want 120, 48, %d, 12", requests, unchanged, sent, retagged, want)
	}
}
