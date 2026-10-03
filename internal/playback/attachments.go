package playback

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// The files attached to versions, the fonts their ASS tracks use above
// all. jellyfin-web renders ASS with the fonts listed in MediaAttachments,
// asking for every one as a track starts. A Matroska file keeps them in one
// element, read once for all of them and kept a while; the container
// package bounds its size.

// attachedTime bounds reading a version's attached files: one read of a
// few megabytes.
const attachedTime = time.Minute

// ErrNoAttachment reports a stream that is not a file attached to its
// version.
var ErrNoAttachment = errors.New("no such attached file")

// Attached reports whether a stream is a file attached to its version: an
// attachment, or cover art FFmpeg turns into a picture stream.
func Attached(stream media.Stream) bool {
	return stream.Type == "attachment" || stream.AttachedPicture
}

// Attachment returns the data of the file attached to a version as its
// stream with FFmpeg index index.
func (s *Service) Attachment(ctx context.Context, version library.Version, analysis media.Analysis, index int) ([]byte, error) {
	name, before, ok := attachedFile(analysis, index)
	if !ok || !matroska(analysis) {
		return nil, ErrNoAttachment
	}
	files, err := s.attachedFiles(ctx, version, analysis)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.FileName != name {
			continue
		}
		if before == 0 {
			return file.Data, nil
		}
		before--
	}
	return nil, ErrNoAttachment
}

// attachedFile finds the attached file of stream index: its name, and how
// many attached files of the same name come before it, for files named
// alike. FFmpeg makes their streams in the order the file lists them.
func attachedFile(analysis media.Analysis, index int) (name string, before int, ok bool) {
	at := slices.IndexFunc(analysis.Streams, func(stream media.Stream) bool { return stream.Index == index })
	if at < 0 || !Attached(analysis.Streams[at]) || analysis.Streams[at].FileName == "" {
		return "", 0, false
	}
	name = analysis.Streams[at].FileName
	for _, stream := range analysis.Streams {
		if Attached(stream) && stream.FileName == name && stream.Index < index {
			before++
		}
	}
	return name, before, true
}

// attachedFiles reads the files attached to a version, kept a while: the
// fonts of a track are asked for together.
func (s *Service) attachedFiles(ctx context.Context, version library.Version, analysis media.Analysis) ([]container.Attachment, error) {
	if files, ok := s.attached.Get(version.ID); ok {
		return files, nil
	}
	results := s.flight.DoChan("attached "+version.ID.String(), func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), attachedTime)
		defer cancel()
		src := s.open(version)
		defer src.Release()
		size, err := s.sizeOf(ctx, src, analysis)
		if err != nil {
			return nil, err
		}
		m, err := container.OpenMatroska(ctx, src, size)
		if err != nil {
			return nil, err
		}
		files, err := m.Attachments(ctx)
		if err != nil {
			s.logger.Info("The attached files of a version could not be read", "addon", version.Addon, "error", err)
			return nil, err
		}
		s.attached.Put(version.ID, files)
		return files, nil
	})
	select {
	case result := <-results:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Val.([]container.Attachment), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
