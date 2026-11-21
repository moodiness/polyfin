package stremio

import (
	"encoding/json"
	"testing"
)

// The streamData some addons add is read leniently: cached as a boolean,
// a string or a number, and only with a service named; seeders as a number
// or a numeric string. A value of another type reads as unknown, and the
// stream is listed all the same. Written back, it reads the same.
func TestStreamDataIsReadLeniently(t *testing.T) {
	t.Parallel()
	yes, no := true, false
	for _, tc := range []struct {
		data    string
		cached  *bool
		seeders Number
	}{
		{`{"service":{"id":"svc","cached":true},"torrent":{"seeders":42}}`, &yes, 42},
		{`{"service":{"id":"svc","cached":false}}`, &no, 0},
		{`{"service":{"id":"svc","cached":"True"},"torrent":{"seeders":"17"}}`, &yes, 17},
		{`{"service":{"id":"svc","cached":"no"}}`, &no, 0},
		{`{"service":{"id":"svc","cached":1}}`, &yes, 0},
		{`{"service":{"id":"svc","cached":0}}`, &no, 0},
		{`{"service":{"id":"svc","cached":"maybe"}}`, nil, 0},
		{`{"service":{"id":"svc"}}`, nil, 0},
		{`{"service":{"cached":true}}`, nil, 0},
		{`{"service":{"id":" ","cached":true}}`, nil, 0},
		{`{"service":"svc","torrent":{"seeders":-3}}`, nil, 0},
		{`{"service":[1],"torrent":"many"}`, nil, 0},
		{`{"torrent":{"seeders":{"count":5}}}`, nil, 0},
		{`"cached"`, nil, 0},
		{`null`, nil, 0},
	} {
		listed, err := decodeList([]byte(`[{"url":"https://f.example/a.mkv","streamData":`+tc.data+`}]`), MaxListed,
			&room{left: listedRoom}, decodeStream, tells)
		if err != nil || len(listed) != 1 {
			t.Fatalf("%s: %d streams, %v", tc.data, len(listed), err)
		}
		got := listed[0].Data
		if (got.Service.Cached == nil) != (tc.cached == nil) || got.Service.Cached != nil && *got.Service.Cached != *tc.cached ||
			got.Torrent.Seeders != tc.seeders {
			t.Errorf("%s: cached %v, seeders %d", tc.data, got.Service.Cached, got.Torrent.Seeders)
		}
		written, err := json.Marshal(listed[0])
		if err != nil {
			t.Fatal(err)
		}
		var again Stream
		if err := json.Unmarshal(written, &again); err != nil {
			t.Fatal(err)
		}
		if (again.Data.Service.Cached == nil) != (got.Service.Cached == nil) || again.Data.Torrent.Seeders != got.Torrent.Seeders {
			t.Errorf("%s written back as %s", tc.data, written)
		}
	}
}
