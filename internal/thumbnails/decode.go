package thumbnails

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
)

// Keyframes are decoded by FFmpeg, read alone from the source: Polyfin
// writes them to FFmpeg as a Matroska stream of their own, with the
// source's codec description, one keyframe a second, and FFmpeg writes
// back each scaled down, as YUV4MPEG frames. Every frame a keyframe, each
// decodes alone; a constant output rate of one frame a second keeps
// FFmpeg's frames in step with the keyframes written, repeating the one
// before a keyframe that does not decode.

// decoder is FFmpeg decoding keyframes into images.
type decoder struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	writer *bufio.Writer
	// written counts the keyframes written.
	written int
	// read reports, once FFmpeg's output ends, the images read and why
	// reading stopped.
	read   chan decoded
	stderr *bytes.Buffer
	// ended is set once finish or kill waited for FFmpeg.
	ended bool
}

type decoded struct {
	images int
	err    error
}

// maxImageSide bounds the size of the images FFmpeg sends back.
const maxImageSide = 8192

// startDecoder starts FFmpeg decoding the keyframes of video written to
// it, on hw when not nil, through filter, which must end with yuv420p
// frames. onImage receives each image in order, valid until it returns.
func startDecoder(ctx context.Context, ffmpeg string, hw *hls.Hardware, video *container.Video, filter string,
	onImage func(i int, img *image.YCbCr) error) (*decoder, error) {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	args = append(args, hw.DecodeInputs()...)
	args = append(args, "-f", "matroska", "-i", "pipe:0", "-map", "0:v:0", "-an", "-sn", "-dn",
		"-vf", filter, "-fps_mode", "cfr", "-r", "1", "-f", "yuv4mpegpipe", "pipe:1")
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = &limitedBuffer{b: stderr, limit: 4 << 10}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start FFmpeg: %w", err)
	}
	d := &decoder{cmd: cmd, stdin: stdin, writer: bufio.NewWriterSize(stdin, 256<<10), read: make(chan decoded, 1), stderr: stderr}
	go func() {
		n, err := readY4M(bufio.NewReaderSize(stdout, 1<<20), onImage)
		if err != nil {
			// FFmpeg must not block on a pipe no one reads.
			_, _ = io.Copy(io.Discard, stdout)
		}
		d.read <- decoded{n, err}
	}()
	if _, err := d.writer.Write(streamHead(video)); err != nil {
		d.kill()
		return nil, fmt.Errorf("write to FFmpeg: %w", err)
	}
	return d, nil
}

// write writes the next keyframe.
func (d *decoder) write(frame []byte) error {
	_, err := d.writer.Write(cluster(int64(d.written)*1000, frame))
	d.written++
	if err != nil {
		return fmt.Errorf("write to FFmpeg: %w", err)
	}
	return nil
}

// finish ends the stream and waits for FFmpeg, returning the images read.
func (d *decoder) finish() (int, error) {
	d.ended = true
	err := d.writer.Flush()
	if closeErr := d.stdin.Close(); err == nil {
		err = closeErr
	}
	result := <-d.read
	waitErr := d.cmd.Wait()
	switch {
	case result.err != nil:
		return result.images, result.err
	case waitErr != nil:
		return result.images, fmt.Errorf("FFmpeg: %w: %s", waitErr, strings.TrimSpace(d.stderr.String()))
	case err != nil:
		return result.images, fmt.Errorf("write to FFmpeg: %w", err)
	}
	return result.images, nil
}

// kill stops FFmpeg, unless finish waited for it, and waits for it.
func (d *decoder) kill() {
	if d.ended {
		return
	}
	d.ended = true
	if d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
	}
	_ = d.stdin.Close()
	<-d.read
	_ = d.cmd.Wait()
}

