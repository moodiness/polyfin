package main

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/config"
)

// polyfin_disk_free_bytes gives the free space of each folder the settings
// chose, besides the cache's and those of the recordings and backups in
// use.
func TestDiskFreeOfChosenFolders(t *testing.T) {
	t.Parallel()
	work := config.Folders{Conversion: t.TempDir(), Reading: t.TempDir(), Cache: t.TempDir()}
	backups := t.TempDir()
	var folders []string
	for _, sample := range diskFreeSamples(metricFolders(work, func() string { return "" }, func() string { return backups })) {
		if sample.Value <= 0 {
			t.Errorf("%v: %v bytes free", sample.Labels, sample.Value)
		}
		folders = append(folders, sample.Labels...)
	}
	if want := []string{"cache", "conversion", "reading", "backups"}; !slices.Equal(folders, want) {
		t.Errorf("folders %v, want %v", folders, want)
	}
}
