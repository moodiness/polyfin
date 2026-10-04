package jellyfin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// ChapterInfo is a chapter of a video, as Jellyfin describes it. Polyfin
// makes no chapter images: Jellyfin then leaves out the image's path and
// tag, and dates it at its unset date.
type ChapterInfo struct {
	StartPositionTicks int64
	Name               string
	ImageDateModified  Time
}

// setChapters fills the chapters of an item that asked for them with those
// of the version it plays first: the version it was opened as, else the
// addons' first. Polyfin reads chapters when it analyzes a version, on its
// first play, so the chapters of a version never played are not known yet.
func (h *Handler) setChapters(ctx context.Context, dto *BaseItemDto, versions []library.Version) {
	if dto.Chapters == nil {
		return
	}
	chapters := []ChapterInfo{}
	if len(versions) > 0 {
		if analysis, ok := h.Playback.Analyzed(ctx, versions[0].ID); ok {
			chapters = chapterInfos(analysis.Chapters, h.Accounts.Settings().Language)
		}
	}
	dto.Chapters = &chapters
}

// chapterInfos describes chapters as Jellyfin does: their start to the
// millisecond, and "Chapter N" for those whose title is empty or a time, as
// some rippers write.
func chapterInfos(chapters []media.Chapter, language string) []ChapterInfo {
	infos := make([]ChapterInfo, len(chapters))
	for i, chapter := range chapters {
		name := chapter.Title
		if strings.TrimSpace(name) == "" || timeSpan(name) {
			name = library.ChapterName(language, i+1)
		}
		infos[i] = ChapterInfo{
			StartPositionTicks: int64(chapter.Start.Round(time.Millisecond) / 100),
			Name:               name,
		}
	}
	return infos
}

// timeSpan reports whether a chapter title reads as a time the way .NET
// reads one without regard to culture, which is how Jellyfin tells: whole
// days ("7"), or hours and minutes with optional days, seconds and
// fraction ("00:05", "1.02:03:04.5", "1:02:03:04").
func timeSpan(title string) bool {
	s := strings.TrimPrefix(strings.TrimSpace(title), "-")
	if decimalDigits(s) {
		return true
	}
	clock, hasDays := s, false
	if day, rest, ok := strings.Cut(s, "."); ok && decimalDigits(day) && strings.Contains(rest, ":") {
		clock, hasDays = rest, true
	}
	if colon := strings.LastIndexByte(clock, ':'); colon >= 0 {
		if seconds, fraction, ok := strings.Cut(clock[colon+1:], "."); ok {
			if len(fraction) > 7 || !decimalDigits(fraction) || strings.Count(clock, ":") < 2 {
				return false
			}
			clock = clock[:colon+1] + seconds
		}
	}
	parts := strings.Split(clock, ":")
	switch {
	case len(parts) == 4 && !hasDays:
		// The days come first, before a colon.
		if !decimalDigits(parts[0]) {
			return false
		}
		parts = parts[1:]
	case len(parts) != 2 && len(parts) != 3:
		return false
	}
	limits := []int{24, 60, 60}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if !decimalDigits(part) || err != nil || n >= limits[i] {
			return false
		}
	}
	return true
}

// decimalDigits reports whether s is a non-empty run of decimal digits.
func decimalDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
