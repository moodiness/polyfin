package jellyfin

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// members reads a JSON object's members in their order.
func members(t *testing.T, object []byte) ([]string, map[string]json.RawMessage) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(object))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		t.Fatalf("not an object: %s", object)
	}
	var keys []string
	values := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			t.Fatal(err)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, token.(string))
		values[token.(string)] = value
	}
	return keys, values
}

// listedItems reads the items of a listing, each as written.
func listedItems(t *testing.T, body []byte) []json.RawMessage {
	t.Helper()
	var listing struct{ Items []json.RawMessage }
	if err := json.Unmarshal(body, &listing); err != nil {
		t.Fatalf("%v in %s", err, body)
	}
	return listing.Items
}

// sameValues reports the members of lean whose value differs from the
// one full gives.
func sameValues(t *testing.T, what string, lean, full json.RawMessage) {
	t.Helper()
	_, leanValues := members(t, lean)
	_, fullValues := members(t, full)
	for key, value := range leanValues {
		if want, ok := fullValues[key]; ok && !bytes.Equal(value, want) {
			t.Errorf("%s: %s is %s, the full description's %s", what, key, value, want)
		}
	}
}

// jellyfin-web's guide asks for its programmes without images nor user
// data, by GET or, for many channels, by POST: they come as Jellyfin
// 12.4 describes them for that request, the members it pins, with the
// values of the full description. A programme's lean description has no
// IsFolder nor LocationType, which Jellyfin leaves out of programmes.
func TestGuideProgramsAsJellyfinDescribesThem(t *testing.T) {
	t.Parallel()
	addon := newTVAddon(t, true, "")
	s, token, user := tuned(t, addon)
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 2 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	ids := channels.Items[0].Id + "," + channels.Items[1].Id
	now := time.Now().UTC()
	minEnd, maxStart := now.Add(-30*time.Minute).Format(time.RFC3339), now.Add(24*time.Hour).Format(time.RFC3339)
	guide := "?UserId=" + user.ID.String() + "&MaxStartDate=" + url.QueryEscape(maxStart) + "&MinEndDate=" + url.QueryEscape(minEnd) +
		"&channelIds=" + ids + "&ImageTypeLimit=1&SortBy=StartDate&EnableTotalRecordCount=false"
	list := func(query string) []byte {
		t.Helper()
		status, body := s.call(http.MethodGet, "/LiveTv/Programs"+guide+query, app("tv", token), nil)
		if status != http.StatusOK {
			t.Fatalf("programmes %s: %d %s", query, status, body)
		}
		return body
	}

	lean := list("&EnableImages=false&EnableUserData=false")
	// jellyfin-web's POST, its channels joined by commas.
	status, posted := s.call(http.MethodPost, "/LiveTv/Programs", app("tv", token), map[string]any{
		"UserId": user.ID.String(), "MaxStartDate": maxStart, "MinEndDate": minEnd, "channelIds": ids, "ImageTypeLimit": 1,
		"EnableImages": false, "SortBy": "StartDate", "EnableTotalRecordCount": false, "EnableUserData": false})
	if status != http.StatusOK || !bytes.Equal(posted, lean) {
		t.Errorf("the guide's POST: %d\n%s\nthe GET:\n%s", status, posted, lean)
	}
	compareLive(t, "livetv-guided-programs", lean)
	full := list("&EnableImages=false")
	leanItems, fullItems := listedItems(t, lean), listedItems(t, full)
	if len(leanItems) != 2 || len(fullItems) != 2 {
		t.Fatalf("programmes: %s", lean)
	}
	pinned := []string{"Name", "ServerId", "Id", "ChannelId", "RunTimeTicks", "Type", "ImageBlurHashes", "MediaType", "EndDate", "StartDate"}
	for i, category := range []string{"IsNews", "IsMovie"} {
		keys, values := members(t, leanItems[i])
		if want := append(slices.Clone(pinned), category); !slices.Equal(keys, want) {
			t.Errorf("programme %d's members: %v, want %v", i, keys, want)
		}
		if string(values["ImageBlurHashes"]) != "{}" || string(values["Type"]) != `"Program"` || string(values[category]) != "true" {
			t.Errorf("programme %d: %s", i, leanItems[i])
		}
		sameValues(t, "the guide's programme", leanItems[i], fullItems[i])
	}
	if !strings.HasSuffix(string(lean), "],\"TotalRecordCount\":2,\"StartIndex\":0}\n") {
		t.Errorf("the listing's end: %s", lean)
	}

	// The fields asked add theirs; those Jellyfin's LiveTvManager removes
	// (CanDelete, CanDownload, Etag, DisplayPreferencesId) stay out.
	withFields := list("&EnableImages=false&EnableUserData=false&Fields=ChannelInfo,Overview,CanDelete,Etag")
	keys, values := members(t, listedItems(t, withFields)[0])
	want := []string{"Name", "ServerId", "Id", "ChannelId", "ChannelName", "Overview", "RunTimeTicks", "Type", "ImageBlurHashes",
		"MediaType", "EndDate", "ChannelNumber", "StartDate", "IsNews"}
	if !slices.Equal(keys, want) || string(values["ChannelName"]) != `"One"` || string(values["ChannelNumber"]) != `"1"` ||
		string(values["Overview"]) != `"The news."` {
		t.Errorf("a programme with fields: %v %s", keys, withFields)
	}

	// Every other request keeps the full description.
	for query, wanted := range map[string][]string{
		"":                      {"IsFolder", "LocationType", "UserData", "ImageTags", "BackdropImageTags"},
		"&EnableImages=false":   {"IsFolder", "LocationType", "UserData"},
		"&EnableUserData=false": {"IsFolder", "LocationType", "ImageTags", "BackdropImageTags"},
		"&EnableUserData=true":  {"IsFolder", "LocationType", "UserData", "ImageTags"},
	} {
		keys, _ := members(t, listedItems(t, list(query))[0])
		for _, key := range wanted {
			if !slices.Contains(keys, key) {
				t.Errorf("programmes%s: no %s in %v", query, key, keys)
			}
		}
	}
	status, recommended := s.call(http.MethodGet, "/LiveTv/Programs/Recommended?IsAiring=true&EnableImages=false&EnableUserData=false",
		app("tv", token), nil)
	if keys, _ := members(t, listedItems(t, recommended)[0]); status != http.StatusOK || !slices.Contains(keys, "IsFolder") {
		t.Errorf("recommended programmes: %d %s", status, recommended)
	}
}

