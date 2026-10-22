package jellyfin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// textBurnFile writes the text subtitle of Jellyfin index index (a subtitle
// file among files, or a text track inside the version) where FFmpeg reads
// it to burn it in, as apps ask with SubtitleMethod=Encode: an ASS or SSA
// script as it is, which keeps its styles, any other as SubRip. It returns
// the file's path and whether it is a styled script; ok is false for a
// subtitle that is not text, or that cannot be read now, which then plays
// without it.
func (h *Handler) textBurnFile(ctx context.Context, version library.Version, analysis media.Analysis, files []library.ExternalSubtitle,
	index int) (path string, styled bool, ok bool) {
	var text subtitleText
	var key string
	switch {
	case index < 0:
		return "", false, false
	case index < len(files):
		file := files[index]
		if pictureFile(file) {
			return "", false, false
		}
		read, err := h.subtitleFile(ctx, file)
		if err != nil {
			h.Logger.Warn("A subtitle file to burn in could not be read", "error", err)
			return "", false, false
		}
		text, key = read, "file "+file.ID.String()
	default:
		stream := index - len(files)
		if !playback.ExtractableSubtitle(analysis, stream) {
			return "", false, false
		}
		track, err := h.Playback.SubtitleTrack(ctx, version, analysis, stream)
		if err != nil || track.Format == "sup" {
			h.Logger.Warn("A subtitle track to burn in could not be read", "stream", stream, "error", err)
			return "", false, false
		}
		read, err := readSubtitleText(track.Data)
		if err != nil {
			return "", false, false
		}
		text, key = read, "track "+version.ID.String()+" "+strconv.Itoa(stream)
	}
	format, ext := "srt", ".srt"
	if text.script != nil {
		format, ext, styled = "ass", ".ass", true
	}
	data, _, written := text.write(format, 0)
	if !written || len(data) == 0 {
		return "", false, false
	}
	sum := sha256.Sum256(append([]byte(key+"\x00"), data...))
	dir := filepath.Join(h.CacheDir, "burned-subtitles")
	path = filepath.Join(dir, hex.EncodeToString(sum[:12])+ext)
	if _, err := os.Stat(path); err == nil {
		return path, styled, true
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, false
	}
	partial := path + ".partial"
	if err := os.WriteFile(partial, data, 0o600); err != nil {
		return "", false, false
	}
	if err := os.Rename(partial, path); err != nil {
		return "", false, false
	}
	return path, styled, true
}
