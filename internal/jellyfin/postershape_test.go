package jellyfin

import (
	"testing"

	"github.com/moodiness/polyfin/internal/library"
)

// A collection's wide poster, as addons name it, is told to apps as wide,
// which then show it on a wide card; other posters keep their kind's shape.
func TestPosterShapeGivesTheImageRatio(t *testing.T) {
	t.Parallel()
	h := &Handler{}
	for _, c := range []struct {
		kind  library.Kind
		shape string
		want  float64
	}{
		{library.KindCollection, "landscape", 16.0 / 9.0},
		{library.KindCollection, "square", 1},
		{library.KindCollection, "", 2.0 / 3.0},
		{library.KindMovie, "poster", 2.0 / 3.0},
		{library.KindEpisode, "", 16.0 / 9.0},
	} {
		dto := BaseItemDto{ImageTags: map[string]string{}}
		h.setImages(&dto, library.Item{Kind: c.kind, PosterShape: c.shape, Images: library.Images{Primary: "https://images.example/p.jpg"}})
		if dto.PrimaryImageAspectRatio == nil || *dto.PrimaryImageAspectRatio != c.want {
			t.Errorf("%v %q: ratio %v, want %v", c.kind, c.shape, dto.PrimaryImageAspectRatio, c.want)
		}
	}
}