// guideProgramme is a programme with every member a lean description
// can give, on a channel with a number and a logo, its texts needing
// JSON's escapes.
func guideProgramme(i int, channel *library.Item) library.Item {
	start := time.Date(2026, 11, 16, 12, 0, 0, 123456700, time.UTC).Add(time.Duration(i) * time.Minute)
	end := start.Add(30 * time.Minute)
	premiere := time.Date(2001, 2, 3, 0, 0, 0, 0, time.UTC)
	critic := 81.5
	return library.Item{
		ID: accounts.ID{byte(i >> 8), byte(i), 1}, Kind: library.KindProgram, ParentID: channel.ID, Channel: channel,
		Name: "Tom & Jerry <live> \"quoted\" \\ \u2028 é \xff", OriginalTitle: "Original", SortName: "Tom",
		Overview: "Line\nnext\ttab", Genres: []string{"Movie", "Kids"}, OfficialRating: "TV-PG", CustomRating: "Custom",
		CommunityRating: 7.25, CriticRating: &critic, ProductionYear: 2001, PremiereDate: &premiere,
		IndexNumber: 3, ParentIndexNumber: 2, Repeat: true, EpisodeTitle: "Episode </script>",
		Taglines: []string{"A tagline"}, Tags: []string{"tag"}, Studios: []string{"Studio"},
		ProviderIDs: map[string]string{"Imdb": "tt0000001"},
		People:      []library.Person{{Name: "Someone", ID: accounts.ID{9}, Role: "Host", Type: "Actor"}},
		Trailers:    []library.Trailer{{Name: "Trailer", URL: "https://example.com/t"}},
		Images:      library.Images{Primary: "https://example.com/p.png"},
		StartDate:   &start, EndDate: &end, Runtime: end.Sub(start),
	}
}

