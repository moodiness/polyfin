package jellyfin

import (
	"net/http"
	"strings"
	"testing"
)

// The reason a PlaybackInfo body is refused for can quote a value of the
// body: the warning holds its start only, whatever the value's length.
func TestAnUnreadablePlaybackInfosReasonIsCutInTheLog(t *testing.T) {
	t.Parallel()
	p := playing(t)
	const message = "A PlaybackInfo request could not be read"
	for i, body := range []string{
		`{"MaxStreamingBitrate":` + strings.Repeat("9", 500_000) + `}`,
		`{"MaxStreamingBitrate":"` + strings.Repeat("é", 250_000) + `"}`,
	} {
		status, answer := p.postRaw("/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), body)
		if status != http.StatusBadRequest || !strings.Contains(string(answer), "The JSON value could not be converted.") {
			t.Fatalf("body %d: %d %s", i, status, answer)
		}
		logged := p.warnings.attrsOf(message)
		if len(logged) != i+1 {
			t.Fatalf("body %d: logged %d times", i, len(logged))
		}
		attrs := logged[i]
		if attrs["app"] != "Test App" || attrs["device"] != "tv" || !strings.HasPrefix(attrs["error"], "strconv.ParseInt: parsing ") {
			t.Errorf("body %d: logged %.200v", i, attrs)
		}
		if reason := attrs["error"]; len(reason) > maxLogged+len("…") || !strings.HasSuffix(reason, "…") {
			t.Errorf("body %d: the reason is %d bytes: %.100s", i, len(reason), reason)
		}
	}
}
