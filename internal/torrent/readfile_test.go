package torrent

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A file read whole, as a subtitle file beside the video is, costs the
// pieces that hold it alone: no piece past it, nor the start and the end
// of the video, which no playback read. A file past the limit is not read.
func TestReadFileFetchesItsOwnPiecesAlone(t *testing.T) {
	t.Parallel()
	const piece = 32 << 10
	video, english, filler, french := torrenttest.Bytes(10*piece), torrenttest.Bytes(5000), torrenttest.Bytes(4*piece), torrenttest.Bytes(3000)
	tor := torrenttest.Build("Movie", piece, torrenttest.File{Name: "Movie.mkv", Data: video},
		torrenttest.File{Name: "Movie.en.srt", Data: english}, torrenttest.File{Name: "Movie.nfo", Data: filler},
		torrenttest.File{Name: "Movie.fr.srt", Data: french})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	proxy := torrenttest.StartProxy(t, true, "", "")
	e := newEngine(t, proxied(proxy), 64<<20)
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	played, files, err := e.Files(ctx, spec)
	if err != nil || played.Path != "Movie/Movie.mkv" || len(files) != 4 || files[1].Path != "Movie/Movie.en.srt" {
		t.Fatalf("files: %+v of %+v, %v", played, files, err)
	}
	data, err := e.ReadFile(ctx, spec, 1, 1<<20)
	if err != nil || !bytes.Equal(data, english) {
		t.Fatalf("the English file: %d bytes, %v", len(data), err)
	}
	// The English file lies within the piece after the video's last.
	if sent := seeder.Sent(); sent != piece {
		t.Errorf("reading %d bytes cost %d bytes, not one piece of %d", len(english), sent, piece)
	}
	if seeder.Asked(0, len(video)) || seeder.Asked(len(video)+len(english)+len(filler), len(french)) {
		t.Error("blocks of the video or of the French file were asked")
	}
	if _, err := e.ReadFile(ctx, spec, 3, 1000); err == nil {
		t.Error("a file past the limit was read")
	}
}

// BenchmarkSubtitleFetch measures, on the local swarm, what fetching a
// subtitle file of 60 KB inside a torrent costs, by the torrent's piece
// length: the bytes downloaded and the time, from one peer sending a
// block of 16 KiB every 10 ms (about 1.6 MB/s), once the torrent started:
//
//	go test -run '^$' -bench SubtitleFetch -benchtime 3x ./internal/torrent
func BenchmarkSubtitleFetch(b *testing.B) {
	for _, mib := range []int{1, 4, 16} {
		b.Run(strconv.Itoa(mib)+"MiB", func(b *testing.B) {
			piece := mib << 20
			subtitle := torrenttest.Bytes(60 << 10)
			tor := torrenttest.Build("Movie", piece, torrenttest.File{Name: "Movie.mkv", Data: torrenttest.Bytes(2 * piece)},
				torrenttest.File{Name: "Movie.en.srt", Data: subtitle}, torrenttest.File{Name: "Movie.nfo", Data: torrenttest.Bytes(piece)})
			var took time.Duration
			var sent int64
			for range b.N {
				seeder := torrenttest.Seed(b, tor, "127.0.0.1")
				seeder.Delay(10 * time.Millisecond)
				e := newEngine(b, proxied(torrenttest.StartProxy(b, true, "", "")), 256<<20)
				spec, _ := NewSpec(tor.HashHex(), nil, []string{"tracker:" + torrenttest.HTTPTracker(b, "127.0.0.1", seeder.Address()).URL()})
				stop := e.Play(spec)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				if _, _, err := e.Files(ctx, spec); err != nil {
					b.Fatal(err)
				}
				started := time.Now()
				data, err := e.ReadFile(ctx, spec, 1, 1<<20)
				if err != nil || !bytes.Equal(data, subtitle) {
					b.Fatalf("the subtitle file: %d bytes, %v", len(data), err)
				}
				took += time.Since(started)
				sent += seeder.Sent()
				cancel()
				stop()
				seeder.Close()
			}
			b.ReportMetric(float64(sent)/float64(b.N)/(1<<20), "MiB-downloaded")
			b.ReportMetric(took.Seconds()/float64(b.N), "s-fetch")
			b.ReportMetric(0, "ns/op")
		})
	}
}
