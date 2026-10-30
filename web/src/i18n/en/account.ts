/** My account: password, devices and tracking services. */
const account = {
  account: {
    title: 'My account',
    signedInAs: 'Signed in as ',
    sectionsLabel: 'Account sections',
    sections: {
      statistics: 'Statistics',
      tracking: 'Tracking',
      resume: 'Continue Watching',
      hidden: 'Hidden titles',
      homeRows: 'Home rows',
      metadataLanguage: 'Metadata language',
      webTheme: 'Web player theme',
      sharedLinks: 'Shared links',
      reminders: 'Reminders',
      import: 'Import from another server',
      appImport: 'Import from an app',
      notifications: 'Notifications',
      devices: 'Devices',
      password: 'Password',
      twoStep: 'Two-step sign-in',
    },
    /** My account › Metadata language. */
    metadataLanguage: {
      help: 'The language of titles’ names, overviews and images in your apps, when one of the server’s addons describes them in it; titles no addon describes in it keep the server’s description. Genres, ratings and people stay the same for everyone. The names Polyfin makes itself, such as seasons, follow it when Polyfin speaks it: English, French, German, Spanish, Italian or Portuguese.',
      label: 'Metadata language',
      server: 'The server’s',
      noLanguage:
        'No addon of the server has a language yet, so titles are described in the server’s. An administrator gives addons a language under Content › Sources.',
      saved: 'Metadata language saved.',
    },
    /** My account › Web player theme, and the choice of a user's on their page under Users. */
    webTheme: {
      help: 'The web player’s look on every browser you sign in with. jellyfin-web’s own look is lighter, for TVs and older phones; browsers too old for LumaaGlaass get it by themselves. A browser shows a change once the web player is opened again, or right after you sign in there.',
      label: 'Web player theme',
      server: (theme: string) => `The server’s (${theme})`,
      choices: { lumaaglaass: 'LumaaGlaass', none: 'jellyfin-web’s own look', custom: 'Custom' },
      locked: 'Your administrator chose your web player theme.',
      saved: 'Web player theme saved.',
    },
    /** My account › Home rows. */
    homeRows: {
      help: 'The rows the administrator adds to the web player’s home page for you. Turn one off to leave it out of your home page, and out of your libraries in other apps when it is one there too. The web player’s own sections stay in its Settings › Home.',
      empty: 'No home row',
      emptyHelp: 'The administrator added no row to the web player’s home page for you.',
      shown: (name: string) => `Show ${name} on your home page`,
      on: (name: string) => `${name} is back on your home page.`,
      off: (name: string) => `${name} is off your home page.`,
    },
    /** My account › Continue Watching and Hidden titles. */
    watching: {
      resumeHelp:
        'The titles under way in your Continue Watching, for the apps that cannot take one off. Remove takes a title off without changing whether it is played. An import from a tracking service or an app brings it back only with a more recent resume point.',
      hiddenHelp:
        'The titles you hid from your libraries, home rows, searches and similar titles. They still open from a link. In the web player, a title’s ⋮ menu hides it.',
      remove: 'Remove',
      removeLabel: (name: string) => `Remove ${name} from Continue Watching`,
      removed: (name: string) => `${name} is off Continue Watching.`,
      resumeEmpty: 'Nothing under way',
      resumeEmptyHelp: 'The movies and episodes you start show here until you finish them.',
      progress: (position: string, runtime: string) => `${position} of ${runtime}`,
      showAgain: 'Show again',
      showLabel: (name: string) => `Show ${name} again`,
      shown: (name: string) => `${name} is back in your lists.`,
      hiddenEmpty: 'No hidden title',
      hiddenEmptyHelp: 'Hide a title from its ⋮ menu in the web player.',
      hiddenOn: 'Hidden',
      kinds: {
        movie: 'Movie',
        series: 'Series',
        episode: 'Episode',
        video: 'Home video',
        replay: 'Replay',
      } as Record<string, string>,
    },
    /** My account › Reminders, and the reminder settings under Notifications. */
    reminders: {
      help: 'The Live TV programmes you asked to be reminded of from their page in the web player. Each reminder comes on your targets that chose Programme starting, as long before as you set under Notifications.',
      empty: 'No reminder',
      emptyHelp: 'On the page of a programme to come in the web player, Remind me sets one.',
      everyAiring: (time: string) => `Every airing at about ${time}`,
      next: 'Next ',
      noAiring: 'No airing in the guide',
      delete: 'Delete',
      deleteLabel: (name: string) => `Delete the reminder of ${name}`,
      deleted: (name: string) => `The reminder of ${name} is deleted.`,
      settingsTitle: 'Reminders',
      before: 'Remind me before',
      beforeHelp: 'How long before a programme starts its reminder comes.',
      minutes: (minutes: number) =>
        minutes === 1 ? '1 minute' : minutes === 60 ? '1 hour' : `${minutes} minutes`,
      onApps: 'Also show reminders on my apps',
      onAppsHelp:
        'Each reminder also shows on your Jellyfin apps that accept messages, such as the web player while it is open, as Send a message shows one.',
      noTarget:
        'None of your targets chose Programme starting, and your apps are off: your reminders would reach you nowhere.',
      saved: 'Reminder settings saved.',
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
        'Bring your played movies and episodes, resume points and favorites over from a Jellyfin, Emby or Plex server you used before, or your movies from your Letterboxd export. They go into your account only, and only add to what Polyfin already has. Nothing is written to the other server.',
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
    /** My account › Import from another server, from Plex, signing in to Plex. */
    plexImport: {
      help: 'Sign in to Plex on Plex’s own page, then choose the server. Polyfin reads there, as you, the movies and episodes you played, marked as watched or synced, your resume points and your ratings, and your Plex Watchlist. Polyfin never sees your Plex password: what Plex gives it is used for this import only, then signed out.',
      signIn: 'Sign in with Plex',
      signingIn: 'Asking Plex…',
      approve: 'Sign in to Plex on its page, which opens in a new tab, then come back here.',
      open: 'Open Plex’s sign-in page',
      waiting: 'Waiting for you to sign in to Plex…',
      signedInAs: (name: string) => `Signed in to Plex as ${name}.`,
      profile: 'Plex Home profile',
      profileHelp:
        'The profile whose watch data comes over. A managed profile has no sign-in of its own: choose it here.',
      managed: (name: string) => `${name} (managed)`,
      pin: 'Profile PIN',
      pinHelp: 'The PIN this profile asks for on Plex.',
      useProfile: 'Use this profile',
      switching: 'Switching…',
      server: 'Server',
      serverHelp:
        'The Plex servers this profile reaches. Polyfin reaches the one chosen at the addresses Plex gives for it.',
      owned: (name: string) => `${name} (yours)`,
      noServers: 'This profile reaches no Plex server.',
      watchlist: 'Watchlist to my Watchlist',
      watchlistHelp: 'The movies and shows of your Plex Watchlist go to your Watchlist in Polyfin.',
      cancel: 'Cancel',
    },
    /** My account › Import from another server, with Letterboxd's export file. */
    letterboxd: {
      option: 'Letterboxd (export file)',
      help: 'In Letterboxd, open Settings › Import & Export and choose Export your data: Letterboxd gives a ZIP file. Choose it here as it is. Polyfin reads it in memory, and never keeps it.',
      watchedHelp:
        'The movies you watched are marked played, on the latest date your diary gives, as many times as it logs them. Each movie is searched by its name and year in the server’s metadata addons, and kept when they know exactly one movie of that name and year.',
      noSearch:
        'No metadata addon of this server searches movies: no movie of the export can be found. Ask an administrator to add one.',
      file: 'Export file',
      fileHelp: 'The ZIP file Letterboxd gives, of 20 MB at most.',
      choose: 'Choose the file',
      chooseAnother: 'Choose another file',
      tooLarge: 'This file is larger than 20 MB: choose the ZIP file Letterboxd gives.',
      watchlist: 'Watchlist to my Watchlist',
      watchlistHelp:
        'The movies of your Letterboxd watchlist go to your Watchlist in Jellyfin apps, until you start them.',
      ratings: 'Ratings',
      ratingsHelp:
        'Your stars become your rating out of 10: 3.5 stars is 7. A rating you already have in Polyfin stays.',
      likes: 'Liked movies as favorites',
      likesHelp: 'The movies you liked on Letterboxd become your favorites.',
      /** The same, on an administrator's page importing into a user's account. */
      forUser: {
        watchedHelp:
          'The movies they watched are marked played, on the latest date their diary gives, as many times as it logs them. Each movie is searched by its name and year in the server’s metadata addons, and kept when they know exactly one movie of that name and year.',
        watchlist: 'Watchlist to their Watchlist',
        watchlistHelp:
          'The movies of their Letterboxd watchlist go to their Watchlist in Jellyfin apps, until they start them.',
        ratingsHelp:
          'Their stars become their rating out of 10: 3.5 stars is 7. A rating they already have in Polyfin stays.',
        likesHelp: 'The movies they liked on Letterboxd become their favorites.',
      },
      start: 'Import from Letterboxd',
      starting: 'Sending…',
      started: 'The import has started. You can leave this page: it goes on.',
      currentTitle: 'Import in progress',
      lastTitle: 'Last import',
      ownLastTitle: 'Your last import',
      from: 'From a Letterboxd export',
      startedLabel: 'Started',
      endedLabel: 'Ended',
      startedBy: (name: string) => `Started by ${name}`,
      states: { running: 'Running', done: 'Done', failed: 'Failed' },
      problem:
        'Polyfin ran into an error during the import: the server log tells more. What was imported is kept.',
      runningHelp:
        'You can leave this page: the import goes on. If Polyfin restarts meanwhile, the import stops: what it imported stays, and importing again adds nothing twice.',
      searching: (searched: number, movies: number) =>
        `Searching the movies: ${searched} of ${movies}.`,
      searchingLabel: 'Movies searched',
      summary: (movies: number, added: string) =>
        `${movies === 1 ? '1 movie' : `${movies} movies`}: ${added}.`,
      played: (count: number) => `${count} marked played`,
      addedToWatchlist: (count: number) => `${count} added to the Watchlist`,
      rated: (count: number) => `${count} rated`,
      favorites: (count: number) => `${count} made favorites`,
      unmatched: (count: number) =>
        count === 1 ? '1 movie not found' : `${count} movies not found`,
      unmatchedTitle: 'Movies not found',
      unmatchedHelp:
        'These movies were not imported. Each one is searched by its name and year, and kept only when the server’s metadata addons know exactly one movie of that name and year. Open it on Letterboxd to check them.',
      movieColumn: 'Movie',
      reasonColumn: 'Reason',
      onLetterboxd: 'On Letterboxd',
      reasons: {
        not_found: 'Not found',
        ambiguous: 'Several movies of that name and year',
        search_failed: 'Search failed',
      },
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
          'Stremio keeps a history for each profile, or one for the whole account without profiles: choose the one to import. Polyfin keeps what it read for a few minutes only.',
      },
      stayConnected: 'Stay connected',
      stayConnectedHelp: {
        nuvio: (every: string) =>
          `Polyfin keeps the session, encrypted, never your password, and imports the profile you choose again ${every}. It only reads the account. You can disconnect at any time.`,
        stremio: (every: string) =>
          `Polyfin keeps the auth key, encrypted, and for a profile with a PIN that profile’s session, never the PIN, and imports the profile you choose again ${every}. It only reads the account. You can disconnect at any time. Stremio’s terms forbid extracting its data by automated means: reading your account ${every} makes that grey area more pronounced than a one-off import.`,
      },
      staysConnected: (every: string) =>
        `You stay connected: this profile is imported again ${every}, the same way. Addons are installed now only.`,
      wholeAccount: 'The whole account',
      profile: 'Profile',
      profileNumber: (id: string) => `Profile ${id}`,
      profileCounts: (played: number, resumes: number, library: number) =>
        `${played} watched, ${resumes === 1 ? '1 resume point' : `${resumes} resume points`}, ${library} in the library.`,
      lockedProfile: (name: string) => `${name} (PIN)`,
      pin: 'PIN',
      pinHelp:
        'This profile asks for its PIN. Polyfin sends it to Stremio only, to open the profile, and never keeps it.',
      unlock: 'Open this profile',
      unlocking: 'Opening…',
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
          profile_locked: (app: string) =>
            `The profile imported now asks for a PIN, or another one, in ${app}. Sign in again to choose it with its PIN.`,
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
    sharedLinksHelp:
      'Share a movie or an episode with someone who has no account: they watch it in their browser through a link that expires, and count among your playbacks at once.',
    statisticsHelp:
      'What you watched on this server: hours, titles, apps and devices, and your recent playbacks.',
    notificationsHelp:
      'Where Polyfin tells you of new episodes of the series you follow, your recordings and playbacks, your connections that stop, your account’s blocks and its sign-ins on new devices, reminds you of the programmes you chose, and sends you your weekly summary and your month and year in review.',
    devicesHelp:
      'The apps signed in with your account. A device you sign out asks for your password the next time it opens.',
    passwordHelp: 'Changing your password signs out all your Jellyfin devices.',
    currentPassword: 'Current password',
    newPassword: 'New password',
    confirmPassword: 'Confirm new password',
    changePassword: 'Change password',
    changingPassword: 'Changing…',
    passwordChanged: 'Your password has been changed.',
    /** My account › Two-step sign-in, and its steps at sign-in. */
    twoStep: {
      help: 'After your password, this admin app asks for a code from an authenticator app on your phone. Jellyfin apps cannot ask for one: once it is on, they sign in with Quick Connect, which you approve here, and your password alone no longer signs in to them. The apps and browsers signed in already stay signed in.',
      off: 'Off',
      on: 'On',
      offText: 'Your password alone signs you in.',
      onText: 'You sign in with your password, then a code from your authenticator app.',
      required:
        'Administrators must use two-step sign-in on this server. Turn it on now, or you will be asked to at your next sign-in.',
      requiredOn: 'Administrators must use two-step sign-in on this server: it stays on.',
      unreadable:
        'POLYFIN_SECRET_KEY can no longer read the key of your authenticator app, as after a restore under another key: only your recovery codes sign you in. Turn two-step sign-in off with your password, then on again.',
      turnOn: 'Turn on',
      starting: 'Preparing…',
      scan: 'Scan this QR code with your authenticator app, or enter the key by hand. Polyfin made the code itself: the key goes nowhere else.',
      qrLabel: 'QR code of the key, for your authenticator app',
      key: 'Key',
      openInApp: 'Open in an authenticator app on this device',
      code: 'Code from the app',
      codeHelp: 'The 6-digit code the app shows for Polyfin. It changes every 30 seconds.',
      confirm: 'Turn on',
      confirming: 'Checking…',
      turnedOn: 'Two-step sign-in is on.',
      recoveryTitle: 'Your recovery codes',
      recoveryHelp:
        'Each code signs you in once, in place of a code from your app, should you lose your phone. Keep them somewhere safe, such as a password manager: they are shown only now.',
      copy: 'Copy the codes',
      copied: 'The codes are copied.',
      copyFailed: 'The codes could not be copied: select them and copy them by hand.',
      saved: 'I saved them',
      codesLeft: (n: number) => (n === 1 ? '1 recovery code left' : `${n} recovery codes left`),
      newCodes: 'New recovery codes',
      newCodesTitle: 'Make new recovery codes?',
      newCodesText: 'The new codes void the former ones.',
      making: 'Making…',
      turnOff: 'Turn off',
      turnOffTitle: 'Turn two-step sign-in off?',
      turnOffText: 'Your password alone then signs you in, here and in Jellyfin apps.',
      turningOff: 'Turning off…',
      turnedOff: 'Two-step sign-in is off.',
      confirmWith: 'Confirm with',
      withCode: 'A code',
      withPassword: 'My password',
      codeOrRecovery: 'Code from the app, or a recovery code',
      password: 'Password',
    },
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
      pinIntro: (name: string) =>
        `Connect your ${name} lists: allow Polyfin on the ${name} site, then paste here the code it shows you.`,
      redirectIntro: (name: string) =>
        `Connect your ${name} list: allow Polyfin on the ${name} site, which brings you back here.`,
      pinStep: (site: string) =>
        `Go to ${site}, sign in if asked, allow Polyfin, then copy the code it shows you here.`,
      redirectStep: (site: string) =>
        `Go to ${site}, sign in if asked, and allow Polyfin: it brings you back here once done.`,
      codeField: (name: string) => `${name} code`,
      codeFieldHelp: (name: string) => `The code ${name} shows once you allow Polyfin.`,
      addressField: 'Address or code',
      addressFieldHelp: (name: string) =>
        `If your browser could not open the page ${name} sent it to, copy that page’s address from the address bar here.`,
      pasteEnded: 'This request expired before Polyfin was allowed. Start again to try once more.',
      invalidCode: (name: string) =>
        `${name} did not accept this code, or it is for an earlier request. Start again to get a new one.`,
      tokenExpires: (date: string) => `, until ${date}`,
      returning: (name: string) => `Connecting your ${name} account…`,
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
        listsToggle: (name: string) => `Import my ${name} lists`,
        listsToggleHelp: (name: string, every: string) =>
          `Marks played in Polyfin the episodes your ${name} lists count watched: up to each entry’s progress for Watching, On hold and Dropped, all of them for Completed. Imported ${every}; it only adds.`,
        listCounts: (played: number, unmapped: number) =>
          `${played === 1 ? '1 title' : `${played} titles`} marked played, ${unmapped === 1 ? '1 title' : `${unmapped} titles`} not found in Polyfin.`,
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
          lists: {
            trakt: 'watchlist',
            simkl: 'Plan to Watch list',
            anilist: 'Plan to Watch list',
            myanimelist: 'Plan to Watch list',
          },
          toggle: (name: string, list: string) => `Import my ${name} ${list}`,
          toggleHelp: (name: string, list: string) =>
            `Puts the movies and series of your ${name} ${list} in the Watchlist view of your Jellyfin apps, with each import. A title leaves it once you start it, or once ${name} no longer lists it. Nothing is sent back to ${name}.`,
          counts: (added: number, unmapped: number) =>
            `${added} added to the Watchlist, ${unmapped === 1 ? '1 title' : `${unmapped} titles`} not found in Polyfin.`,
          added: (added: number) => `${added} added to the Watchlist.`,
        },
        ratings: {
          toggle: 'Sync my ratings',
          toggleHelp: (name: string) =>
            `Sends the ratings you give movies and series to ${name} as your scores, and gives your unrated titles your ${name} scores with each import. A series ${name} splits into several entries is skipped.`,
          counts: (rated: number, skipped: number) =>
            `${rated === 1 ? '1 title' : `${rated} titles`} rated, ${skipped === 1 ? '1 rating' : `${skipped} ratings`} skipped.`,
        },
      },
    },
  },
  /** My account › Shared links, and the Shared links of a user's page. */
  shareLinks: {
    share: 'Share a title',
    off: 'You cannot make share links now: an administrator turned them off for this server or for your account. Links still active do not play meanwhile.',
    empty: 'No shared link',
    emptyHelp:
      'Share a title gives a movie or an episode through a link. In the web player, a movie’s or an episode’s page has Share too.',
    userEmpty: 'This user has no shared link.',
    expiresLabel: 'Expires',
    expiredLabel: 'Expired',
    revokedLabel: 'Revoked',
    plays: (count: number) => (count === 1 ? '1 play' : `${count} plays`),
    viewers: (count: number) => (count === 1 ? '1 viewer at once' : `${count} viewers at once`),
    states: { active: 'Active', expired: 'Expired', revoked: 'Revoked' },
    revoke: 'Revoke',
    revokeLabel: (title: string) => `Revoke the link to ${title}`,
    revokeTitle: 'Revoke this shared link?',
    revokeConfirm:
      'The link stops working at once, and whoever watches through it stops within seconds.',
    revoking: 'Revoking…',
    revoked: 'The shared link is revoked.',
    search: 'Search your libraries',
    searchHelp: 'A movie, or a series to pick one of its episodes.',
    searchButton: 'Search',
    searching: 'Searching…',
    noTitles: 'No movie or series found.',
    titlesFound: (term: string) => `Titles found for ${term}`,
    pickTitle: (name: string, details: string) => `Share ${name}, ${details}`,
    episodesOf: (series: string) => `Episodes of ${series}`,
    noEpisodes: 'This series has no episode you can share.',
    back: 'Back to the results',
    chosen: 'Title',
    change: 'Change',
    expires: 'Expires after',
    expiresHelp: 'Nobody can open the link after that. You can revoke it sooner.',
    after: (hours: number) =>
      hours === 1 ? '1 hour' : hours === 24 ? '1 day' : `${hours / 24} days`,
    viewersLabel: 'Viewers at once',
    viewersHelp:
      'How many people may watch through the link at the same time. Each counts as one of your playbacks at once.',
    submit: 'Create link',
    creating: 'Creating…',
    createdTitle: 'Link created',
    createdNotice: (title: string) =>
      `Anyone with this link can watch ${title} in a browser, without an account, until it expires. Copy it now: it is shown this once.`,
    copy: 'Copy the link',
    copied: 'Link copied.',
    copyFailed: 'The link could not be copied. Select it and copy it by hand.',
    done: 'Done',
  },
  resetPreferences: {
    button: 'Reset my preferences',
    title: 'Reset your preferences?',
    help: 'What you chose yourself goes back to the server’s defaults:',
    metadataLanguage: 'Metadata language: The server’s',
    webTheme: 'Web player theme: The server’s, unless your administrator chose yours',
    homeRows: 'Home rows: every row is shown again',
    appPreferences:
      'The preferences Jellyfin apps saved in your account: audio and subtitle languages and mode, home screen and display preferences',
    kept: 'What you watched, your favorites, your resume points, your connections and imports stay.',
    confirm: 'Reset',
    resetting: 'Resetting…',
    done: 'Your preferences have been reset.',
  },
}

export default account
