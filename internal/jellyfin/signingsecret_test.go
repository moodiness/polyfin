package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/database"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/secrets"
)

// With POLYFIN_SECRET_KEY, Polyfin signs with what the key derives from
// the secret its database stores: a grant signed with the stored secret
// alone, for a user and a version of the database, plays nothing, nor
// does one signed with what another key derives from the same stored
// secret, while the play sessions and media addresses the server gives
// play. Without a key, the stored secret signs, as it did.
func TestWithAKeyTheStoredSecretAloneSignsNoGrant(t *testing.T) {
	t.Parallel()
	key, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	another, err := secrets.New(bytes.Repeat([]byte{8}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		box    *secrets.Box
		stored int
	}{
		"with a key":    {key, http.StatusUnauthorized},
		"without a key": {nil, http.StatusPartialContent},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := playingOn(t, newServerSealing(t, 10, "ffprobe-not-installed", "ffmpeg-not-installed", nil, test.box))
			stored, err := database.Secret(t.Context(), p.pool)
			if err != nil || len(stored) != 32 {
				t.Fatalf("the stored secret: %d bytes, %v", len(stored), err)
			}
			version := p.versions[1].ID
			stream := p.url + "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + version.String()
			fetch := func(target string) int {
				t.Helper()
				request, _ := http.NewRequest(http.MethodGet, target, nil)
				request.Header.Set("Range", "bytes=0-3")
				response, err := testClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, _ := io.ReadAll(response.Body)
				if response.StatusCode == http.StatusPartialContent && string(body) != "\x1a\x45\xdf\xa3" {
					t.Fatalf("%s: %q, want the start of the file", target, body)
				}
				return response.StatusCode
			}

			// What the server signs plays.
			status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
				map[string]any{"MediaSourceId": version.String()})
			var info playbackInfoResponse
			if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil {
				t.Fatalf("playback info: %d %s", status, data)
			}
			for what, target := range map[string]string{
				"play session": stream + "&playSessionId=" + info.PlaySessionId,
				"media path":   info.MediaSources[0].Path,
			} {
				if got := fetch(target); got != http.StatusPartialContent {
					t.Errorf("the server's own %s: %d", what, got)
				}
			}

			// A grant tells when it was signed: one that does not is
			// refused whoever signed it.
			grant := playback.Grant{Version: version, User: p.user.ID, Signed: time.Now()}
			signed := playback.NewSigner(stored).Sign(grant)
			if got := fetch(stream + "&playSessionId=" + signed); got != test.stored {
				t.Errorf("a grant signed with the stored secret: %d, want %d", got, test.stored)
			}
			signed = playback.NewSigner(another.SigningSecret(stored)).Sign(grant)
			if got := fetch(stream + "&playSessionId=" + signed); got != http.StatusUnauthorized {
				t.Errorf("a grant signed with another key and the stored secret: %d", got)
			}
		})
	}
}
