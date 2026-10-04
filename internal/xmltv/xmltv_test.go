package xmltv

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

const guide = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE tv SYSTEM "xmltv.dtd">
<tv generator-info-name="test">
  <channel id="one.fr">
    <display-name lang="fr">Une ᴴᴰ</display-name>
    <display-name>FR: Une</display-name>
    <icon src="https://img.example/one.png"/>
  </channel>
  <channel id="  "><display-name>No id</display-name></channel>
  <programme start="20260104200000 +0100" stop="20260104213000 +0100" channel="one.fr">
    <title lang="en">The News</title>
    <title lang="fr">Le Journal</title>
    <sub-title lang="fr">Édition du soir</sub-title>
    <desc lang="fr">Les nouvelles &amp; la météo&nbsp;du jour.</desc>
    <category lang="fr">News</category>
    <category lang="en">news</category>
    <category lang="en">Talk</category>
    <episode-num system="onscreen">S9E99</episode-num>
    <episode-num system="xmltv_ns">2 . 4/10 . 0/1</episode-num>
    <icon src="https://img.example/news.jpg"/>
    <rating system="CSA"><value>-10</value></rating>
  </programme>
  <programme start="20260104213000" stop="20260104230000" channel="one.fr">
    <title>A Movie</title>
    <episode-num system="onscreen">S03E07</episode-num>
  </programme>
  <programme start="20260104230000 +0000" stop="20260104220000 +0000" channel="one.fr"><title>Ends before it starts</title></programme>
  <programme start="20260104230000 +0000" stop="20260105000000 +0000" channel="one.fr"><title> </title></programme>
  <programme start="nonsense" stop="20260105000000 +0000" channel="one.fr"><title>Bad start</title></programme>
