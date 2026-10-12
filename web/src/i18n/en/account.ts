/** My account: password, devices and tracking services. */
const account = {
  account: {
    title: 'My account',
    signedInAs: 'Signed in as ',
    sectionsLabel: 'Account sections',
    sections: {
      statistics: 'Statistics',
      tracking: 'Tracking',
      import: 'Import from another server',
      appImport: 'Import from an app',
      notifications: 'Notifications',
      devices: 'Devices',
      password: 'Password',
    },
    /** How often connected accounts are imported, from the server's setting, in minutes. */
    importEvery: (minutes: number) =>
      minutes < 60
        ? `every ${minutes} minutes`
        : minutes === 60
          ? 'every hour'
          : `every ${minutes / 60} hours`,
    serverImport: {
      description:
        'Bring your played movies and episodes, resume points and favorites over from a Jellyfin or Emby server you used before. They go into your account only, matched by IMDb, TMDB or TVDB, and only add to what Polyfin already has. Nothing is written to the other server.',
      serverKind: 'Server',
      address: 'Server address',
      addressHelp:
        'The address the server opens at in a browser, such as http://192.168.1.10:8096.',
      name: 'Your user name there',
      password: 'Your password there',
      passwordHelp:
        'Used for this import only, and never kept. Polyfin signs in as you, as the server’s apps do, then signs out. Leave it empty if your account has none.',
      start: 'Import my watch history',
      starting: 'Starting…',
      started: 'The import has started. You can leave this page: it goes on.',
      lastTitle: 'Your last import',
      from: (server: string) => `From ${server}`,
    },
    appImport: {
      description:
        'Bring over what you watched in an app built on addons: the movies and episodes you played with their dates, your resume points, and the app’s library, which goes to your Watchlist in Jellyfin apps, matched by IMDb, TMDB or Kitsu. They only add to what Polyfin already has, and nothing is written to the app.',
      app: 'App',
      apps: { nuvio: 'Nuvio', stremio: 'Stremio' },
      backend: 'Nuvio backend',
      backendHelp:
        'Leave it empty for Nuvio’s own. A self-hosted backend works too, at its address, on the local network as well.',
      email: 'Email',
      password: 'Password',
      passwordHelp:
        'Used to read the account only, and never kept. Polyfin signs in as Nuvio’s apps do, reads the account, then signs out, unless you stay connected.',
      signIn: 'Sign in to Nuvio',
      signingIn: 'Signing in…',
      chooseTitle: 'Choose what to import',
      chooseHelp: {
        nuvio:
          'Nuvio keeps a history for each profile: choose the one to import. Polyfin keeps what it read for a few minutes only.',
        stremio:
          'Stremio keeps one history for the whole account. Polyfin keeps what it read for a few minutes only.',
      },
      stayConnected: 'Stay connected',
      stayConnectedHelp: {
        nuvio: (every: string) =>
          `Polyfin keeps the session, encrypted, never your password, and imports the profile you choose again ${every}. It only reads the account. You can disconnect at any time.`,
        stremio: (every: string) =>
          `Polyfin keeps the auth key, encrypted, and imports the account again ${every}. It only reads the account. You can disconnect at any time. Stremio’s terms forbid extracting its data by automated means: reading your account ${every} makes that grey area more pronounced than a one-off import.`,
      },
      staysConnected: (every: string) =>
        `You stay connected: this profile is imported again ${every}, the same way. Addons are installed now only.`,
      wholeAccount: 'The whole account',
      profile: 'Profile',
      profileNumber: (id: string) => `Profile ${id}`,
      profileCounts: (played: number, resumes: number, library: number) =>
        `${played} watched, ${resumes === 1 ? '1 resume point' : `${resumes} resume points`}, ${library} in the library.`,
      addons: 'Addons',
      addonsHelp:
        'Each addon ticked becomes one of the account’s own addons, under My sources. Its address may hold settings and keys: Polyfin never shows it whole.',
      addonsOff: 'Own addons are turned off for this account: the app’s addons are not offered.',
      noAddons: 'This profile has no addon.',
      back: 'Sign in again',
      start: 'Import this profile',
      starting: 'Starting…',
      started: 'The import has started. You can leave this page: it goes on.',
      currentTitle: 'Import in progress',
      lastTitle: 'Last import',
      ownLastTitle: 'Your last import',
      from: (app: string, profile: string) =>
        profile === '' ? `From ${app}` : `From ${app}, profile ${profile}`,
      startedLabel: 'Started',
      endedLabel: 'Ended',
      startedBy: (name: string) => `Started by ${name}`,
      states: { running: 'Running', done: 'Done', failed: 'Failed' },
      problem:
        'Polyfin ran into an error during the import: the server log tells more. What was imported is kept.',
      runningHelp:
        'You can leave this page: the import goes on. If Polyfin restarts meanwhile, the import stops: what it imported stays, and importing again adds nothing twice.',
      read: (count: number) => (count === 1 ? '1 title read.' : `${count} titles read.`),
      counts: (read: number, played: number, resumed: number, watchlist: number) =>
        `${read === 1 ? '1 title read' : `${read} titles read`}: ${played} marked played, ${resumed === 1 ? '1 resume point' : `${resumed} resume points`}, ${watchlist} added to the Watchlist.`,
      unmatched: (count: number) =>
        count === 1 ? '1 title not found' : `${count} titles not found`,
      unmatchedTitle: 'Titles not found',
      unmatchedHelp:
        'These titles were not imported: the app names them by an identifier Polyfin does not match, or no Polyfin title has it. For a series, the app may also tell the episodes watched by their place in a list of episodes that Polyfin’s addons list otherwise.',
      titleColumn: 'Title',
      kindColumn: 'Kind',
      reasonColumn: 'Reason',
      reasons: {
        other_identifier: 'Not an IMDb, TMDB or Kitsu identifier',
        not_found: 'Not in Polyfin',
        episode_list: 'Its episodes do not match Polyfin’s list: none was marked played',
      },
      addonInstalled: 'added',
      addonWaiting: 'waiting',
      addonProblems: {
        not_allowed: 'not added: own addons were turned off',
        exists: 'installed already',
        invalid_address: 'not added: not an addon’s address',
        private_network:
          'not added: on a local network, which only an administrator’s import reaches',
        unreachable: 'not added: the addon did not answer',
        invalid_manifest: 'not added: not an addon',
        internal: 'not added: Polyfin ran into an error',
      },
      stremio: {
        way: 'Sign in with',
        ways: { link: 'A link code', password: 'Email and password', key: 'An auth key' },
        linkHelp:
          'Polyfin asks Stremio for a code, as Stremio’s TV apps do. You approve it in a browser where you are signed in to Stremio: this works however your account signs in, Facebook and Apple included, and Polyfin never sees your password.',
        getCode: 'Get a link code',
        gettingCode: 'Asking Stremio…',
        codeLabel: 'Link code',
        approve:
          'Open the link in a browser where you are signed in to Stremio, check that it shows this code, and approve it.',
        openLink: 'Open the link',
        waiting:
          'Waiting for your approval. The code lasts 5 minutes; once you approve it, Polyfin reads the account, then signs out, unless you stay connected.',
        cancel: 'Cancel',
        email: 'Email',
        password: 'Password',
        passwordHelp:
          'Used to read the account only, and never kept. Polyfin signs in as Stremio’s apps do, reads the account, then signs out, unless you stay connected. An account made with Facebook or Apple has no password: use a link code.',
        signIn: 'Sign in to Stremio',
        signingIn: 'Signing in…',
        key: 'Auth key',
        keyHelp:
          'The key Stremio’s web app keeps once you sign in: in your browser’s console there, JSON.parse(localStorage.getItem("profile")).auth.key gives it. Used to read the account, and kept only if you stay connected. Polyfin never signs it out, which would sign your browser out.',
        readKey: 'Read the account',
        reading: 'Reading…',
      },
      connections: {
        title: 'Connected apps',
        help: (every: string) =>
          `Imported ${every}, the way they were first imported: the same profile, without their addons. Polyfin only reads them.`,
        connectedAs: 'Connected as ',
        connected: 'Connected',
        connectedOn: (date: string) => ` since ${date}`,
        connectedBy: (name: string) => ` by ${name}`,
        status: {
          connected: 'Connected',
          importing: 'Importing',
          paused: 'Paused',
          stopped: 'Sign in again',
        },
        never: 'Not imported yet.',
        lastImport: 'Last import ',
        colon: ': ',
        nextImport: 'Next import ',
        paused:
          'The server no longer lets users import their watch history: this connection is paused until it does.',
        stopped: (app: string) =>
          `${app} refused the sign-in Polyfin kept: nothing is imported until you sign in again below, with Stay connected ticked. Disconnecting forgets it.`,
        problems: {
          unreachable: (app: string) =>
            `${app} did not answer at the last import. The next one tries again.`,
          rate_limited: (app: string) =>
            `${app} asked Polyfin to wait at the last import. The next one tries again.`,
          unexpected: (app: string) =>
            `${app} answered the last import in a way this version of Polyfin does not read. The next one tries again.`,
          profile_missing: (app: string) =>
            `The profile imported is no longer in the ${app} account. Sign in again to choose another.`,
          sign_in_refused: (app: string) => `${app} refused the sign-in Polyfin kept.`,
          internal: () =>
            'Polyfin ran into an error during the last import: the server log tells more. The next one tries again.',
        },
        importNow: 'Import now',
        importing: 'Importing…',
        disconnect: 'Disconnect',
        disconnectTitle: (app: string) => `Disconnect ${app}?`,
        disconnectBody: (app: string) =>
          `Polyfin stops importing this ${app} account and forgets what it kept to read it, after signing out the session it opened. An auth key you pasted is only forgotten. What was imported stays.`,
        disconnected: (app: string) => `${app} disconnected.`,
      },
    },
    statisticsHelp:
      'What you watched on this server: hours, titles, apps and devices, and your recent playbacks.',
    notificationsHelp:
      'Where Polyfin tells you when a new episode of a series you follow is out, when your recordings end, or when your playbacks start and stop, and sends you the summary of your week.',
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
        'Connect the services that keep track of what you watch and listen to. Polyfin tells each connected service the movies and episodes you watch, or the songs you play, in your Jellyfin apps.',
      loading: 'Loading tracking services…',
      status: {
        connected: 'Connected',
        notConnected: 'Not connected',
        waiting: 'Waiting for the code',
        waitingSignIn: 'Waiting for you',
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
      signInIntro: (name: string) =>
        `Connect your ${name} account: sign in on the ${name} site and allow Polyfin to scrobble the songs you play.`,
      tokenIntro: (name: string) =>
        `Connect your ${name} account with your ${name} user token, to have the songs you play scrobbled.`,
      keyLabel: (name: string) => `${name} API key`,
      tokenLabel: (name: string) => `${name} user token`,
      savedKeyHelp: 'Only you can show it. To use another key, disconnect, then connect again.',
      keyHelp: {
        mdblist: 'Find it on mdblist.com, in Preferences, under API key.',
        publicmetadb: 'Find it in your PublicMetaDB account.',
        listenbrainz: 'Find it on listenbrainz.org, in your settings, under User token.',
      },
      connect: 'Connect',
      connecting: 'Connecting…',
      checking: 'Checking the key…',
      reconnect: 'Connect again',
      newCode: 'Get a new code',
      signInAgain: 'Start again',
      enterCode: (site: string) => `Go to ${site}, sign in, and enter this code:`,
      codeLabel: 'Code to enter',
      openSite: (site: string) => `Open ${site}`,
      waiting: 'This page updates by itself once you have entered it.',
      expires: (when: string) => `The code expires ${when}.`,
      codeEnded:
        'The code expired or was refused before it was entered. Get a new code to try again.',
      signInStep: (site: string) =>
        `Go to ${site}, sign in if asked, and allow Polyfin to use your account.`,
      signInWaiting: 'This page updates by itself once you have allowed it.',
      signInExpires: (when: string) => `This request expires ${when}.`,
      signInEnded: 'Polyfin was not allowed in time, or was refused. Start again to try once more.',
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
      invalidToken: (name: string) =>
        `${name} did not accept this token. Copy it again from your ${name} settings.`,
      serviceUnreachable: (name: string) =>
        `${name} could not be reached. Try again in a few minutes.`,
      notAvailable: (name: string) =>
        `${name} is not set up on this server: an administrator has to add the ${name} app first.`,
      appRefused: (name: string) =>
        `${name} refused this server’s app. An administrator has to check its credentials under Settings › Tracking.`,
      history: {
        toggle: (name: string) => `Import my ${name} history`,
        toggleHelp: (name: string, every: string) =>
          `Marks played in Polyfin what you watched on ${name}, and picks up where you stopped, so Continue Watching and Next Up match. Imported ${every}; nothing is sent back to ${name}.`,
        importNow: 'Import now',
        importing: 'Importing…',
        importingStatus: 'Importing your history…',
        never: 'Not imported yet.',
        lastImport: 'Last import',
        colon: ': ',
        nextImport: 'Next import',
        counts: (played: number, resumed: number, unmapped: number) =>
          `${played === 1 ? '1 title' : `${played} titles`} marked played, ${resumed === 1 ? '1 resume point' : `${resumed} resume points`}, ${unmapped === 1 ? '1 title' : `${unmapped} titles`} not found in Polyfin.`,
        problem: {
          reconnect: (name: string) =>
            `${name} refused the connection during the last import. Connect again to import your history.`,
          unreachable: (name: string) =>
            `${name} could not be read whole during the last import. What was read was imported; the next import tries again.`,
          rate_limited: (name: string) =>
            `${name} asked Polyfin to wait during the last import. What was read was imported; the next import tries again.`,
        },
        watchlist: {
          lists: { trakt: 'watchlist', simkl: 'Plan to Watch list' },
          toggle: (name: string, list: string) => `Import my ${name} ${list}`,
          toggleHelp: (name: string, list: string) =>
            `Puts the movies and series of your ${name} ${list} in the Watchlist view of your Jellyfin apps, with each import. A title leaves it once you start it, or once ${name} no longer lists it. Nothing is sent back to ${name}.`,
          counts: (added: number, unmapped: number) =>
            `${added} added to the Watchlist, ${unmapped === 1 ? '1 title' : `${unmapped} titles`} not found in Polyfin.`,
          added: (added: number) => `${added} added to the Watchlist.`,
        },
      },
    },
  },
}

export default account
