package iptv

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// An M3U playlist's catch-up: the #EXTM3U line's attributes for every
// channel, each channel's own over them, catchup-type and tvg-rec read as
// catchup and catchup-days, unknown kinds and templates missing their
// source as no archive, and days bounded.
func TestM3UCatchupIsRead(t *testing.T) {
	entries, err := parse(t, "#EXTM3U url-tvg=\"https://guide.example/epg.xml\" catchup=\"shift\" catchup-days=\"3\"\n"+
		"#EXTINF:-1,Inherits\nhttps://live.example/1.ts\n"+
		"#EXTINF:-1 catchup=\"append\" catchup-days=\"7\" catchup-source=\"?start={utc}\",Appends\nhttps://live.example/2.ts\n"+
		"#EXTINF:-1 catchup-type=\"Flussonic-TS\" tvg-rec=\"2\",Aliases\nhttps://live.example/3/index.m3u8\n"+
		"#EXTINF:-1 catchup=\"vod\",Unknown\nhttps://live.example/4.ts\n"+
		"#EXTINF:-1 catchup=\"default\",No Source\nhttps://live.example/5.ts\n"+
		"#EXTINF:-1 catchup=\"xc\" catchup-days=\"1000\",Too Many Days\nhttps://live.example/live/u/p/6.ts\n"+
		"#EXTINF:-1 catchup-days=\"\",Empty Days\nhttps://live.example/7.ts\n")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Catchup{
		"Inherits":      {Type: CatchupShift, Days: 3},
		"Appends":       {Type: CatchupAppend, Days: 7, Source: "?start={utc}"},
		"Aliases":       {Type: CatchupFlussonic, Days: 2},
		"Unknown":       {},
		"No Source":     {},
		"Too Many Days": {Type: CatchupXtream, Days: maxCatchupDays},
		"Empty Days":    {Type: CatchupShift, Days: 3},
	}
	if len(entries) != len(want) {
		t.Fatalf("channels: %+v", entries)
	}
	for _, e := range entries {
		if e.Catchup != want[e.Name] {
			t.Errorf("%s: %+v, want %+v", e.Name, e.Catchup, want[e.Name])
		}
	}

	// A kind without days reaches a day back; a playlist whose header
	// names no catch-up gives its channels none.
	entries, err = parse(t, "#EXTM3U\n#EXTINF:-1 catchup=\"shift\",Kind Only\nhttps://live.example/1.ts\n#EXTINF:-1,Plain\nhttps://live.example/2.ts\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Catchup != (Catchup{Type: CatchupShift, Days: 1}) || entries[1].Catchup != (Catchup{}) {
		t.Errorf("defaults: %+v", entries)
	}
}

// An Xtream account's channels carry their archive's days, and its login
// the zone its timeshift addresses are written in; a zone of no known
// name is none.
func TestXtreamArchivesAreRead(t *testing.T) {
	client := requester{client: stremio.NewClient("test"), pacer: NewPacer(0)}
	for zone, want := range map[string]string{"Europe/Paris": "Europe/Paris", "Mars/Olympus": ""} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Query().Get("action") {
			case "":
				_ = json.NewEncoder(w).Encode(map[string]any{"user_info": map[string]any{"auth": 1}, "server_info": map[string]any{"timezone": zone}})
			case "get_live_categories":
				_, _ = w.Write([]byte(`[{"category_id": "1", "category_name": "News"}]`))
			case "get_live_streams":
				_, _ = w.Write([]byte(`[
					{"name": "Zeb One", "stream_id": 1, "category_id": "1", "tv_archive": 1, "tv_archive_duration": "5"},
					{"name": "Orbe", "stream_id": 2, "category_id": "1", "tv_archive": "1", "tv_archive_duration": 0},
					{"name": "Quill", "stream_id": 3, "category_id": "1", "tv_archive": 0, "tv_archive_duration": 7}
				]`))
			default:
				http.NotFound(w, r)
			}
		}))
		account := Account{Kind: addons.KindXtream, Server: server.URL, Username: "user", Password: "secret"}
		address, err := account.address()
		if err != nil {
			t.Fatal(err)
		}
		list, err := fetch(t.Context(), client, accountOf(addons.KindXtream, address), false, livePart)
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if list.timezone != want {
			t.Errorf("zone %q read as %q", zone, list.timezone)
		}
		if len(list.entries) != 3 || list.entries[0].Catchup != (Catchup{Type: CatchupXtream, Days: 5}) ||
			list.entries[1].Catchup != (Catchup{Type: CatchupXtream, Days: 1}) || list.entries[2].Catchup != (Catchup{}) {
			t.Errorf("archives: %+v", list.entries)
		}
	}
}

