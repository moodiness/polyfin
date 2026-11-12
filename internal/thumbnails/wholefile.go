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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/process"
)

// A local folder's file whose container has no index Polyfin reads, as
// AVI, MPEG-TS, M2TS and MPEG program streams have none, has its images
// made by FFmpeg reading it whole, decoding its keyframes alone
// (-skip_frame nokey), as Jellyfin's keyframe-only extraction does: one
// thumbnail every interval, showing the keyframe at its time, as FFmpeg's
// fps filter picks it. A chapter image shows the keyframe of the
// thumbnail nearest its start.

// fileChapterInterval spaces the keyframes chapter images are chosen
// among, when no thumbnail is made.
const fileChapterInterval = 10 * time.Second

// makeFromFile makes what a local folder's file lacks of its images, from
// the whole file, FFmpeg running at the lowest priority. It waits while
// that file plays: a play starting stops FFmpeg, which starts again once
// it is over.
func (s *Service) makeFromFile(ctx context.Context, version library.Version, analysis media.Analysis, want wanted) (reads, error) {
	stream, ok := videoStream(analysis)
	if !ok {
		return reads{}, errNoVideo
	}
	for {
		for s.playsFile(version.URL) {
			if err := sleep(ctx, s.Poll); err != nil {
				return reads{}, err
			}
		}
		run, cancel := context.WithCancel(ctx)
		played := make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			ticker := time.NewTicker(s.Poll)
			defer ticker.Stop()
			for {
				select {
				case <-run.Done():
					return
				case <-ticker.C:
					if s.playsFile(version.URL) {
						close(played)
						cancel()
						return
					}
				}
			}
		})
		read, err := s.decodeFile(run, version, analysis, stream, want)
		cancel()
		wg.Wait()
		select {
		case <-played:
			if err != nil && ctx.Err() == nil {
				continue
			}
		default:
		}
		return read, err
	}
}