// fullProgramme is a programme's description in other listings without
// images nor user data, as programs writes it.
func fullProgramme(t *testing.T, h *Handler, item library.Item, fields fieldSet) json.RawMessage {
	t.Helper()
	dtos := []BaseItemDto{h.newItemDto(item, fields, false, userState{})}
	dtos[0].UserData = UserItemData{}
	body, err := json.Marshal(imagelessProgram{BaseItemDto: &dtos[0]})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// leanListing writes programmes as the guide's listing does.
func leanListing(t *testing.T, h *Handler, w http.ResponseWriter, items []library.Item, fields fieldSet) {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/LiveTv/Programs", nil)
	h.writeLeanPrograms(w, r, items, len(items)+5, 5, fields)
}

// A lean programme gives, in BaseItemDto's order, the members Jellyfin
// 12.4 sets on a programme for the fields asked, with the full
// description's values, and those only Jellyfin's gives with Jellyfin's
// values.
func TestLeanProgrammesGiveJellyfinsMembers(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	channel := &library.Item{ID: accounts.ID{7}, Kind: library.KindChannel, Name: "Channel <7>", Number: "7",
		Images: library.Images{Primary: "https://example.com/logo.png"}}
	item := guideProgramme(1, channel)
	all := "ChannelInfo,ChannelImage,Overview,Genres,People,Studios,ProviderIds,ExternalUrls,RemoteTrailers,Tags,Taglines," +
		"SortName,Settings,DateCreated,OriginalTitle,CustomRating,ParentId,PlayAccess,EnableMediaSourceDisplay,Chapters," +
		"LocalTrailerCount,SpecialFeatureCount,PrimaryImageAspectRatio,CanDelete,CanDownload,Etag,DisplayPreferencesId,IsHD"
	base := []string{"Name", "ServerId", "Id", "PremiereDate", "OfficialRating", "ChannelId", "CommunityRating", "CriticRating",
		"RunTimeTicks", "ProductionYear", "IndexNumber", "ParentIndexNumber", "Type", "ImageBlurHashes", "MediaType", "EndDate",
		"StartDate", "IsMovie", "IsKids", "IsRepeat", "EpisodeTitle"}
	for _, c := range []struct {
		fields string
		keys   []string
		// jellyfinOnly are the members the full description has not, with
		// their values.
		jellyfinOnly map[string]string
	}{
		{"", base, nil},
		{"IsHD", base, nil},
		{"ChannelInfo", []string{"Name", "ServerId", "Id", "PremiereDate", "OfficialRating", "ChannelId", "ChannelName",
			"CommunityRating", "CriticRating", "RunTimeTicks", "ProductionYear", "IndexNumber", "ParentIndexNumber", "Type",
			"ImageBlurHashes", "MediaType", "EndDate", "ChannelNumber", "StartDate", "IsMovie", "IsKids", "IsRepeat", "EpisodeTitle"}, nil},
		{all, []string{"Name", "OriginalTitle", "ServerId", "Id", "DateCreated", "SortName", "ForcedSortName", "PremiereDate",
			"ExternalUrls", "EnableMediaSourceDisplay", "OfficialRating", "ChannelId", "ChannelName", "Overview", "Taglines", "Genres",
			"CommunityRating", "CriticRating", "CustomRating", "RunTimeTicks", "PlayAccess", "ProductionYear", "IndexNumber",
			"ParentIndexNumber", "RemoteTrailers", "ProviderIds", "ParentId", "Type", "People", "Studios", "GenreItems",
			"LocalTrailerCount", "SpecialFeatureCount", "Tags", "PrimaryImageAspectRatio", "ImageBlurHashes", "Chapters", "MediaType",
			"EndDate", "LockedFields", "LockData", "ChannelNumber", "ChannelPrimaryImageTag", "StartDate", "IsMovie", "IsKids",
			"IsRepeat", "EpisodeTitle"},
			map[string]string{"OriginalTitle": `"Original"`, "ForcedSortName": `"Tom"`, "ExternalUrls": `[{"Url":"https://www.imdb.com/title/tt0000001","Name":"IMDb"}]`,
				"EnableMediaSourceDisplay": "true", "PlayAccess": `"Full"`, "RemoteTrailers": `[{"Url":"https://example.com/t","Name":"Trailer"}]`,
				"GenreItems": "", "LocalTrailerCount": "0", "SpecialFeatureCount": "0", "LockedFields": "[]", "LockData": "false",
				"ChannelPrimaryImageTag": `"` + library.ImageTag(channel.Images.Primary) + `"`}},
	} {
		fields := fieldSet{}
		for field := range strings.SplitSeq(c.fields, ",") {
			if field != "" {
				fields[strings.ToLower(field)] = true
			}
		}
		recorder := httptest.NewRecorder()
		leanListing(t, s.handler, recorder, []library.Item{item}, fields)
		lean := listedItems(t, recorder.Body.Bytes())[0]
		keys, values := members(t, lean)
		if !slices.Equal(keys, c.keys) {
			t.Errorf("Fields=%s: members %v, want %v", c.fields, keys, c.keys)
		}
		full := fullProgramme(t, s.handler, item, fields)
		sameValues(t, "Fields="+c.fields, lean, full)
		_, fullValues := members(t, full)
		for key, want := range c.jellyfinOnly {
			if _, inFull := fullValues[key]; inFull {
				continue
			}
			if want != "" && string(values[key]) != want {
				t.Errorf("Fields=%s: %s is %s, want %s", c.fields, key, values[key], want)
			}
		}
		for _, key := range keys {
			if _, inFull := fullValues[key]; !inFull {
				if _, known := c.jellyfinOnly[key]; !known {
					t.Errorf("Fields=%s: %s is not in the full description", c.fields, key)
				}
			}
		}
	}
}

// gzipWriter compresses an answer as it is written, as the server's
// compression does, and counts the writes it gets.
type gzipWriter struct {
	http.ResponseWriter
	gzip   *gzip.Writer
	writes int
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	g.writes++
	return g.gzip.Write(p)
}

// A large lean listing is written a chunk at a time, valid JSON with the
// end json.Encoder gives, and decodes alike compressed or not.
func TestLeanListingsStreamThroughCompression(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	channel := &library.Item{ID: accounts.ID{7}, Kind: library.KindChannel, Name: "Seven", Number: "7"}
	items := make([]library.Item, 3000)
	for i := range items {
		items[i] = guideProgramme(i, channel)
	}
	fields := fieldSet{"channelinfo": true}
	plain := httptest.NewRecorder()
	leanListing(t, s.handler, plain, items, fields)
	compressed := httptest.NewRecorder()
	zipped := &gzipWriter{ResponseWriter: compressed, gzip: gzip.NewWriter(compressed.Body)}
	leanListing(t, s.handler, zipped, items, fields)
	if err := zipped.gzip.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(compressed.Body)
	if err != nil {
		t.Fatal(err)
	}
	unzipped, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	body := plain.Body.Bytes()
	if !bytes.Equal(unzipped, body) {
		t.Fatal("the compressed listing differs from the plain one")
	}
	if len(body) < 4*jsonChunk || zipped.writes < len(body)/(2*jsonChunk) {
		t.Errorf("%d bytes in %d writes: not streamed by chunks", len(body), zipped.writes)
	}
	if !json.Valid(body) || body[len(body)-1] != '\n' || plain.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("not a JSON answer: %q", body[len(body)-min(len(body), 80):])
	}
	var listing struct {
		Items            []BaseItemDto
		TotalRecordCount int
		StartIndex       int
	}
	if err := json.Unmarshal(body, &listing); err != nil || len(listing.Items) != 3000 || listing.TotalRecordCount != 3005 ||
		listing.StartIndex != 5 {
		t.Fatalf("listing: %v %d %d %d", err, len(listing.Items), listing.TotalRecordCount, listing.StartIndex)
	}
	// Each programme is the one written alone: chunks split nothing.
	for _, i := range []int{0, 1234, 2999} {
		alone := httptest.NewRecorder()
		leanListing(t, s.handler, alone, items[i:i+1], fields)
		if got := listedItems(t, body)[i]; !bytes.Equal(got, listedItems(t, alone.Body.Bytes())[0]) {
			t.Errorf("programme %d: %s", i, got)
		}
	}
}

