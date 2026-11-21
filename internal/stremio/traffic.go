package stremio

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/logs"
)

// Origin is the work a request to an addon was made for, which Health
// counts its requests by (see WithOrigin): a user waiting, or one of the
// kinds of background work.
type Origin uint8

const (
	// OriginApps: a user waits for the answer, as when a title is opened
	// or played, or a catalog browsed. A request no work marked is one.
	OriginApps Origin = iota
	// OriginPreload: the next episodes preloaded, and the titles prepared
	// ahead of a play.
	OriginPreload
	// OriginWatchlists: the watchlists checked for versions, and their
	// titles made playlist entries.
	OriginWatchlists
	// OriginCatalogs: the libraries' first pages, the collections and the
	// series' episodes read, and the titles of home rows listed ahead.
	OriginCatalogs
	// OriginRatings: the ratings and genres refreshed.
	OriginRatings
	// OriginRefresh: the sources' manifests and configuration hints read
	// again.
	OriginRefresh
	// OriginImports: the titles of imports and tracking services found.
	OriginImports
	// OriginOther: any other background work, such as the follow-ups of
	// an answer, the scheduled tasks not above, and the links renewed for
	// trickplay images.
	OriginOther
	origins
)

// originNames name the origins, as the admin API tells them.
var originNames = [origins]string{"apps", "preload", "watchlists", "catalogs", "ratings", "refresh", "imports", "other"}

func (o Origin) String() string { return originNames[o] }

type originKey struct{}

// WithOrigin marks ctx's requests to addons as made for origin, whatever
// marked them before.
func WithOrigin(ctx context.Context, origin Origin) context.Context {
	return context.WithValue(ctx, originKey{}, origin)
}

// InBackground marks ctx's requests to addons as background work's: those
// a kind of background work marked keep it, the others are OriginOther.
func InBackground(ctx context.Context) context.Context {
	if originOf(ctx) != OriginApps {
		return ctx
	}
	return WithOrigin(ctx, OriginOther)
}

// originOf is the origin ctx's requests are marked with, OriginApps when
// none.
func originOf(ctx context.Context) Origin {
	origin, _ := ctx.Value(originKey{}).(Origin)
	return origin
}

// Counts are requests by origin (an Origin as index), and those of them
// that failed.
type Counts struct {
	Requests [origins]int
	Failures [origins]int
}

// Traffic counts an addon's requests of the last hour, the current minute
// and the 59 before, and of the last day, the current hour and the 23
// before.
type Traffic struct {
	Hour, Day Counts
}

const (
	trafficMinutes = 60
	trafficHours   = 24
)

// trafficBucket counts the requests of one minute, or one hour, by origin.
// period is that minute, or hour, since the Unix epoch.
type trafficBucket struct {
	period   atomic.Int64
	requests [origins]atomic.Uint32
	failures [origins]atomic.Uint32
}

// traffic counts an addon's requests in per-minute buckets for the last
// hour and hourly ones for the last day, without a lock: a bucket is
// emptied under rolling once, when the first request of a new period
// reaches it. A reader adds up the buckets of its window as they stand.
type traffic struct {
	rolling sync.Mutex
	minutes [trafficMinutes]trafficBucket
	hours   [trafficHours]trafficBucket
}

// count counts a request for origin made at now, failed or not.
func (t *traffic) count(now time.Time, origin Origin, failed bool) {
	unix := now.Unix()
	t.add(t.minutes[:], unix/60, origin, failed)
	t.add(t.hours[:], unix/3600, origin, failed)
}

func (t *traffic) add(buckets []trafficBucket, period int64, origin Origin, failed bool) {
	b := &buckets[period%int64(len(buckets))]
	if b.period.Load() != period {
		t.rolling.Lock()
		if b.period.Load() < period {
			for i := range b.requests {
				b.requests[i].Store(0)
				b.failures[i].Store(0)
			}
			b.period.Store(period)
		}
		t.rolling.Unlock()
		// A request counted after its bucket moved on to a later period
		// is left out.
		if b.period.Load() != period {
			return
		}
	}
	b.requests[origin].Add(1)
	if failed {
		b.failures[origin].Add(1)
	}
}

// totals adds up the requests of the last hour and of the last day at now.
func (t *traffic) totals(now time.Time) Traffic {
	unix := now.Unix()
	return Traffic{Hour: sum(t.minutes[:], unix/60), Day: sum(t.hours[:], unix/3600)}
}

func sum(buckets []trafficBucket, current int64) Counts {
	var counts Counts
	oldest := current - int64(len(buckets)) + 1
	for i := range buckets {
		b := &buckets[i]
		if at := b.period.Load(); at < oldest || at > current {
			continue
		}
		for origin := range b.requests {
			counts.Requests[origin] += int(b.requests[origin].Load())
			counts.Failures[origin] += int(b.failures[origin].Load())
		}
	}
	return counts
}

const (
	// noticesKept bounds the distinct notices kept of each addon: past it,
	// the one seen least lately is forgotten.
	noticesKept = 10
	// noticeBytes bounds the text of a notice kept, and noticeReadBytes
	// what of it is read before it is redacted.
	noticeBytes     = 300
	noticeReadBytes = 2048
	// noticeLife is how long a notice not seen again is kept.
	noticeLife = 24 * time.Hour
)

