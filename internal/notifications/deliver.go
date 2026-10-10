package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
)

// delivery is a message waiting for a target: an event, when it was
// queued, and how many times it was tried.
type delivery struct {
	event    Event
	queued   time.Time
	attempts int
}

// lane sends one target's messages, one at a time, in order.
type lane struct {
	queue   []*delivery
	running bool
	// failures counts the deliveries in a row that failed.
	failures int
}

// dispatch queues ev for every target that receives its type and that
// recipient accepts.
func (s *Service) dispatch(ev Event, recipient func(target) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.targets {
		if t.wants(ev.Type) && recipient(t) {
			s.push(id, ev)
		}
	}
}

// push queues ev for the target id, and starts its lane. It is called with
// s.mu held.
func (s *Service) push(id accounts.ID, ev Event) {
	if s.closed {
		return
	}
	l := s.lanes[id]
	if l == nil {
		l = &lane{}
		s.lanes[id] = l
	}
	if len(l.queue) >= maxQueue {
		s.logger.Warn("Too many notifications wait for a target: the oldest was dropped", "target", id.String())
		l.queue = l.queue[1:]
	}
	l.queue = append(l.queue, &delivery{event: ev, queued: s.now()})
	if !l.running {
		l.running = s.spawn(func() { s.drive(id, l) })
	}
}

// drive sends the messages of a lane until none waits.
func (s *Service) drive(id accounts.ID, l *lane) {
	for {
		s.mu.Lock()
		if len(l.queue) == 0 || s.ctx.Err() != nil {
			l.running = false
			if len(l.queue) == 0 && s.lanes[id] == l {
				delete(s.lanes, id)
			}
			s.mu.Unlock()
			return
		}
		d := l.queue[0]
		t, ok := s.targets[id]
		confined := ok && t.owner != nil && !s.admins[*t.owner]
		s.mu.Unlock()
		wait := time.Duration(0)
		if ok && t.enabled {
			wait = s.attempt(id, l, t, d, confined)
		}
		if wait == 0 {
			s.mu.Lock()
			if len(l.queue) > 0 && l.queue[0] == d {
				l.queue = l.queue[1:]
			}
			s.mu.Unlock()
			continue
		}
		if !s.sleep(wait) {
			s.mu.Lock()
			l.running = false
			s.mu.Unlock()
			return
		}
	}
}

// attempt sends d to t once, and returns how long to wait before trying
// again, zero once it is done with: delivered, refused, or given up.
func (s *Service) attempt(id accounts.ID, l *lane, t target, d *delivery, confined bool) time.Duration {
	res := s.send(t, d.event, confined)
	s.mu.Lock()
	d.attempts++
	if res.outcome == retry {
		l.failures++
	} else {
		l.failures = 0
	}
	failures := l.failures
	s.mu.Unlock()
	switch res.outcome {
	case delivered:
		s.delivered(id)
		return 0
	case unreadable:
		return 0
	case refused, rejected:
		s.failing(id, res)
		s.logger.Warn("A notification target refused a message, which was dropped", "target", id.String(), "kind", t.kind,
			"event", d.event.Type, "status", res.status)
		return 0
	}
	backoff := s.timing.retryFirst
	for range min(d.attempts-1, 20) {
		backoff *= 2
		if backoff >= s.timing.retryMax {
			backoff = s.timing.retryMax
			break
		}
	}
	wait := max(backoff, res.after)
	if failures >= s.timing.unreachableAfter {
		s.failing(id, res)
	}
	if s.now().Add(wait).Sub(d.queued) > s.timing.giveUp {
		s.failing(id, res)
		s.logger.Warn("A notification could not be delivered in time and was dropped", "target", id.String(), "kind", t.kind,
			"event", d.event.Type, "attempts", d.attempts, "status", res.status, "error", res.err)
		return 0
	}
	return wait
}

// outcome is what came of a send.
type outcome int

