package notifications

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/crashes"
	"github.com/moodiness/polyfin/internal/i18n"
)

// CrashJSON is a crash of the server: when its trace was written, and
// the first lines of it, with addresses and tokens hidden as in the log.
type CrashJSON struct {
	At    time.Time `json:"at"`
	Trace string    `json:"trace"`
}

const (
	// keepCrashes is how many crashes are kept.
	keepCrashes = 10
	// crashMessageLines is how many lines of the trace a message tells;
	// the webhook's JSON has them all.
	crashMessageLines = 6
)

// ServerCrashed keeps the crash the start found, for System › Health, and
// tells the server's targets and administrators' own of it once the
// targets are read. It returns once the crash is kept: an error leaves it
// to the next start.
func (s *Service) ServerCrashed(ctx context.Context, crash crashes.Report) error {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO server_crashes (crashed_at, trace, found_at) VALUES ($1, $2, $3)", crash.At, crash.Trace, s.now()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM server_crashes WHERE id <= (SELECT id FROM server_crashes ORDER BY id DESC OFFSET $1 LIMIT 1)`, keepCrashes)
		return err
	})
	if err != nil {
		return err
	}
	ev := s.crashEvent(crash)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawn(func() {
		select {
		case <-s.loaded:
		case <-s.ctx.Done():
			return
		}
		s.dispatchAdministrators(ev)
	})
	return nil
}

// LastCrash is the last crash kept, nil when there was none.
func (s *Service) LastCrash(ctx context.Context) (*crashes.Report, error) {
	var crash crashes.Report
	err := s.db.QueryRow(ctx, "SELECT crashed_at, trace FROM server_crashes ORDER BY id DESC LIMIT 1").Scan(&crash.At, &crash.Trace)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &crash, nil
}

// crashEvent tells of a crash, in the server language: when it happened,
// in the server's time zone, and the start of its trace.
func (s *Service) crashEvent(crash crashes.Report) Event {
	ev := s.newEvent(ServerCrashed)
	ev.Crash = &CrashJSON{At: crash.At.UTC(), Trace: crash.Trace}
	ev.Title = s.phrase(i18n.Text{"Polyfin crashed", "Polyfin a planté", "Polyfin ist abgestürzt", "Polyfin se bloqueó", "Polyfin si è arrestato in modo anomalo", "O Polyfin travou"})
	ev.Message = s.phrase(i18n.Text{"Polyfin stopped on an error at %s, and started again. The trace begins:", "Polyfin s’est arrêté sur une erreur le %s, puis a redémarré. La trace commence par :", "Polyfin wurde am %s durch einen Fehler beendet und neu gestartet. Der Trace beginnt mit:", "Polyfin se detuvo por un error el %s y volvió a iniciarse. El rastro empieza así:", "Polyfin si è fermato per un errore il %s ed è ripartito. La traccia inizia così:", "O Polyfin parou por um erro em %s e foi iniciado novamente. O rastreamento começa assim:"},
		crash.At.In(s.zone).Format("2006-01-02 15:04"))
	lines := strings.Split(crash.Trace, "\n")
	ev.Message += "\n" + strings.Join(lines[:min(len(lines), crashMessageLines)], "\n")
	ev.URL = s.adminLink("/system/health#process")
	return ev
}
