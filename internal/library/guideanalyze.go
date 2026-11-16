package library

import "context"

// analyzeGuides refreshes the planner's statistics of the guides'
// programmes and channels once a download is current. A download writes
// its programmes in one COPY, which commits only at its end: an automatic
// ANALYZE that runs meanwhile counts none of them, and every guide read is
// then planned as if the table were empty, until the next one, a minute
// or more later. On a guide of 143,873 programmes, a read of what airs now
// took 3 seconds instead of 8 milliseconds, and the search of a programme
// by its identifier 27 seconds. ANALYZE reads a sample of 30,000 rows of
// each table. A failure is logged, and leaves the download as it is.
func (s *Service) analyzeGuides(ctx context.Context) {
	if _, err := s.db.Exec(ctx, "ANALYZE live_guide_programmes, live_guide_channels"); err != nil && ctx.Err() == nil {
		s.logger.Debug("The guides' statistics could not be refreshed", "error", err)
	}
}
