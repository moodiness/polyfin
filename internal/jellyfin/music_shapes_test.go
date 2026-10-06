package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
)

// listening installs a music addon for the server, whose catalog rows
// become libraries, and signs a listener in: their token and identifier,
// and the libraries' identifiers by name.
func listening(t *testing.T, s testServer, addonURL string) (token, user string, views map[string]string) {
	t.Helper()
	if _, err := s.addons.Install(t.Context(), addons.Shared(), addonURL, false); err != nil {
		t.Fatal(err)
	}
	listener := s.user("listener", nil)
	token = s.signIn("listener", "web")
	var result QueryResult
	s.get(t, "/UserViews", token, &result)
	views = map[string]string{}
	for _, view := range result.Items {
		views[view.Name] = view.Id
	}
	return token, listener.ID.String(), views
}

// musicShapes lists what Polyfin cannot match in the music fixtures.
var musicShapes = shapeRules{
	dynamic: []string{"ImageTags", "ImageBlurHashes", "ProviderIds"},
	absent: map[string][]string{
		// Polyfin has no files.
		"Path": {"*"},
		// Addons give no genres per track or album, nor people: Jellyfin read
		// them from the files' tags.
		"Genres": {"music/song", "music/album"}, "GenreItems": {"music/song", "music/album"}, "People": {"music/song", "music/album"},
		// The size of a remote catalog row is unknown, and Jellyfin's root
		// folder holds the libraries; Polyfin has none.
		"ChildCount": {"music/views"}, "ParentId": {"music/views", "music/artist", "music/album"},
		// Jellyfin found the artists' MusicBrainz pages.
		"ExternalUrls": {"music/artist"},
		// Addons give a track's number and year on its album's page only:
		// those listed elsewhere, top tracks and search results, have none.
		"IndexNumber":    {"music/artist-songs", "music/search-songs", "music/search-hints"},
		"ProductionYear": {"music/artist-songs", "music/search-songs", "music/search-hints"},
		"PremiereDate":   {"music/artist-songs", "music/search-songs"},
		// A song is described from its addon's stream reply, which gives its
		// codec, container, sample rate and bit depth but not its channels,
		// bitrate or size: Polyfin does not probe what it need not.
		"Bitrate": {"music/song", "music/playback-info"}, "Size": {"music/song", "music/playback-info"},
		"BitRate": {"music/song", "music/playback-info"}, "Channels": {"music/song", "music/playback-info"},
		"ChannelLayout": {"music/song", "music/playback-info"}, "TimeBase": {"music/song", "music/playback-info"},
	},
	returned: map[string][]string{
		// The addon's songs and artists have pictures; the recorded songs had
		// theirs only in their album's folder, and the artists none.
		"PrimaryImageAspectRatio": {"music/songs", "music/album-songs", "music/song", "music/search-songs", "music/instant-mix",
			"music/artist-songs", "music/search-hints", "music/artist", "music/artist-by-name"},
		"PrimaryImageTag": {"music/search-hints"},
		// The addon's artist has a biography.
		"Overview": {"music/artist", "music/artist-by-name"},
		// Polyfin lists songs' backdrops (none) whatever image types are
		// asked; Jellyfin leaves them out when Backdrop is not asked.
		"BackdropImageTags": {"music/songs", "music/artist-songs"},
	},
}

