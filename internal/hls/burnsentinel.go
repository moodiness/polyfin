package hls

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FFmpeg 9.0 drops the end of a subtitle file burned in when it reads the
// whole file before its filter graph is set up, which a short file and a
// slow first video frame allow; overlay then waits for that end forever
// once the video passes the file's last picture. FFmpeg is given a copy of
// the file instead, a blank picture, the sentinel, appended past the end of
// the video: overlay always has a later picture queued and never needs the
// end, and the end of the video still ends the run.
const (
	// sentinelMargin is how far past the version's duration the sentinel
	// is shown. With -copyts the video keeps the source's times, which may
	// start late, and a container's duration may come short of its last
	// frame; FFmpeg never waits on the gap, overlay only needing the
	// sentinel queued, so a wide margin costs nothing.
	sentinelMargin = time.Hour
	// sentinelDefault is when it is shown in a version whose duration is
	// not known: past the end of any film.
	sentinelDefault = 12 * time.Hour
	// vobSubLatest is the last time an .idx file's two-digit hours, as
	// FFmpeg reads them, can write.
	vobSubLatest = 100*time.Hour - time.Millisecond
)

// errNoSentinel marks a subtitle file the sentinel cannot be added to:
// FFmpeg reads it as it is.
var errNoSentinel = errors.New("no sentinel can be added to the subtitle file")

// sentinelTime is when the sentinel of a version lasting duration, zero
// when not known, is shown, before each format's own limits.
func sentinelTime(duration time.Duration) time.Duration {
	if duration <= 0 {
		return sentinelDefault
	}
	return duration + sentinelMargin
}

// burnCopy is the copy, with a sentinel, of a subtitle file that the jobs
// of an encoding burn in: made once at name, by the first job that needs
// it, and removed with the encoding's folder. file is what FFmpeg reads
// once made: the copy, or the file itself when no copy could be made.
type burnCopy struct {
	name string
	mu   sync.Mutex
	made bool
	file string
}

// burnInput returns input reading the encoding's copy of the subtitle file
// it burns in, made at the first call (see writeBurnCopy), or input itself
// when it burns none. A job stopped while the copy is made leaves it to
// the next.
func (e *encoding) burnInput(ctx context.Context, input *Remux) *Remux {
	if input.Encode == nil || input.Encode.BurnFile == "" {
		return input
	}
	original := input.Encode.BurnFile
	e.mu.Lock()
	c := e.burns[original]
	if c == nil {
		c = &burnCopy{name: filepath.Join(e.dir, "burn-"+strconv.Itoa(len(e.burns)))}
		if e.burns == nil {
			e.burns = map[string]*burnCopy{}
		}
		e.burns[original] = c
	}
	e.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.made {
		file, ok := e.m.sentinelCopy(ctx, input, c.name)
		if !ok && ctx.Err() != nil {
			return input
		}
		c.made, c.file = true, file
	}
	return burning(input, c.file)
}

// sentinelCopy writes a copy of the subtitle file r burns in, with a
// sentinel, at name and the file's extension, and returns its path and
// true; when it cannot, it logs why and returns the file r burns in and
// false.
func (m *Manager) sentinelCopy(ctx context.Context, r *Remux, name string) (string, bool) {
	original := r.Encode.BurnFile
	file, err := writeBurnCopy(ctx, original, name, sentinelTime(max(r.Duration, r.Plan.duration)+r.Shift))
	if err != nil {
		if ctx.Err() == nil {
			m.logger.Info("A subtitle file is burned in as it is, without a sentinel: FFmpeg 9.0 may stop for good at its end", "file", original, "error", err)
		}
		return original, false
	}
	return file, true
}

// burning returns a copy of r burning file in, r itself when it already
// does: the Remux of a job is shared with the encoding's other jobs.
func burning(r *Remux, file string) *Remux {
	if r.Encode.BurnFile == file {
		return r
	}
	burned, encode := *r, *r.Encode
	encode.BurnFile = file
	burned.Encode = &encode
	return &burned
}

