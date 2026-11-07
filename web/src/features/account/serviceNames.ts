import type { TrackingServiceName } from '@/api'

/** The tracking services' names, which no language translates. */
export const serviceNames: Record<TrackingServiceName, string> = {
  trakt: 'Trakt',
  simkl: 'Simkl',
  mdblist: 'MDBList',
  publicmetadb: 'PublicMetaDB',
  anilist: 'AniList',
  myanimelist: 'MyAnimeList',
  lastfm: 'Last.fm',
  listenbrainz: 'ListenBrainz',
}
