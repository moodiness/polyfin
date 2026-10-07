/** Streamyfin home: the rows of the home screen of Streamyfin, a Jellyfin app. */
const streamyfin = {
  streamyfin: {
    title: 'Streamyfin home',
    description:
      'Choose the rows of the home screen of Streamyfin, a Jellyfin app, for every user.',
    rowsTitle: 'Rows',
    rowsHelp:
      'Streamyfin shows these rows on its home screen, in this order. With no row, Streamyfin shows its own home screen. Users see only the rows of libraries and collections they can see.',
    refreshHelp:
      'Changes reach Streamyfin when it refreshes its settings: when the app comes back to the foreground, or when the home screen is pulled down.',
    loading: 'Loading rows…',
    listLabel: 'Rows of the Streamyfin home screen',
    columnRow: 'Row',
    titleLabel: 'Title',
    kinds: {
      resume: 'Continue watching',
      nextUp: 'Next up',
      library: 'Library',
      collection: 'Collection',
    },
    inLibrary: (name: string) => `In ${name}`,
    unavailable: 'Unavailable',
    unavailableHelp: 'Its library or collection is gone or turned off: the row shows nothing.',
    removeLabel: (name: string) => `Remove ${name}`,
    removedLive: (name: string) => `${name} removed from the rows.`,
    addedLive: (name: string, position: number) => `${name} added as row ${position}.`,
    emptyTitle: 'No rows',
    empty: 'Streamyfin shows its own home screen.',
    emptyDraft: 'Once you save, Streamyfin shows its own home screen.',
    suggest: 'Suggested rows',
    suggestedLive: (count: number) =>
      `${count} suggested rows added. Save to show them in Streamyfin.`,
    addTitle: 'Add a row',
    addHelp: 'Choose what the row shows. It is added at the end of the list.',
    shows: 'Shows',
    choices: {
      resume: 'Continue watching',
      nextUp: 'Next up',
      library: 'A library',
      collection: 'A collection’s titles',
    },
    choiceHelp: {
      resume: 'The titles each user started and did not finish.',
      nextUp: 'The next episodes of the series each user watches.',
      library:
        'A movie or series library shows its titles. A collection library shows its collections.',
      collection: 'The movies and series of one collection.',
    },
    library: 'Library',
    chooseLibrary: 'Choose a library',
    collectionLibrary: 'Collection library',
    collection: 'Collection',
    chooseCollection: 'Choose a collection',
    collectionsLoading: 'Loading collections…',
    noLibraries: 'The server has no movie, series or collection library yet.',
    noCollectionLibraries: 'The server has no collection library.',
    noCollections: 'This library has no collections.',
    added: (name: string) => `${name} (added)`,
    add: 'Add row',
    full: (max: number) =>
      `Streamyfin home screens hold at most ${max} rows. Remove one to add another.`,
    saved: 'Streamyfin home saved.',
  },
}

export default streamyfin