// TestMusicResponsesMatchJellyfin compares the answers of music listings,
// details, search, instant mixes and playback with those recorded from a
// Jellyfin 12.2 music library by scripts/jellyfin-fixtures-music.sh, with
// the same requests.
func TestMusicResponsesMatchJellyfin(t *testing.T) {
	s := newTestServer(t, 10)
	addon := newFakeEclipse(t, "")
	token, user, views := listening(t, s, addon.url)
	music := views["New Releases"]
	first := func(path string) string {
		var page QueryResult
		s.get(t, path, token, &page)
		if len(page.Items) == 0 {
			t.Fatalf("nothing in %s", path)
		}
		return page.Items[0].Id
	}
	album := first("/Items?ParentId=" + music + "&IncludeItemTypes=MusicAlbum&Recursive=true&SortBy=SortName&SortOrder=Descending")
	song := first("/Items?ParentId=" + album)
	artist := first("/Artists?ParentId=" + music)

	list := "/Users/" + user + "/Items?SortBy=SortName&SortOrder=Ascending&Recursive=true&Fields=PrimaryImageAspectRatio,SortName" +
		"&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Banner,Thumb&StartIndex=0&Limit=100&ParentId=" + music
	artists := "SortBy=SortName&SortOrder=Ascending&Recursive=true&Fields=PrimaryImageAspectRatio,SortName&ImageTypeLimit=1" +
		"&EnableImageTypes=Primary,Backdrop,Banner,Thumb&StartIndex=0&Limit=100&ParentId=" + music + "&userId=" + user
	search := "&Recursive=true&Limit=24&Fields=PrimaryImageAspectRatio,CanDelete,MediaSourceCount&ImageTypeLimit=1&EnableTotalRecordCount=false"
	for fixture, path := range map[string]string{
		"albums": list + "&IncludeItemTypes=MusicAlbum",
		"songs": "/Users/" + user + "/Items?SortBy=Album,SortName&SortOrder=Ascending&IncludeItemTypes=Audio&Recursive=true" +
			"&Fields=AudioInfo,ParentId&StartIndex=0&ImageTypeLimit=1&EnableImageTypes=Primary&Limit=100&ParentId=" + music,
		"album-artists":  "/Artists/AlbumArtists?" + artists,
		"artists":        "/Artists?" + artists,
		"artist-by-name": "/Artists/" + url.PathEscape("Tone Quartet") + "?userId=" + user,
		"latest": "/Users/" + user + "/Items/Latest?IncludeItemTypes=Audio&Limit=16&Fields=PrimaryImageAspectRatio&ParentId=" + music +
			"&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Thumb",
		"album":       "/Users/" + user + "/Items/" + album,
		"album-songs": "/Users/" + user + "/Items?ParentId=" + album + "&Fields=ItemCounts,PrimaryImageAspectRatio,CanDelete,MediaSourceCount&SortBy=ParentIndexNumber,IndexNumber,SortName",
		"artist":      "/Users/" + user + "/Items/" + artist,
		"artist-albums": "/Users/" + user + "/Items?SortOrder=Descending,Descending,Ascending&IncludeItemTypes=MusicAlbum&Recursive=true" +
			"&Fields=ParentId,PrimaryImageAspectRatio,ParentId&ImageTypeLimit=1&EnableImageTypes=Primary,Backdrop,Thumb" +
			"&SortBy=PremiereDate,ProductionYear,SortName&ArtistIds=" + artist,
		"artist-songs": "/Users/" + user + "/Items?SortBy=SortName&SortOrder=Ascending&IncludeItemTypes=Audio&Recursive=true" +
			"&Fields=AudioInfo,ParentId&Limit=100&StartIndex=0&ImageTypeLimit=1&EnableImageTypes=Primary&ArtistIds=" + artist,
		"song":           "/Users/" + user + "/Items/" + song,
		"search-songs":   "/Users/" + user + "/Items?searchTerm=wind&IncludeItemTypes=Audio" + search,
		"search-albums":  "/Users/" + user + "/Items?searchTerm=sine&IncludeItemTypes=MusicAlbum" + search,
		"search-artists": "/Artists?userId=" + user + "&searchTerm=tone&Limit=24&Fields=PrimaryImageAspectRatio,CanDelete,MediaSourceCount&ImageTypeLimit=1&EnableTotalRecordCount=false",
		"search-hints":   "/Search/Hints?userId=" + user + "&searchTerm=wind&includeItemTypes=Audio,MusicAlbum,MusicArtist&limit=10",
		"instant-mix":    "/Items/" + song + "/InstantMix?userId=" + user + "&limit=20&Fields=PrimaryImageAspectRatio",
		"music-genres":   "/MusicGenres?SortBy=SortName&SortOrder=Ascending&Recursive=true&Fields=PrimaryImageAspectRatio,ItemCounts&StartIndex=0&ParentId=" + music + "&userId=" + user,
	} {
		status, body := s.call(http.MethodGet, path, app("web", token), nil)
		if status != http.StatusOK {
			t.Errorf("%s: %d %s", fixture, status, body)
			continue
		}
		matchesFixture(t, "music/"+fixture, body, musicShapes)
	}

	// The music library among the views, described as Jellyfin's.
	status, body := s.call(http.MethodGet, "/UserViews?userId="+user, app("web", token), nil)
	if status != http.StatusOK {
		t.Fatalf("views: %d", status)
	}
	matchesFixture(t, "music/views", body, musicShapes)

	// A song keeps its number on its album once a search, which gives
	// none, lists it again.
	var children, found QueryResult
	s.get(t, "/Items?ParentId="+album, token, &children)
	s.get(t, "/Items?searchTerm=wind&IncludeItemTypes=Audio&Recursive=true", token, &found)
	var details BaseItemDto
	if len(found.Items) == 1 {
		s.get(t, "/Items/"+found.Items[0].Id, token, &details)
	}
	if details.IndexNumber == nil || *details.IndexNumber != 2 || details.ProductionYear == nil {
		t.Errorf("a song found by a search lost its number or year: %+v", details)
	}

	// A song without lyrics.
	status, body = s.call(http.MethodGet, "/Audio/"+song+"/Lyrics", app("web", token), nil)
	if status != http.StatusNotFound || !jsonHas(body, "title", "Not Found") {
		t.Errorf("lyrics: %d %s", status, body)
	}
	if status, body := s.call(http.MethodGet, "/Audio/"+song+"/RemoteSearch/Lyrics", app("web", token), nil); status != http.StatusOK || string(body) != "[]\n" && string(body) != "[]" {
		t.Errorf("remote lyrics: %d %s", status, body)
	}

	// PlaybackInfo for a song, as jellyfin-web on Chrome asks it.
	profile := readProfile(t, "jellyfin-web-chrome")
	status, body = s.call(http.MethodPost, "/Items/"+song+"/PlaybackInfo?userId="+user, app("web", token),
		map[string]any{"UserId": user, "MediaSourceId": song, "DeviceProfile": profile, "AutoOpenLiveStream": true, "IsPlayback": true})
	if status != http.StatusOK {
		t.Fatalf("playback info: %d %s", status, body)
	}
	matchesFixture(t, "music/playback-info", body, musicShapes)
}

// readProfile reads a recorded device profile.
func readProfile(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(playbackFixtures + "/profiles/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// jsonHas reports whether body is a JSON object whose key is value.
func jsonHas(body []byte, key, value string) bool {
	var object map[string]any
	return json.Unmarshal(body, &object) == nil && object[key] == value
}
