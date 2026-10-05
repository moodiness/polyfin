/** Sources: the server addons, each user's own sources, and music addons. */
const sources = {
  addons: {
    title: 'Addons',
    description:
      'Stremio addons installed here are shared with every user of this server. Each user can turn them off and add their own addons from “My addons”.',
    installTitle: 'Install an addon',
    manifestUrl: 'Manifest URL',
    manifestUrlPlaceholder: 'https://…/manifest.json',
    manifestUrlHint:
      'Copy the install link from the addon’s configure page. It often contains your settings or keys: Polyfin keeps it private and only ever shows a shortened form.',
    install: 'Install',
    installing: 'Installing…',
    installed: (name: string) => `${name} has been installed.`,
    listTitleShared: 'Server addons',
    listTitleMine: 'Your addons',
    listHelp:
      'Addons are used in this order. Turning one off hides its libraries in Jellyfin apps without losing them.',
    emptyShared: 'No addon is installed on this server yet.',
    emptyMine: 'You have not installed any addon yet.',
    version: (version: string) => `Version ${version}`,
    off: 'Turned off',
    enabled: 'Enabled',
    provides: 'Provides',
    types: 'Types',
    catalogs: 'Catalogs',
    catalogCount: (count: number) => (count === 1 ? '1 catalog' : `${count} catalogs`),
    lastRefresh: 'Last refresh',
    refresh: 'Refresh',
    refreshing: 'Refreshing…',
    refreshLabel: (name: string) => `Refresh ${name}`,
    refreshed: 'The manifest has been downloaded again.',
    replace: 'Replace URL',
    replaceLabel: (name: string) => `Replace URL of ${name}`,
    newManifestUrl: 'New manifest URL',
    replaceHint:
      'Use this after reconfiguring the addon on its configure page. Its libraries are kept.',
    replaceSubmit: 'Replace',
    replacing: 'Replacing…',
    replaced: 'The manifest URL has been replaced.',
    remove: 'Remove',
    removing: 'Removing…',
    removeConfirmShared: (name: string) =>
      `Remove ${name}? Its libraries disappear from Jellyfin apps for every user, and this cannot be undone.`,
    removeConfirmMine: (name: string) =>
      `Remove ${name}? Its libraries disappear from your Jellyfin apps, and this cannot be undone.`,
    removed: (name: string) => `${name} has been removed.`,
  },
  myAddons: {
    title: 'My addons',
    description:
      'Add your own Stremio addons and choose which of their catalogs appear as libraries in your Jellyfin apps.',
    preferenceTitle: 'Server addons',
    useShared: 'Use the server’s addons',
    useSharedHelp:
      'On: you get the server’s addons and libraries, followed by your own. Off: you only get your own addons and libraries.',
    preferenceSaved: 'Preference saved.',
    parentalControl:
      'Parental control applies to your account: your Jellyfin apps show the server’s addons and libraries only, as they give the ratings it relies on. Your own addons are kept but not used.',
    personalAddonsOff:
      'An administrator turned your own addons off: your Jellyfin apps show the server’s addons and libraries only. Your own addons are kept but not used.',
  },
  music: {
    content: { music: 'Music', audiobook: 'Audiobooks', podcast: 'Podcasts' } as Record<
      string,
      string
    >,
    library: {
      music: 'Music library',
      audiobook: 'Books library',
      podcast: 'Music library',
    } as Record<string, string>,
    libraryHelp: {
      music: 'Its items appear in the music view of Jellyfin apps.',
      audiobook: 'Its audiobooks appear in a books library of Jellyfin apps.',
      podcast: 'Its episodes appear in the music view of Jellyfin apps.',
    } as Record<string, string>,
    settings: 'Settings',
    settingsLabel: (name: string) => `Settings of ${name}`,
    settingsTitle: 'Addon settings',
    settingsHelp:
      'Declared by the addon and sent with every request Polyfin makes to it. They may hold your account details: only you, or the administrators for the server’s addons, can see them.',
    noSettings: 'This addon has no settings.',
    defaultValue: (value: string) => `Default: ${value}`,
    defaultEmpty: 'Default: empty',
    on: 'On',
    off: 'Off',
    range: (min: string, max: string) => `From ${min} to ${max}.`,
    atLeast: (min: string) => `At least ${min}.`,
    atMost: (max: string) => `At most ${max}.`,
    stepOf: (step: string) => `In steps of ${step}.`,
    maxLength: (count: number) => `${count} characters at most.`,
    tooLong: (count: number) => `Use ${count} characters at most.`,
    notNumber: 'Enter a number.',
    belowMin: (min: string) => `Enter ${min} or more.`,
    aboveMax: (max: string) => `Enter ${max} or less.`,
    offStep: (step: string) => `Use a multiple of ${step}.`,
    resetDefaults: 'Use the defaults',
    saved: 'Settings saved. The addon receives them from its next request.',
  },
}

export default sources
