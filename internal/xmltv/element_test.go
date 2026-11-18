package xmltv

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// counted counts the bytes read from a guide's source.
type counted struct {
	r    io.Reader
	read int
}

func (c *counted) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}

const (
	programmeOpen = `<programme channel="one" start="20260104200000" stop="20260104210000"><title>News</title>`
	// before stands in front of the element under test: one channel and
	// one programme, which a stream hands over before it fails.
	before = `<tv><channel id="one"><display-name>One</display-name></channel>` + programmeOpen + `</programme>`
)

// large are pieces of a guide of twice what one element may take, each
// a single element, or a single piece of markup outside of them. The
// tests share them.
var large = sync.OnceValue(func() map[string]string {
	size := 2 * maxElement
	return map[string]string{
		"a programme of empty children": programmeOpen + strings.Repeat("<desc/>", size/7) + `</programme>`,
		"a programme of one text":       programmeOpen + `<desc>` + strings.Repeat("a", size) + `</desc></programme>`,
		"a programme of one attribute":  `<programme channel="` + strings.Repeat("a", size) + `"></programme>`,
		"a programme of nested children": programmeOpen + strings.Repeat("<a>", size/7) + strings.Repeat("</a>", size/7) +
			`</programme>`,
		"a channel of empty children": `<channel id="two">` + strings.Repeat("<icon/>", size/7) + `</channel>`,
		"a channel of one text":       `<channel id="two"><display-name>` + strings.Repeat("a", size) + `</display-name></channel>`,
		"another element":             `<lineup>` + strings.Repeat("<station/>", size/10) + `</lineup>`,
		"a comment":                   `<!--` + strings.Repeat("a", size) + `-->`,
		"a text":                      strings.Repeat(" ", size),
		"an instruction":              `<?note ` + strings.Repeat("a", size) + `?>`,
	}
})

// One element of a guide has a bound of its own, and so has what stands
// between the elements: the read fails once the bound is passed, without
// asking the source for the rest, which is what keeps the element out of
// memory. What came before was handed over.
func TestAnElementOverItsBoundEndsTheRead(t *testing.T) {
	t.Parallel()
	// The two buffers between the source and the decoder each read one
	// block ahead.
	const ahead = 2 * 4096
	for name, piece := range large() {
		document := before + piece + `</tv>`
		source := &counted{r: strings.NewReader(document)}
		channels, programmes := 0, 0
		err := Read(source, Options{}, func(Channel) error { channels++; return nil }, func(Programme) error { programmes++; return nil })
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("%s of %d bytes: %v, want ErrTooLarge", name, len(piece), err)
		}
		if channels != 1 || programmes != 1 {
			t.Errorf("%s: %d channels and %d programmes handed over before it, want 1 and 1", name, channels, programmes)
		}
		if most := len(before) + maxElement + ahead; source.read > most {
			t.Errorf("%s: %d bytes read of %d, want %d at most: the bound and what is read ahead, from where it starts",
				name, source.read, len(document), most)
		}
	}
}

// The bound is on the XML, not on what is downloaded: a compressed guide
// follows the same rule.
func TestAnElementOverItsBoundEndsTheReadOfACompressedGuide(t *testing.T) {
	t.Parallel()
	// A guide of a few kilobytes once compressed.
	for _, name := range []string{"a programme of empty children", "a channel of one text"} {
		document := []byte(before + large()[name] + `</tv>`)
		if _, programmes, err := read(t, gzipped(document), Options{}); !errors.Is(err, ErrTooLarge) || len(programmes) != 1 {
			t.Errorf("%s, compressed: %v after %d programmes, want ErrTooLarge after 1", name, err, len(programmes))
		}
	}
	for name, piece := range large() {
		document := []byte(before + piece + `</tv>`)
		// Stored without compression, the source's bytes are the XML's
		// plus a few of framing: how many were read tells how much XML was
		// inflated.
		var stored bytes.Buffer
		writer, err := gzip.NewWriterLevel(&stored, gzip.NoCompression)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = writer.Write(document)
		_ = writer.Close()
		source := &counted{r: bytes.NewReader(stored.Bytes())}
		err = Read(source, Options{}, func(Channel) error { return nil }, func(Programme) error { return nil })
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("%s, stored: %v, want ErrTooLarge", name, err)
		}
		// Inflating reads a stored block of 64 KiB whole, beside the two
		// buffers' blocks.
		if most := len(before) + maxElement + 2*65536; source.read > most {
			t.Errorf("%s, stored: %d bytes read of %d, want %d at most", name, source.read, stored.Len(), most)
		}
	}
}

// An element of exactly the bound is read whole, texts included; one byte
// more is too much. Elements each under the bound add up to any size.
func TestAnElementOfItsBoundIsKeptWhole(t *testing.T) {
	t.Parallel()
	const closing = `</desc></programme>`
	fill := maxElement - len(programmeOpen) - len(`<desc>`) - len(closing)
	element := func(description int) string {
		return programmeOpen + `<desc>` + strings.Repeat("a", description) + closing
	}
	if len(element(fill)) != maxElement {
		t.Fatalf("the programme has %d bytes, want %d", len(element(fill)), maxElement)
	}
	for name, data := range map[string][]byte{
		"plain": []byte(`<tv>` + element(fill) + element(fill) + element(fill) + `</tv>`),
		"gzip":  gzipped([]byte(`<tv>` + element(fill) + element(fill) + element(fill) + `</tv>`)),
	} {
		_, programmes, err := read(t, data, Options{})
		if err != nil || len(programmes) != 3 {
			t.Fatalf("%s: three programmes of the bound: %v, %d programmes", name, err, len(programmes))
		}
		for _, p := range programmes {
			if len(p.Description) != fill || p.Title != "News" {
				t.Errorf("%s: a description of %d bytes under the title %q, want %d under News", name, len(p.Description), p.Title, fill)
			}
		}
	}
	_, programmes, err := read(t, []byte(`<tv>`+element(fill)+element(fill+1)+`</tv>`), Options{})
	if !errors.Is(err, ErrTooLarge) || len(programmes) != 1 {
		t.Errorf("a programme one byte over the bound: %v after %d programmes, want ErrTooLarge after 1", err, len(programmes))
	}
}