// writeBurnCopy writes a copy of original, a PGS file or a VobSub pair's
// .idx file, by path or http URL, with a sentinel at, or later in a file
// with later pictures, at name and its extension, and returns the copy's
// path: name.sup, or name.idx beside name.sub. A file whose format or
// layout it does not know is errNoSentinel; nothing is left behind on an
// error.
func writeBurnCopy(ctx context.Context, original, name string, at time.Duration) (string, error) {
	if len(original) < 4 || original[len(original)-4] != '.' {
		return "", errNoSentinel
	}
	switch strings.ToLower(original[len(original)-3:]) {
	case "sup":
		out := name + ".sup"
		if err := writeCopy(ctx, original, out, func(w io.Writer, r io.Reader) error { return copyPGS(w, r, at) }); err != nil {
			return "", err
		}
		return out, nil
	case "idx":
		// FFmpeg reads the .sub file of the same name, the extension's
		// last three letters replaced by SUB after IDX, else by sub.
		sub := original[:len(original)-3] + "sub"
		if original[len(original)-3:] == "IDX" {
			sub = original[:len(original)-3] + "SUB"
		}
		var idx []byte
		if err := readSource(ctx, original, func(r io.Reader) (err error) {
			idx, err = io.ReadAll(r)
			return err
		}); err != nil {
			return "", err
		}
		if err := writeCopy(ctx, sub, name+".sub", func(w io.Writer, r io.Reader) error {
			size, err := io.Copy(w, r)
			if err != nil {
				return err
			}
			marked, packs, err := vobSubSentinel(string(idx), size, at)
			if err != nil {
				return err
			}
			if _, err := w.Write(packs); err != nil {
				return err
			}
			idx = []byte(marked)
			return nil
		}); err != nil {
			return "", err
		}
		out := name + ".idx"
		if err := os.WriteFile(out, idx, 0o600); err != nil {
			_ = os.Remove(out)
			_ = os.Remove(name + ".sub")
			return "", err
		}
		return out, nil
	}
	return "", errNoSentinel
}

// writeCopy writes to the file out what write makes of the file original,
// removing out when either fails.
func writeCopy(ctx context.Context, original, out string, write func(io.Writer, io.Reader) error) error {
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	err = readSource(ctx, original, func(r io.Reader) error { return write(w, r) })
	if err == nil {
		err = w.Flush()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return errors.Join(err, os.Remove(out))
	}
	return nil
}

// readSource calls use with the file at name, a path or an http URL.
func readSource(ctx context.Context, name string, use func(io.Reader) error) error {
	if !strings.HasPrefix(name, "http://") && !strings.HasPrefix(name, "https://") {
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		return use(bufio.NewReader(f))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, name, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", name, resp.Status)
	}
	return use(bufio.NewReader(resp.Body))
}

// pgsSegment is a segment's header in a PGS file: PG, its presentation
// and decoding times, its type and its size.
const pgsSegment = 13

