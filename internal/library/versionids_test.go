package library

import (
	"encoding/json"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// The version of a stream an addon tells a size of keeps its identifier
// now that the size is decoded into 64 bits on every target: the
// identifiers below are those Polyfin gave on amd64 and arm64 before,
// where the size was decoded into a 64-bit int already, so that no title
// of an existing library sees its versions change.
func TestVersionIDsOfSizedStreamsAreKept(t *testing.T) {
	t.Parallel()
	item := accounts.ID{0x10, 0x86, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
	for _, c := range []struct{ size, identity, id string }{
		{`4e9`, "Movie.2008.2160p.mkv|4000000000", "bca767c79d44a073e659202f7de21925"},
		{`4000000000`, "Movie.2008.2160p.mkv|4000000000", "bca767c79d44a073e659202f7de21925"},
		{`"4000000000"`, "Movie.2008.2160p.mkv|4000000000", "bca767c79d44a073e659202f7de21925"},
		{`1234567890`, "Movie.2008.2160p.mkv|1234567890", "e941248447e4e34381aa1cc5cd759d3b"},
		{`3.5e9`, "Movie.2008.2160p.mkv|3500000000", "6c8c629305138664ba79dc168708fd07"},
		{`"8589934592"`, "Movie.2008.2160p.mkv|8589934592", "b2d776f7f8ca497259be57d3f3151013"},
		{`7.3e9`, "Movie.2008.2160p.mkv|7300000000", "8b0ac32157a52c1c054fd61318b344e2"},
		{`1.5`, "Movie.2008.2160p.mkv|1", "23819013ff381938a0237bdd386e8b56"},
		{`0`, "https://host.example/file/token", "eaa4c3633c81e0409f8ad038fe89f87d"},
		{`null`, "https://host.example/file/token", "eaa4c3633c81e0409f8ad038fe89f87d"},
	} {
		var stream stremio.Stream
		if err := json.Unmarshal([]byte(`{"url": "https://host.example/file/token",
			"behaviorHints": {"filename": "Movie.2008.2160p.mkv", "videoSize": `+c.size+`}}`), &stream); err != nil {
			t.Fatal(err)
		}
		identity := streamIdentity(stream)
		if id := versionID(item, identity).String(); identity != c.identity || id != c.id {
			t.Errorf("videoSize %s: version %s (%s), was %s (%s)", c.size, id, identity, c.id, c.identity)
		}
	}
}
