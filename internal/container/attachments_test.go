package container

import (
	"bytes"
	"context"
	"testing"
)

// The files attached are copied out of the span they are read in: one
// kept keeps its own bytes, not the element around it.
func TestAttachedFilesAreCopiedOutOfTheSpanRead(t *testing.T) {
	t.Parallel()
	font := patterned(64 << 10)
	// The file comes first in an element of 2 MiB, the rest of which is
	// not attached files.
	data := testFile{tracks: [][]byte{subtitleTrack(1, "S_TEXT/ASS", nil)}, attachments: ebmlElement(idAttachments,
		ebmlElement(idAttachedFile, ebmlElement(idFileName, []byte("A.ttf")), ebmlElement(idFileData, font)),
		ebmlElement(idVoid, make([]byte, 2<<20)),
	)}.build()
	attachments, err := openMemory(t, data).Attachments(context.Background())
	if err != nil || len(attachments) != 1 || !bytes.Equal(attachments[0].Data, font) {
		t.Fatalf("got %d attached files, %v", len(attachments), err)
	}
	if kept := cap(attachments[0].Data); kept > 2*len(font) {
		t.Errorf("a file of %d bytes keeps %d bytes", len(font), kept)
	}
}
