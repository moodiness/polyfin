package library

import (
	"context"

	"github.com/moodiness/polyfin/internal/addons"
)

// VersionAddon is the source that listed a version, as installed: a
// Stremio or Eclipse addon, an IPTV source or a local folder. It fails
// with addons.ErrNotFound for a source removed since.
func (s *Service) VersionAddon(ctx context.Context, version Version) (addons.Addon, error) {
	return s.addons.Find(ctx, version.Origin.Addon)
}
