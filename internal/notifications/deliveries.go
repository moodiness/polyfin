package notifications

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/moodiness/polyfin/internal/accounts"
)

// The outcomes of a delivery, as Recent deliveries shows them.
const (
	// DeliveryDelivered: the target accepted the message.
	DeliveryDelivered = "delivered"
	// DeliveryRefused: the target refused Polyfin (401, 403, 404, 410).
	DeliveryRefused = "refused"
	// DeliveryRejected: the target refused the message (another 4xx).
	DeliveryRejected = "rejected"
	// DeliveryFailed: the target could not be reached, or answered server
	// errors, until the message was given up.
	DeliveryFailed = "failed"
	// DeliveryDropped: too many messages waited for the target, and this
	// one was the oldest.
	DeliveryDropped = "dropped"
	// DeliveryUnreadable: the target's address or token cannot be
	// decrypted.
	DeliveryUnreadable = "unreadable"
)

const (
	// keepDeliveries is how long deliveries are kept, and maxDeliveries
	// how many of each target's.
	keepDeliveries = 30 * 24 * time.Hour
	maxDeliveries  = 100
	// maxDeliveryTitle bounds the title kept, in characters.
	maxDeliveryTitle = 200
)

// Delivery is what came of a message sent to a target: the event, by its
// identifier, type and title; how many events it held, more than one for
// a grouped message; its outcome, one of the Delivery constants; the
// status the target answered, zero when it did not, and why it did not
// accept the message, without its address or tokens; how many times it
// was tried; when it was queued and when it was done with.
type Delivery struct {
	ID       int64
	EventID  string
	Type     string
	Title    string
	Events   int
	Outcome  string
	Status   int
	Error    string
	Attempts int
	QueuedAt time.Time
	At       time.Time
}

// outcomeOf is the outcome of a delivery res ended.
func outcomeOf(res result) string {
	switch res.outcome {
	case delivered:
		return DeliveryDelivered
	case refused:
		return DeliveryRefused
	case rejected:
		return DeliveryRejected
	case unreadable:
		return DeliveryUnreadable
	}
	return DeliveryFailed
}

// record keeps what came of d for the target id, and forgets the target's
// deliveries past its last maxDeliveries.
func (s *Service) record(id accounts.ID, d *delivery, outcome string, res result) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), s.timing.request)
	defer cancel()
	var status *int
	if res.status != 0 {
		status = &res.status
	}
	_, err := s.db.Exec(ctx, `INSERT INTO notification_deliveries (target_id, event_id, type, title, events, outcome, status, error, attempts, queued_at, at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		id, d.event.ID, d.event.Type, clip(d.event.Title, maxDeliveryTitle), max(len(d.event.Events), 1), outcome, status, res.err, d.attempts,
		d.queued, s.now())
	if err == nil {
		_, err = s.db.Exec(ctx, `DELETE FROM notification_deliveries WHERE target_id = $1 AND id <= (
			SELECT id FROM notification_deliveries WHERE target_id = $1 ORDER BY id DESC OFFSET $2 LIMIT 1)`, id, maxDeliveries)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		// The target was deleted meanwhile, its deliveries with it.
		return
	}
	if err != nil {
		s.logger.Warn("A notification delivery could not be recorded", "target", id.String(), "error", err)
	}
}

// Deliveries lists the last deliveries of the target id of owner, nil for
// the server, newest first: ErrNotFound when owner has no such target.
func (s *Service) Deliveries(ctx context.Context, owner *accounts.User, id accounts.ID) ([]Delivery, error) {
	if _, err := s.load(ctx, owner, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id, event_id, type, title, events, outcome, coalesce(status, 0), error, attempts, queued_at, at
		FROM notification_deliveries WHERE target_id = $1 AND at >= $2 ORDER BY id DESC LIMIT $3`, id, s.now().Add(-keepDeliveries), maxDeliveries)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Delivery, error) {
		var d Delivery
		err := row.Scan(&d.ID, &d.EventID, &d.Type, &d.Title, &d.Events, &d.Outcome, &d.Status, &d.Error, &d.Attempts, &d.QueuedAt, &d.At)
		return d, err
	})
}

// pruneDeliveries forgets the deliveries older than keepDeliveries.
func (s *Service) pruneDeliveries(ctx context.Context) {
	if _, err := s.db.Exec(ctx, "DELETE FROM notification_deliveries WHERE at < $1", s.now().Add(-keepDeliveries)); err != nil && ctx.Err() == nil {
		s.logger.Warn("Old notification deliveries could not be deleted", "error", err)
	}
}