// copyPGS copies a PGS file from r to w, then a display set clearing the
// screen at at, or a second after the file's last segment when later:
// a presentation of no objects, with the canvas size and frame rate of the
// file's last one, which a change of canvas would need FFmpeg to accept,
// and its end segment. A file whose segments do not follow each other to
// its end, or without a presentation, is errNoSentinel.
func copyPGS(w io.Writer, r io.Reader, at time.Duration) error {
	var (
		header [pgsSegment]byte
		last   [11]byte
		found  bool
		latest uint32
		buf    []byte
	)
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			if err == io.EOF {
				break
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return errNoSentinel
			}
			return err
		}
		if header[0] != 'P' || header[1] != 'G' {
			return errNoSentinel
		}
		size := int(binary.BigEndian.Uint16(header[11:]))
		if cap(buf) < size {
			buf = make([]byte, size)
		}
		payload := buf[:size]
		if _, err := io.ReadFull(r, payload); err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				return errNoSentinel
			}
			return err
		}
		// A presentation composition segment, 11 bytes before its
		// objects.
		if header[10] == 0x16 && size >= len(last) {
			copy(last[:], payload)
			found = true
		}
		latest = max(latest, binary.BigEndian.Uint32(header[2:]))
		if _, err := w.Write(header[:]); err != nil {
			return err
		}
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	if !found {
		return errNoSentinel
	}
	ticks := max(uint64(at.Milliseconds())*90, uint64(latest)+90000)
	if ticks > 0xFFFFFFFF {
		// Within the clock, if still past the last segment.
		if latest >= 0xFFFFFFFF {
			return errNoSentinel
		}
		ticks = 0xFFFFFFFF
	}
	segment := func(kind byte, payload []byte) error {
		h := [pgsSegment]byte{'P', 'G', 10: kind}
		binary.BigEndian.PutUint32(h[2:], uint32(ticks))
		binary.BigEndian.PutUint32(h[6:], uint32(ticks))
		binary.BigEndian.PutUint16(h[11:], uint16(len(payload)))
		if _, err := w.Write(h[:]); err != nil {
			return err
		}
		_, err := w.Write(payload)
		return err
	}
	// The last one's width, height and frame rate; the next composition
	// number; a normal state, no palette update, palette 0, no objects.
	number := binary.BigEndian.Uint16(last[5:]) + 1
	pcs := [11]byte{last[0], last[1], last[2], last[3], last[4], byte(number >> 8), byte(number)}
	if err := segment(0x16, pcs[:]); err != nil {
		return err
	}
	return segment(0x80, nil)
}

// The lines of an .idx file FFmpeg reads, as it reads them: a track's
// start, its pictures' times and places in the .sub file, and a delay
// added to the times of the lines after it.
var (
	idxTrack     = regexp.MustCompile(`^id:\s*[^,]+,\s*index:\s*(\d+)`)
	idxTimestamp = regexp.MustCompile(`^timestamp:\s*(\d{1,2}):(\d{1,2}):(\d{1,2}):(\d{1,3}),\s*filepos:\s*[0-9A-Fa-f]+`)
	idxDelay     = regexp.MustCompile(`^delay:\s*([-+]?)(\d+):(\d+):(\d+):(\d+)`)
)

// vobSubTracks is how many tracks FFmpeg reads from an .idx file.
const vobSubTracks = 32

