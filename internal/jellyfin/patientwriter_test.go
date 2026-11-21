package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/plays"
)

// patientRefusal checks that a patient answer is spaces, then a refusal
// for NoCompatibleStream alone, with a 200.
func patientRefusal(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	body := recorder.Body.String()
	var answer noCompatibleStream
	if recorder.Code != http.StatusOK || json.Unmarshal([]byte(body), &answer) != nil || answer.ErrorCode != "NoCompatibleStream" ||
		!strings.HasPrefix(strings.TrimLeft(body, " "), "{") || !strings.HasSuffix(body, "}\n") {
		t.Fatalf("answer: %d %q", recorder.Code, body)
	}
}

// A refusal written while a space is due waits for the goroutine sending
// the spaces to stop and return: the space comes first, then the
// refusal, and no space after it.
func TestPatientWriterRefusalWhileASpaceIsDue(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	ticks := make(chan time.Time)
	halted := false
	p := newPatientWriterTicking(recorder, ticks, func() { halted = true })
	// The goroutine took the tick: it sends the status and a space while
	// the refusal is written.
	ticks <- time.Time{}
	(&Handler{}).refusePlay(p, httptest.NewRequest(http.MethodPost, "/Items/x/PlaybackInfo", nil), accounts.User{}, library.Item{},
		"NoCompatibleStream", plays.FailedNoStream)
	patientRefusal(t, recorder)
	if !strings.HasPrefix(recorder.Body.String(), " ") || recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("the space and the status before the refusal: %q %v", recorder.Body.String(), recorder.Header())
	}
	select {
	case <-p.ended:
	default:
		t.Error("the goroutine sending the spaces is still running")
	}
	select {
	case ticks <- time.Time{}:
		t.Error("a space was due after the refusal")
	default:
	}
	if !halted {
		t.Error("the ticks were not stopped")
	}
	// A refusal before any space sends the handler's headers and status.
	recorder = httptest.NewRecorder()
	p = newPatientWriterTicking(recorder, make(chan time.Time), func() {})
	writeJSON(p, http.StatusOK, noCompatibleStream{MediaSources: []MediaSourceInfo{}, ErrorCode: "NoCompatibleStream"})
	patientRefusal(t, recorder)
	if strings.HasPrefix(recorder.Body.String(), " ") || recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("a refusal before any space: %q %v", recorder.Body.String(), recorder.Header())
	}
}

// Spaces due at once, as fast as the clock goes, race the refusal the
// handler writes: under the race detector, every access to the response
// must be ordered. Each round ends in valid JSON.
func TestPatientWriterRefusalRacingItsSpaces(t *testing.T) {
	t.Parallel()
	for range 200 {
		recorder := httptest.NewRecorder()
		p := newPatientWriter(recorder, time.Nanosecond)
		(&Handler{}).refusePlay(p, httptest.NewRequest(http.MethodPost, "/Items/x/PlaybackInfo", nil), accounts.User{}, library.Item{},
			"NoCompatibleStream", plays.FailedNoStream)
		p.stop()
		patientRefusal(t, recorder)
	}
}
