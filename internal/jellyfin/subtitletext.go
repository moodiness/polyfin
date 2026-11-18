package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/speech"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// subtitleText is a subtitle track to serve in the format an app asks: its
// cues, and its script when it is ASS, whose styles, positions and fonts no
// other format keeps.
type subtitleText struct {
	cues   []subtitles.Cue
	script *subtitles.Script
	// bytes is the text's size once subtitleFile measured it (see size),
	// 0 before.
	bytes int
}

// readSubtitleText reads a subtitle file: SubRip, WebVTT or an ASS script.
func readSubtitleText(data []byte) (subtitleText, error) {
	if script, err := subtitles.ParseScript(data); err == nil {
		return subtitleText{cues: script.Cues(), script: script}, nil
	}
	cues, err := subtitles.Parse(data)
	return subtitleText{cues: cues}, err
}

// size is about what t takes in memory, as the cache of subtitle texts
// counts it: the length of its cues and script in JSON, measured once for
// a text read from a file (see subtitleFile).
func (t subtitleText) size() int {
	if t.bytes > 0 {
		return t.bytes
	}
	return cache.JSONSize(t.cues) + cache.JSONSize(t.script)
}

// errSubtitleTooLarge reports a subtitle file whose text is larger than
// the cache of subtitle texts keeps.
var errSubtitleTooLarge = errors.New("subtitle text too large to keep")

// errSubtitleReadPanicked reports a read of a subtitle file that ended on
// a panic.
var errSubtitleReadPanicked = errors.New("reading the subtitle file ended on a panic")

const (
	// keptSubtitleTexts is how many bytes of subtitle texts the cache
	// keeps (see New): a text larger than that on its own is not kept.
	// A file added from an app is checked against it, while the cache
	// itself tells subtitleFile what it keeps.
	keptSubtitleTexts = 32 << 20
	// maxRefusedSubtitles bounds the files remembered as refused for their
	// size, and refusedSubtitleLife how long each is: as many files, and
	// as long, as the cache of subtitle texts keeps texts (see New).
	maxRefusedSubtitles = 200
	refusedSubtitleLife = time.Hour
	// subtitleReadTime bounds a read of a subtitle file that requests
	// share, which no request's end stops: longer than an addon's file
	// may take to download, 20 seconds, and than a torrent is waited for
	// to start and give a piece, 105 seconds (see
	// playback.Service.TorrentSubtitle, which bounds its own fetch), and
	// as long as a request waits for a track inside a file (see
	// trackWait).
	subtitleReadTime = 2 * time.Minute
)

// subtitleReads shares the reads of subtitle files and remembers the
// files refused for their size, so that asking for one again costs
// nothing while it is remembered. Its zero value is ready.
type subtitleReads struct {
	// flight reads a file once for the requests asking for it at the same
	// time.
	flight singleflight.Group
	// reading, which tests set, is called as a read that requests share
	// begins, with its context; nil calls nothing.
	reading atomic.Pointer[func(ctx context.Context, file accounts.ID)]
	mu      sync.Mutex
	// refused are the files refused for their size, the oldest first, at
	// most maxRefusedSubtitles of them.
	refused []refusedSubtitle
}

// refusedSubtitle is a file refused for its size: when, and why.
type refusedSubtitle struct {
	file accounts.ID
	at   time.Time
	err  error
}

// refusal is why file was refused for its size, when it was at most
// refusedSubtitleLife before now; nil otherwise.
func (r *subtitleReads) refusal(file accounts.ID, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, refused := range r.refused {
		if refused.file == file && now.Sub(refused.at) <= refusedSubtitleLife {
			return refused.err
		}
	}
	return nil
}

// refuse remembers from now that file is refused with err. The refusals
// older than refusedSubtitleLife are dropped, then the oldest ones past
// maxRefusedSubtitles.
func (r *subtitleReads) refuse(file accounts.ID, err error, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refused = slices.DeleteFunc(r.refused, func(old refusedSubtitle) bool {
		return old.file == file || now.Sub(old.at) > refusedSubtitleLife
	})
	if over := len(r.refused) + 1 - maxRefusedSubtitles; over > 0 {
		r.refused = slices.Delete(r.refused, 0, over)
	}
	r.refused = append(r.refused, refusedSubtitle{file: file, at: now, err: err})
}

// forget drops the refusal of file, if any: a file replaced under the
// identifier it had is read when next asked for.
func (r *subtitleReads) forget(file accounts.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refused = slices.DeleteFunc(r.refused, func(old refusedSubtitle) bool { return old.file == file })
}

// keptSubtitle is what is kept of a subtitle file read lately: its text,
// or why it was refused for its size. known is false when neither is.
func (h *Handler) keptSubtitle(file accounts.ID) (text subtitleText, known bool, err error) {
	if text, ok := h.subtitleCache.Get(file); ok {
		return text, true, nil
	}
	if err := h.subtitleReads.refusal(file, h.now()); err != nil {
		return subtitleText{}, true, err
	}
	return subtitleText{}, false, nil
}