// Notice is a text an addon's answers carried instead of something to
// play or describe, such as the failure of an addon it gathers. Its URLs
// and what looks like a key are hidden (see logs.RedactText). The texts
// alike but for their digits (see NoticeForm) are one notice, which tells
// the latest of them.
type Notice struct {
	Text string
	// RateLimited tells that the text says the addon, or one it gathers,
	// refused requests for their rate (see NoticeRateLimited).
	RateLimited bool
	// Answers counts the answers that carried it since First, Background
	// of them to background work's requests; Last is the latest.
	Answers, Background int
	First, Last         time.Time
	// key is what tells it apart (see keyOf).
	key noticeKey
}

// noticeKey tells notices apart: by their text's form (see NoticeForm),
// and whether it tells a refusal for the rate, so that a refusal never
// merges with a summary that happens to count 429 of something.
type noticeKey struct {
	form        string
	rateLimited bool
}

func keyOf(text string) noticeKey {
	return noticeKey{NoticeForm(text), NoticeRateLimited(text)}
}

// NoticeForm is a notice's text as notices are told apart: each run of
// digits one '#', as an addon's summary changes its counts and timings
// from one answer to the next, and would otherwise fill Health's
// noticesKept places, and the log, with one notice.
func NoticeForm(text string) string {
	var form strings.Builder
	digits := false
	for _, r := range text {
		isDigit := r >= '0' && r <= '9'
		switch {
		case !isDigit:
			form.WriteRune(r)
		case !digits:
			form.WriteByte('#')
		}
		digits = isDigit
	}
	return form.String()
}

// rateLimitHints are what a notice saying a request was refused for its
// rate may say besides SaysRateLimited's words, in lower case. They pause
// no request: Health marks the notice, and the library remembers an answer
// of such notices alone a minute only (see NoticeRateLimited).
var rateLimitHints = regexp.MustCompile(`\b429\b|retry[ -]after`)

// NoticeRateLimited reports whether a notice's text tells a refusal for
// the rate of requests: too many requests, a rate limit, 429, or a time to
// retry after. Health marks such a notice Too many requests.
func NoticeRateLimited(text string) bool {
	return SaysRateLimited(text) || rateLimitHints.MatchString(strings.ToLower(text))
}

// Notice reports whether the stream is a notice: text alone, with nothing
// to play, open or download.
func (s Stream) Notice() bool {
	return s.URL == "" && s.InfoHash == "" && s.ExternalURL == "" && s.YouTubeID == "" && s.Usenet == "" && !s.Archive()
}

// NoticeText is what a notice tells: its name, title and description, each
// on one line, those that repeat another left out, joined by ": ".
func (s Stream) NoticeText() string {
	return joinTexts(s.Name, s.Title, s.Description)
}

func joinTexts(texts ...string) string {
	var parts []string
	for _, part := range texts {
		if part = strings.Join(strings.Fields(part), " "); part != "" && !slices.Contains(parts, part) {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ": ")
}

// keptNotice returns text as a notice is kept: noticeReadBytes of it at
// most redacted (see logs.RedactText), then cut to noticeBytes.
func keptNotice(text string) string {
	return cutText(logs.RedactText(cutText(text, noticeReadBytes)), noticeBytes)
}

// cutText cuts text to most bytes at most, on a character's boundary,
// with an ellipsis when it was cut.
func cutText(text string, most int) string {
	if len(text) <= most {
		return text
	}
	end := most - len("…")
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}

// noticesOf returns the distinct texts of the notices of an answer, as
// they are kept, noticesKept at most: one of the texts alike but for
// their digits (see keyOf).
func noticesOf(streams []Stream) []string {
	var texts []string
	var keys []noticeKey
	for _, s := range streams {
		if !s.Notice() {
			continue
		}
		text := s.NoticeText()
		if text == "" {
			continue
		}
		text = keptNotice(text)
		if key := keyOf(text); !slices.Contains(keys, key) {
			keys = append(keys, key)
			if texts = append(texts, text); len(texts) == noticesKept {
				break
			}
		}
	}
	return texts
}

// note records that an answer to a request for origin carried texts at
// now, in notices, which it returns: a text seen before, or one alike but
// for its digits (see keyOf), counts one more answer and becomes the
// notice's text; a new one forgets those not seen within noticeLife, and
// the one seen least lately when noticesKept are kept.
func note(notices []Notice, texts []string, origin Origin, now time.Time) []Notice {
	for _, text := range texts {
		key := keyOf(text)
		i := slices.IndexFunc(notices, func(n Notice) bool { return n.key == key })
		if i < 0 {
			notices = slices.DeleteFunc(notices, func(n Notice) bool { return now.Sub(n.Last) >= noticeLife })
			if len(notices) >= noticesKept {
				oldest := 0
				for j, n := range notices {
					if n.Last.Before(notices[oldest].Last) {
						oldest = j
					}
				}
				notices = slices.Delete(notices, oldest, oldest+1)
			}
			notices = append(notices, Notice{RateLimited: key.rateLimited, First: now, key: key})
			i = len(notices) - 1
		}
		n := &notices[i]
		n.Text = text
		n.Answers++
		if origin != OriginApps {
			n.Background++
		}
		n.Last = now
	}
	return notices
}

// recent returns a copy of the notices seen within noticeLife at now, the
// latest first.
func recent(notices []Notice, now time.Time) []Notice {
	result := make([]Notice, 0, len(notices))
	for _, n := range notices {
		if now.Sub(n.Last) < noticeLife {
			result = append(result, n)
		}
	}
	slices.SortFunc(result, func(a, b Notice) int { return b.Last.Compare(a.Last) })
	return result
}
