package playback

import (
	"testing"

	"github.com/moodiness/polyfin/internal/library"
)

// The torrent's file a movie's version plays is told by the movie's title
// and year too, which its version carries to the engine's spec.
func TestATorrentSpecCarriesTheMoviesTitleAndYear(t *testing.T) {
	t.Parallel()
	spec, err := torrentSpec(library.Version{Filename: "pack.mkv", Torrent: &library.Torrent{
		InfoHash: "0123456789abcdef0123456789abcdef01234567", Title: "Harbor", Year: 1990}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.File != -1 || spec.Name != "pack.mkv" || spec.Title != "Harbor" || spec.Year != 1990 {
		t.Errorf("spec: %+v, want the movie Harbor of 1990", spec)
	}
}