// Of the children an element repeats, the first maxRepeated of each name
// are kept, and the element after it is read as usual.
func TestAnElementKeepsTheFirstOfWhatItRepeats(t *testing.T) {
	t.Parallel()
	const repeated = 4000
	var channel, programme strings.Builder
	channel.WriteString(`<channel id="one">`)
	programme.WriteString(`<programme channel="one" start="20260104200000" stop="20260104210000">`)
	for i := range repeated {
		fmt.Fprintf(&channel, `<display-name>Name %d</display-name><icon src="ftp://img.example/%d.png"/>`, i, i)
		fmt.Fprintf(&programme, `<title lang="l%d">Title %d</title><sub-title>Sub %d</sub-title><desc>Text %d</desc>`+
			`<category>Category %d</category><episode-num system="s%d">E%d</episode-num><icon src="ftp://img.example/%d.png"/>`,
			i, i, i, i, i, i, i, i)
	}
	channel.WriteString(`</channel>`)
	programme.WriteString(`</programme>`)
	if channel.Len() > maxElement || programme.Len() > maxElement {
		t.Fatalf("the elements have %d and %d bytes, over the bound of one", channel.Len(), programme.Len())
	}

	var c channelElement
	if err := xml.Unmarshal([]byte(channel.String()), &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Names) != maxRepeated || len(c.Icons) != maxRepeated {
		t.Errorf("a channel of %d names and icons keeps %d and %d, want %d of each", repeated, len(c.Names), len(c.Icons), maxRepeated)
	}
	var p programmeElement
	if err := xml.Unmarshal([]byte(programme.String()), &p); err != nil {
		t.Fatal(err)
	}
	for name, kept := range map[string]int{
		"titles": len(p.Titles), "sub-titles": len(p.SubTitles), "descriptions": len(p.Description),
		"categories": len(p.Categories), "numberings": len(p.Episodes), "icons": len(p.Icons),
	} {
		if kept != maxRepeated {
			t.Errorf("a programme of %d %s keeps %d, want %d", repeated, name, kept, maxRepeated)
		}
	}

	document := `<tv>` + channel.String() + programme.String() +
		`<channel id="two"><display-name>Two</display-name></channel>` + programmeOpen + `</programme></tv>`
	channels, programmes, err := read(t, []byte(document), Options{Language: "l3"})
	if err != nil || len(channels) != 2 || len(programmes) != 2 {
		t.Fatalf("read: %v, %d channels, %d programmes", err, len(channels), len(programmes))
	}
	if len(channels[0].Names) != maxRepeated || channels[0].Names[maxRepeated-1] != fmt.Sprintf("Name %d", maxRepeated-1) {
		t.Errorf("the channel's names: %d, the last %q", len(channels[0].Names), channels[0].Names[len(channels[0].Names)-1])
	}
	first := programmes[0]
	// The first numbering that says something is the second one, E1.
	if len(first.Categories) != maxRepeated || first.Title != "Title 3" || first.SubTitle != "Sub 0" || first.Description != "Text 0" || first.Episode != 1 {
		t.Errorf("the programme: %d categories, %q, %q, %q, episode %d", len(first.Categories), first.Title, first.SubTitle, first.Description, first.Episode)
	}
	if channels[1].ID != "two" || len(channels[1].Names) != 1 || programmes[1].Title != "News" {
		t.Errorf("the elements after: %+v, %+v", channels[1], programmes[1])
	}
}

// A guide in another encoding is read through the same count, on its text
// once converted.
func TestAGuideInAnotherEncodingHasTheSameBound(t *testing.T) {
	t.Parallel()
	const declaration = `<?xml version="1.0" encoding="ISO-8859-1"?>`
	latin := func(document string) []byte {
		data := make([]byte, 0, len(document))
		for _, r := range document {
			data = append(data, byte(r))
		}
		return data
	}
	guide := declaration + `<tv><channel id="one"><display-name>Télé Une</display-name></channel>` +
		`<programme channel="one" start="20260104200000" stop="20260104210000"><title>Le Journal</title><desc>La météo, ` +
		strings.Repeat("é", 8192) + `</desc></programme></tv>`
	channels, programmes, err := read(t, latin(guide), Options{})
	if err != nil || len(channels) != 1 || len(programmes) != 1 {
		t.Fatalf("read: %v, %d channels, %d programmes", err, len(channels), len(programmes))
	}
	if channels[0].Names[0] != "Télé Une" || programmes[0].Description != "La météo, "+strings.Repeat("é", 8192) {
		t.Errorf("texts: %q, a description of %d bytes", channels[0].Names[0], len(programmes[0].Description))
	}
	for name, piece := range large() {
		// The pieces are ASCII, the same bytes in both encodings.
		data := []byte(declaration + before + piece + `</tv>`)
		source := &counted{r: bytes.NewReader(data)}
		err := Read(source, Options{}, func(Channel) error { return nil }, func(Programme) error { return nil })
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("%s: %v, want ErrTooLarge", name, err)
		}
		// Converting reads a block ahead too.
		if most := len(declaration) + len(before) + maxElement + 4*4096; source.read > most {
			t.Errorf("%s: %d bytes read of %d, want %d at most", name, source.read, len(data), most)
		}
	}
}
