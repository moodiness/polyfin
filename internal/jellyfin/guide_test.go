package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
)

// A catalog's XMLTV guide feeds the programme listings, the programme
// each channel airs now, the programmes' details and their images, for the
// channels without Native EPG programmes: a channel with them keeps them.
func TestXMLTVGuideFillsChannelsWithoutNativeEPG(t *testing.T) {
	addon := newTVAddon(t, true, "")
	s, token, _ := tuned(t, addon)
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(d).Format("20060102150405 -0700") }
	body := fmt.Sprintf(`<?xml version="1.0" encoding="ISO-8859-1"?>
<tv>
  <channel id="one-x"><display-name>One</display-name></channel>
  <channel id="two-x"><display-name>FR: TWO | HD</display-name></channel>
  <programme channel="one-x" start="%s" stop="%s"><title>Guide One</title></programme>
  <programme channel="two-x" start="%s" stop="%s">
    <title>Talk Show</title><sub-title>Pilot</sub-title><desc>Caf%s.</desc><icon src="ICON"/>
    <category>Sports</category><episode-num system="xmltv_ns">1.4.</episode-num>
  </programme>
  <programme channel="two-x" start="%s" stop="%s"><title>Late Movie</title><category>Movie</category></programme>
</tv>`, at(-time.Hour), at(time.Hour), at(-time.Hour), at(time.Hour), "\xe9", at(time.Hour), at(3*time.Hour))
	guide := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/talk.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
			return
		}
		_, _ = io.WriteString(w, strings.ReplaceAll(body, "ICON", "http://"+r.Host+"/talk.png"))
	}))
	t.Cleanup(guide.Close)
	libraries, err := s.addons.Libraries(t.Context(), addons.Shared())
	if err != nil || len(libraries) != 1 {
		t.Fatal(libraries, err)
	}
	key := addons.LibraryKey{AddonID: libraries[0].AddonID, CatalogType: "tv", CatalogID: "channels"}
	if _, err := s.addons.SetGuide(t.Context(), addons.Shared(), key, guide.URL+"/epg"); err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}

	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 2 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	one, two := channels.Items[0], channels.Items[1]
	if one.CurrentProgram == nil || one.CurrentProgram.Name != "Now" {
		t.Errorf("a channel with Native EPG programmes keeps them: %+v", one.CurrentProgram)
	}
	if two.CurrentProgram == nil || two.CurrentProgram.Name != "Talk Show" {
		t.Errorf("a channel without takes its guide's: %+v", two.CurrentProgram)
	}

	var programs QueryResult
	s.get(t, "/LiveTv/Programs?channelIds="+two.Id+"&Fields=Overview", token, &programs)
	if len(programs.Items) != 2 || programs.TotalRecordCount != 2 {
		t.Fatalf("programmes of the guided channel: %+v", programs.Items)
	}
	talk := programs.Items[0]
	if talk.Name != "Talk Show" || talk.EpisodeTitle != "Pilot" || talk.IsSports == nil || talk.IsMovie != nil ||
		talk.ParentIndexNumber == nil || *talk.ParentIndexNumber != 2 || talk.IndexNumber == nil || *talk.IndexNumber != 5 ||
		talk.Overview == nil || *talk.Overview != "Café." || *talk.ChannelId != two.Id {
		t.Errorf("guide programme: %+v", talk)
	}
	// Its image is relayed from the guide's address, which no item keeps.
	if status, header, _ := s.send(http.MethodGet, "/Items/"+talk.Id+"/Images/Primary?tag="+talk.ImageTags["Primary"], "", "", ""); talk.ImageTags["Primary"] == "" ||
		status != http.StatusOK || header.Get("Content-Type") != "image/png" {
		t.Errorf("the programme's image: tag %q, %d %s", talk.ImageTags["Primary"], status, header.Get("Content-Type"))
	}
	s.get(t, "/LiveTv/Programs?IsMovie=true", token, &programs)
	if names := programNames(programs); !slices.Equal(names, []string{"Next", "Late Movie"}) {
		t.Errorf("movies of both guides: %q", names)
	}
	s.get(t, "/LiveTv/Programs/Recommended?IsAiring=true", token, &programs)
	if names := programNames(programs); !slices.Equal(names, []string{"Now", "Talk Show"}) {
		t.Errorf("airing now: %q", names)
	}
	status, raw := s.call(http.MethodPost, "/LiveTv/Programs", app("tv", token),
		map[string]any{"ChannelIds": []string{two.Id}, "Limit": 1, "SortBy": []string{"SortName"}, "SortOrder": []string{"Descending"}})
	if status != http.StatusOK || json.Unmarshal(raw, &programs) != nil || programs.TotalRecordCount != 2 ||
		!slices.Equal(programNames(programs), []string{"Talk Show"}) {
		t.Errorf("posted query: %d %s", status, raw)
	}
	// A search finds the guide's programmes too, by name and category.
	s.get(t, "/Items?recursive=true&searchTerm=talk&includeItemTypes=LiveTvProgram&isSports=true", token, &programs)
	if names := programNames(programs); !slices.Equal(names, []string{"Talk Show"}) {
		t.Errorf("programmes searched: %q", names)
	}

	// Upcoming, by start time, limited without a count: read no further.
	s.get(t, "/LiveTv/Programs/Recommended?IsAiring=false&HasAired=false&Limit=1&EnableTotalRecordCount=false", token, &programs)
	if names := programNames(programs); !slices.Equal(names, []string{"Next"}) && !slices.Equal(names, []string{"Late Movie"}) {
		t.Errorf("the next programme: %q", names)
	}
	s.get(t, "/LiveTv/Programs?Limit=1&EnableTotalRecordCount=false&MinStartDate="+now.Add(30*time.Minute).Format(time.RFC3339), token, &programs)
	if len(programs.Items) != 1 {
		t.Errorf("one programme of a listing: %q", programNames(programs))
	}
	// A guide's programmes are no items: they are read from the guide. The
	// two kept are the Native EPG programmes listed.
	var kept int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM items WHERE kind = 'program'").Scan(&kept); err != nil || kept != 2 {
		t.Errorf("%d programmes stored, want the 2 of Native EPG (%v)", kept, err)
	}

	for _, path := range []string{"/LiveTv/Programs/" + talk.Id, "/Items/" + talk.Id} {
		var detail BaseItemDto
		if status := s.get(t, path, token, &detail); status != http.StatusOK || detail.Name != "Talk Show" ||
			detail.EpisodeTitle != "Pilot" || detail.ChannelName != "Two" || detail.Type != "Program" {
			t.Errorf("%s: %d %+v", path, status, detail)
		}
	}
}

func programNames(result QueryResult) []string {
	names := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		names = append(names, item.Name)
	}
	return names
}
