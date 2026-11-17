package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// versionedFields are the fields a listing reads its items' known versions
// for (see versionFields), with those the home page asks for.
const versionedFields = "fields=PrimaryImageAspectRatio,DateCreated,Path,MediaSourceCount,MediaSources,MediaStreams,CanDownload,Chapters,Trickplay"

// grants are the signed grants in media sources' addresses, made anew at
// each answer: compared answers leave them out.
var grants = regexp.MustCompile(`PolyfinGrant=[^\\"&]*`)

func withoutGrants(answer string) string { return grants.ReplaceAllString(answer, "PolyfinGrant=") }

// nextUpOfEight is Next Up of eight series, of an Xtream source or an
// addon, each with an episode played.
func nextUpOfEight(t *testing.T, iptv bool) shows {
	t.Helper()
	var x shows
	if iptv {
		x = newIPTVShows(t, 8, 2, 4)
	} else {
		x = newAddonShows(t, 8, 4)
	}
	base := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	for i := range 8 {
		name := fmt.Sprintf("Show %d", i)
		if iptv {
			name = fmt.Sprintf("Show %02d", i)
		}
		x.played(t, name, "S1E1", base.Add(time.Duration(i)*time.Hour))
	}
	return x
}

// rawItems returns the items path answers, each as encoded.
func (x shows) rawItems(t *testing.T, path string) []json.RawMessage {
	t.Helper()
	var page struct{ Items []json.RawMessage }
	if status := x.get(t, path, x.token, &page); status != http.StatusOK {
		t.Fatalf("%s: %d", path, status)
	}
	return page.Items
}

// A listing whose fields read its items' versions describes them a few at
// a time, and answers as when it described them one by one: each item of
// the page is the item a page of that one item gives, field for field,
// Path and media sources included.
func TestListingsWithVersionsAnswerAsOneByOne(t *testing.T) {
	t.Parallel()
	for _, iptv := range []bool{true, false} {
		t.Run(fmt.Sprintf("iptv=%v", iptv), func(t *testing.T) {
			t.Parallel()
			x := nextUpOfEight(t, iptv)
			page := x.rawItems(t, "/Shows/NextUp?"+versionedFields)
			if len(page) != 8 {
				t.Fatalf("Next Up: %d items", len(page))
			}
			paths := 0
			for i, item := range page {
				alone := x.rawItems(t, fmt.Sprintf("/Shows/NextUp?startIndex=%d&limit=1&%s", i, versionedFields))
				if len(alone) != 1 || withoutGrants(string(alone[0])) != withoutGrants(string(item)) {
					t.Errorf("item %d:\n%s\nalone:\n%s", i, item, alone)
				}
				if strings.Contains(string(item), `"Path":`) {
					paths++
				}
			}
			if iptv && paths != len(page) {
				t.Errorf("%d of %d items with a path", paths, len(page))
			}
		})
	}
}

// Listings that read their items' versions, sent together, describe their
// items at once, each a few at a time: run with the race detector, nothing
// they share races, and each answers the same.
func TestListingsWithVersionsRunTogether(t *testing.T) {
	t.Parallel()
	x := nextUpOfEight(t, true)
	paths := []string{"/Shows/NextUp?" + versionedFields, "/Shows/NextUp?enableRewatching=true&" + versionedFields,
		"/UserItems/Resume?" + versionedFields, "/Users/" + x.member.ID.String() + "/Items?ParentId=" + x.library +
			"&Recursive=true&IncludeItemTypes=Episode&Limit=20&" + versionedFields}
	want := make([]string, len(paths))
	for i, path := range paths {
		want[i] = withoutGrants(rawBody(t, x, path))
	}
	var wg sync.WaitGroup
	got := make([][]string, 4)
	for round := range got {
		got[round] = make([]string, len(paths))
		for i, path := range paths {
			wg.Go(func() { got[round][i] = withoutGrants(rawBody(t, x, path)) })
		}
	}
	wg.Wait()
	for round := range got {
		for i, path := range paths {
			if got[round][i] != want[i] {
				t.Errorf("%s, together: %s\nalone: %s", path, got[round][i], want[i])
			}
		}
	}
}

// rawBody returns what path answers, with its status, from any goroutine:
// an error is told as the body.
func rawBody(t *testing.T, x shows, path string) string {
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, x.url+path, nil)
	if err != nil {
		return err.Error()
	}
	request.Header.Set("Authorization", app("tv", x.token))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err.Error()
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("%d %s", response.StatusCode, body)
}
