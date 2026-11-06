import { SourcesPage } from './SourcesPage'

/**
 * `/sources`, for administrators: the server's sources (Stremio and music addons, IPTV sources,
 * tuners and folders), which they order and manage, then each user's own sources, marked with
 * their owner, beside the page of the one open. Other users' are shown without actions.
 */
export default function SourcesRoute() {
  return <SourcesPage area="content" />
}
