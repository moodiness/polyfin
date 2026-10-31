package notifications

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Preview is a summary as an email target receives it: its subject, its
// plain text and its HTML, the period it tells, and whether it has
// nothing to tell, in which case none is sent.
type Preview struct {
	Subject string
	Text    string
	HTML    string
	Start   time.Time
	End     time.Time
	Empty   bool
}

// Preview makes the summary of event, WeeklySummary, MonthlySummary or
// YearlySummary, as "Send the weekly summary" and the like would send it
// now: of the server for user nil, else of that user. Nothing is sent.
// ErrNoPreview reports another event, ErrNotFound a user there is none
// of.
func (s *Service) Preview(ctx context.Context, event string, user *accounts.ID) (Preview, error) {
	if !slices.Contains(summaryEvents, event) {
		return Preview{}, ErrNoPreview
	}
	now := s.now()
	start, end := weekBefore(now, s.zone), now
	if event != WeeklySummary {
		start, end = reviewLast(event, now, s.zone)
	}
	var sum SummaryJSON
	var who *accounts.User
	var err error
	if user == nil {
		sum, err = s.serverSummary(ctx, event, start, end)
	} else {
		found, lookupErr := s.accounts.User(ctx, *user)
		if errors.Is(lookupErr, accounts.ErrNotFound) {
			return Preview{}, ErrNotFound
		} else if lookupErr != nil {
			return Preview{}, lookupErr
		}
		who = &found
		sum, err = s.userSummary(ctx, event, found, start, end)
	}
	if err != nil {
		return Preview{}, err
	}
	ev := s.summaryEvent(event, who, sum)
	text, page, err := s.emailBody(s.accounts.Settings(), ev)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Subject: oneLine(ev.Title), Text: text, HTML: page, Start: start, End: end, Empty: sum.empty()}, nil
}