// failingWriter fails every write, as an answer to an app that left.
type failingWriter struct {
	http.ResponseWriter
	writes int
}

func (f *failingWriter) Write([]byte) (int, error) {
	f.writes++
	return 0, io.ErrClosedPipe
}

// Once a write fails, the rest of the listing is not encoded.
func TestLeanListingsStopWhenTheAppLeaves(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	channel := &library.Item{ID: accounts.ID{7}, Kind: library.KindChannel, Name: "Seven"}
	items := make([]library.Item, 3000)
	for i := range items {
		items[i] = guideProgramme(i, channel)
	}
	w := &failingWriter{ResponseWriter: httptest.NewRecorder()}
	leanListing(t, s.handler, w, items, fieldSet{})
	if w.writes != 1 {
		t.Errorf("%d writes after the first failed", w.writes-1)
	}
}

// The stream's encoders write what encoding/json writes.
func TestStreamEncodersWriteAsEncodingJSON(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"", "plain", "Tom & Jerry <b>", "\"\\/", "\b\f\n\r\t\x00\x01\x1f\x7f", "é ü 日本 😀",
		"\u2028\u2029", "bad \xff\xfe utf8 \xc3", "\xed\xa0\x80", "\u00e9\u0301"} {
		want, _ := json.Marshal(s)
		if got := appendJSONString(nil, s); !bytes.Equal(got, want) {
			t.Errorf("string %q: %s, want %s", s, got, want)
		}
	}
	for _, f := range []float64{0, 1, -1, 7.25, 81.5, 16.0 / 9.0, 2.0 / 3.0, 1e-7, 1.5e-9, 1e21, 123456789012345678, 1e20, -2.5e-7, math.SmallestNonzeroFloat64} {
		want, _ := json.Marshal(f)
		if got := appendJSONFloat(nil, f); !bytes.Equal(got, want) {
			t.Errorf("float %v: %s, want %s", f, got, want)
		}
	}
	for _, at := range []time.Time{time.Unix(0, 0), time.Date(2026, 11, 16, 12, 0, 0, 123456700, time.UTC),
		time.Date(2026, 11, 16, 12, 0, 0, 100000000, time.FixedZone("x", 3600)), time.Date(2026, 1, 2, 3, 4, 5, 99, time.UTC)} {
		want, _ := Time(at).MarshalJSON()
		if got := appendJSONTime(nil, at); !bytes.Equal(got, want) {
			t.Errorf("time %v: %s, want %s", at, got, want)
		}
	}
	id := accounts.ID{0xab, 0x01, 0xff}
	if got := appendJSONID(nil, id); string(got) != `"`+id.String()+`"` {
		t.Errorf("id: %s", got)
	}
}

