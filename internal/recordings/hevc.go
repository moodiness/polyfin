package recordings

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/diskspace"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
)

// Converter converts files to HEVC; hls.Manager is one.
type Converter interface {
	// Hardware is the GPU conversions use, nil for none, and Encoders the
	// encoders of the installed FFmpeg.
	Hardware() *hls.Hardware
	Encoders() []string
	// ConvertFile converts a file whole, once a conversion of video may
	// start, until ctx ends (see hls.Manager.ConvertFile).
	ConvertFile(ctx context.Context, c hls.FileConversion, progress func(time.Duration)) error
	// DecodedLength decodes the video of a file to its end, and reports
	// how far it went.
	DecodedLength(ctx context.Context, path string) (time.Duration, error)
}

// Prober analyzes files; media.Prober is one.
type Prober interface {
	ProbeInput(ctx context.Context, in media.Input) (media.Analysis, error)
}

// Where a recording is in its conversion to HEVC, as live_recordings.hevc
// keeps it.
const (
	// hevcToDo is a recording not converted yet, or whose conversion was
	// stopped at the end of its hours, to start over.
	hevcToDo = ""
	// hevcPending is a recording whose copy passed its checks, and waits
	// for the recording's playbacks to stop to replace it.
	hevcPending = "pending"
	// hevcConverted is a recording its copy replaced.
	hevcConverted = "converted"
	// hevcKept is a recording kept as it was, never tried again: its copy
	// failed a check, or it could not, or need not, be converted.
	hevcKept = "kept"
)

// copyPath is where the HEVC copy of a recording is written, in dir.
func copyPath(dir string, recording accounts.ID) string {
	return filepath.Join(dir, recording.String()+".hevc.mkv")
}

// WakeConversions has the conversions look at the recordings and the
// settings now.
func (s *Service) WakeConversions() {
	if s == nil {
		return
	}
	select {
	case s.convertWake <- struct{}{}:
	default:
	}
}

