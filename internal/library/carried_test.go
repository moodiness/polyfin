package library

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// mostCompared is how many streams a stream is compared with at most to
// find its file among others (see sameFiles): it shares five keys at most
// and, for each, is compared with the two streams that share it or, when
// more do, with two of each of the four classes its file's name and size
// agree with.
const mostCompared = 5 * 4 * 2

// pairedIdentities is carryIdentities as it was first written: each stream
// of the answer not known before is compared with every stream the answer
// no longer lists, both links parsed for each pair. However the streams to
// compare are found, the identities carried are these.
func pairedIdentities(answer, before []stremio.Stream) []stremio.Stream {
	listed := make(map[string]bool, len(answer))
	for _, stream := range answer {
		listed[streamIdentity(stream)] = true
	}
	known := make(map[string]bool, len(before))
	var gone []stremio.Stream
	for _, stream := range before {
		identity := streamIdentity(stream)
		if !known[identity] && !listed[identity] && listable(stream) {
			gone = append(gone, stream)
		}
		known[identity] = true
	}
	match := make([]int, len(answer))
	claims := make([]int, len(gone))
	for i, stream := range answer {
		match[i] = -1
		if !listable(stream) || known[streamIdentity(stream)] {
			continue
		}
		several := false
		for j, old := range gone {
			if !pairedFile(old, stream) {
				continue
			}
			claims[j]++
			several = several || match[i] >= 0
			match[i] = j
		}
		if several {
			match[i] = -1
		}
	}
	carried := slices.Clone(answer)
	for i, j := range match {
		if j >= 0 && claims[j] == 1 {
			carried[i].Identity = streamIdentity(gone[j])
		}
	}
	return carried
}

// pairedFile is sameFile for two streams as they are listed.
func pairedFile(a, b stremio.Stream) bool {
	ha, hb := a.BehaviorHints, b.BehaviorHints
	if ha.Filename != "" && hb.Filename != "" && ha.Filename != hb.Filename || ha.VideoSize > 0 && hb.VideoSize > 0 && ha.VideoSize != hb.VideoSize {
		return false
	}
	if a.Torrent() || b.Torrent() {
		return a.Torrent() && b.Torrent() && torrentLink(a) == torrentLink(b)
	}
	if a.URL == b.URL || ha.Filename != "" && ha.Filename == hb.Filename {
		return true
	}
	host, file := linkFile(a.URL)
	if otherHost, otherFile := linkFile(b.URL); file == "" || host != otherHost || file != otherFile {
		return false
	}
	return file == ha.Filename || file == hb.Filename || versionName(a) == versionName(b)
}

// alikeStreams lists count streams as addons list them, many of them
// alike: each takes its link, its name, its file's name and its file's
// size among variety values at most. Some are torrents' files, some
// notices, some links without a last part, and some carry an identity
// already.
func alikeStreams(random *rand.Rand, count, variety int) []stremio.Stream {
	some := func() int { return random.IntN(variety) }
	pick := func(values ...string) string { return values[random.IntN(len(values))] }
	streams := make([]stremio.Stream, count)
	for i := range streams {
		stream := stremio.Stream{Name: pick("", fmt.Sprintf("Source %d", some())), Description: pick("", "1080p")}
		if random.IntN(2) == 0 {
			stream.BehaviorHints.Filename = fmt.Sprintf("%d.mkv", some())
		}
		if random.IntN(2) == 0 {
			stream.BehaviorHints.VideoSize = stremio.Number64(1000 * (1 + some()))
		}
		if random.IntN(8) == 0 {
			stream.Identity = fmt.Sprintf("carried %d", some())
		}
		switch random.IntN(10) {
		case 0:
			stream.ExternalURL = "https://addon.example/"
		case 1, 2:
			stream.InfoHash = fmt.Sprintf("%040x", some()%3)
			if index := some(); index%2 == 0 {
				stream.FileIdx = &index
			}
		case 3:
			stream.URL = pick("https://cdn.example", "https://cdn.example/folder/", "https://cdn.example/%zz")
		default:
			stream.URL = fmt.Sprintf("https://%s.example/%s%d.mkv%s", pick("cdn", "alt"), pick("", "1/", "2/"), some(), pick("", "?token=1", "?token=2"))
		}
		streams[i] = stream
	}
	return streams
}

