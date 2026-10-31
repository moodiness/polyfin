package playback

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// The channels Polyfin airs itself (see package virtualtv) play as a
// channel's live HLS stream does: their one version is the playlist of
// their playout (see hls.Manager.Playout), served on the loopback
// interface, which players are relayed and FFmpeg and recordings read as
// they read any channel's playlist. One playout airs a channel, whoever
// watches it.

// playoutSlate bounds the slate that airs when no version of a programme
// plays: its versions are tried again after it.
const playoutSlate = time.Minute

// Playouts finds the channels Polyfin airs, and the versions of their
// programmes (see UsePlayouts).
type Playouts interface {
	// Channel returns a channel, virtualtv.ErrNotFound when there is none
	// or it is off.
	Channel(ctx context.Context, id accounts.ID) (virtualtv.Channel, error)
	// Versions lists a movie's or an episode's versions, in the order the
	// version rules give them.
	Versions(ctx context.Context, item accounts.ID) ([]library.Version, error)
}

// UsePlayouts airs the channels playouts finds; without it, their
// playlists are not found.
func (s *Service) UsePlayouts(playouts Playouts) {
	s.loopback.mu.Lock()
	defer s.loopback.mu.Unlock()
	s.loopback.playout = func(w http.ResponseWriter, r *http.Request, channel accounts.ID, file string) {
		s.servePlayout(w, r, playouts, channel, file)
	}
}

// PlayoutVersion is the one version of a channel Polyfin airs: its
// playout's playlist, on the loopback interface. item is the channel's
// item.
func (s *Service) PlayoutVersion(item accounts.ID, channel virtualtv.Channel) library.Version {
	sum := sha256.Sum256([]byte("polyfin:playout:" + channel.ID.String()))
	var id accounts.ID
	copy(id[:], sum[:16])
	return library.Version{ID: id, Item: item, Name: channel.Name, Filename: "live.m3u8",
		URL: "http://" + s.loopback.listener.Addr().String() + "/playout/" + s.loopback.fileKey + "/" + channel.ID.String() + "/live.m3u8"}
}

// StopPlayout stops a channel's playout, if it runs: the channel was
// deleted, or turned off.
func (s *Service) StopPlayout(channel accounts.ID) {
	s.segments.StopPlayout(channel.String())
}

