/** Live TV: TV catalogs with their guides, IPTV channels, and recordings. */
const livetv = {
  livetv: {
    title: 'Live TV',
    description:
      'TV catalogs come from your sources. Open a catalog’s guides to give its channels a programme.',
    recordings: 'Recording settings',
    settings: 'Live TV settings',
    catalogsTitle: 'TV catalogs',
    catalogsHelp:
      'A TV catalog shown as a library puts its channels in Live TV in Jellyfin apps. Its guides give the channels their programmes.',
    catalogsLabel: 'TV catalogs',
    catalogsLoading: 'Loading TV catalogs…',
    catalogsEmptyTitle: 'No TV catalog yet',
    catalogsEmpty:
      'Install an addon that has a TV catalog, or add an IPTV source, under Sources: its channels are listed here.',
    addSource: 'Add a source',
    kinds: { stremio: 'Stremio addon', iptv: 'IPTV source' },
    guides: (count: number) => (count === 1 ? '1 guide' : `${count} guides`),
    guideNone: 'No programme guide',
    failing: (count: number) =>
      count === 1
        ? 'The last download of 1 guide failed.'
        : `The last download of ${count} guides failed.`,
    mapped: (mapped: string, channels: string) => `${mapped} of ${channels} channels with a guide`,
    guideLine: (position: number, message: string) => `Guide ${position}: ${message}`,
    manage: 'Guides and mapping',
    lineup: 'Line-up and guides',
    lineupLabel: (name: string) => `Line-up and guides of ${name}`,
    manageLabel: (name: string) => `Guides and mapping of ${name}`,
    notShownHelp: 'Show it as a library to put its channels in Live TV and give it guides.',
    openLibraries: 'Open Libraries',
    states: {
      notShown: 'Not in Live TV',
      addonOff: 'Addon turned off',
      missing: 'No longer available',
      failed: 'Guide download failed',
      noGuide: 'No guide',
      notFetched: 'Not fetched yet',
      incomplete: 'Guide incomplete',
      complete: 'Guide complete',
    },
    iptvTitle: 'IPTV channels',
    iptvHelp: 'The live channels of each IPTV source, as its line-up keeps them.',
    iptvLoading: 'Loading IPTV sources…',
    iptvEmptyTitle: 'No IPTV source yet',
    iptvEmpty:
      'Add an M3U playlist or an Xtream Codes account under Sources: its channels are counted here.',
    kindM3u: 'M3U playlist',
    kindXtream: 'Xtream Codes account',
    turnedOff: 'Turned off',
    lastDownload: 'Last download',
    nextDownload: 'next',
    never: 'never',
    figuresLabel: (name: string) => `Channels of ${name}`,
    figures: {
      entries: 'Live entries',
      enabled: 'Channels on',
      shown: 'Shown in apps',
      mapped: 'With a guide',
      unmapped: 'Without a guide',
    },
    noLiveTv: 'This source imports no live channels.',
    importOptions: 'Import options',
    channels: 'Channels',
    channelsLabel: (name: string) => `Channels of ${name}`,
    openMapping: 'Open the guide mapping',
    recordingsTitle: 'Recordings',
    recordingsLoading: 'Loading recordings…',
    recordingsOffTitle: 'Recording is off',
    recordingsOff:
      'Set POLYFIN_RECORDINGS_DIR on the container to turn it on. Padding and how long recordings are kept are under Settings › Recordings.',
    noTimersTitle: 'No recording scheduled',
    noTimers:
      'Recordings are scheduled from the programme guide in Jellyfin apps. They are listed here until they end.',
    recordingNow: 'Recording',
    scheduled: 'Scheduled',
    failed: 'Failed',
    series: 'Series',
    scheduledBy: (name: string) => `Scheduled by ${name}`,
    unknownChannel: 'Channel not found',
    guidesOf: (name: string) => `Guides of ${name}`,
  },
}

export default livetv
