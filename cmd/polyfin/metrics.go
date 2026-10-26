package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/config"
	"github.com/moodiness/polyfin/internal/diskspace"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/metrics"
)

// metricFolder is a folder Polyfin writes to whose disk the metrics
// measure: its name, as System › Health names it, and where it is, empty
// for a folder not in use.
type metricFolder struct {
	name string
	path func() string
}

// metricFolders are the folders whose disks the metrics measure: those
// of the run (see config.Folders.Disks), then recordings' and backups'.
func metricFolders(work config.Folders, recordings, backups func() string) []metricFolder {
	var folders []metricFolder
	for _, disk := range work.Disks() {
		folders = append(folders, metricFolder{disk.Name, func() string { return disk.Path }})
	}
	return append(folders, metricFolder{"recordings", recordings}, metricFolder{"backups", backups})
}

// diskFreeSamples are the bytes free on the disks of folders, by name,
// those not in use or that cannot be measured left out.
func diskFreeSamples(folders []metricFolder) []metrics.Sample {
	var samples []metrics.Sample
	for _, folder := range folders {
		path := folder.path()
		if path == "" {
			continue
		}
		if free, _, _, ok := diskspace.Measure(path); ok {
			samples = append(samples, metrics.Sample{Labels: []string{folder.name}, Value: float64(free)})
		}
	}
	return samples
}

// registerMetrics has the metrics read, when Prometheus asks, the
// playbacks under way, the conversions, the database's connections and
// the disk space left in folders, and name the addons whose requests they
// count.
func registerMetrics(installed *addons.Store, playing *jellyfin.Handler, segments *hls.Manager, pool *pgxpool.Pool, folders []metricFolder) {
	metrics.AddonNames(installed.Names)
	metrics.Register(
		metrics.NewGauges("polyfin_playbacks", "Playbacks under way, by kind (video, live_tv, music, audiobook) and delivery (direct_play, remux, conversion).",
			[]string{"kind", "delivery"}, func(ctx context.Context) []metrics.Sample {
				samples, _ := playing.PlaybackCounts(ctx)
				return samples
			}),
		metrics.NewGauges("polyfin_conversions", "Conversions under way, of files and channels, by method: nvidia, vaapi, videotoolbox, qsv, amf or processor.",
			[]string{"method"}, func(context.Context) []metrics.Sample { return segments.ConversionsByMethod() }),
		metrics.NewGauges("polyfin_database_connections", "Connections to PostgreSQL, by state: in_use, idle, or max, the most the pool opens.",
			[]string{"state"}, func(context.Context) []metrics.Sample {
				stat := pool.Stat()
				return []metrics.Sample{{Labels: []string{"in_use"}, Value: float64(stat.AcquiredConns())},
					{Labels: []string{"idle"}, Value: float64(stat.IdleConns())}, {Labels: []string{"max"}, Value: float64(stat.MaxConns())}}
			}),
		metrics.NewGauges("polyfin_disk_free_bytes", "Bytes free to Polyfin on the disks of the folders it writes to: cache, conversion, reading (files being read), recordings and backups.",
			[]string{"folder"}, func(context.Context) []metrics.Sample { return diskFreeSamples(folders) }),
	)
}
