package iptv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// How a stream of a line-up failed when a channel was opened, as playback
// classifies it.
const (
	// FailureDead is an answer that is not a stream: an error status but a
	// refusal, a web page, an empty or short body, bytes of no known
	// container. It is left out of its channel for an hour, then six, then
	// a day (see deadBackoff).
	FailureDead = "dead"
	// FailureRefused is a provider that would not serve the stream now:
	// 401, 403, 429, 458, its connections all in use. It hides nothing:
	// the stream is tried again at the next start.
	FailureRefused = "refused"
	// FailureTimeout is a stream from which nothing came in time. A
	// provider can be slow once: a stream that played at its address is
	// left out for timeoutBackoff only once silent twice in a row, one that
	// never played at once.
	FailureTimeout = "timeout"
)

// timeoutBackoff is how long a silent stream is left out of its channel.
const timeoutBackoff = 10 * time.Minute

// deadBackoff is how long a stream found dead failures times in a row is
// left out of its channel.
func deadBackoff(failures int) time.Duration {
	switch {
	case failures >= 3:
		return 24 * time.Hour
	case failures == 2:
		return 6 * time.Hour
	default:
		return time.Hour
	}
}

// health is what is known of a stream: when it last played at the address
// its health was found at, and its last failures in a row.
type health struct {
	okAt     *time.Time
	failure  *string
	failedAt *time.Time
	failures int
}

// hidden reports whether a stream that failed is left out of its channel
// at now.
func (h health) hidden(now time.Time) bool {
	if h.failure == nil || h.failedAt == nil {
		return false
	}
	switch *h.failure {
	case FailureDead:
		return now.Before(h.failedAt.Add(deadBackoff(h.failures)))
	case FailureTimeout:
		return (h.okAt == nil || h.failures >= 2) && now.Before(h.failedAt.Add(timeoutBackoff))
	}
	return false
}

// addressHash identifies the address a stream's health was found at: a
// list that gives the stream another address forgets it.
func addressHash(address string) string {
	sum := sha256.Sum256([]byte(address))
	return hex.EncodeToString(sum[:16])
}

// ReportStream records how a stream of a channel (its Stremio identifier)
// answered when it was opened: failure is one of the Failure codes, empty
// when it played. The stream is the channel's stream at address; a
// channel that is not one of the source's, or a stream it no longer has,
// records nothing.
func (s *Service) ReportStream(ctx context.Context, source accounts.ID, channel, address, failure string) error {
	key, ok := strings.CutPrefix(channel, prefix(source))
	if !ok || strings.HasPrefix(key, "vod:") || strings.HasPrefix(key, "ep:") || strings.HasPrefix(key, "series:") {
		return nil
	}
	hash := addressHash(address)
	match := `s.addon_id = $1 AND s.channel_id = $2 AND coalesce(s.custom_url,
		(SELECT e.url FROM iptv_entries e WHERE e.addon_id = s.addon_id AND e.key = s.key)) = $3`
	var err error
	if failure == "" {
		_, err = s.db.Exec(ctx, `UPDATE iptv_streams s SET health_url = $4, ok_at = $5, failure = NULL, failed_at = NULL, failures = 0
			WHERE `+match, source, key, address, hash, s.now())
	} else {
		// The time it played belongs to the address: a failure at another
		// one forgets it.
		_, err = s.db.Exec(ctx, `UPDATE iptv_streams s SET failures = CASE WHEN s.health_url = $4 AND s.failure = $6 THEN s.failures + 1 ELSE 1 END,
			ok_at = CASE WHEN s.health_url = $4 THEN s.ok_at END, health_url = $4, failure = $6, failed_at = $5 WHERE `+match, source, key, address, hash, s.now(), failure)
	}
	return err
}

// StreamHealth is how a stream of a line-up last answered, for the admin
// app: OKAt the last time it played; Failure, FailedAt and Failures its
// last failures in a row; HiddenUntil when it is left out of its channel
// until then.
type StreamHealth struct {
	OKAt        *time.Time `json:"okAt"`
	Failure     string     `json:"failure"`
	FailedAt    *time.Time `json:"failedAt"`
	Failures    int        `json:"failures"`
	HiddenUntil *time.Time `json:"hiddenUntil"`
}

// streamHealth describes a stream's health at now; it is empty when it was
// found at another address than the stream's.
func streamHealth(address, checked string, h health, now time.Time) StreamHealth {
	if checked == "" || checked != addressHash(address) {
		return StreamHealth{}
	}
	result := StreamHealth{OKAt: h.okAt, FailedAt: h.failedAt, Failures: h.failures}
	if h.failure != nil {
		result.Failure = *h.failure
		if h.hidden(now) {
			until := h.failedAt.Add(timeoutBackoff)
			if *h.failure == FailureDead {
				until = h.failedAt.Add(deadBackoff(h.failures))
			}
			result.HiddenUntil = &until
		}
	}
	return result
}

// ForgetStreamHealth clears what is known of the health of the streams of
// a channel of the scope's source, by its item identifier, so that they are
// all tried at its next start: the admin app's "try again".
func (s *Service) ForgetStreamHealth(ctx context.Context, scope addons.Scope, source, item accounts.ID) error {
	if err := s.owned(ctx, scope, source); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `UPDATE iptv_streams s SET health_url = NULL, ok_at = NULL, failure = NULL, failed_at = NULL, failures = 0
		FROM iptv_lineup l WHERE l.addon_id = $1 AND l.item_id = $2 AND s.addon_id = l.addon_id AND s.channel_id = l.id`, source, item)
	if err == nil && tag.RowsAffected() == 0 {
		var exists bool
		if err := s.db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM iptv_lineup WHERE addon_id = $1 AND item_id = $2)", source, item).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return addons.ErrNotFound
		}
	}
	return err
}

// Connections is how many streams of a source may play at once, as its
// Xtream server said at the last download: 0 for no limit or when it did
// not say. An unknown source has no limit.
func (s *Service) Connections(ctx context.Context, source accounts.ID) (int, error) {
	var limit *int
	err := s.db.QueryRow(ctx, "SELECT max_connections FROM iptv_sources WHERE addon_id = $1", source).Scan(&limit)
	if errors.Is(err, pgx.ErrNoRows) || limit == nil {
		return 0, nil
	}
	return *limit, err
}
