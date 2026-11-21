package torrent

import (
	"crypto/rand"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// BenchmarkStoreTorrents has torrents write pieces to the store and read
// each in 64 KiB reads once it is kept, as playbacks behind their
// downloads: a torrent writes no further ahead of its read than the
// read-ahead window, 16 MiB and 4 pieces at least. The store fills, and
// removes pieces for room. Each iteration is a piece of each torrent. It
// reports the reads' latency percentiles and the processor time per MB
// written and read. Set POLYFIN_STORE_BENCH_DIR to a folder on the disk
// to measure, as the system's temporary folder may be in memory.
func BenchmarkStoreTorrents(b *testing.B) {
	for _, c := range []struct {
		name     string
		torrents int
		piece    int
	}{
		{"torrents=4/piece=256KiB", 4, 256 << 10},
		{"torrents=4/piece=4MiB", 4, 4 << 20},
	} {
		b.Run(c.name, func(b *testing.B) {
			dir := os.Getenv("POLYFIN_STORE_BENCH_DIR")
			if dir == "" {
				dir = b.TempDir()
			} else {
				var err error
				if dir, err = os.MkdirTemp(dir, "store-"); err != nil {
					b.Fatal(err)
				}
				defer os.RemoveAll(dir)
			}
			const budget = 512 << 20
			s, err := newStore(filepath.Join(dir, "torrents"), func() int64 { return budget }, slog.New(slog.DiscardHandler))
			if err != nil {
				b.Fatal(err)
			}
			defer s.close()
			data := make([]byte, c.piece)
			rand.Read(data)
			const chunk = 64 << 10
			latencies := make([][]time.Duration, c.torrents)
			var wg sync.WaitGroup
			cpu := processCPU()
			b.ResetTimer()
			for n := range c.torrents {
				hash := Hash{byte(n + 1)}
				written := make(chan int, max(4, readWindow/c.piece))
				wg.Go(func() {
					defer close(written)
					for i := range b.N {
						if err := s.put(pieceKey{hash, i}, data); err != nil {
							b.Error(err)
							return
						}
						written <- i
					}
				})
				wg.Go(func() {
					p := make([]byte, chunk)
					times := make([]time.Duration, 0, b.N*c.piece/chunk)
					for i := range written {
						for off := 0; off < c.piece; off += chunk {
							start := time.Now()
							found, err := s.read(pieceKey{hash, i}, p, int64(off))
							times = append(times, time.Since(start))
							if err != nil || !found {
								b.Errorf("piece %d: found %v, %v", i, found, err)
							}
						}
					}
					latencies[n] = times
				})
			}
			wg.Wait()
			b.StopTimer()
			used := processCPU() - cpu
			all := slices.Concat(latencies...)
			slices.Sort(all)
			if len(all) == 0 {
				return
			}
			at := func(q float64) float64 {
				return float64(all[min(len(all)-1, int(q*float64(len(all))))].Nanoseconds()) / 1e3
			}
			b.ReportMetric(at(0.5), "p50-µs")
			b.ReportMetric(at(0.9), "p90-µs")
			b.ReportMetric(at(0.99), "p99-µs")
			b.ReportMetric(at(0.999), "p99.9-µs")
			b.ReportMetric(float64(all[len(all)-1].Nanoseconds())/1e3, "max-µs")
			if used > 0 {
				moved := 2 * int64(b.N) * int64(c.torrents) * int64(c.piece)
				b.ReportMetric(float64(used.Microseconds())/float64(moved>>20), "cpu-µs/MB")
			}
		})
	}
}
