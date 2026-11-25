package jellyfin

import (
	"net/http"
	"strings"
	"testing"
)

// A channel gives its parent, the Live TV view, in a listing that asks for
// ParentId, as in its details and as Jellyfin's channels do; not unasked.
func TestListedChannelsGiveTheirParentWhenAsked(t *testing.T) {
	t.Parallel()
	s, token, _ := tuned(t, newTVAddon(t, false, ""))
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) == 0 || channels.Items[0].ParentId != "" {
		t.Fatalf("channels listed unasked: %+v", channels.Items)
	}
	channel := channels.Items[0].Id
	for _, path := range []string{"/Items?ids=" + channel + "&Fields=ParentId", "/LiveTv/Channels?Fields=ParentId"} {
		var page QueryResult
		if status := s.get(t, path, token, &page); status != http.StatusOK || len(page.Items) == 0 || page.Items[0].ParentId != liveTvViewID.String() {
			t.Errorf("%s: %d %+v", path, status, page.Items)
		}
	}
	var details BaseItemDto
	if s.get(t, "/Items/"+channel, token, &details); details.ParentId != liveTvViewID.String() {
		t.Errorf("details: parent %q", details.ParentId)
	}
}

// A channel listed with Fields=MediaSources or MediaStreams carries the
// placeholder source its details start with, as Jellyfin's listings carry
// its channels' sources; the lineup is never read for a listing.
func TestListedChannelsGiveAPlaceholderSource(t *testing.T) {
	t.Parallel()
	s, token, _ := tuned(t, newTVAddon(t, false, ""))
	for _, fields := range []string{"MediaSources", "MediaStreams", "MediaSources,MediaStreams"} {
		var page QueryResult
		s.get(t, "/LiveTv/Channels?Fields="+fields, token, &page)
		if len(page.Items) == 0 {
			t.Fatalf("no channel listed")
		}
		channel := page.Items[0]
		wantSources, wantStreams := strings.Contains(fields, "MediaSources"), strings.Contains(fields, "MediaStreams")
		if sources := channel.MediaSources; wantSources != (sources != nil) ||
			sources != nil && (len(*sources) != 1 || (*sources)[0].Type != "Placeholder" || (*sources)[0].Id != channel.Id) {
			t.Errorf("Fields=%s: sources %+v", fields, sources)
		}
		if streams := channel.MediaStreams; wantStreams != (streams != nil) || streams != nil && len(*streams) != 0 {
			t.Errorf("Fields=%s: streams %+v", fields, streams)
		}
	}
	var page QueryResult
	if s.get(t, "/LiveTv/Channels", token, &page); len(page.Items) == 0 || page.Items[0].MediaSources != nil {
		t.Errorf("unasked: %+v", page.Items)
	}
}
