package thumbnails

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/hls"
)

// Keyframes are decoded by FFmpeg, each alone, as soon as it is read:
// Polyfin writes it to FFmpeg as a Matroska stream of its own, with the
// source's codec description, and FFmpeg writes back the image, scaled
// down, as a YUV4MPEG frame for each size asked. Each keyframe is decoded
// by its own FFmpeg, so that its image is tied to it whatever the order the
// keyframes are read in: one decoder fed several keyframes may output them
// in another order, as HEVC's does with the CRA pictures of open GOPs,
// whose picture order counts it compares across keyframes.

// maxImageSide bounds the size of the images FFmpeg sends back.
const maxImageSide = 8192

// errNoImage reports a keyframe FFmpeg decoded no image of.
var errNoImage = errors.New("FFmpeg decoded no image of the keyframe")

// decodeKeyframe decodes frame, a keyframe of video, on hw when not nil,
// and returns its image through each of filters, which must end with
// yuv420p frames.
func decodeKeyframe(ctx context.Context, ffmpeg string, hw *hls.Hardware, video *container.Video, frame []byte, filters []string) ([]*image.YCbCr, error) {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	args = append(args, hw.DecodeInputs()...)
	args = append(args, "-f", "matroska", "-i", "pipe:0")
	if len(filters) == 1 {
		args = append(args, "-map", "0:v:0", "-vf", filters[0], "-frames:v", "1", "-f", "yuv4mpegpipe", "pipe:1")
	} else {
		graph := "[0:v:0]split=" + strconv.Itoa(len(filters))
		for i := range filters {
			graph += "[in" + strconv.Itoa(i) + "]"
		}
		for i, filter := range filters {
			graph += ";[in" + strconv.Itoa(i) + "]" + filter + "[out" + strconv.Itoa(i) + "]"
		}
		args = append(args, "-filter_complex", graph)
		for i := range filters {
			// The first output goes to the standard output, the others to
			// the descriptors after the standard error.
			fd := 1
			if i > 0 {
				fd = 2 + i
			}
			args = append(args, "-map", "[out"+strconv.Itoa(i)+"]", "-frames:v", "1", "-f", "yuv4mpegpipe", "pipe:"+strconv.Itoa(fd))
		}
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stdin = bytes.NewReader(append(streamHead(video), cluster(0, frame)...))
	stderr := &bytes.Buffer{}
	cmd.Stderr = &limitedBuffer{b: stderr, limit: 4 << 10}
	outputs := make([]io.ReadCloser, len(filters))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	outputs[0] = stdout
	var writers []*os.File
	defer func() {
		for _, w := range writers {
			_ = w.Close()
		}
	}()
	for i := 1; i < len(filters); i++ {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		outputs[i] = r
		defer r.Close()
		writers = append(writers, w)
		cmd.ExtraFiles = append(cmd.ExtraFiles, w)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start FFmpeg: %w", err)
	}
	// FFmpeg holds the write ends now: those here are closed, so that a
	// read ends with FFmpeg.
	for _, w := range writers {
		_ = w.Close()
	}
	writers = nil
	images := make([]*image.YCbCr, len(filters))
	errs := make([]error, len(filters))
	var wg sync.WaitGroup
	for i, output := range outputs {
		wg.Go(func() {
			images[i], errs[i] = firstImage(output)
			// FFmpeg must not block on a pipe no one reads.
			_, _ = io.Copy(io.Discard, output)
		})
	}
	wg.Wait()
	waitErr := cmd.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	for _, img := range images {
		if img == nil {
			if waitErr != nil {
				return nil, fmt.Errorf("%w: %w: %s", errNoImage, waitErr, strings.TrimSpace(stderr.String()))
			}
			return nil, errNoImage
		}
	}
	return images, nil
}

// errFirstImage stops readY4M once it read the first image.
var errFirstImage = errors.New("first image read")

// firstImage reads the first frame of a YUV4MPEG stream, nil when it has
// none.
func firstImage(r io.Reader) (*image.YCbCr, error) {
	var first *image.YCbCr
	_, err := readY4M(bufio.NewReaderSize(r, 1<<20), func(_ int, img *image.YCbCr) error {
		kept := *img
		kept.Y, kept.Cb, kept.Cr = slices.Clone(img.Y), slices.Clone(img.Cb), slices.Clone(img.Cr)
		first = &kept
		return errFirstImage
	})
	if errors.Is(err, errFirstImage) {
		err = nil
	}
	return first, err
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