// The identities carried are those of comparing every pair of streams,
// for lists whose streams are alike in each way sameFile tells: short
// lists where most streams have a match, and longer ones where many share
// a link, a name or a file's name. The work stays within its bound for
// each.
func TestCarriedIdentitiesAreThoseOfComparingEveryPair(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewPCG(1, 2))
	identity := func(a, b stremio.Stream) bool { return a.Identity == b.Identity }
	carried, shared := 0, 0
	for round := range 3000 {
		count, variety := 1+random.IntN(6), 1+random.IntN(3)
		if round%10 == 0 {
			count, variety = 10+random.IntN(40), 2+random.IntN(30)
		}
		before, answer := alikeStreams(random, count, variety), alikeStreams(random, count, variety)
		// Half of the answer is the list before, as an addon asked again
		// lists it: under a new token, or with other hints.
		for i := range answer {
			if random.IntN(2) == 0 {
				continue
			}
			again := before[random.IntN(len(before))]
			again.Identity = ""
			if link, _, _ := strings.Cut(again.URL, "?"); link != "" && random.IntN(2) == 0 {
				again.URL = fmt.Sprintf("%s?token=%d", link, 3+random.IntN(variety))
			}
			if random.IntN(3) == 0 {
				again.BehaviorHints.Filename = ""
			}
			if random.IntN(3) == 0 {
				again.BehaviorHints.VideoSize = 0
			}
			answer[i] = again
		}
		given := slices.Clone(answer)
		got, cost := carriedIdentities(answer, before)
		if want := pairedIdentities(answer, before); !slices.EqualFunc(got, want, identity) {
			for i := range got {
				if got[i].Identity != want[i].Identity {
					t.Errorf("round %d: stream %d %+v carries %q, want %q", round, i, answer[i], got[i].Identity, want[i].Identity)
				}
			}
			t.Fatalf("round %d: the list before was %+v", round, before)
		}
		if !slices.EqualFunc(answer, given, identity) {
			t.Fatalf("round %d: the answer itself changed", round)
		}
		if streams := len(before) + len(answer); cost.parsed > streams || cost.compared > mostCompared*streams {
			t.Fatalf("round %d: %d links parsed and %d pairs compared for %d streams", round, cost.parsed, cost.compared, streams)
		}
		for i := range got {
			if got[i].Identity != answer[i].Identity {
				carried++
			}
		}
		if cost.compared > 2*len(answer) {
			shared++
		}
	}
	// What the lists drawn must hold to prove anything.
	if carried < 1000 || shared < 100 {
		t.Errorf("%d identities carried, and %d lists whose streams were compared with more than one other each", carried, shared)
	}
}

// A stream takes the identity of the one stream it is among many that
// share its link's last part, its name or its magnet link, and none when
// several of them may be it.
func TestCarriedIdentitiesAmongManyStreamsAlike(t *testing.T) {
	t.Parallel()
	link := func(token int, filename string, size int) stremio.Stream {
		return stremio.Stream{Name: "A", URL: fmt.Sprintf("https://cdn.example/%d/video", token),
			BehaviorHints: stremio.StreamBehavior{Filename: filename, VideoSize: stremio.Number64(size)}}
	}
	file := func(filename string, size int) stremio.Stream {
		return stremio.Stream{InfoHash: strings.Repeat("ab", 20), BehaviorHints: stremio.StreamBehavior{Filename: filename, VideoSize: stremio.Number64(size)}}
	}
	files := []stremio.Stream{link(1, "a.mkv", 1000), link(2, "b.mkv", 2000), link(3, "c.mkv", 3000), link(4, "d.mkv", 4000)}
	tokens := []stremio.Stream{link(1, "", 0), link(2, "", 0), link(3, "", 0), link(4, "", 0)}
	told := []stremio.Stream{link(1, "a.mkv", 0), link(2, "", 2000), link(3, "b.mkv", 2000), link(4, "a.mkv", 3000)}
	for _, tc := range []struct {
		name           string
		before, answer []stremio.Stream
		// want is, for each stream of answer, the stream of before whose
		// identity it takes, -1 for its own.
		want []int
	}{
		{"one of the files at its link's last part", files, []stremio.Stream{link(9, "b.mkv", 0)}, []int{1}},
		{"each of the files at its link's last part", files,
			[]stremio.Stream{link(9, "d.mkv", 0), link(8, "", 1000), link(7, "c.mkv", 0), link(6, "", 2000)}, []int{3, 0, 2, 1}},
		{"none of the files at its link's last part", files, []stremio.Stream{link(9, "e.mkv", 0), link(8, "a.mkv", 5000)}, []int{-1, -1}},
		{"any of the files at its link's last part", files, []stremio.Stream{link(9, "", 0)}, []int{-1}},
		{"any of the streams with its name", tokens, []stremio.Stream{link(9, "", 0)}, []int{-1}},
		{"any of the streams whose last part it names", tokens, []stremio.Stream{link(9, "video", 0)}, []int{-1}},
		{"the one stream many may be", tokens[:1], []stremio.Stream{link(9, "", 0), link(8, "", 0), link(7, "", 0)}, []int{-1, -1, -1}},
		{"the one stream that tells no other name or size", told, []stremio.Stream{link(9, "z.mkv", 2000)}, []int{1}},
		{"two streams that tell its name or its size", told, []stremio.Stream{link(9, "a.mkv", 2000)}, []int{-1}},
		{"two streams that tell its name or none", told, []stremio.Stream{link(9, "b.mkv", 0)}, []int{-1}},
		{"one of its torrent's files", []stremio.Stream{file("a.mkv", 1000), file("b.mkv", 2000), file("c.mkv", 3000)},
			[]stremio.Stream{file("c.mkv", 0), file("", 1000)}, []int{2, 0}},
		{"any of its torrent's files", []stremio.Stream{file("a.mkv", 1000), file("b.mkv", 2000), file("c.mkv", 3000)},
			[]stremio.Stream{file("", 0)}, []int{-1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			carried := carryIdentities(tc.answer, tc.before)
			for i, stream := range carried {
				want := streamIdentity(tc.answer[i])
				if tc.want[i] >= 0 {
					want = streamIdentity(tc.before[tc.want[i]])
				}
				if got := streamIdentity(stream); got != want {
					t.Errorf("stream %d is %q, want %q", i, got, want)
				}
			}
			if !slices.EqualFunc(carried, pairedIdentities(tc.answer, tc.before), func(a, b stremio.Stream) bool { return a.Identity == b.Identity }) {
				t.Error("comparing every pair carries other identities")
			}
		})
	}
}