// RunConversions converts finished recordings to HEVC until ctx ends, as
// the settings ask (see ConvertNext). It does nothing without a Converter.
// The copies a stop left unfinished are deleted first, once recording is
// on: their recordings start over.
func (s *Service) RunConversions(ctx context.Context) {
	if s.converter == nil || s.prober == nil {
		return
	}
	cleaned := false
	for {
		if dir := s.Dir(); !cleaned && dir != "" {
			s.removeLeftovers(ctx, dir)
			cleaned = true
		}
		wait := s.ConvertNext(ctx)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.convertWake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// ConvertNext replaces the recordings whose checked copy waited for their
// playbacks to stop, then, with ConvertRecordings on and within the hours
// the settings give, converts the recording that ended first of those not
// tried yet, and returns how long to wait before the next. A conversion
// runs on the GPU conversions use, if it has an HEVC encoder, else on the
// processor with libx265 when ConvertRecordingsOnProcessor allows it, else
// not at all. The copy keeps the recording's size, tracks and length, its
// video converted and the rest copied, and replaces the recording once its
// checks pass (see checkCopy): one that fails them is deleted, and the
// recording kept as it was, never tried again. A conversion still running
// at the end of the hours is stopped, its copy deleted, and starts over
// the next time. A recording whose file cannot be opened, in a folder that
// reads, is passed over until the pass ends, when ConvertNext first waits.
func (s *Service) ConvertNext(ctx context.Context) time.Duration {
	wait := s.convertNext(ctx)
	if wait != 0 {
		// The recordings passed over are tried again in the next pass, and
		// logged again only if their file could be opened in between.
		s.missingLogged, s.passedOver = s.passedOver, nil
	}
	return wait
}

func (s *Service) convertNext(ctx context.Context) time.Duration {
	dir := s.Dir()
	if dir == "" || s.converter == nil || s.prober == nil {
		return maxWait
	}
	s.replacePending(ctx, dir)
	settings := s.users.Settings()
	if !settings.ConvertRecordings {
		return maxWait
	}
	end, ok := conversionEnd(s.now(), settings.ConvertRecordingsFrom, settings.ConvertRecordingsUntil)
	if !ok {
		return maxWait
	}
	encoding, ok := s.hevcEncoder(settings)
	if !ok {
		if !s.noEncoderLogged {
			s.logger.Info("Recordings are not converted to HEVC: no graphics card encodes HEVC, and the processor is not allowed to")
			s.noEncoderLogged = true
		}
		return maxWait
	}
	s.noEncoderLogged = false
	passedOver := s.passedOver
	if passedOver == nil {
		// A nil slice is NULL, which would leave out every recording.
		passedOver = []accounts.ID{}
	}
	var r Recording
	err := s.db.QueryRow(ctx, `SELECT id, file, size FROM live_recordings WHERE status = 'Completed' AND hevc = '' AND file <> ''
		AND NOT (id = ANY($1)) ORDER BY ended_at, id LIMIT 1`, passedOver).Scan(&r.ID, &r.File, &r.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return maxWait
	}
	if err != nil {
		s.logger.Warn("The recordings to convert could not be listed", "error", err)
		return maxWait
	}
	if !s.convert(ctx, dir, r, encoding, settings, end) {
		return maxWait
	}
	return 0
}

// conversionEnd is when the hours from to until, in now's time zone, end,
// if now is within them; until may be past midnight.
func conversionEnd(now time.Time, from, until int) (time.Time, bool) {
	hour := now.Hour()
	if from < until && (hour < from || hour >= until) || from > until && hour < from && hour >= until {
		return time.Time{}, false
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), until, 0, 0, 0, now.Location())
	if !end.After(now) {
		end = time.Date(now.Year(), now.Month(), now.Day()+1, until, 0, 0, 0, now.Location())
	}
	return end, true
}

// hevcEncoder is the encoder recordings are converted with: the GPU's
// HEVC encoder, else libx265 when the settings allow the processor.
func (s *Service) hevcEncoder(settings accounts.Settings) (hls.VideoEncoding, bool) {
	if hw := s.converter.Hardware(); hw != nil {
		if i := slices.IndexFunc(hw.Encoders, func(e string) bool { return strings.HasPrefix(e, "hevc_") }); i >= 0 {
			return hls.VideoEncoding{Encoder: hw.Encoders[i], Hardware: hw}, true
		}
	}
	if settings.ConvertRecordingsOnProcessor && slices.Contains(s.converter.Encoders(), "libx265") {
		return hls.VideoEncoding{Encoder: "libx265"}, true
	}
	return hls.VideoEncoding{}, false
}

// convert converts recording r of dir with encoding until end, checks its
// copy, and replaces r with it, or marks it waiting for its playbacks to
// stop. It reports false when the conversion did not go through and r was
// left to be tried again: for want of free space, stopped, or with the
// folder unreadable or ffprobe failing to run, as when the recordings'
// disk is not mounted yet, which says nothing of the recording itself. A
// recording whose file alone cannot be opened is passed over for the
// pass, and the next one tried.
func (s *Service) convert(ctx context.Context, dir string, r Recording, encoding hls.VideoEncoding, settings accounts.Settings, end time.Time) bool {
	source := filepath.Join(dir, r.File)
	analysis, err := s.analyze(ctx, dir, source)
	switch {
	case err == nil:
	case ctx.Err() != nil:
		return false
	case errors.Is(err, errFileUnreadable):
		s.passedOver = append(s.passedOver, r.ID)
		if !slices.Contains(s.missingLogged, r.ID) {
			s.logger.Warn("A recording's file could not be opened to be converted to HEVC; it is tried again next time", "recording", r.ID.String(), "error", err)
		}
		return true
	case errors.Is(err, media.ErrNotMedia):
		s.keep(ctx, r.ID, "it could not be analyzed", err)
		return true
	default:
		if !s.unreadableLogged {
			s.logger.Warn("Recordings could not be read to be converted to HEVC; they are tried again", "error", err)
			s.unreadableLogged = true
		}
		return false
	}
	s.unreadableLogged = false
	video, reason := convertible(analysis)
	if reason != "" {
		s.keep(ctx, r.ID, reason, nil)
		return true
	}
	original := size(source)
	if free, ok := s.free(dir); ok && free < original {
		if s.spaceLogged != r.ID {
			s.logger.Info("A recording waits for free space to be converted to HEVC", "recording", r.ID.String(), "size", original, "free", free)
			s.spaceLogged = r.ID
		}
		return false
	}
	encoding = videoEncoding(encoding, video, analysis, settings)
	target := copyPath(dir, r.ID)
	s.logger.Info("A recording is converted to HEVC", "recording", r.ID.String(), "encoder", encoding.Encoder)
	work, cancel := context.WithTimeout(ctx, end.Sub(s.now()))
	defer cancel()
	err = s.converter.ConvertFile(work, hls.FileConversion{Input: source, Output: target, Video: encoding}, nil)
	if err == nil {
		reason, err = s.checkCopy(work, analysis, source, target)
	}
	if work.Err() != nil {
		_ = os.Remove(target)
		s.logger.Info("The conversion of a recording to HEVC stopped at the end of its hours; it starts over next time", "recording", r.ID.String())
		return false
	}
	if err != nil || reason != "" {
		_ = os.Remove(target)
		s.keep(ctx, r.ID, cmp.Or(reason, "its conversion failed"), err)
		return true
	}
	if s.playing(r.ID) {
		if _, err := s.db.Exec(ctx, "UPDATE live_recordings SET hevc = 'pending' WHERE id = $1 AND hevc = ''", r.ID); err != nil {
			s.logger.Warn("A converted recording could not be marked waiting", "error", err)
		}
		s.logger.Info("A recording converted to HEVC is replaced once its playback stops", "recording", r.ID.String())
		return true
	}
	s.replace(ctx, dir, r.ID)
	return true
}

// errFileUnreadable is a recording's file that cannot be opened in a
// folder that reads.
var errFileUnreadable = errors.New("the recording's file cannot be opened")

// analyze analyzes the recording at source, in dir, once it can open it.
// Its error wraps errFileUnreadable when the file alone cannot be opened,
// and media.ErrNotMedia only when the file opened, and ffprobe ran and
// could not read it.
func (s *Service) analyze(ctx context.Context, dir, source string) (media.Analysis, error) {
	file, err := os.Open(source)
	if err != nil {
		if dirErr := readable(dir); dirErr != nil {
			return media.Analysis{}, dirErr
		}
		return media.Analysis{}, fmt.Errorf("%w: %w", errFileUnreadable, err)
	}
	_ = file.Close()
	return s.prober.ProbeInput(ctx, media.Input{Path: source})
}

// readable reports why the folder dir cannot be listed, nil when it can.
func readable(dir string) error {
	folder, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer folder.Close()
	if _, err := folder.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// convertible returns the video track of a recording to convert, or why it
// is not converted: none, already in a codec as small as HEVC, or in more
// than 8 bits or HDR, which conversions bring to 8-bit SDR.
func convertible(analysis media.Analysis) (media.Stream, string) {
	i := slices.IndexFunc(analysis.Streams, func(s media.Stream) bool { return s.Type == "video" && !s.AttachedPicture })
	if i < 0 {
		return media.Stream{}, "it has no video"
	}
	video := analysis.Streams[i]
	switch {
	case slices.Contains([]string{"hevc", "av1", "vp9"}, strings.ToLower(video.Codec)):
		return video, "its video is " + strings.ToUpper(video.Codec) + " already"
	case video.BitDepth > 8 || video.ColorTransfer == "smpte2084" || video.ColorTransfer == "arib-std-b67":
		return video, "its video is HDR or more than 8 bits"
	case video.Width <= 0 || video.Height <= 0 || analysis.Duration <= 0:
		return video, "its size or length is unknown"
	}
	return video, ""
}

// videoEncoding completes encoding for video: its size and frame rate,
// deinterlaced, and the settings' quality, preset and deinterlacing, as
// conversions for playback are.
func videoEncoding(encoding hls.VideoEncoding, video media.Stream, analysis media.Analysis, settings accounts.Settings) hls.VideoEncoding {
	rate := video.AverageRate
	if rate <= 0 {
		rate = video.FrameRate
	}
	encoding.Deinterlace = video.FieldOrder != "" && video.FieldOrder != "progressive" && video.FieldOrder != "unknown"
	// Like Jellyfin, frames are doubled up to 30 a second.
	encoding.DoubleRate = encoding.Deinterlace && settings.DeinterlaceDoubleRate && rate > 0 && rate <= 30
	if encoding.DoubleRate {
		rate *= 2
	}
	encoding.Width, encoding.Height, encoding.FrameRate = video.Width, video.Height, rate
	encoding.Deinterlacer = settings.DeinterlaceMethod
	encoding.Quality = settings.HevcQuality
	if settings.EncoderPreset != "auto" {
		encoding.Preset = settings.EncoderPreset
	}
	encoding.Bitrate = video.Bitrate
	if encoding.Bitrate <= 0 {
		encoding.Bitrate = analysis.Bitrate
	}
	if encoding.Hardware != nil {
		encoding.DecodeOnCPU = !slices.Contains(settings.HardwareDecodingCodecs, strings.ToLower(video.Codec))
	}
	return encoding
}

// checkCopy reports why the copy at target may not replace the recording
// at source, which analysis describes, empty when it may: its length is
// within a second of the recording's, it holds the same tracks, its video
// HEVC, it decodes to its end, and it is smaller.
func (s *Service) checkCopy(ctx context.Context, analysis media.Analysis, source, target string) (string, error) {
	copied, err := s.prober.ProbeInput(ctx, media.Input{Path: target})
	if err != nil {
		return "its copy could not be analyzed", err
	}
	if d := copied.Duration - analysis.Duration; d > time.Second || d < -time.Second {
		return "its copy does not last as long", nil
	}
	if !sameTracks(analysis, copied) {
		return "its copy does not hold the same tracks", nil
	}
	decoded, err := s.converter.DecodedLength(ctx, target)
	if err != nil {
		return "its copy does not decode", err
	}
	if decoded < analysis.Duration-time.Second {
		return "its copy does not decode to its end", nil
	}
	if size(target) >= size(source) {
		return "its copy is not smaller", nil
	}
	return "", nil
}

// sameTracks reports whether a copy holds the tracks of the original, in
// the same order: the video in HEVC at the same size, the others as they
// were.
func sameTracks(original, copied media.Analysis) bool {
	if len(original.Streams) != len(copied.Streams) {
		return false
	}
	for i, o := range original.Streams {
		c := copied.Streams[i]
		switch {
		case o.Type != c.Type:
			return false
		case o.Type == "video" && !o.AttachedPicture:
			if c.Codec != "hevc" || c.Width != o.Width || c.Height != o.Height {
				return false
			}
		case o.Codec != c.Codec || o.Channels != c.Channels || o.Language != c.Language:
			return false
		}
	}
	return true
}

// playing reports whether someone plays a recording now.
func (s *Service) playing(id accounts.ID) bool {
	return s.inUse != nil && s.inUse(id)
}

// keep marks a recording kept as it was, never converted again.
func (s *Service) keep(ctx context.Context, id accounts.ID, reason string, err error) {
	if _, dbErr := s.db.Exec(ctx, "UPDATE live_recordings SET hevc = 'kept' WHERE id = $1 AND hevc IN ('', 'pending')", id); dbErr != nil {
		s.logger.Warn("A recording could not be marked kept as it was", "error", dbErr)
	}
	args := []any{"recording", id.String(), "reason", reason}
	if err != nil {
		args = append(args, "error", err)
	}
	s.logger.Info("A recording is kept as it was, not converted to HEVC", args...)
}

// replacePending replaces the recordings of dir whose copy waited for
// their playbacks to stop, once they did. A copy that went missing has
// its recording converted again.
func (s *Service) replacePending(ctx context.Context, dir string) {
	rows, err := s.db.Query(ctx, "SELECT id FROM live_recordings WHERE hevc = 'pending' ORDER BY id")
	if err != nil {
		s.logger.Warn("The converted recordings waiting could not be listed", "error", err)
		return
	}
	pending, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		s.logger.Warn("The converted recordings waiting could not be listed", "error", err)
		return
	}
	for _, id := range pending {
		if s.playing(id) {
			continue
		}
		if _, err := os.Stat(copyPath(dir, id)); err != nil {
			if _, err := s.db.Exec(ctx, "UPDATE live_recordings SET hevc = '' WHERE id = $1 AND hevc = 'pending'", id); err != nil {
				s.logger.Warn("A converted recording could not be marked to convert again", "error", err)
			}
			continue
		}
		s.replace(ctx, dir, id)
	}
}

// replace puts the copy of recording id in dir in its file's place,
// keeping its identifier, and so its played state, resume points and
// commercials, and forgets what was learned of the file it replaced: its
// analysis, keyframes and seek lookback, which playback learns again.
func (s *Service) replace(ctx context.Context, dir string, id accounts.ID) {
	target := copyPath(dir, id)
	var file string
	var original int64
	err := s.db.QueryRow(ctx, "SELECT file, size FROM live_recordings WHERE id = $1", id).Scan(&file, &original)
	if errors.Is(err, pgx.ErrNoRows) {
		// Deleted while it was converted.
		_ = os.Remove(target)
		return
	}
	if err != nil {
		s.logger.Warn("A converted recording could not be read", "error", err)
		return
	}
	converted := size(target)
	name := id.String() + ".mkv"
	if err := os.Rename(target, filepath.Join(dir, name)); err != nil {
		s.logger.Warn("A converted recording could not replace its original", "error", err)
		_ = os.Remove(target)
		s.keep(ctx, id, "its copy could not replace it", err)
		return
	}
	if file != name {
		_ = os.Remove(filepath.Join(dir, file))
	}
	tag, err := s.db.Exec(ctx, `UPDATE live_recordings SET file = $2, size = $3, original_size = $4, hevc = 'converted' WHERE id = $1`,
		id, name, converted, original)
	if err == nil && tag.RowsAffected() == 0 {
		_ = os.Remove(filepath.Join(dir, name))
		return
	}
	if err != nil {
		s.logger.Warn("A converted recording could not be marked converted", "error", err)
	}
	for _, query := range []string{
		"DELETE FROM media_analyses WHERE version_id = $1",
		"DELETE FROM media_keyframes WHERE version_id = $1",
		"DELETE FROM media_subtitle_index WHERE version_id = $1",
	} {
		if _, err := s.db.Exec(ctx, query, id); err != nil {
			s.logger.Warn("What was learned of a converted recording could not be forgotten", "error", err)
		}
	}
	if _, err := s.db.Exec(ctx, "DELETE FROM seek_lookbacks WHERE kind = 'version' AND name = $1", id.String()); err != nil {
		s.logger.Warn("The seek lookback of a converted recording could not be removed", "error", err)
	}
	s.recorder.ForgetFile(id)
	s.logger.Info("A recording was converted to HEVC", "recording", id.String(), "size", converted, "original", original)
}

// removeLeftovers deletes the copies in dir of the conversions a stop
// interrupted, whose recordings start over, and those of recordings
// deleted since.
func (s *Service) removeLeftovers(ctx context.Context, dir string) {
	copies, err := filepath.Glob(filepath.Join(dir, "*.hevc.mkv"))
	if err != nil || len(copies) == 0 {
		return
	}
	rows, err := s.db.Query(ctx, "SELECT id FROM live_recordings WHERE hevc = 'pending'")
	if err != nil {
		return
	}
	pending, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return
	}
	for _, path := range copies {
		id, err := accounts.ParseID(strings.TrimSuffix(filepath.Base(path), ".hevc.mkv"))
		if err == nil && slices.Contains(pending, id) {
			continue
		}
		if err := os.Remove(path); err != nil {
			s.logger.Warn("An unfinished conversion of a recording could not be deleted", "error", err)
		}
	}
}

// SpaceSaved counts the recordings converted to HEVC, and the bytes their
// conversion saved.
func (s *Service) SpaceSaved(ctx context.Context) (recordings int, saved int64, err error) {
	err = s.db.QueryRow(ctx, "SELECT count(*), COALESCE(sum(original_size - size), 0)::bigint FROM live_recordings WHERE hevc = 'converted'").
		Scan(&recordings, &saved)
	return recordings, saved, err
}

// freeSpace is the space free on the disk of dir, through diskspace.
func freeSpace(dir string) (int64, bool) {
	free, _, _, ok := diskspace.Measure(dir)
	return free, ok
}
