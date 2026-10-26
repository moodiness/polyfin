package silo

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

type audioTrack struct {
	Codec    string `json:"codec,omitempty"`
	Channels int    `json:"channels,omitempty"`
	Layout   string `json:"layout,omitempty"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  bool   `json:"default"`
}

type subtitleTrack struct {
	Index           int    `json:"index"`
	Codec           string `json:"codec,omitempty"`
	Language        string `json:"language,omitempty"`
	Title           string `json:"title,omitempty"`
	Default         bool   `json:"default"`
	External        bool   `json:"external"`
	Forced          bool   `json:"forced"`
	HearingImpaired bool   `json:"hearing_impaired"`
}

// fileVersion describes a version of a title: Polyfin's versions are the
// apps' files.
type fileVersion struct {
	FileID         string          `json:"file_id"`
	FileName       string          `json:"file_name,omitempty"`
	FileSize       int64           `json:"file_size"`
	Container      string          `json:"container"`
	CodecVideo     string          `json:"codec_video"`
	CodecAudio     string          `json:"codec_audio"`
	Resolution     string          `json:"resolution"`
	HDR            bool            `json:"hdr"`
	Bitrate        int64           `json:"bitrate"`
	Duration       int64           `json:"duration"`
	AddedAt        string          `json:"added_at"`
	AudioTracks    []audioTrack    `json:"audio_tracks"`
	SubtitleTracks []subtitleTrack `json:"subtitle_tracks"`
}

// resolution names a height as the apps do.
func resolution(height int) string {
	switch {
	case height >= 1600:
		return "2160p"
	case height >= 900:
		return "1080p"
	case height >= 650:
		return "720p"
	case height >= 500:
		return "576p"
	case height > 0:
		return "480p"
	}
	return ""
}

// streamsOf are a version's streams of a type, in their order.
func streamsOf(source mediaSource, kind string) []playback.MediaStream {
	var streams []playback.MediaStream
	for _, stream := range source.MediaStreams {
		if stream.Type == kind {
			streams = append(streams, stream)
		}
	}
	return streams
}

func intOf(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

// defaultAudio is the audio track a version plays unless one is chosen.
func defaultAudio(source mediaSource) (playback.MediaStream, bool) {
	audio := streamsOf(source, "Audio")
	for _, stream := range audio {
		if source.DefaultAudioStreamIndex != nil && stream.Index == *source.DefaultAudioStreamIndex {
			return stream, true
		}
	}
	for _, stream := range audio {
		if stream.IsDefault {
			return stream, true
		}
	}
	if len(audio) > 0 {
		return audio[0], true
	}
	return playback.MediaStream{}, false
}

// versionOf describes a version, numbered as number.
func versionOf(source mediaSource, number string, added time.Time) fileVersion {
	v := fileVersion{FileID: number, FileName: source.Name, Container: strings.Split(source.Container, ",")[0], AddedAt: instant(added),
		AudioTracks: []audioTrack{}, SubtitleTracks: []subtitleTrack{}, Duration: int64(seconds(source.RunTimeTicks))}
	if source.Size != nil {
		v.FileSize = *source.Size
	}
	if source.Bitrate != nil {
		v.Bitrate = *source.Bitrate / 1000
	}
	if video := streamsOf(source, "Video"); len(video) > 0 {
		v.CodecVideo, v.Resolution, v.HDR = video[0].Codec, resolution(intOf(video[0].Height)), video[0].VideoRange == "HDR"
	}
	if audio, ok := defaultAudio(source); ok {
		v.CodecAudio = audio.Codec
	}
	for _, stream := range streamsOf(source, "Audio") {
		v.AudioTracks = append(v.AudioTracks, audioTrack{Codec: stream.Codec, Channels: intOf(stream.Channels), Layout: stream.ChannelLayout,
			Language: stream.Language, Title: stream.Title, Default: stream.IsDefault})
	}
	for n, stream := range streamsOf(source, "Subtitle") {
		v.SubtitleTracks = append(v.SubtitleTracks, subtitleTrack{Index: n, Codec: stream.Codec, Language: stream.Language,
			Title: stream.Title, Default: stream.IsDefault, External: stream.IsExternal, Forced: stream.IsForced,
			HearingImpaired: stream.IsHearingImpaired})
	}
	return v
}

// versionsOf describes an item's versions, as the Jellyfin API describes
// them: those the version rules and the user's quality group offer.
func (s *Server) versionsOf(ctx context.Context, i item) ([]fileVersion, error) {
	ids := make([]accounts.ID, 0, len(i.MediaSources))
	for _, source := range i.MediaSources {
		if id, err := accounts.ParseID(source.Id); err == nil {
			ids = append(ids, id)
		}
	}
	numbers, err := s.numbers.of(ctx, ids...)
	if err != nil {
		return nil, err
	}
	added := time.Unix(0, 0)
	if i.DateCreated != nil {
		added = *i.DateCreated
	}
	versions := []fileVersion{}
	for _, source := range i.MediaSources {
		id, err := accounts.ParseID(source.Id)
		if err != nil {
			continue
		}
		version := versionOf(source, strconv.Itoa(int(numbers[id])), added)
		if version.Duration == 0 {
			version.Duration = int64(seconds(i.RunTimeTicks))
		}
		versions = append(versions, version)
	}
	return versions, nil
}

// subtitlesOf are the subtitles of a title's first version.
func subtitlesOf(sources []mediaSource) []subtitleInfo {
	subtitles := []subtitleInfo{}
	if len(sources) == 0 {
		return subtitles
	}
	for _, stream := range streamsOf(sources[0], "Subtitle") {
		source := "embedded"
		if stream.IsExternal {
			source = "external"
		}
		subtitles = append(subtitles, subtitleInfo{Language: stream.Language, Codec: stream.Codec, Title: stream.Title, Source: source,
			Forced: stream.IsForced, HearingImpaired: stream.IsHearingImpaired})
	}
	return subtitles
}

// watchVersion is a version as the player reads it: its length in
// seconds, its bitrate in bits per second.
type watchVersion struct {
	FileID          string          `json:"file_id"`
	FileName        string          `json:"file_name,omitempty"`
	FileSize        int64           `json:"file_size"`
	Container       string          `json:"container"`
	CodecVideo      string          `json:"codec_video"`
	CodecAudio      string          `json:"codec_audio"`
	Resolution      string          `json:"resolution"`
	HDR             bool            `json:"hdr"`
	Bitrate         int64           `json:"bitrate"`
	DurationSeconds int64           `json:"duration_seconds"`
	AddedAt         string          `json:"added_at"`
	AudioTracks     []audioTrack    `json:"audio_tracks"`
	SubtitleTracks  []subtitleTrack `json:"subtitle_tracks"`
	MarkerSegments  []struct{}      `json:"marker_segments"`
	Trickplay       bool            `json:"trickplay_available"`
}

type watchDetail struct {
	ContentID     string         `json:"content_id"`
	Title         string         `json:"title"`
	Type          string         `json:"type"`
	Overview      string         `json:"overview,omitempty"`
	Year          int            `json:"year,omitempty"`
	SeriesID      string         `json:"series_id,omitempty"`
	SeriesTitle   string         `json:"series_title,omitempty"`
	SeasonNumber  *int           `json:"season_number,omitempty"`
	EpisodeNumber *int           `json:"episode_number,omitempty"`
	PosterURL     string         `json:"poster_url,omitempty"`
	BackdropURL   string         `json:"backdrop_url,omitempty"`
	Subtitles     []subtitleInfo `json:"subtitles"`
	Versions      []watchVersion `json:"versions"`
	UserData      *rollup        `json:"user_data,omitempty"`
}

// watch describes what the player needs of a movie or an episode: its
// versions and the user's progress.
func (s *Server) watch(w http.ResponseWriter, r *http.Request, v viewer) {
	i, ok := s.itemOf(w, r, v, r.PathValue("id"))
	if !ok {
		return
	}
	if i.Type != "Movie" && i.Type != "Episode" && i.Type != "Video" {
		problem(w, http.StatusNotFound, "not_found", "Only movies and episodes play.")
		return
	}
	versions, err := s.versionsOf(r.Context(), i)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	card := catalogItemOf(r, i)
	detail := watchDetail{ContentID: i.Id, Title: i.Name, Type: card.Type, Overview: card.Overview, Year: card.Year,
		SeriesID: i.SeriesId, SeriesTitle: i.SeriesName, SeasonNumber: card.SeasonNumber, EpisodeNumber: card.EpisodeNumber,
		PosterURL: card.PosterURL, BackdropURL: card.BackdropURL, Subtitles: subtitlesOf(i.MediaSources), Versions: []watchVersion{},
		UserData: rollupOf(i)}
	for _, version := range versions {
		detail.Versions = append(detail.Versions, watchVersion{FileID: version.FileID, FileName: version.FileName, FileSize: version.FileSize,
			Container: version.Container, CodecVideo: version.CodecVideo, CodecAudio: version.CodecAudio, Resolution: version.Resolution,
			HDR: version.HDR, Bitrate: version.Bitrate * 1000, DurationSeconds: version.Duration, AddedAt: version.AddedAt,
			AudioTracks: version.AudioTracks, SubtitleTracks: version.SubtitleTracks, MarkerSegments: []struct{}{}})
	}
	writeJSON(w, http.StatusOK, detail)
}
