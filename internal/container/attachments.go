package container

import (
	"context"
	"fmt"
)

// Attachment is a file attached to a Matroska file, such as a font its
// ASS subtitles use.
type Attachment struct {
	FileName, MimeType, Description string
	UID                             uint64
	Data                            []byte
}

const (
	// maxAttachments bounds the Attachments element read: fonts of a few
	// MB each, tens of them for typeset anime.
	maxAttachments = 128 << 20
	// maxAttachedFiles bounds the files read from it.
	maxAttachedFiles = 4096
)

// Attachments reads the files attached to m, through its Reader, with one
// read; none when it has none.
func (m *Matroska) Attachments(ctx context.Context) ([]Attachment, error) {
	at, ok := m.positions[idAttachments]
	if !ok {
		return nil, nil
	}
	data, err := m.file(ctx).master(at, idAttachments, m.segment.end, maxAttachments)
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
			var err error
			switch id {
			case idFileName:
				attachment.FileName = text(data)
			case idFileMediaType:
				attachment.MimeType = text(data)
			case idFileDescription:
				attachment.Description = text(data)
			case idFileUID:
				attachment.UID, err = unsigned(data)
			case idFileData:
				attachment.Data = data
			}
			return err
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
