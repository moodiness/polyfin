/** My account: password, devices and tracking services. */
const account = {
  account: {
    title: 'My account',
    signedInAs: 'Signed in as ',
    sectionsLabel: 'Account sections',
    sections: { tracking: 'Tracking', devices: 'Devices', password: 'Password' },
    devicesHelp:
      'The apps signed in with your account. A device you sign out asks for your password the next time it opens.',
    passwordHelp: 'Changing your password signs out all your Jellyfin devices.',
    currentPassword: 'Current password',
    newPassword: 'New password',
    confirmPassword: 'Confirm new password',
    changePassword: 'Change password',
    changingPassword: 'Changing…',
    passwordChanged: 'Your password has been changed.',
    tracking: {
      title: 'Tracking',
      description:
        'Connect the services that keep track of what you watch. Polyfin tells each connected service the movies and episodes you watch in your Jellyfin apps.',
      loading: 'Loading tracking services…',
      status: {
        connected: 'Connected',
        notConnected: 'Not connected',
        waiting: 'Waiting for the code',
        reconnect: 'Needs connecting again',
        unreachable: 'Retrying',
        unavailable: 'Not set up',
        appRefused: 'App refused',
      },
      unavailable: (name: string) =>
        `${name} is not set up on this server yet: an administrator has to add the ${name} app in the settings first.`,
      setUpApp: (name: string) => `Set up the ${name} app`,
      codeIntro: (name: string) =>
        `Connect your ${name} account: Polyfin gives you a code to enter on the ${name} site.`,
      keyIntro: (name: string) => `Connect your ${name} account with your ${name} API key.`,
      keyLabel: (name: string) => `${name} API key`,
      savedKeyHelp: 'Only you can show it. To use another key, disconnect, then connect again.',
      keyHelp: {
        mdblist: 'Find it on mdblist.com, in Preferences, under API key.',
        publicmetadb: 'Find it in your PublicMetaDB account.',
      },
      connect: 'Connect',
      connecting: 'Connecting…',
      checking: 'Checking the key…',
      reconnect: 'Connect again',
      newCode: 'Get a new code',
      enterCode: (site: string) => `Go to ${site}, sign in, and enter this code:`,
      codeLabel: 'Code to enter',
      openSite: (site: string) => `Open ${site}`,
      waiting: 'This page updates by itself once you have entered it.',
      expires: (when: string) => `The code expires ${when}.`,
      codeEnded:
        'The code expired or was refused before it was entered. Get a new code to try again.',
      connectedAs: 'Connected as ',
      connectedWithKey: 'Connected with your API key',
      connected: 'Connected',
      connectedOn: (date: string) => ` on ${date}`,
      lastSent: ', last sent ',
      nothingSent: ', nothing sent yet',
      connectedToast: (name: string) => `${name} connected.`,
      disconnectedToast: (name: string) => `${name} disconnected.`,
      problemReconnect: (name: string) =>
        `${name} no longer accepts this connection, so nothing is sent to it. Connect again to start sending what you watch.`,
      problemUnreachable: (name: string) =>
        `${name} could not be reached lately. Polyfin keeps trying, and sends what you watched once it answers.`,
      disconnect: 'Disconnect',
      disconnectTitle: (name: string) => `Disconnect ${name}?`,
      disconnectBody: (name: string) =>
        `Polyfin stops telling it what you watch. What it already has stays on ${name}.`,
      invalidKey: (name: string) =>
        `${name} did not accept this key. Copy it again from your ${name} account.`,
      serviceUnreachable: (name: string) =>
        `${name} could not be reached. Try again in a few minutes.`,
      notAvailable: (name: string) =>
        `${name} is not set up on this server: an administrator has to add the ${name} app first.`,
      appRefused: (name: string) =>
        `${name} refused this server’s app. An administrator has to check its client ID and secret under Settings › Tracking.`,
      history: {
        toggle: (name: string) => `Import my ${name} history`,
        toggleHelp: (name: string) =>
          `Marks played in Polyfin what you watched on ${name}, and picks up where you stopped, so Continue Watching and Next Up match. Imported every 6 hours; nothing is sent back to ${name}.`,
        importNow: 'Import now',
        importing: 'Importing…',
        importingStatus: 'Importing your history…',
        never: 'Not imported yet.',
        lastImport: 'Last import',
        colon: ': ',
        counts: (played: number, resumed: number, unmapped: number) =>
          `${played === 1 ? '1 title' : `${played} titles`} marked played, ${resumed === 1 ? '1 resume point' : `${resumed} resume points`}, ${unmapped === 1 ? '1 title' : `${unmapped} titles`} not found in Polyfin.`,
        problem: {
          reconnect: (name: string) =>
            `${name} refused the connection during the last import. Connect again to import your history.`,
          unreachable: (name: string) =>
            `${name} could not be read whole during the last import. What was read was imported; Polyfin tries again in 6 hours.`,
          rate_limited: (name: string) =>
            `${name} asked Polyfin to wait during the last import. What was read was imported; Polyfin tries again in 6 hours.`,
        },
      },
    },
  },
}

export default account