// subtitleFile reads a subtitle file (see readSubtitleFile), keeping its
// text for a while. The requests asking for a file at the same time share
// one read (see sharedSubtitleRead), which none of them ends by giving
// up; each waits on its own ctx. A text larger than the cache of subtitle
// texts keeps is refused, as a file that cannot be read is. Such a file,
// and one holding more cues than subtitles reads, is remembered for as
// long as a text is kept: asked for meanwhile, it is refused without
// being read again.
func (h *Handler) subtitleFile(ctx context.Context, file library.ExternalSubtitle) (subtitleText, error) {
	if text, known, err := h.keptSubtitle(file.ID); known {
		return text, err
	}
	detached := context.WithoutCancel(ctx)
	results := h.subtitleReads.flight.DoChan(string(file.ID[:]), func() (any, error) {
		return h.sharedSubtitleRead(detached, file)
	})
	select {
	case result := <-results:
		if result.Err != nil {
			return subtitleText{}, result.Err
		}
		return result.Val.(subtitleText), nil
	case <-ctx.Done():
		return subtitleText{}, ctx.Err()
	}
}

// sharedSubtitleRead is the read of file that the requests asking for it
// share. It runs outside any request, on ctx, which no request ends, so
// it bounds itself: what follows ctx ends after subtitleReadTime. A panic
// of what reads the file, which no request's handler stands above to
// recover, fails the read for the requests waiting for it and is logged
// with its stack; the server goes on, and the file is read again when
// next asked for.
func (h *Handler) sharedSubtitleRead(ctx context.Context, file library.ExternalSubtitle) (text subtitleText, err error) {
	ctx, cancel := context.WithTimeout(ctx, subtitleReadTime)
	defer cancel()
	defer func() {
		if cause := recover(); cause != nil {
			h.Logger.Error("Reading a subtitle file ended on a panic", "subtitle", file.ID.String(), "addon", file.Addon,
				"panic", fmt.Sprint(cause), "stack", string(debug.Stack()))
			text, err = subtitleText{}, fmt.Errorf("%w: %v", errSubtitleReadPanicked, cause)
		}
	}()
	// A read that ended as this one was asked for left its answer.
	if text, known, err := h.keptSubtitle(file.ID); known {
		return text, err
	}
	if reading := h.subtitleReads.reading.Load(); reading != nil {
		(*reading)(ctx, file.ID)
	}
	text, err = h.readSubtitleFile(ctx, file)
	if err == nil {
		// The text is measured here, once, rather than as the cache
		// stores it, which then tells at once whether it keeps it.
		text.bytes = text.size()
		if !h.subtitleCache.Update(file.ID, func(subtitleText, bool) (subtitleText, bool) { return text, true }) {
			text, err = subtitleText{}, errSubtitleTooLarge
		}
	}
	if errors.Is(err, errSubtitleTooLarge) || errors.Is(err, subtitles.ErrTooManyCues) {
		h.subtitleReads.refuse(file.ID, err, h.now())
	}
	return text, err
}

// write gives the text from start in format, with its content type: an
// ASS script as it is when the app asks for ASS. Other formats get an ASS
// track's text without its override tags, which apps would show as
// written: Jellyfin keeps them in SubRip and JSON, and drops them only in
// WebVTT. ok is false for a format Polyfin does not write.
func (t subtitleText) write(format string, start time.Duration) (data []byte, contentType string, ok bool) {
	cues := subtitles.From(t.cues, start)
	switch strings.ToLower(format) {
	case "vtt", "webvtt":
		return subtitles.WebVTT(cues), "text/vtt", true
	case "srt", "subrip":
		return subtitles.SubRip(cues), "application/x-subrip", true
	case "js", "json":
		return subtitles.TrackEvents(cues), "application/json", true
	case "ass", "ssa":
		if t.script != nil {
			return t.script.From(start).Bytes(), "text/x-ssa", true
		}
		return subtitles.ASS(cues), "text/x-ssa", true
	}
	return nil, "", false
}

// retimed is t with its times mapped by timing: its cues, and the events
// of its script. Those timing leaves out, ending before the version starts
// or in a scene the version lacks, are left out.
func (t subtitleText) retimed(timing speech.Timing) subtitleText {
	retimed := subtitleText{cues: timing.Apply(t.cues)}
	if t.script != nil {
		script := &subtitles.Script{Header: t.script.Header, Footer: t.script.Footer, Events: make([]subtitles.Event, 0, len(t.script.Events))}
		for _, event := range t.script.Events {
			var ok bool
			if event.Start, event.End, ok = timing.Map(event.Start, event.End); ok {
				script.Events = append(script.Events, event)
			}
		}
		retimed.script = script
	}
	return retimed
}
