package library

import (
	"sync"
	"sync/atomic"
)

// The picture behind an artwork URL may change while the URL stays the
// same, as a poster showing ratings does. Apps keep an image by its tag
// (see ImageTag): the image cache, finding such a picture changed, stamps
// its URL, and the stamp is folded into the tag, so that apps ask for the
// new picture. A URL whose picture never changed keeps its own tag.
var (
	imageStamps sync.Map // URL to stamp
	// stampedImages counts the stamps, so that ImageTag looks none up
	// while there is none.
	stampedImages atomic.Int64
)

// StampImage folds stamp into the tag of the artwork at url (see
// ImageTag); an empty stamp gives the URL its own tag back.
func StampImage(url, stamp string) {
	if stamp == "" {
		if _, loaded := imageStamps.LoadAndDelete(url); loaded {
			stampedImages.Add(-1)
		}
		return
	}
	if _, loaded := imageStamps.Swap(url, stamp); !loaded {
		stampedImages.Add(1)
	}
}

// ImageStamp is the stamp folded into the tag of the artwork at url, empty
// when there is none.
func ImageStamp(url string) string {
	if stampedImages.Load() == 0 {
		return ""
	}
	stamp, _ := imageStamps.Load(url)
	s, _ := stamp.(string)
	return s
}