// vobSubSentinel returns a VobSub pair's .idx file listing, in each track
// with pictures, two packs that only stop the display, at at, or a second
// after the pair's last picture when later, and a second after that, and
// those packs, to append to its .sub file of size bytes. FFmpeg makes a
// stream of each such track and burns the first, but each track gets the
// packs, its index in their substream as FFmpeg requires. They decode to no
// picture, but each packet read makes FFmpeg repeat the canvas shown: the
// first ends the last picture at its end, which the file's end does
// otherwise, and the second repeats the blank canvas past the end of the
// video. An .idx file FFmpeg would refuse, or whose times would pass the
// last it can write, is errNoSentinel.
func vobSubSentinel(idx string, size int64, at time.Duration) (string, []byte, error) {
	lines := strings.SplitAfter(idx, "\n")
	type track struct {
		index int
		// after is the line of the track's last picture, and delay the
		// delay in effect there.
		after int
		delay time.Duration
	}
	var tracks []*track
	byIndex := map[int]*track{}
	current := -1
	var delay, latest time.Duration
	for i, line := range lines {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "id:"):
			// FFmpeg takes a line it cannot read for index 0.
			current = 0
			if m := idxTrack.FindStringSubmatch(line); m != nil {
				n, err := strconv.Atoi(m[1])
				if err != nil || n >= vobSubTracks {
					return "", nil, errNoSentinel
				}
				current = n
			}
		case strings.HasPrefix(line, "timestamp:"):
			m := idxTimestamp.FindStringSubmatch(line)
			if current < 0 || m == nil {
				return "", nil, errNoSentinel
			}
			var parts [4]int
			for k := range parts {
				parts[k], _ = strconv.Atoi(m[k+1])
			}
			t := time.Duration(parts[0])*time.Hour + time.Duration(parts[1])*time.Minute + time.Duration(parts[2])*time.Second +
				time.Duration(parts[3])*time.Millisecond
			latest = max(latest, t+delay)
			tr := byIndex[current]
			if tr == nil {
				tr = &track{index: current}
				byIndex[current] = tr
				tracks = append(tracks, tr)
			}
			tr.after, tr.delay = i, delay
		case strings.HasPrefix(line, "delay:"):
			m := idxDelay.FindStringSubmatch(line)
			if m == nil {
				return "", nil, errNoSentinel
			}
			var parts [4]int
			for k := range parts {
				parts[k], _ = strconv.Atoi(m[k+2])
			}
			delay = time.Duration(parts[0])*time.Hour + time.Duration(parts[1])*time.Minute + time.Duration(parts[2])*time.Second +
				time.Duration(parts[3])*time.Millisecond
			if m[1] == "-" {
				delay = -delay
			}
		}
	}
	if len(tracks) == 0 {
		return "", nil, errNoSentinel
	}
	at = max(at, latest+time.Second)
	var packs []byte
	added := make(map[int]string, len(tracks))
	for _, tr := range tracks {
		// The times written are before the track's delay, which FFmpeg
		// adds.
		first := min(at-tr.delay, vobSubLatest-time.Second)
		if first+tr.delay <= latest {
			return "", nil, errNoSentinel
		}
		ending := "\n"
		if strings.HasSuffix(lines[tr.after], "\r\n") {
			ending = "\r\n"
		}
		var listed strings.Builder
		if !strings.HasSuffix(lines[tr.after], "\n") {
			listed.WriteString(ending)
		}
		for _, t := range []time.Duration{first, first + time.Second} {
			ms := t.Milliseconds()
			fmt.Fprintf(&listed, "timestamp: %02d:%02d:%02d:%03d, filepos: %09x%s", ms/3600000, ms/60000%60, ms/1000%60, ms%1000,
				size+int64(len(packs)), ending)
			packs = vobSubPack(packs, tr.index, t+tr.delay)
		}
		added[tr.after] = listed.String()
	}
	var out strings.Builder
	out.Grow(len(idx) + len(tracks)*2*48)
	for i, line := range lines {
		out.WriteString(line)
		out.WriteString(added[i])
	}
	return out.String(), packs, nil
}

// vobSubPack appends to packs a 2048-byte pack of the .sub file of a
// VobSub pair, holding a subpicture of track index at at that only stops
// the display.
func vobSubPack(packs []byte, index int, at time.Duration) []byte {
	const packSize = 2048
	start := len(packs)
	// The subpicture: its size and its control sequence's offset, then
	// that sequence, the last, at once: stop the display.
	spu := []byte{0, 10, 0, 4, 0, 0, 0, 4, 0x02, 0xFF}
	pts := uint64(at.Milliseconds()) * 90
	// The private stream 1 packet: MPEG-2 flags, the time, then the
	// substream, 0x20 and the track's index.
	pes := []byte{0x81, 0x80, 5, 0x21 | byte(pts>>29)&0x0E, byte(pts >> 22), byte(pts>>14) | 1, byte(pts >> 7), byte(pts<<1) | 1,
		0x20 + byte(index)}
	length := len(pes) + len(spu)
	// The pack header, then the packet.
	packs = append(packs, 0, 0, 1, 0xBA, 0x44, 0, 4, 0, 4, 1, 1, 0x89, 0xC3, 0xF8, 0, 0, 1, 0xBD, byte(length>>8), byte(length))
	packs = append(append(packs, pes...), spu...)
	// A padding packet fills the pack.
	padding := packSize - (len(packs) - start) - 6
	packs = append(packs, 0, 0, 1, 0xBE, byte(padding>>8), byte(padding))
	for range padding {
		packs = append(packs, 0xFF)
	}
	return packs
}
