import { useQuery } from '@tanstack/react-query'
import { fetchLiveSessions, queryKeys, type LiveSession } from '@/api'
import type { Messages } from '@/i18n'

/** How often the playbacks and the bandwidth are read again, while some play and while none do. */
export const playingPoll = 3_000
export const idlePoll = 15_000

/**
 * The playbacks under way, refreshed every 3 seconds while some play, every 15 otherwise; shared
 * by Home's summary and tiles, a member's Home, and the confirmation of what stops them.
 */
export function useLiveSessions() {
  return useQuery({
    queryKey: queryKeys.liveSessions,
    queryFn: ({ signal }) => fetchLiveSessions(signal),
    refetchInterval: (query) => ((query.state.data?.length ?? 0) > 0 ? playingPoll : idlePoll),
  })
}

/** What a playback plays: an episode's series, else the item's name. */
export function titleOf(session: LiveSession, t: Messages): string {
  const item = session.item
  if (item === null) return t.dashboard.live.unknownTitle
  return item.kind === 'episode' && item.seriesName ? item.seriesName : item.name
}
