const en = {
  documentTitle: 'Server status · Polyfin',
  header: {
    productName: 'Polyfin',
    subtitle: 'Administration',
  },
  language: {
    label: 'Interface language',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
  },
  status: {
    title: 'Server status',
    description: 'Health of this Polyfin server at a glance.',
    version: 'Version',
    serverId: 'Server ID',
    database: 'Database',
    databaseReady: 'Ready',
    databaseUnavailable: 'Unavailable',
    loading: 'Loading server status…',
    errorTitle: 'Server unreachable',
    errorBody:
      'The administration API did not respond. Check that Polyfin is running, then try again.',
    staleWarning: 'The latest refresh failed. The details below may be out of date.',
    retry: 'Try again',
    retrying: 'Retrying…',
    autoRefresh: 'Refreshes automatically every 10 seconds.',
    updatedAt: (time: string) => `Last updated at ${time}`,
  },
  footer: {
    sourceCode: 'Source code on GitHub',
  },
}

export type Messages = typeof en

export default en
