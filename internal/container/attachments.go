package container

import (
	"context"
	"fmt"
)

// Attachment is a file attached to a Matroska file, such as a font its
// ASS subtitles use.
type Attachment struct {
	FileName, MimeType string
	Data               []byte
}

const (
	// maxAttachments bounds the Attachments element read: fonts of a few
	// MB each, tens of them for typeset anime.
	maxAttachments = 128 << 20
	// maxAttachedFiles bounds the files read from it.
	maxAttachedFiles = 4096
)

// Attachments reads the files attached to m, through its Reader, with one
// read; none when it has none. Attachments of more than maxAttachments
// bytes fail from their header, without being read.
func (m *Matroska) Attachments(ctx context.Context) ([]Attachment, error) {
	at, ok := m.positions[idAttachments]
	if !ok {
		return nil, nil
	}
	f := m.file(ctx)
	e, err := f.element(at, m.segment.end)
	if err != nil {
		return nil, err
	}
	switch {
	case e.id != idAttachments:
		return nil, fmt.Errorf("element %X at %d instead of Attachments: %w", e.id, at, errInvalid)
	case e.unknown:
		return nil, fmt.Errorf("Attachments of unknown size: %w", errInvalid)
	case e.end-e.data > maxAttachments:
		return nil, fmt.Errorf("Attachments of %d bytes: %w", e.end-e.data, errInvalid)
	}
	data, err := f.span(e.data, e.end-e.data)
	if err != nil {
		return nil, err
	}
	var attachments []Attachment
	err = children(data, func(id uint32, data []byte) error {
		if id != idAttachedFile {
			return nil
		}
		if len(attachments) == maxAttachedFiles {
			return fmt.Errorf("more than %d attached files: %w", maxAttachedFiles, errInvalid)
		}
		var attachment Attachment
		err := children(data, func(id uint32, data []byte) error {
			switch id {
			case idFileName:
				attachment.FileName = text(data)
			case idFileMediaType:
				attachment.MimeType = text(data)
			case idFileData:
				attachment.Data = data
			}
			return nil
		})
		if err != nil {
			return err
		}
		attachments = append(attachments, attachment)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return attachments, nil
}