// servePlayout answers the requests for a playout's files on the loopback
// interface: its playlist, live.m3u8, which starts it, and its segments.
func (s *Service) servePlayout(w http.ResponseWriter, r *http.Request, playouts Playouts, id accounts.ID, file string) {
	key := id.String()
	if file != "live.m3u8" {
		segment, err := s.segments.PlayoutSegment(key, file)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer segment.Close()
		w.Header().Set("Content-Type", "video/mp2t")
		http.ServeContent(w, r, "", time.Time{}, segment)
		return
	}
	channel, err := playouts.Channel(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := s.segments.Playout(r.Context(), key, channel.Name, s.programmer(playouts, id), func(name string) string { return name }, 0)
	switch {
	case errors.Is(err, hls.ErrBusy):
		http.Error(w, "too many encodings", http.StatusServiceUnavailable)
		return
	case err != nil:
		if r.Context().Err() == nil {
			s.logger.Warn("A channel's playout could not start", "channel", channel.Name, "error", err)
		}
		http.Error(w, "the channel does not air", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// programmer tells a playout what airs (see hls.PlayoutNext): the
// programme the channel's schedule has at the moment asked, from where it
// is. Its versions are tried in turn, those of local folders first, then
// in the version rules' order, at most as many as the settings' version
// attempts: one that fails, at its start or later, passes the programme
// to the next from where it stopped. When none plays, a slate airs for a
// minute at most, after which they are tried again; a programme that
// ends before its time is followed by a slate until its time ends.
func (s *Service) programmer(playouts Playouts, id accounts.ID) hls.PlayoutNext {
	var current struct {
		start time.Time
		item  accounts.ID
		tried int
		slate bool
	}
	slate := func(length time.Duration) (hls.PlayoutPiece, func(), error) {
		return hls.PlayoutPiece{Slate: true, Length: length}, func() {}, nil
	}
	return func(ctx context.Context, at time.Time, failure error) (hls.PlayoutPiece, func(), error) {
		channel, err := playouts.Channel(ctx, id)
		if errors.Is(err, virtualtv.ErrNotFound) {
			return hls.PlayoutPiece{}, nil, fmt.Errorf("%w: %w", hls.ErrStopped, err)
		}
		if err != nil {
			return hls.PlayoutPiece{}, nil, err
		}
		airing, ok := channel.AiringAt(at, time.Local)
		if !ok {
			return slate(playoutSlate)
		}
		remaining := airing.End.Sub(at)
		if !airing.Start.Equal(current.start) || airing.Entry.Item != current.item {
			current.start, current.item, current.tried, current.slate = airing.Start, airing.Entry.Item, 0, false
		} else {
			switch {
			case errors.Is(failure, hls.ErrEndedEarly):
				return slate(remaining)
			case failure != nil:
				current.tried++
			case current.slate:
				current.tried, current.slate = 0, false
			}
		}
		versions, err := playouts.Versions(ctx, airing.Entry.Item)
		if err != nil && ctx.Err() == nil {
			s.logger.Info("The versions of a programme could not be listed", "channel", channel.Name, "programme", airing.Entry.Name, "error", err)
		}
		versions = s.playoutOrder(versions)
		for ; current.tried < len(versions); current.tried++ {
			piece, release, err := s.programmePiece(ctx, versions[current.tried], at.Sub(airing.Start), remaining)
			if err == nil {
				return piece, release, nil
			}
			if ctx.Err() != nil {
				return hls.PlayoutPiece{}, nil, ctx.Err()
			}
			s.logger.Info("A version of a programme could not air", "channel", channel.Name, "programme", airing.Entry.Name, "addon", versions[current.tried].Addon,
				"error", err)
		}
		current.slate = true
		s.logger.Info("No version of a programme plays: a slate airs instead", "channel", channel.Name, "programme", airing.Entry.Name)
		return slate(min(remaining, playoutSlate))
	}
}

// playoutOrder is the versions a programme is aired from, in turn: those
// of local folders first, then the others in their order, at most the
// settings' version attempts, leaving out those that failed lately.
func (s *Service) playoutOrder(versions []library.Version) []library.Version {
	versions = slices.DeleteFunc(slices.Clone(versions), func(v library.Version) bool { return s.Failed(v.ID) })
	slices.SortStableFunc(versions, func(a, b library.Version) int {
		switch local, other := s.localVersion(a), s.localVersion(b); {
		case local && !other:
			return -1
		case other && !local:
			return 1
		}
		return 0
	})
	if tries := s.settings().VersionAttempts; tries > 0 && len(versions) > tries {
		versions = versions[:tries]
	}
	return versions
}

// localVersion reports whether a version is a file of a local folder.
func (s *Service) localVersion(version library.Version) bool {
	return strings.HasPrefix(version.URL, s.localBase()) || s.ReadsInput(version)
}

// programmePiece is the piece airing version from offset for length: its
// video converted to H.264 at most the size the processor or the GPU
// converts, and as the settings cap it, its audio to stereo AAC, so that
// every programme of a channel plays on every app the same way.
func (s *Service) programmePiece(ctx context.Context, version library.Version, offset, length time.Duration) (hls.PlayoutPiece, func(), error) {
	analysis, err := s.Analyze(ctx, version)
	if err != nil {
		return hls.PlayoutPiece{}, nil, err
	}
	video, ok := videoOf(analysis)
	if !ok {
		return hls.PlayoutPiece{}, nil, ErrNotRemuxable
	}
	settings := s.settings()
	tuning := TuningOf(settings)
	var described MediaStream
	for _, stream := range MediaStreams(analysis, nil, settings.Language) {
		if stream.Type == "Video" && stream.Index == video.Index {
			described = stream
		}
	}
	conversion := ConvertVideo("h264", Limits{Server: settings.MaxConversionHeight}, described, s.Capabilities())
	if conversion == nil {
		return hls.PlayoutPiece{}, nil, errors.New("its video cannot be converted to H.264")
	}
	audio := playoutAudio(analysis)
	remux := Remux{ConvertVideo: conversion, Format: hls.TS}
	if stream, ok := streamOf(analysis, audio); ok {
		remux.ConvertAudio = ConvertAudio("aac", "2", stream.Channels, stream.ChannelLayout, 0, tuning)
	}
	r := hls.Remux{Video: video.Index, Audio: audio, Format: hls.TS}
	release := func() {}
	if s.ReadsInput(version) {
		in, err := s.input(ctx, version)
		if err != nil {
			return hls.PlayoutPiece{}, nil, err
		}
		r.Input, r.InputOptions = in.Path, append(in.Args(), "-start_at_zero")
	} else {
		src := s.open(version)
		unurge := src.Urge()
		target, unregister := s.loopback.register(src)
		r.Input = target
		release = func() {
			unregister()
			unurge()
			src.Release()
		}
	}
	remux.convert(&r, video, tuning)
	return hls.PlayoutPiece{Remux: r, Offset: offset, Length: length}, release, nil
}

// playoutAudio is the FFmpeg index of the audio track a programme airs:
// the version's default track, else its first, -1 for none.
func playoutAudio(analysis media.Analysis) int {
	first := -1
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" {
			continue
		}
		if stream.Default {
			return stream.Index
		}
		if first < 0 {
			first = stream.Index
		}
	}
	return first
}