const (
	// delivered: the target accepted the message.
	delivered outcome = iota
	// retry: it may accept it later.
	retry
	// refused: it refused Polyfin (401, 403, 404, 410).
	refused
	// rejected: it refused the message (another 4xx).
	rejected
	// unreadable: its address or token cannot be decrypted.
	unreadable
)

// result is what came of a send: the status the target answered, zero
// when it did not, how long it asked to wait, and why it could not be
// reached, without its address.
type result struct {
	outcome outcome
	status  int
	after   time.Duration
	err     string
}

// send sends ev to t once.
func (s *Service) send(t target, ev Event, confined bool) result {
	secret, err := s.box.Open(t.secret)
	if err != nil {
		return result{outcome: unreadable}
	}
	var request *http.Request
	if t.kind != Email {
		if request, err = s.request(t, secret, ev); err != nil {
			return result{outcome: rejected, err: "malformed request"}
		}
	}
	client := s.trusted
	if confined {
		client = s.confined
	}
	select {
	case s.sending <- struct{}{}:
	case <-s.ctx.Done():
		return result{outcome: retry, err: "stopping"}
	}
	defer func() { <-s.sending }()
	ctx, cancel := context.WithTimeout(s.ctx, s.timing.request)
	defer cancel()
	if t.kind == Email {
		return s.sendEmail(ctx, t, ev)
	}
	response, err := client.Do(request.WithContext(ctx))
	if err != nil {
		return result{outcome: retry, err: describeError(err)}
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxReply))
	_ = response.Body.Close()
	return classify(t.kind, response.StatusCode, response.Header, body)
}

// describeError says why a request failed in a few words, never with the
// address, which may hold a secret.
func describeError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	var timeout interface{ Timeout() bool }
	switch {
	case errors.Is(err, stremio.ErrPrivateNetwork):
		return "on a local network"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout():
		return "no answer in time"
	}
	return "could not connect"
}

// classify reads the status of the answer of a target of kind.
func classify(kind string, status int, header http.Header, body []byte) result {
	res := result{status: status}
	switch {
	case status >= 200 && status < 300:
		res.outcome = delivered
	case status == http.StatusTooManyRequests:
		res.outcome = retry
		res.after = retryAfter(header, body)
	case status == http.StatusUnauthorized, status == http.StatusForbidden, status == http.StatusNotFound, status == http.StatusGone,
		status == http.StatusBadRequest && refusedRequest(kind, body):
		res.outcome = refused
	case status >= 500 || status == http.StatusRequestTimeout:
		res.outcome = retry
		res.after = retryAfter(header, nil)
	default:
		res.outcome = rejected
	}
	return res
}

// refusedRequest reports whether a 400 answer from a target of kind
// refuses Polyfin rather than the message: Telegram tells a chat it does
// not know, Pushover a user key or application token it does not.
func refusedRequest(kind string, body []byte) bool {
	var answer struct {
		Description string `json:"description"`
		User        string `json:"user"`
		Token       string `json:"token"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return false
	}
	switch kind {
	case Telegram:
		return strings.Contains(strings.ToLower(answer.Description), "chat not found")
	case Pushover:
		return answer.User == "invalid" || answer.Token == "invalid"
	}
	return false
}

// retryAfter reads how long a target asked to wait: its Retry-After
// header, in seconds, possibly with a fraction, or as a date, else the
// retry_after field of a JSON body, in seconds, as Discord sends it, or of
// its parameters, as Telegram does.
func retryAfter(header http.Header, body []byte) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds >= 0 && !math.IsInf(seconds, 0) {
		return time.Duration(math.Ceil(min(seconds, 24*3600) * float64(time.Second)))
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(time.Until(at), 0)
	}
	var answer struct {
		RetryAfter float64 `json:"retry_after"`
		Parameters struct {
			RetryAfter float64 `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return 0
	}
	if seconds := max(answer.RetryAfter, answer.Parameters.RetryAfter); seconds > 0 {
		return time.Duration(math.Ceil(min(seconds, 24*3600) * float64(time.Second)))
	}
	return 0
}

