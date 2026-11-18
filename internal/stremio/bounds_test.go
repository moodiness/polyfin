package stremio

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unsafe"
)

// listOf is a JSON list of count elements, element giving each.
func listOf(count int, element func(i int) string) string {
	var list strings.Builder
	list.WriteByte('[')
	for i := range count {
		if i > 0 {
			list.WriteByte(',')
		}
		list.WriteString(element(i))
	}
	list.WriteByte(']')
	return list.String()
}

// emptyObjects is a JSON list of count empty objects.
func emptyObjects(count int) string {
	return listOf(count, func(int) string { return "{}" })
}

// answering serves an addon that answers each path with its body,
// compressed as addons' servers do, and returns its manifest's address.
func answering(t *testing.T, bodies map[string]string) string {
	t.Helper()
	packed := map[string][]byte{}
	for path, body := range bodies {
		var buffer bytes.Buffer
		zipper := gzip.NewWriter(&buffer)
		_, _ = zipper.Write([]byte(body))
		_ = zipper.Close()
		packed[path] = buffer.Bytes()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := packed[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// An answer is bounded in bytes once inflated, and its lists in elements:
// 2 MiB of empty objects, a few kilobytes on the wire, are a page of
// MaxMetas titles, a description of MaxVideos videos, and no stream nor
// subtitle at all. A list of more streams or subtitles than are read is
// cut at MaxListed, the first ones kept.
func TestAnAnswerOfEmptyObjectsIsCutAtItsBound(t *testing.T) {
	t.Parallel()
	empties := emptyObjects(maxResponseBytes / 4 / 3)
	manifest := answering(t, map[string]string{
		"/catalog/movie/top.json":   `{"metas":` + empties + `}`,
		"/meta/series/tt1.json":     `{"meta":{"id":"tt1","type":"series","videos":` + empties + `}}`,
		"/stream/movie/tt1.json":    `{"streams":` + empties + `}`,
		"/subtitles/movie/tt1.json": `{"subtitles":` + empties + `}`,
		"/stream/movie/tt2.json": `{"streams":` + listOf(MaxListed+50, func(i int) string {
			return fmt.Sprintf(`{"url":"https://f.example/%d.mkv"}`, i)
		}) + `}`,
		"/subtitles/movie/tt2.json": `{"subtitles":` + listOf(MaxListed+50, func(i int) string {
			return fmt.Sprintf(`{"id":%d,"url":"https://f.example/%d.srt","lang":"eng"}`, i, i)
		}) + `}`,
	})
	client := NewClient("test")
	t.Run("metas", func(t *testing.T) {
		t.Parallel()
		metas, err := client.Catalog(t.Context(), manifest, "movie", "top", nil, false)
		if err != nil || len(metas) != MaxMetas {
			t.Errorf("a page of empty objects: %d titles, %v; want %d", len(metas), err, MaxMetas)
		}
	})
	t.Run("videos", func(t *testing.T) {
		t.Parallel()
		meta, err := client.Meta(t.Context(), manifest, "series", "tt1", false)
		if err != nil || len(meta.Videos) != MaxVideos {
			t.Errorf("a description of empty objects: %d videos, %v; want %d", len(meta.Videos), err, MaxVideos)
		}
	})
	t.Run("streams", func(t *testing.T) {
		t.Parallel()
		streams, err := client.Streams(t.Context(), manifest, "movie", "tt1", false)
		if err != nil || len(streams) != 0 {
			t.Errorf("streams of empty objects: %d, %v; want none", len(streams), err)
		}
		streams, err = client.Streams(t.Context(), manifest, "movie", "tt2", false)
		if err != nil || len(streams) != MaxListed || streams[MaxListed-1].URL != fmt.Sprintf("https://f.example/%d.mkv", MaxListed-1) {
			t.Errorf("more streams than are read: %d, %v; want the first %d", len(streams), err, MaxListed)
		}
	})
	t.Run("subtitles", func(t *testing.T) {
		t.Parallel()
		subtitles, err := client.Subtitles(t.Context(), manifest, "movie", "tt1", nil, false)
		if err != nil || len(subtitles) != 0 {
			t.Errorf("subtitles of empty objects: %d, %v; want none", len(subtitles), err)
		}
		subtitles, err = client.Subtitles(t.Context(), manifest, "movie", "tt2", nil, false)
		if err != nil || len(subtitles) != MaxListed || subtitles[MaxListed-1].URL != fmt.Sprintf("https://f.example/%d.srt", MaxListed-1) {
			t.Errorf("more subtitles than are read: %d, %v; want the first %d", len(subtitles), err, MaxListed)
		}
	})
}

// An answer that holds as many elements as are read of its list is kept
// whole, in its order.
func TestAnAnswerAtItsBoundIsKeptWhole(t *testing.T) {
	t.Parallel()
	manifest := answering(t, map[string]string{
		"/catalog/movie/top.json": `{"metas":` + listOf(MaxMetas, func(i int) string {
			return fmt.Sprintf(`{"id":"tt%d","type":"movie","name":"Title %d","genres":["Drama"]}`, i, i)
		}) + `}`,
		"/meta/series/tt1.json": `{"meta":{"id":"tt1","type":"series","name":"Daily","videos":` + listOf(MaxVideos, func(i int) string {
			return fmt.Sprintf(`{"id":"tt1:1:%d","season":1,"episode":%d,"title":"Episode %d"}`, i+1, i+1, i+1)
		}) + `}}`,
		"/stream/movie/tt1.json": `{"streams":` + listOf(MaxListed, func(i int) string {
			return fmt.Sprintf(`{"name":"Source","url":"https://f.example/%d.mkv","behaviorHints":{"filename":"%d.mkv"}}`, i, i)
		}) + `}`,
		"/subtitles/movie/tt1.json": `{"subtitles":` + listOf(MaxListed, func(i int) string {
			return fmt.Sprintf(`{"id":%d,"url":"https://f.example/%d.srt","lang":"eng"}`, i, i)
		}) + `}`,
	})
	client := NewClient("test")
	t.Run("metas", func(t *testing.T) {
		t.Parallel()
		metas, err := client.Catalog(t.Context(), manifest, "movie", "top", nil, false)
		if err != nil || len(metas) != MaxMetas {
			t.Fatalf("%d titles, %v; want %d", len(metas), err, MaxMetas)
		}
		if last := metas[MaxMetas-1]; last.ID != fmt.Sprintf("tt%d", MaxMetas-1) || len(last.Genres) != 1 {
			t.Errorf("the last title: %+v", last)
		}
	})
	t.Run("videos", func(t *testing.T) {
		t.Parallel()
		meta, err := client.Meta(t.Context(), manifest, "series", "tt1", false)
		if err != nil || len(meta.Videos) != MaxVideos {
			t.Fatalf("%d videos, %v; want %d", len(meta.Videos), err, MaxVideos)
		}
		if last := meta.Videos[MaxVideos-1]; last.Episode != MaxVideos || last.Title != fmt.Sprintf("Episode %d", MaxVideos) {
			t.Errorf("the last video: %+v", last)
		}
	})
	t.Run("streams", func(t *testing.T) {
		t.Parallel()
		streams, err := client.Streams(t.Context(), manifest, "movie", "tt1", false)
		if err != nil || len(streams) != MaxListed {
			t.Fatalf("%d streams, %v; want %d", len(streams), err, MaxListed)
		}
		if last := streams[MaxListed-1]; last.BehaviorHints.Filename != fmt.Sprintf("%d.mkv", MaxListed-1) {
			t.Errorf("the last stream: %+v", last)
		}
	})
	t.Run("subtitles", func(t *testing.T) {
		t.Parallel()
		subtitles, err := client.Subtitles(t.Context(), manifest, "movie", "tt1", nil, false)
		if err != nil || len(subtitles) != MaxListed || subtitles[MaxListed-1].ID != Text(fmt.Sprint(MaxListed-1)) {
			t.Fatalf("%d subtitles, %v; want %d", len(subtitles), err, MaxListed)
		}
	})
}

// A stream that tells nothing is left out of its answer, and one Polyfin
// does not play but that tells something is kept: a notice's text, a web
// page, a file's name. A subtitle that cannot be downloaded is left out.
func TestElementsThatHoldNothingAreLeftOut(t *testing.T) {
	t.Parallel()
	manifest := answering(t, map[string]string{
		"/stream/movie/tt1.json": `{"streams":[{}, null, {"unknown":true}, {"name":""},
			{"name":"Wait a minute"}, {"externalUrl":"https://page.example"}, {"ytId":"abc"}, {"nzbUrl":"https://n.example/a.nzb"},
			{"rarUrls":[{"url":"https://r.example/a.rar"}]}, {"behaviorHints":{"filename":"a.mkv"}}, {"infoHash":"not a hash"},
			{"url":"https://f.example/a.mkv"}]}`,
		"/subtitles/movie/tt1.json": `{"subtitles":[{}, {"id":"1","lang":"eng"}, {"id":"2","url":"file:///a.srt","lang":"eng"},
			{"id":"3","url":"https://f.example/a.srt","lang":"eng"}]}`,
		"/meta/series/tt1.json": `{"meta":{"id":"tt1","type":"series","videos":[{"id":"tt1:1:1","streams":[{}, {"name":"Soon"}, {"url":"https://f.example/e.mkv"}]}]}}`,
	})
	client := NewClient("test")
	streams, err := client.Streams(t.Context(), manifest, "movie", "tt1", false)
	if err != nil || len(streams) != 8 || streams[0].Name != "Wait a minute" || !streams[7].Playable() {
		t.Errorf("streams: %d, %v; want the 8 that tell something: %+v", len(streams), err, streams)
	}
	subtitles, err := client.Subtitles(t.Context(), manifest, "movie", "tt1", nil, false)
	if err != nil || len(subtitles) != 1 || subtitles[0].ID != "3" {
		t.Errorf("subtitles: %+v, %v; want the one that can be downloaded", subtitles, err)
	}
	meta, err := client.Meta(t.Context(), manifest, "series", "tt1", false)
	if err != nil || len(meta.Videos) != 1 || len(meta.Videos[0].Streams) != 2 {
		t.Errorf("a video's own streams: %+v, %v; want the 2 that tell something", meta.Videos, err)
	}
}

// The lists of an answer take their elements' size from one room, however
// they nest: videos of 2,000 streams each, or titles of 1,000 videos each,
// stop where the answer's room is spent, where each list alone is within
// its own bound.
func TestListsWithinListsShareTheRoomOfTheirAnswer(t *testing.T) {
	t.Parallel()
	streams := listOf(maxNested, func(int) string { return `{"ytId":"a"}` })
	videos := emptyObjects(1000)
	manifest := answering(t, map[string]string{
		"/meta/series/tt1.json": `{"meta":{"id":"tt1","type":"series","videos":` + listOf(100, func(i int) string {
			return fmt.Sprintf(`{"id":"tt1:1:%d","streams":%s}`, i, streams)
		}) + `}}`,
		"/catalog/tv/guide.json": `{"metasDetailed":` + listOf(400, func(i int) string {
			return fmt.Sprintf(`{"id":"channel%d","type":"tv","videos":%s}`, i, videos)
		}) + `}`,
	})
	client := NewClient("test")
	t.Run("a description", func(t *testing.T) {
		t.Parallel()
		meta, err := client.Meta(t.Context(), manifest, "series", "tt1", false)
		if err != nil || len(meta.Videos) == 0 || len(meta.Videos[0].Streams) != maxNested {
			t.Fatalf("%d videos, %v", len(meta.Videos), err)
		}
		held := len(meta.Videos) * int(unsafe.Sizeof(Video{}))
		for _, video := range meta.Videos {
			held += len(video.Streams) * int(unsafe.Sizeof(Stream{}))
		}
		if held > metaRoom || held < metaRoom-int(unsafe.Sizeof(Stream{})) {
			t.Errorf("the description's lists hold %d bytes of elements, want the %d of an answer's room", held, metaRoom)
		}
	})
	t.Run("a page", func(t *testing.T) {
		t.Parallel()
		metas, err := client.Catalog(t.Context(), manifest, "tv", "guide", nil, false)
		if err != nil || len(metas) == 0 || len(metas[0].Videos) != 1000 {
			t.Fatalf("%d titles, %v", len(metas), err)
		}
		held := len(metas) * int(unsafe.Sizeof(Meta{}))
		for _, meta := range metas {
			held += len(meta.Videos) * int(unsafe.Sizeof(Video{}))
		}
		if held > metaRoom || held < metaRoom-int(unsafe.Sizeof(Meta{})) {
			t.Errorf("the page's lists hold %d bytes of elements, want the %d of an answer's room", held, metaRoom)
		}
	})
}

// Every other list of an element is read up to maxNested elements, and
// collections within collections maxCollections deep.
func TestTheListsOfAnElementAreCutAtTheirBound(t *testing.T) {
	t.Parallel()
	const long = maxNested + 500
	texts := listOf(long, func(i int) string { return fmt.Sprintf(`"n%d"`, i) })
	collection := `{"id":"deepest","type":"collection"}`
	for depth := maxCollections + 2; depth > 0; depth-- {
		collection = fmt.Sprintf(`{"id":"c%d","type":"collection","collection":{"items":[%s]}}`, depth, collection)
	}
	manifest := answering(t, map[string]string{
		"/meta/series/tt1.json": `{"meta":{"id":"tt1","type":"series","genres":` + texts + `,"director":` + texts +
			`,"links":` + listOf(long, func(i int) string { return fmt.Sprintf(`{"name":"Actor %d","category":"Cast"}`, i) }) +
			`,"trailers":` + listOf(long, func(i int) string { return fmt.Sprintf(`{"source":"t%d"}`, i) }) +
			`,"app_extras":{"cast":` + listOf(long, func(i int) string { return fmt.Sprintf(`{"name":"Actor %d"}`, i) }) +
			`,"seasonPosters":` + texts + `},"collection":{"sources":` +
			listOf(long, func(i int) string { return fmt.Sprintf(`{"type":"movie","catalogId":"c%d"}`, i) }) + `}}}`,
		"/meta/collection/c1.json": `{"meta":` + collection + `}`,
		"/stream/movie/tt1.json": `{"streams":[{"infoHash":"0123456789abcdef0123456789abcdef01234567","sources":` + texts +
			`,"subtitles":` + listOf(long, func(i int) string { return fmt.Sprintf(`{"id":%d,"url":"https://f.example/%d.srt"}`, i, i) }) +
			`,"rarUrls":` + texts + `}]}`,
	})
	client := NewClient("test")
	meta, err := client.Meta(t.Context(), manifest, "series", "tt1", false)
	if err != nil || meta.Extras == nil || meta.Collection == nil {
		t.Fatalf("the description: %v", err)
	}
	stream, err := client.Streams(t.Context(), manifest, "movie", "tt1", false)
	if err != nil || len(stream) != 1 {
		t.Fatalf("the streams: %d, %v", len(stream), err)
	}
	for list, read := range map[string]int{
		"genres": len(meta.Genres), "directors": len(meta.Director), "cast from links": len(meta.Cast), "trailers": len(meta.Trailers),
		"cast": len(meta.Extras.Cast), "season posters": len(meta.Extras.SeasonPosters), "a collection's catalogs": len(meta.Collection.Sources),
		"a stream's trackers": len(stream[0].Sources), "a stream's subtitles": len(stream[0].Subtitles), "an archive's files": len(stream[0].RARs),
	} {
		if read != maxNested {
			t.Errorf("%s: %d read of %d, want %d", list, read, long, maxNested)
		}
	}
	if meta.Genres[maxNested-1] != fmt.Sprintf("n%d", maxNested-1) || meta.Extras.Cast[0].Name != "Actor 0" {
		t.Errorf("the lists are not the first elements, in order")
	}
	nest, err := client.Meta(t.Context(), manifest, "collection", "c1", false)
	if err != nil {
		t.Fatal(err)
	}
	depth := 0
	for within := &nest; within.Collection != nil && len(within.Collection.Items) == 1; within = &within.Collection.Items[0] {
		depth++
	}
	if depth != maxCollections {
		t.Errorf("collections were read %d deep, want %d", depth, maxCollections)
	}
}

// An answer whose list is of another type than read, or holds an element
// of another type, is read as it was before its lists were bounded: a
// stream list or a subtitle list is refused whole, a page keeps every
// title's place, and a description loses only the odd field.
func TestOddAnswersAreReadAsBefore(t *testing.T) {
	t.Parallel()
	manifest := answering(t, map[string]string{
		"/stream/movie/none.json":      `{}`,
		"/stream/movie/null.json":      `{"streams":null}`,
		"/stream/movie/empty.json":     `{"streams":[]}`,
		"/stream/movie/object.json":    `{"streams":{}}`,
		"/stream/movie/name.json":      `{"streams":[{"url":"https://f.example/a.mkv"},{"name":5}]}`,
		"/stream/movie/sources.json":   `{"streams":[{"url":"https://f.example/a.mkv","sources":[5]}]}`,
		"/stream/movie/subtitles.json": `{"streams":[{"url":"https://f.example/a.mkv","subtitles":{}}]}`,
		"/subtitles/movie/object.json": `{"subtitles":{}}`,
		"/subtitles/movie/lang.json":   `{"subtitles":[{"id":1,"url":"https://f.example/a.srt","lang":[]}]}`,
		"/catalog/movie/null.json":     `{"metas":null,"metasDetailed":[{"id":"a"}]}`,
		"/catalog/movie/empty.json":    `{"metas":[],"metasDetailed":[{"id":"a"}]}`,
		"/catalog/movie/none.json":     `{}`,
		"/catalog/movie/object.json":   `{"metas":{}}`,
		"/catalog/movie/odd.json": `{"metas":[5, {"id":"a","genres":{"odd":1},"videos":"none","trailers":[7,{"source":"t"}],
			"collection":3,"name":"Kept"}, {}, null]}`,
		"/meta/movie/a.json": `{"meta":{"id":"a","type":"movie","videos":[3,{"id":"a:1","season":"x","streams":{"odd":1}},{"id":"a:2","streams":[{"name":7,"url":"https://f.example/a.mkv"}]}],
			"trailers":{"odd":1},"collection":{"items":"none","sources":[{"type":"movie","catalogId":"top"}]},"name":"Kept"}}`,
	})
	client := NewClient("test")
	for id, want := range map[string]int{"none": 0, "null": 0, "empty": 0, "object": -1, "name": -1, "sources": -1, "subtitles": -1} {
		streams, err := client.Streams(t.Context(), manifest, "movie", id, false)
		if refused := errors.Is(err, ErrInvalidResponse); refused != (want < 0) || err != nil && !refused || err == nil && len(streams) != want {
			t.Errorf("streams %q: %d, %v", id, len(streams), err)
		}
	}
	for _, id := range []string{"object", "lang"} {
		if _, err := client.Subtitles(t.Context(), manifest, "movie", id, nil, false); !errors.Is(err, ErrInvalidResponse) {
			t.Errorf("subtitles %q: %v", id, err)
		}
	}
	for id, want := range map[string]int{"null": 1, "empty": 0, "none": 0, "object": -1, "odd": 4} {
		metas, err := client.Catalog(t.Context(), manifest, "movie", id, nil, false)
		if refused := errors.Is(err, ErrInvalidResponse); refused != (want < 0) || err != nil && !refused || err == nil && (len(metas) != want || metas == nil) {
			t.Errorf("catalog %q: %d, %v", id, len(metas), err)
			continue
		}
		if id == "odd" {
			kept := metas[1]
			if metas[0].ID != "" || kept.ID != "a" || kept.Name != "Kept" || kept.Genres != nil || kept.Videos != nil ||
				len(kept.Trailers) != 2 || kept.Trailers[1].Source != "t" || kept.Collection == nil {
				t.Errorf("the odd page: %+v", metas)
			}
		}
	}
	meta, err := client.Meta(t.Context(), manifest, "movie", "a", false)
	if err != nil || meta.Name != "Kept" || len(meta.Videos) != 3 || meta.Videos[1].ID != "a:1" || meta.Videos[1].Streams != nil ||
		len(meta.Videos[2].Streams) != 1 || !meta.Videos[2].Streams[0].Playable() || meta.Trailers != nil ||
		meta.Collection == nil || meta.Collection.Items != nil || len(meta.Collection.Sources) != 1 {
		t.Errorf("the odd description: %+v, %v", meta, err)
	}
}