// Carrying identities takes work as the streams it compares, not as their
// product: each link is parsed once, and each stream compared with a few
// others, whether each stream is its own file, every stream may be any
// other, or all share a link but no file's name.
func TestCarryingIdentitiesCostsAsTheStreamsNotAsTheirProduct(t *testing.T) {
	t.Parallel()
	named := func(list string, n int) stremio.StreamBehavior {
		return stremio.StreamBehavior{Filename: fmt.Sprintf("%s%d.mkv", list, n), VideoSize: stremio.Number64(1000 + n)}
	}
	for _, tc := range []struct {
		name string
		// stream is the nth stream of a list.
		stream func(list string, n int) stremio.Stream
		// every tells whether each stream replaced has its one match in
		// the answer, none having any otherwise.
		every bool
	}{
		{"each stream its own file", func(list string, n int) stremio.Stream {
			return stremio.Stream{URL: fmt.Sprintf("https://f.example/%s/%d", list, n)}
		}, true},
		{"every stream at the same last part of a path", func(list string, n int) stremio.Stream {
			return stremio.Stream{URL: fmt.Sprintf("https://f.example/%s%d/video", list, n)}
		}, false},
		{"one link under as many file names", func(list string, n int) stremio.Stream {
			return stremio.Stream{URL: "https://f.example/video", BehaviorHints: named(list, n)}
		}, false},
		{"one name under as many file names", func(list string, n int) stremio.Stream {
			return stremio.Stream{Name: "A", URL: fmt.Sprintf("https://f.example/%s%d/video", list, n), BehaviorHints: named(list, n)}
		}, false},
		{"one torrent under as many file names", func(list string, n int) stremio.Stream {
			return stremio.Stream{InfoHash: strings.Repeat("ab", 20), BehaviorHints: named(list, n)}
		}, false},
		{"each file's size told only now, under one name", func(list string, n int) stremio.Stream {
			hints := stremio.StreamBehavior{Filename: fmt.Sprintf("%d.mkv", n)}
			if list == "b" {
				hints.VideoSize = stremio.Number64(1000 + n)
			}
			return stremio.Stream{Name: "A", URL: fmt.Sprintf("https://f.example/%s%d/video", list, n), BehaviorHints: hints}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var once carryCost
			for _, count := range []int{500, 1000} {
				before, answer := make([]stremio.Stream, count), make([]stremio.Stream, count+1)
				links := 0
				for n := range before {
					before[n] = tc.stream("a", n)
				}
				for n := range answer {
					answer[n] = tc.stream("b", n)
				}
				for _, stream := range slices.Concat(before, answer) {
					if stream.URL != "" {
						links++
					}
				}
				carried, cost := carriedIdentities(answer, before)
				took, want := 0, 0
				for i := range carried {
					if carried[i].Identity != "" {
						took++
					}
				}
				if tc.every {
					want = count
				}
				if took != want {
					t.Errorf("%d streams replaced by %d: %d identities carried, want %d", count, count+1, took, want)
				}
				if cost.parsed != links {
					t.Errorf("%d streams replaced by %d: %d links parsed, want each of the %d once", count, count+1, cost.parsed, links)
				}
				if most := mostCompared * (2*count + 1); cost.compared > most {
					t.Errorf("%d streams replaced by %d: %d pairs compared, want %d at most", count, count+1, cost.compared, most)
				}
				if count == 500 {
					once = cost
				} else if cost.compared > 2*once.compared+mostCompared {
					t.Errorf("twice the streams: %d pairs compared, %d before", cost.compared, once.compared)
				}
			}
		})
	}
}
