package iptv

import "context"

// analyzeTitles refreshes the planner's statistics of the sources' movies
// and series once a download is committed. A new catalogue is written in
// one COPY, and the table then stays without statistics until
// autovacuum's first visit, one to three minutes later or more on a busy
// server. Meanwhile the lookup of a title by its key, which every IPTV
// description runs, is planned on the index of its category and reads
// every title of the source and type: 2.4 ms instead of 0.07 on a
// catalogue of 16,378 titles, and Home's Next Up 270 ms instead of 155.
// ANALYZE reads a sample of 30,000 rows, about 0.1 s. A failure is
// logged, and leaves the download as it is.
func (s *Service) analyzeTitles(ctx context.Context) {
	if _, err := s.db.Exec(ctx, "ANALYZE iptv_titles"); err != nil && ctx.Err() == nil {
		s.logger.Debug("The IPTV titles' statistics could not be refreshed", "error", err)
	}
}