// delivered records that the target id accepted a message, which clears
// its problem.
func (s *Service) delivered(id accounts.ID) {
	now := s.now()
	ctx := context.WithoutCancel(s.ctx)
	if _, err := s.db.Exec(ctx, `UPDATE notification_targets SET last_sent_at = $2, problem = NULL, problem_status = NULL, problem_at = NULL
		WHERE id = $1`, id, now); err != nil {
		s.logger.Warn("A delivered notification could not be recorded", "target", id.String(), "error", err)
	}
	s.mu.Lock()
	if t, ok := s.targets[id]; ok {
		t.lastSent, t.problem, t.problemStatus, t.problemAt = &now, "", nil, nil
		s.targets[id] = t
	}
	s.mu.Unlock()
}

// failing records the problem res shows with the target id, for the admin
// app: it refused Polyfin, rejected a message, or could not be reached.
func (s *Service) failing(id accounts.ID, res result) {
	problem := ProblemUnreachable
	switch res.outcome {
	case refused:
		problem = ProblemRefused
	case rejected:
		problem = ProblemRejected
	}
	var status *int
	if res.status != 0 {
		status = &res.status
	}
	now := s.now()
	ctx := context.WithoutCancel(s.ctx)
	if _, err := s.db.Exec(ctx, "UPDATE notification_targets SET problem = $2, problem_status = $3, problem_at = $4 WHERE id = $1",
		id, problem, status, now); err != nil {
		s.logger.Warn("A notification target's problem could not be recorded", "target", id.String(), "error", err)
	}
	s.mu.Lock()
	if t, ok := s.targets[id]; ok {
		t.problem, t.problemStatus, t.problemAt = problem, status, &now
		s.targets[id] = t
	}
	s.mu.Unlock()
}

// TestResult is what came of "Send a test": whether the target accepted
// the message, the status it answered, zero when it did not, and the
// target as it stands after.
type TestResult struct {
	Delivered bool
	Status    int
	Target    Target
}

// Test sends a test message to the target id of owner, once, at once, and
// records what came of it as a message would.
func (s *Service) Test(ctx context.Context, owner *accounts.User, id accounts.ID) (TestResult, error) {
	t, err := s.load(ctx, owner, id)
	if err != nil {
		return TestResult{}, err
	}
	res := s.send(t, s.testEvent(), owner != nil && !owner.IsAdministrator)
	switch res.outcome {
	case unreadable:
		return TestResult{}, ErrUnreadable
	case delivered:
		s.delivered(id)
	default:
		s.failing(id, res)
		s.logger.Info("A notification target did not accept a test message", "target", id.String(), "kind", t.kind,
			"status", res.status, "error", res.err)
	}
	if t, err = s.load(ctx, owner, id); err != nil {
		return TestResult{}, err
	}
	return TestResult{Delivered: res.outcome == delivered, Status: res.status, Target: s.describe(t)}, nil
}

// request builds the request that sends ev to t, whose secret is opened.
func (s *Service) request(t target, secret string, ev Event) (*http.Request, error) {
	var address string
	var payload any
	header := http.Header{}
	switch t.kind {
	case Webhook:
		address, payload = secret, ev
		header.Set("X-Polyfin-Event", ev.Type)
	case Discord:
		address, payload = secret, discordMessage(ev)
	case Ntfy:
		address, payload = t.address+"/", ntfyMessage(t.topic, ev)
		if secret != "" {
			header.Set("Authorization", "Bearer "+secret)
		}
	case Telegram:
		address, payload = s.telegramAPI+"/bot"+secret+"/sendMessage", telegramMessage(t.topic, ev, s.phrase("Open", "Ouvrir"))
	case Gotify:
		address, payload = t.address+"/message", gotifyMessage(ev)
		header.Set("X-Gotify-Key", secret)
	case Pushover:
		user, token, _ := strings.Cut(secret, "\n")
		address, payload = s.pushoverAPI, pushoverMessage(user, token, ev)
	default:
		return nil, ErrInvalidKind
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for name, values := range header {
		request.Header[name] = values
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Polyfin/"+s.version)
	return request, nil
}