// limitedBuffer keeps the first bytes FFmpeg writes to its error output.
type limitedBuffer struct {
	b     *bytes.Buffer
	limit int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.limit - l.b.Len(); room > 0 {
		l.b.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// The Matroska elements of the stream written to FFmpeg.
const (
	idEBML               = 0x1A45DFA3
	idEBMLVersion        = 0x4286
	idEBMLReadVersion    = 0x42F7
	idEBMLMaxIDLength    = 0x42F2
	idEBMLMaxSizeLength  = 0x42F3
	idDocType            = 0x4282
	idDocTypeVersion     = 0x4287
	idDocTypeReadVersion = 0x4285
	idSegment            = 0x18538067
	idInfo               = 0x1549A966
	idTimestampScale     = 0x2AD7B1
	idTracks             = 0x1654AE6B
	idTrackEntry         = 0xAE
	idTrackNumber        = 0xD7
	idTrackUID           = 0x73C5
	idTrackType          = 0x83
	idCodecID            = 0x86
	idCodecPrivate       = 0x63A2
	idDefaultDuration    = 0x23E383
	idVideo              = 0xE0
	idCluster            = 0x1F43B675
	idTimestamp          = 0xE7
	idSimpleBlock        = 0xA3
)

// streamHead is the start of the stream: the EBML header, a Segment of
// unknown size, its timestamps in milliseconds, and the video track.
func streamHead(video *container.Video) []byte {
	var b []byte
	b = element(b, idEBML, concat(
		uintElement(idEBMLVersion, 1), uintElement(idEBMLReadVersion, 1),
		uintElement(idEBMLMaxIDLength, 4), uintElement(idEBMLMaxSizeLength, 8),
		element(nil, idDocType, []byte("matroska")),
		uintElement(idDocTypeVersion, 4), uintElement(idDocTypeReadVersion, 2)))
	b = appendID(b, idSegment)
	b = append(b, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
	b = element(b, idInfo, uintElement(idTimestampScale, 1_000_000))
	entry := concat(uintElement(idTrackNumber, 1), uintElement(idTrackUID, 1), uintElement(idTrackType, 1),
		element(nil, idCodecID, []byte(video.CodecID)), uintElement(idDefaultDuration, 1_000_000_000))
	if len(video.CodecPrivate) > 0 {
		entry = element(entry, idCodecPrivate, video.CodecPrivate)
	}
	if len(video.Settings) > 0 {
		entry = element(entry, idVideo, video.Settings)
	}
	return element(b, idTracks, element(nil, idTrackEntry, entry))
}

// cluster is a Cluster holding frame alone, as a keyframe of track 1 at
// timestamp milliseconds.
func cluster(timestamp int64, frame []byte) []byte {
	block := make([]byte, 0, 4+len(frame))
	block = append(block, 0x81, 0, 0, 0x80)
	block = append(block, frame...)
	return element(nil, idCluster, concat(uintElement(idTimestamp, uint64(timestamp)), element(nil, idSimpleBlock, block)))
}

// element appends an element of data, its size on 8 bytes.
func element(b []byte, id uint32, data []byte) []byte {
	b = appendID(b, id)
	size := uint64(len(data))
	b = append(b, 0x01, byte(size>>48), byte(size>>40), byte(size>>32), byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
	return append(b, data...)
}

func uintElement(id uint32, value uint64) []byte {
	return element(nil, id, []byte{byte(value >> 56), byte(value >> 48), byte(value >> 40), byte(value >> 32),
		byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
}

func appendID(b []byte, id uint32) []byte {
	switch {
	case id > 0xFFFFFF:
		return append(b, byte(id>>24), byte(id>>16), byte(id>>8), byte(id))
	case id > 0xFFFF:
		return append(b, byte(id>>16), byte(id>>8), byte(id))
	case id > 0xFF:
		return append(b, byte(id>>8), byte(id))
	}
	return append(b, byte(id))
}

func concat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

// errY4M reports output that is not the YUV4MPEG stream asked for.
var errY4M = errors.New("FFmpeg wrote an unexpected stream")

// readY4M reads a YUV4MPEG stream of 4:2:0 frames, passing each to fn,
// and returns how many it read.
func readY4M(r *bufio.Reader, fn func(i int, img *image.YCbCr) error) (int, error) {
	line, err := r.ReadString('\n')
	if errors.Is(err, io.EOF) && line == "" {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errY4M, err)
	}
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "YUV4MPEG2" {
		return 0, fmt.Errorf("%w: no YUV4MPEG header", errY4M)
	}
	width, height := 0, 0
	for _, field := range fields[1:] {
		switch field[0] {
		case 'W':
			width, _ = strconv.Atoi(field[1:])
		case 'H':
			height, _ = strconv.Atoi(field[1:])
		case 'C':
			if !strings.HasPrefix(field, "C420") {
				return 0, fmt.Errorf("%w: chroma %s", errY4M, field)
			}
		}
	}
	if width <= 0 || height <= 0 || width > maxImageSide || height > maxImageSide {
		return 0, fmt.Errorf("%w: %dx%d", errY4M, width, height)
	}
	img := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	for n := 0; ; n++ {
		line, err := r.ReadString('\n')
		if errors.Is(err, io.EOF) && line == "" {
			return n, nil
		}
		if err != nil || !strings.HasPrefix(line, "FRAME") {
			return n, fmt.Errorf("%w: frame %d", errY4M, n)
		}
		for _, plane := range [][]byte{img.Y, img.Cb, img.Cr} {
			if _, err := io.ReadFull(r, plane); err != nil {
				return n, fmt.Errorf("%w: frame %d: %v", errY4M, n, err)
			}
		}
		if err := fn(n, img); err != nil {
			return n, err
		}
	}
}