</tv>`

func read(t *testing.T, data []byte, options Options) ([]Channel, []Programme, error) {
	t.Helper()
	var channels []Channel
	var programmes []Programme
	err := Read(bytes.NewReader(data), options,
		func(c Channel) error { channels = append(channels, c); return nil },
		func(p Programme) error { programmes = append(programmes, p); return nil })
	return channels, programmes, err
}

func gzipped(data []byte) []byte {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	_, _ = writer.Write(data)
	_ = writer.Close()
	return buffer.Bytes()
}

// A guide reads the same plain or compressed, told by its content.
func TestGuidesReadPlainOrCompressed(t *testing.T) {
	for name, data := range map[string][]byte{"plain": []byte(guide), "gzip": gzipped([]byte(guide))} {
		channels, programmes, err := read(t, data, Options{Language: "fr"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(channels) != 1 || channels[0].ID != "one.fr" || len(channels[0].Names) != 2 || channels[0].Names[0] != "Une ᴴᴰ" {
			t.Errorf("%s: channels %+v", name, channels)
		}
		if len(programmes) != 2 {
			t.Fatalf("%s: programmes %+v", name, programmes)
		}
		news, movie := programmes[0], programmes[1]
		start := time.Date(2026, 1, 4, 19, 0, 0, 0, time.UTC)
		if !news.Start.Equal(start) || !news.Stop.Equal(start.Add(90*time.Minute)) {
			t.Errorf("%s: news times %v %v", name, news.Start, news.Stop)
		}
		if news.Title != "Le Journal" || news.SubTitle != "Édition du soir" || news.Description != "Les nouvelles & la météo\u00a0du jour." {
			t.Errorf("%s: news texts in the server language: %+v", name, news)
		}
		// xmltv_ns counts from 0 and wins over on-screen numbering.
		if news.Season != 3 || news.Episode != 5 || news.Icon != "https://img.example/news.jpg" {
			t.Errorf("%s: news numbering and icon: %+v", name, news)
		}
		if strings.Join(news.Categories, ",") != "News,Talk" {
			t.Errorf("%s: categories, each once: %v", name, news.Categories)
		}
		// Without an offset, a time is in UTC.
		if !movie.Start.Equal(time.Date(2026, 1, 4, 21, 30, 0, 0, time.UTC)) || movie.Season != 3 || movie.Episode != 7 {
			t.Errorf("%s: movie %+v", name, movie)
		}
	}
	// Without a title in the language asked for, the first one.
	_, programmes, _ := read(t, []byte(guide), Options{Language: "de"})
	if programmes[0].Title != "The News" {
		t.Errorf("fallback title: %q", programmes[0].Title)
	}
}

func TestMalformedGuidesFail(t *testing.T) {
	compressedJunk := gzipped([]byte("not xml at all"))
	for name, data := range map[string][]byte{
		"html":          []byte("<html><body>Not found</body></html>"),
		"json":          []byte(`{"error": "unauthorized"}`),
		"empty":         nil,
		"truncated":     []byte(guide[:len(guide)/2]),
		"broken gzip":   compressedJunk[:len(compressedJunk)-6],
		"gzipped junk":  compressedJunk,
		"unclosed root": []byte(`<tv><channel id="a"><display-name>A</display-name></channel>`),
	} {
		if _, _, err := read(t, data, Options{}); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: %v, want ErrMalformed", name, err)
		}
	}
}

func TestGuidesOverTheLimitFail(t *testing.T) {
	data := []byte(guide)
	if _, _, err := read(t, data, Options{Limit: int64(len(data))}); err != nil {
		t.Errorf("a guide of exactly the limit: %v", err)
	}
	if _, _, err := read(t, data, Options{Limit: int64(len(data)) - 1}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a guide one byte over the limit: %v", err)
	}
	// A small compressed file may not expand beyond its share either.
	padded := []byte("<tv>" + strings.Repeat("<!-- padding -->", 10000) + "</tv>")
	compressed := gzipped(padded)
	if _, _, err := read(t, compressed, Options{Limit: int64(len(compressed))}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a compressed guide expanding %d times: %v", len(padded)/len(compressed), err)
	}
}

// Read stops at the first callback error, and reads as a stream: the
// programmes before an error in the document were already handed over.
func TestGuidesAreReadAsAStream(t *testing.T) {
	stop := errors.New("stop")
	calls := 0
	err := Read(strings.NewReader(guide), Options{}, func(Channel) error { return nil }, func(Programme) error { calls++; return stop })
	if !errors.Is(err, stop) || calls != 1 {
		t.Errorf("callback error: %v after %d calls", err, calls)
	}
	broken := strings.Replace(guide, "</programme>\n  <programme start=\"20260104213000\"", "</programme>\n  <programme start=\"20260104213000\"<", 1)
	calls = 0
	err = Read(io.MultiReader(strings.NewReader(broken)), Options{}, func(Channel) error { return nil }, func(Programme) error { calls++; return nil })
	if !errors.Is(err, ErrMalformed) || calls != 1 {
		t.Errorf("an error later in the document: %v after %d programmes", err, calls)
	}
}

func TestTimesAndNumbering(t *testing.T) {
	for value, want := range map[string]time.Time{
		"20260104200000 +0100": time.Date(2026, 1, 4, 19, 0, 0, 0, time.UTC),
		"20260104200000-0230":  time.Date(2026, 1, 4, 22, 30, 0, 0, time.UTC),
		"202601042000":         time.Date(2026, 1, 4, 20, 0, 0, 0, time.UTC),
		"20260104":             time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
		"20260104200000 Z":     time.Date(2026, 1, 4, 20, 0, 0, 0, time.UTC),
	} {
		if got, ok := parseTime(value); !ok || !got.Equal(want) {
			t.Errorf("%q: %v %v, want %v", value, got, ok, want)
		}
	}
	for _, value := range []string{"", "2026", "20260104200000 CET", "202601042"} {
		if _, ok := parseTime(value); ok {
			t.Errorf("%q read as a time", value)
		}
	}
	for value, want := range map[string][2]int{"0.0.": {1, 1}, ".11.": {0, 12}, "4/5.": {5, 0}, "x.y": {0, 0}} {
		if s, e := xmltvNS(value); s != want[0] || e != want[1] {
			t.Errorf("xmltv_ns %q: %d %d, want %v", value, s, e, want)
		}
	}
	for value, want := range map[string][2]int{"S01E02": {1, 2}, "s2 e10": {2, 10}, "Ep. 4": {0, 0}, "E4": {0, 4}, "Pilot": {0, 0}} {
		if s, e := onScreen(value); s != want[0] || e != want[1] {
			t.Errorf("on screen %q: %d %d, want %v", value, s, e, want)
		}
	}
}
