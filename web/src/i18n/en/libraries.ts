/** Libraries: which catalogs become libraries, and in which order. */
const libraries = {
  libraries: {
    title: 'Libraries',
    description:
      'Choose which catalogs of the server’s addons appear as libraries in Jellyfin apps, for every user.',
    shownTitle: 'Shown in Jellyfin apps',
    shownHelp:
      'Libraries appear in this order. Leave a name empty to use the catalog name. Libraries that share a name get their type added in apps, for example “Popular (Movies)”.',
    shownHelpMine:
      'Libraries appear in this order, after the server’s libraries if you use them. Leave a name empty to use the catalog name. Libraries that share a name get their type added in apps, for example “Popular (Movies)”.',
    defaultHelp:
      'When an addon is installed, its collection catalogs become libraries: each one shows its collections (for example genres or decades), which open on their movies and series. An addon without collections gets its movie, series and TV catalogs as libraries, up to 20. This is only a starting point: enable as many catalogs as you like. Each library adds a row to the home screen of Jellyfin apps, except TV catalogs, whose channels appear in Live TV.',
    count: (count: number) => (count === 1 ? '1 library' : `${count} libraries`),
    manyWarning: 'More than 20 libraries can make the home screen of Jellyfin apps heavy.',
    noAddons: 'Install an addon first: its catalogs will be listed here.',
    shownEmpty: 'No library: Jellyfin apps show nothing from these addons.',
    name: 'Name in apps',
    appName: (name: string) => `Shown in apps as “${name}”.`,
    liveTv: 'Its channels appear in Live TV in Jellyfin apps.',
    iptvVod: 'From an IPTV source.',
    iptvVodLink: 'Its import options',
    addonOff: 'Addon turned off',
    missing: 'No longer available',
    remove: 'Remove',
    removeLabel: (name: string) => `Remove ${name}`,
    removedLive: (name: string) => `${name} removed from the libraries.`,
    availableTitle: 'Available catalogs',
    availableHelp: 'Adding a catalog shows it as a library, at the end of the list above.',
    filter: 'Filter',
    filterPlaceholder: 'Catalog or addon name',
    type: 'Type',
    allTypes: 'All types',
    add: 'Add',
    addLabel: (name: string) => `Add ${name}`,
    addedLive: (name: string, position: number) => `${name} added as library ${position}.`,
    notBrowsable: 'Unavailable',
    notBrowsableHelp: 'Needs a search or another value, so it cannot be a library.',
    availableEmpty: 'Every catalog is already shown.',
    noMatch: 'No catalog matches this filter.',
    unsaved: 'Unsaved changes',
    upToDate: 'No unsaved changes',
    reset: 'Reset',
    saved: 'Libraries saved.',
    guideAfterSave: 'Save the libraries to add a programme guide to this catalog.',
    guideNone: 'No programme guide.',
    guideFetched: 'Last fetched',
    guideNever: 'Not fetched yet.',
    guideErrors: {
      unreachable:
        'The last fetch failed: the guide could not be downloaded. Check the address and try again.',
      private_network:
        'The last fetch failed: this guide is on a local network address. Only administrators can use such addresses.',
      too_large: 'The last fetch failed: the guide is larger than 300 MB.',
      malformed: 'The last fetch failed: this file is not an XMLTV guide.',
      channels_unreachable:
        'The last fetch failed: the addon did not list this catalog’s channels. Try again later.',
    } as Record<string, string>,
  },
}

export default libraries