// A timer and a series timer mark their programme in the guide's lean
// listing as in the full one.
func TestTimersMarkLeanAndFullListings(t *testing.T) {
	t.Parallel()
	addon := newTVAddon(t, true, "")
	s, _ := recordingServer(t, "ffprobe-not-installed", t.TempDir())
	token, _ := recordingMember(t, s, addon.url, true)
	next := guidePrograms(t, s, token)["Next"]
	defaults := timerDefaults(t, s, token, next.Id)
	if status, body := s.call(http.MethodPost, "/LiveTv/SeriesTimers", app("tv", token), defaults); status != http.StatusNoContent {
		t.Fatalf("a series timer made: %d %s", status, body)
	}
	marks := func(query string) map[string]string {
		t.Helper()
		status, body := s.call(http.MethodGet, "/LiveTv/Programs"+query, app("tv", token), nil)
		if status != http.StatusOK {
			t.Fatalf("programmes%s: %d %s", query, status, body)
		}
		for _, item := range listedItems(t, body) {
			var marked struct{ Name, TimerId, Status, SeriesTimerId string }
			_ = json.Unmarshal(item, &marked)
			if marked.Name == "Next" {
				return map[string]string{"TimerId": marked.TimerId, "Status": marked.Status, "SeriesTimerId": marked.SeriesTimerId}
			}
		}
		t.Fatalf("programmes%s: no Next in %s", query, body)
		return nil
	}
	full, lean := marks(""), marks("?EnableImages=false&EnableUserData=false")
	if full["TimerId"] == "" || full["Status"] != "New" || full["SeriesTimerId"] == "" || !maps.Equal(full, lean) {
		t.Errorf("timers: full %v, lean %v", full, lean)
	}
}
