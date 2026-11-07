package playback

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// outageHost answers every link with file, or, while down, with the same
// errorFile, as a host whose service is down answers a short video for
// every link.
type outageHost struct {
	file, errorFile []byte
	down            atomic.Bool
}

func (h *outageHost) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	data := h.file
	if h.down.Load() {
		data = h.errorFile
	}
	request := httptest.NewRequestWithContext(ctx, method, target, nil)
	request.Header = header.Clone()
	recorder := httptest.NewRecorder()
	http.ServeContent(recorder, request, "", time.Time{}, bytes.NewReader(data))
	response := recorder.Result()
	response.Request = request
	return response, nil
}

// errorFileSize is the size of the error video a host answered for every
// link while its service was down.
const errorFileSize = 1_822_520

// refusedAnswer reports whether err refuses an answer for another file.
func refusedAnswer(err error) bool {
	return errors.Is(err, source.ErrUnavailable) && source.Answer(err) == "an answer of another size than the file"
}

// With an analysis of the file kept, a host answering its error file from
// the first request is refused at the first byte, whatever its addon's
// sizes are worth, and the analysis stays kept.
func TestAKeptAnalysisRefusesAnErrorFileAtItsFirstByte(t *testing.T) {
	const size = 8_350_994_751
	host := &outageHost{errorFile: make([]byte, errorFileSize)}
	host.down.Store(true)
	s := newService(t, host, "ffprobe", nil)
	version := library.Version{ID: accounts.ID{41}, Addon: "addon", URL: "https://93.184.216.34/episode.mkv", Size: size}
	s.keepAnalysis(t.Context(), version, media.Analysis{Size: size, Duration: 43 * time.Minute}, time.Now())
	// Kept in the database, as after a restart: a play reads it first.
	s.analyses.Delete(version.ID)
	if _, ok := s.Analyzed(t.Context(), version.ID); !ok {
		t.Fatal("no analysis kept")
	}
	src := s.OpenSource(version)
	defer src.Release()
	_, err := src.ReadAt(t.Context(), make([]byte, 10), 0)
	if !refusedAnswer(err) {
		t.Fatalf("the error file: %v", err)
	}
	if known, ok := src.KnownSize(); ok {
		t.Errorf("the error file's size taken: %d", known)
	}
	// As the play's watch keeps it.
	s.fail(version, err)
	if analysis, ok := s.Analyzed(t.Context(), version.ID); !ok || analysis.Size != size {
		t.Errorf("the analysis kept: %+v, %v", analysis, ok)
	}
	if !s.Failed(version.ID) {
		t.Error("the version is not kept as failed")
	}
}

// Versions refused while their host answered an error file are tried
// again as soon as it answers a file of its own size, without waiting out
// the time a failed version is skipped.
func TestVersionsRefusedForAnErrorFileAreTriedAgainOnceTheHostRecovers(t *testing.T) {
	// Over ten times the error file: under a tenth of a file's size is
	// what an error file answers (see source.ErrorFiles).
	const size = 20 << 20
	host := &outageHost{file: make([]byte, size), errorFile: make([]byte, errorFileSize)}
	s := newService(t, host, "ffprobe", nil)
	version := func(id byte) library.Version {
		return library.Version{ID: accounts.ID{id}, Addon: "addon", URL: "https://93.184.216.34/" + string('a'+rune(id)) + ".mkv", Size: size}
	}
	read := func(v library.Version) error {
		src := s.OpenSource(v)
		defer src.Release()
		_, err := src.ReadAt(t.Context(), make([]byte, 10), 0)
		return err
	}
	// The addon's sizes proved right once.
	if err := read(version(1)); err != nil {
		t.Fatal(err)
	}
	host.down.Store(true)
	refused := []library.Version{version(2), version(3), version(4)}
	for _, v := range refused {
		err := read(v)
		if !refusedAnswer(err) {
			t.Fatalf("the version %d during the outage: %v", v.ID[0], err)
		}
		// As the play's watch keeps it.
		s.fail(v, err)
		if !s.Failed(v.ID) {
			t.Fatalf("the version %d is not kept as failed", v.ID[0])
		}
	}

	host.down.Store(false)
	if err := read(version(5)); err != nil {
		t.Fatal(err)
	}
	for _, v := range refused {
		if s.Failed(v.ID) {
			t.Errorf("the version %d is still skipped", v.ID[0])
		}
		if err := read(v); err != nil {
			t.Errorf("the version %d after the outage: %v", v.ID[0], err)
		}
	}
}

// An analysis made of another file, as of an error file before its host
// was found answering one, never refuses the file: the size its addon
// announced does not side with it, and the file is read, so that the
// analysis can be corrected or made again.
func TestAnAnalysisOfAnotherFileDoesNotRefuseTheFile(t *testing.T) {
	const size = 4 << 20
	for _, announced := range []int64{size, 0} {
		host := &outageHost{file: make([]byte, size)}
		s := newService(t, host, "ffprobe", nil)
		version := library.Version{ID: accounts.ID{51}, Addon: "addon", URL: "https://93.184.216.34/movie.mkv", Size: announced}
		s.keepAnalysis(t.Context(), version, media.Analysis{Size: errorFileSize, Duration: time.Minute}, time.Now())
		src := s.OpenSource(version)
		got, err := src.Size(t.Context())
		src.Release()
		if err != nil || got != size {
			t.Errorf("announced %d bytes: the file is %d bytes, %v", announced, got, err)
		}
	}
}