// Each kind of catch-up makes the address its providers expect for a
// programme of 44 minutes 30 seconds that started three hours ago, on the
// night Paris moves its clocks forward.
func TestCatchupAddresses(t *testing.T) {
	start := time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC) // 03:30 in Paris, summer time since 01:00
	end, now := start.Add(44*time.Minute+30*time.Second), start.Add(3*time.Hour)
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		catchup Catchup
		stream  string
		zone    *time.Location
		want    string
	}{
		{"xc in the server's zone", Catchup{Type: CatchupXtream}, "http://tv.example:8080/live/user/p%40ss/101.ts", paris,
			"http://tv.example:8080/timeshift/user/p%40ss/45/2026-03-29:03-30/101.ts"},
		{"xc in UTC", Catchup{Type: CatchupXtream}, "http://tv.example/live/user/pass/7", time.UTC,
			"http://tv.example/timeshift/user/pass/45/2026-03-29:01-30/7.ts"},
		{"xc of no known form", Catchup{Type: CatchupXtream}, "http://tv.example/7.ts", time.UTC, ""},
		{"shift", Catchup{Type: CatchupShift}, "https://cdn.example/zeb.m3u8", paris,
			"https://cdn.example/zeb.m3u8?utc=1774747800&lutc=1774758600"},
		{"shift after a query", Catchup{Type: CatchupShift}, "https://cdn.example/zeb.m3u8?token=a", paris,
			"https://cdn.example/zeb.m3u8?token=a&utc=1774747800&lutc=1774758600"},
		{"append", Catchup{Type: CatchupAppend, Source: "?start={utc}&len={duration}"}, "https://cdn.example/zeb.ts", paris,
			"https://cdn.example/zeb.ts?start=1774747800&len=2670"},
		{"append after a query", Catchup{Type: CatchupAppend, Source: "?start={utc}&len={duration}"}, "https://cdn.example/zeb.ts?token=a", paris,
			"https://cdn.example/zeb.ts?token=a&start=1774747800&len=2670"},
		{"append of a path", Catchup{Type: CatchupAppend, Source: "/{Y}{m}{d}.ts"}, "https://cdn.example/zeb", paris,
			"https://cdn.example/zeb/20260329.ts"},
		{"default", Catchup{Type: CatchupDefault, Source: "https://archive.example/zeb.ts?from={utc}&to={utcend}"}, "https://cdn.example/zeb.ts", paris,
			"https://archive.example/zeb.ts?from=1774747800&to=1774750470"},
		{"default that is no web address", Catchup{Type: CatchupDefault, Source: "rtmp://archive.example/{utc}"}, "https://cdn.example/zeb.ts", paris, ""},
		{"flussonic of a playlist", Catchup{Type: CatchupFlussonic}, "https://fs.example/zeb/index.m3u8?token=a", paris,
			"https://fs.example/zeb/archive-1774747800-2670.ts?token=a"},
		{"flussonic of mpegts", Catchup{Type: CatchupFlussonic}, "https://fs.example/zeb/mpegts", paris,
			"https://fs.example/zeb/archive-1774747800-2670.ts"},
		{"flussonic of a name", Catchup{Type: CatchupFlussonic}, "https://fs.example/zeb", paris,
			"https://fs.example/zeb/archive-1774747800-2670.ts"},
		{"flussonic without a path", Catchup{Type: CatchupFlussonic}, "https://fs.example/index.m3u8", paris, ""},
	} {
		got, ok := catchupAddress(tc.catchup, tc.stream, start, end, now, tc.zone)
		if tc.want == "" && ok {
			t.Errorf("%s: %s, want none", tc.name, got)
		} else if tc.want != "" && got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	// An hour earlier, Paris was still on winter time.
	if got, _ := catchupAddress(Catchup{Type: CatchupXtream}, "http://tv.example:8080/user/pass/101.m3u8", start.Add(-time.Hour), end.Add(-time.Hour),
		now, paris); got != "http://tv.example:8080/timeshift/user/pass/45/2026-03-29:01-30/101.ts" {
		t.Errorf("xc before the change of time: %q", got)
	}
}

// Every family of placeholders: the start, the end and now as seconds
// since 1970 under each of their names, the length and the time since the
// start in seconds or in units, the start's fields in the provider's zone,
// and times written in that zone; unknown names stay.
func TestCatchupPlaceholders(t *testing.T) {
	start := time.Date(2026, 3, 29, 1, 30, 5, 0, time.UTC)
	end, now := start.Add(44*time.Minute+30*time.Second), start.Add(3*time.Hour)
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	for template, want := range map[string]string{
		"{utc} {start} ${start}":                      "1774747805 1774747805 1774747805",
		"{utcend} {end} ${end}":                       "1774750475 1774750475 1774750475",
		"{lutc} {now} ${now} ${timestamp}":            "1774758605 1774758605 1774758605 1774758605",
		"{duration} ${duration} {duration:60}":        "2670 2670 45",
		"{offset} ${offset} {offset:3600}":            "10800 10800 3",
		"{Y}-{m}-{d} {H}:{M}:{S}":                     "2026-03-29 03:30:05",
		"{utc:Y-m-d H:M:S} ${start:YmdHMS}":           "2026-03-29 03:30:05 20260329033005",
		"{utcend:H-M} ${end:H} {lutc:H} ${now:d/m/Y}": "04-14 04 06 29/03/2026",
		"{end:HM} ${timestamp:Y}":                     "0414 2026",
		"{nope} {duration:x} {Y:m} {utc":              "{nope} {duration:x} {Y:m} {utc",
	} {
		if got := expandCatchup(template, start, end, now, paris); got != want {
			t.Errorf("%q: %q, want %q", template, got, want)
		}
	}
}