// decodeFile has FFmpeg read the file of version whole, decoding its
// keyframes alone, and keeps the thumbnails and chapter images want asks
// for. A file FFmpeg reads to its end without decoding a keyframe cannot
// give images (container.ErrUnreadable); FFmpeg failing, as when a share
// cannot be read, fails the version, which is tried again.
func (s *Service) decodeFile(ctx context.Context, version library.Version, analysis media.Analysis, stream media.Stream, want wanted) (reads, error) {
	interval := want.interval
	if !want.trickplay {
		interval = fileChapterInterval
	}
	count := thumbnailCount(analysis.Duration, interval)
	_, hdr := s.decoder(stream)
	var filters []string
	if want.trickplay {
		filters = append(filters, thumbnailFilter(want.width, hdr))
	}
	if len(want.chapters) > 0 {
		filters = append(filters, chapterFilter(hdr))
	}
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-skip_frame", "nokey", "-i", version.URL}
	// One frame every interval from the start, each the keyframe at its
	// time, the first padded back to it, and the last kept past the end.
	pick := "fps=fps=1000/" + strconv.FormatInt(interval.Milliseconds(), 10) + ":start_time=0:eof_action=pass"
	args = append(args, outputArgs("0:"+strconv.Itoa(stream.Index), pick, filters)...)
	cmd := exec.CommandContext(ctx, s.FFmpeg, args...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = &limitedBuffer{b: stderr, limit: 4 << 10}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return reads{}, err
	}
	extra, err := process.NewOutputs(cmd, len(filters)-1)
	if err != nil {
		return reads{}, err
	}
	outputs := append([]io.ReadCloser{stdout}, extra.Readers...)
	for _, r := range extra.Readers {
		defer r.Close()
	}
	err = cmd.Start()
	extra.Started(cmd, err)
	if err != nil {
		return reads{}, fmt.Errorf("start FFmpeg: %w", err)
	}
	media.LowerPriority(cmd.Process.Pid)
	// Each output is read as FFmpeg writes it: the thumbnails go into
	// tiles, the chapter images are encoded as their frame comes.
	var thumbnails *fileThumbnails
	var chapters *fileChapters
	readers := make([]func(int, *image.YCbCr) error, 0, len(filters))
	if want.trickplay {
		thumbnails = &fileThumbnails{tiles: newTiler(count, interval), count: count}
		thumbnails.tiles.info.Width = want.width
		readers = append(readers, thumbnails.add)
	}
	if len(want.chapters) > 0 {
		chapters = newFileChapters(want.chapters, interval, count)
		readers = append(readers, chapters.add)
	}
	frames := make([]int, len(outputs))
	errs := make([]error, len(outputs))
	var wg sync.WaitGroup
	for i, output := range outputs {
		wg.Go(func() {
			frames[i], errs[i] = readY4M(bufio.NewReaderSize(output, 1<<20), readers[i])
			// FFmpeg must not block on a pipe no one reads.
			_, _ = io.Copy(io.Discard, output)
		})
	}
	wg.Wait()
	waitErr := cmd.Wait()
	read := reads{keyframes: frames[0]}
	switch {
	case ctx.Err() != nil:
		return read, ctx.Err()
	case waitErr != nil:
		return read, fmt.Errorf("FFmpeg: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if err := errors.Join(errs...); err != nil {
		return read, err
	}
	if slices.Contains(frames, 0) {
		return read, fmt.Errorf("%w: FFmpeg decoded no keyframe", container.ErrUnreadable)
	}
	if thumbnails != nil {
		info, data, err := thumbnails.finish()
		if err != nil {
			return read, err
		}
		if err := s.SaveTrickplay(ctx, version.ID, version.Item, info, data); err != nil {
			return read, err
		}
	}
	if chapters != nil {
		data, err := chapters.finish()
		if err != nil {
			return read, err
		}
		if err := s.SaveChapterImages(ctx, version.ID, version.Item, data); err != nil {
			return read, err
		}
	}
	return read, nil
}

// fileThumbnails packs the count thumbnails FFmpeg writes, those past
// count left out; the last one written stands for those it did not write,
// as its fps filter may stop short of the end.
type fileThumbnails struct {
	tiles *tiler
	count int
	added int
	last  *image.YCbCr
}

func (f *fileThumbnails) add(_ int, img *image.YCbCr) error {
	if f.added == f.count {
		return nil
	}
	f.last = keepImage(f.last, img)
	f.added++
	return f.tiles.add(img)
}

func (f *fileThumbnails) finish() (Info, [][]byte, error) {
	for ; f.added < f.count; f.added++ {
		if err := f.tiles.add(f.last); err != nil {
			return Info{}, nil, err
		}
	}
	return f.tiles.finish()
}

// fileChapters encodes the image of each chapter from the frame FFmpeg
// writes nearest its start, one every interval; the last frame written
// stands for those it did not write.
type fileChapters struct {
	// at is the frame each chapter shows.
	at   []int
	data [][]byte
	last *image.YCbCr
}

func newFileChapters(chapters []media.Chapter, interval time.Duration, count int) *fileChapters {
	f := &fileChapters{at: make([]int, len(chapters)), data: make([][]byte, len(chapters))}
	for i, chapter := range chapters {
		f.at[i] = min(count-1, int((chapter.Start+interval/2)/interval))
	}
	return f
}

func (f *fileChapters) add(n int, img *image.YCbCr) error {
	f.last = keepImage(f.last, img)
	for i, at := range f.at {
		if at == n {
			data, err := encodeJPEG(img)
			if err != nil {
				return err
			}
			f.data[i] = data
		}
	}
	return nil
}

func (f *fileChapters) finish() ([][]byte, error) {
	var last []byte
	for i := range f.data {
		if f.data[i] != nil {
			continue
		}
		if last == nil {
			var err error
			if last, err = encodeJPEG(f.last); err != nil {
				return nil, err
			}
		}
		f.data[i] = last
	}
	return f.data, nil
}

// keepImage copies img into kept, which it returns, allocated at the first
// image: readY4M reuses its image for each frame.
func keepImage(kept, img *image.YCbCr) *image.YCbCr {
	if kept == nil || kept.Rect != img.Rect {
		copied := *img
		copied.Y, copied.Cb, copied.Cr = slices.Clone(img.Y), slices.Clone(img.Cb), slices.Clone(img.Cr)
		return &copied
	}
	copy(kept.Y, img.Y)
	copy(kept.Cb, img.Cb)
	copy(kept.Cr, img.Cr)
	return kept
}
