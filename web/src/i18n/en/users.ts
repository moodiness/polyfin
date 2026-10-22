/** Users: the user list and each user's permissions. */
const users = {
  users: {
    title: 'Users',
    description: 'Create and manage the accounts allowed to use this server.',
    listTitle: 'Accounts',
    empty: 'No user yet.',
    administrator: 'Administrator',
    hidden: 'Hidden from sign-in screen',
    disabled: 'Disabled',
    you: 'You',
    createTitle: 'Create a user',
    name: 'Name',
    password: 'Password',
    isAdministrator: 'Administrator',
    isAdministratorHelp: 'Administrators can manage users and server settings.',
    showOnSignIn: 'Show on the sign-in screen',
    showOnSignInHelp:
      'Visible users are listed on the sign-in screen of Jellyfin apps. Hidden users type their name, like the Jellyfin default.',
    create: 'Create user',
    creating: 'Creating…',
    created: (name: string) => `${name} has been created.`,
    edit: 'Edit',
    close: 'Close',
    rename: 'Rename',
    renamed: 'The name has been changed.',
    resetPassword: 'Reset password',
    newPassword: 'New password',
    resetPasswordHelp: 'Changing the password signs out all of this user’s Jellyfin devices.',
    passwordReset: 'The password has been reset.',
    accessTitle: 'Access',
    isDisabled: 'Disabled',
    isDisabledHelp: 'A disabled user cannot sign in, and all their devices are signed out.',
    ended: 'Ended',
    endsOn: (date: string) => `Ends on ${date}`,
    endedOn: (date: string) => `Ended on ${date}`,
    endTitle: 'Ends on',
    endHelp:
      'From this date, in the server’s time zone, the account is refused as a disabled one is: it cannot sign in, and its devices are refused. Everything it has stays, and a later date, or Never, brings it back.',
    endAdministratorHelp:
      'An administrator’s account never ends: making a user an administrator clears their end date.',
    endDate: 'End date',
    saveEnd: 'Save date',
    endNever: 'Never',
    canTranscode: 'Can use conversion (transcoding)',
    canTranscodeHelp:
      'Lets the server re-encode video and audio when this user’s app cannot play a file as it is. Without it, files play as they are or simply repackaged.',
    canDownload: 'Can download',
    canDownloadHelp: 'Lets this user save titles in Jellyfin apps to watch offline.',
    canPlayTorrents: 'Can play torrent streams',
    canPlayTorrentsHelp:
      'Offers this user the torrents addons list, while Settings › Torrents plays them. Off for everyone, administrators included, until given.',
    noTranscoding: 'No conversion',
    noDownloads: 'No downloads',
    turnOffDownloads: 'Turn off downloads for everyone',
    turnOffDownloadsConfirm:
      'Every user and every group lose the permission to download, also in the Jellyfin apps already signed in. You can give it back to a user or a group on their page, under Access.',
    turningOffDownloads: 'Turning off…',
    downloadsTurnedOff: (count: number) =>
      count === 0
        ? 'Nobody could download.'
        : count === 1
          ? '1 user can no longer download.'
          : `${count} users can no longer download.`,
    lastAdminHelp: 'The last enabled administrator cannot be deleted, demoted or disabled.',
    updated: 'Changes saved.',
    metadataLanguageTitle: 'Metadata language',
    metadataLanguageHelp:
      'The language of titles’ names, overviews and images in this user’s apps, when one of the server’s addons describes them in it; titles no addon describes in it keep the server’s description. Genres, ratings and people stay the same for everyone, so parental control and blocked genres judge every user alike. The names Polyfin makes itself follow it when Polyfin speaks it: English, French, German, Spanish, Italian or Portuguese.',
    metadataLanguageNone:
      'No addon of the server has a language yet. Give one a language on its page under Content › Sources.',
    parentalTitle: 'Parental control',
    maxRating: 'Maximum rating',
    maxRatingHelp: 'Titles rated above this limit are hidden from this user.',
    noLimit: 'No limit',
    blockUnratedMovies: 'Block unrated movies',
    blockUnratedShows: 'Block unrated shows',
    ratingLimit: (rating: string) => `Up to ${rating}`,
    saveParental: 'Save parental control',
    devicesTitle: 'Devices',
    delete: 'Delete user',
    deleteConfirm: (name: string) =>
      `Delete ${name}? Their devices are signed out and this cannot be undone.`,
    deleting: 'Deleting…',
    deleted: (name: string) => `${name} has been deleted.`,
    canAddAddons: 'Can add their own addons',
    canAddAddonsHelp:
      'Lets this user add Stremio addons of their own on their My sources page. Without it, their addons are kept but not used.',
    noPersonalAddons: 'No own addons',
    blockedUntil: (time: string) => `Blocked until ${time}`,
    blockedHelp:
      'Too many wrong passwords were entered for this account: it cannot sign in until then, even with the right password.',
    unblock: 'Unblock',
    unblocking: 'Unblocking…',
    unblocked: 'The account is unblocked.',
    twoStepOn: 'Two-step sign-in is on.',
    twoStepHelp:
      'If this user lost the phone with their authenticator app and has no recovery code left, turn it off: they sign in with their password alone, then set it up again from My account.',
    turnOffTwoStep: 'Turn off two-step sign-in',
    turningOffTwoStep: 'Turning off…',
    turnOffTwoStepTitle: (name: string) => `Turn off the two-step sign-in of ${name}?`,
    turnOffTwoStepConfirm: (name: string) =>
      `${name} then signs in with their password alone. The activity log records that you turned it off.`,
    twoStepTurnedOff: 'Two-step sign-in is off for this user.',
    playbackAccessTitle: 'Playback and access',
    maxPlaybacks: 'Playbacks at once (0 = no limit)',
    maxPlaybacksHelp:
      'How many of this user’s devices can play at the same time. They can still sign in on more devices.',
    maxBitrate: 'Maximum quality',
    maxBitrateHelp:
      'Above it, the video is converted down when this user can use conversion; otherwise a lighter version plays.',
    bitrateNoLimit: 'No limit',
    bitrate4k: '40 Mbit/s (4K)',
    bitrate1080High: '20 Mbit/s (1080p, high quality)',
    bitrate1080: '10 Mbit/s (1080p)',
    bitrate720: '4 Mbit/s (720p)',
    bitrate480: '2 Mbit/s (480p)',
    bitrateOther: (mbits: string) => `${mbits} Mbit/s`,
    liveTv: 'Live TV',
    liveTvHelp: 'Shows the Live TV channels to this user.',
    syncPlay: 'Watch together',
    syncPlayHelp: 'Playing the same title in step with others, in the Jellyfin apps that offer it.',
    syncPlayCreateAndJoin: 'Create and join groups',
    syncPlayJoin: 'Join only',
    syncPlayNone: 'No',
    remoteControl: 'Can control other users’ apps',
    remoteControlHelp:
      'Lets this user play, pause or send messages to the Jellyfin apps of other users. Everyone can control their own apps.',
    liveTvManagement: 'Can record Live TV',
    liveTvManagementHelp: 'Schedule and delete recordings.',
    savePlaybackAccess: 'Save playback and access',
    visibleLibrariesTitle: 'Visible libraries',
    visibleLibrariesHelp:
      'The server’s libraries this user’s apps show. Libraries added later are shown. This only chooses the libraries shown: titles stay reachable through other libraries, search or links. To block titles, use parental control or blocked genres.',
    noServerLibraries: 'The server has no library yet.',
    saveVisibleLibraries: 'Save visible libraries',
    blockedGenresTitle: 'Blocked genres',
    blockedGenresHelp:
      'Titles of any of these genres are hidden from this user everywhere, like titles above their parental control limit.',
    genre: 'Genre',
    genreHint: 'Pick a genre from the server’s libraries, or type one.',
    addGenre: 'Add',
    removeGenre: (genre: string) => `Remove ${genre}`,
    noBlockedGenres: 'No genre is blocked.',
    saveBlockedGenres: 'Save blocked genres',
    allowedHoursTitle: 'Allowed hours',
    allowedHoursHelp:
      'When hours are set, this user can only sign in and use the server during them, in the server’s time zone. Outside them, their apps are refused.',
    noAllowedHours: 'No hours set: this user can use the server at any time.',
    day: 'Day',
    from: 'From',
    to: 'To',
    addHours: 'Add hours',
    removeHours: 'Remove',
    hoursOrder: 'Each end time must come after its start time.',
    saveAllowedHours: 'Save allowed hours',
    days: {
      Sunday: 'Sunday',
      Monday: 'Monday',
      Tuesday: 'Tuesday',
      Wednesday: 'Wednesday',
      Thursday: 'Thursday',
      Friday: 'Friday',
      Saturday: 'Saturday',
      Everyday: 'Every day',
      Weekday: 'Weekdays (Monday to Friday)',
      Weekend: 'Weekends',
    },
    canManageCollections: 'Can manage collections',
    canManageCollectionsHelp:
      'Lets this user create collections of titles in Jellyfin apps, add titles to them or take titles out, and delete them. Every user sees the collections, each with only the titles they may see.',
    resetPinRequested: 'Password reset requested: PIN',
    resetPinValidUntil: (time: string) => `, valid until ${time}`,
    resetPinHelp:
      'Give this PIN to the user: they sign in with it, and it becomes their new password.',
    canManageSubtitles: 'Can manage subtitles',
    canManageSubtitlesHelp:
      'Lets this user add subtitle files to titles from their Jellyfin apps, and search the addons’ subtitles there.',
    canShare: 'Can share titles',
    canShareHelp:
      'Lets this user give a movie or an episode to someone without an account, through a link that expires, from My account or a title’s page in the web player, while Settings › Security › Allow share links is on. Turning it off revokes their links.',
    shareLinksTitle: 'Shared links',
    shareLinksHelp:
      'The links this user made to share a movie or an episode. Revoke stops one at once.',
    qualityGroup: 'Quality group',
    qualityGroupHelp:
      'The highest resolution this user is offered. Higher versions are left out while one fits; if none fits, they are converted down, or refused when this user cannot use conversion. Live TV is converted down too. Only versions that fit are downloaded as they are; with "Converted downloads" on under Settings › Conversion (transcoding), the apps that ask for it download the others converted down.',
    qualityGroupOriginal: 'Original (no limit)',
    member: 'Member',
    lastSignInLabel: 'Last sign-in:',
    active: 'Active',
    pinRequested: 'PIN requested',
    emptyHelp:
      'Create an account for each person who watches here, then sign in with it from a Jellyfin app.',
    devicesEmptyHelp: 'Apps show here once they sign in with this account.',
    notFound: 'User not found',
    notFoundHelp: 'This user no longer exists, or the address is wrong. Pick a user from the list.',
    backToUsers: 'Back to users',
    sectionsLabel: 'Sections of the user',
    profileTitle: 'Name and password',
    accessHelp: 'Each change is saved at once.',
    noServerLibrariesHelp:
      'Add a library in Content › Libraries, then choose here which ones this user sees.',
    jellyfinImport: {
      open: 'Import from another server',
      title: 'Import from another server',
      description:
        'Bring accounts and their watch data over from a Jellyfin, Emby or Plex server: account names and administrator status, then for each user the movies and episodes played with their dates, resume points and favorites, matched by IMDb, TMDB or TVDB. Polyfin’s data is only added to, and nothing is written to the other server. Servers do not give out passwords: you set one for each new account, or keep the one a user signs in with.',
      serverKind: 'Server',
      servers: {
        jellyfin: 'Jellyfin',
        emby: 'Emby',
        plex: 'Plex',
      },
      serverKindHelp: {
        jellyfin: 'Jellyfin, or a server that speaks its API.',
        emby: 'Emby, with its own API keys and users.',
        plex: 'Plex Media Server, read with its owner’s token.',
      },
      connectTitle: (server: string) => `Connect to ${server}`,
      connectHelp:
        'The address, key and passwords are only used for this import: Polyfin does not save them.',
      address: 'Server address',
      addressHelp: (server: string) =>
        `The address ${server} opens at in a browser. A local network address works.`,
      plexAddressHelp:
        'The address of Plex Media Server itself, usually on port 32400. A local network address works.',
      connectWith: 'Connect with',
      withApiKey: 'An API key',
      withAccount: 'A user account',
      apiKey: 'API key',
      apiKeyHelp:
        'Create one in Jellyfin’s dashboard, under API Keys: it reads every user’s watch data.',
      embyApiKeyHelp:
        'Create one in Emby’s settings, under API Keys: it reads every user’s watch data. A user’s own key is not taken: Emby does not tell whose it is.',
      plexToken: 'Plex token',
      plexTokenHelp:
        'The server owner’s token, which Polyfin only reads the server with. To find it, open Plex in a browser, signed in as the owner. Open the menu of any movie, choose Get Info, then View XML: the address of the page that opens ends with X-Plex-Token= followed by the token. Copy what follows it.',
      accountName: 'User name',
      accountNameHelp: (server: string) =>
        `An administrator’s account lists every user. Polyfin signs in as ${server}’s apps do, then signs out.`,
      accountPassword: 'Password',
      accountPasswordHelp: 'Leave it empty if the account has none.',
      connect: 'Connect',
      connecting: 'Connecting…',
      chooseTitle: 'Choose who to import',
      chooseHelp: (server: string, product: string, version: string) =>
        `${server}, ${product} ${version}. For each user, choose the Polyfin account to import into, or create one.`,
      changeServer: 'Change server',
      noUsers: (product: string) => `This ${product} server has no user.`,
      lastActivityLabel: 'Last activity:',
      importAs: 'Import into',
      skip: 'Do not import',
      newUser: 'New user',
      watchData: 'Import watch data',
      watchDataHelp: 'Played titles with their dates, resume points and favorites.',
      plexOwnerWatchDataHelp: 'Played titles with their dates and play counts, and resume points.',
      plexWatchDataHelp:
        'Played titles with their dates and play counts. Plex does not let Polyfin read this account’s resume points.',
      watchDataNeeded: 'Without it, nothing is imported for this user.',
      plexNotice:
        'With the owner’s token, Plex lets Polyfin read the owner’s played titles and resume points, but only the played titles of the other accounts, from their playback history: their resume points stay on Plex. Plex has no favorites to import.',
      ownerNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Signed in as ${owner ?? 'a user'}, Polyfin reads only that user’s watch data.` : `This key is ${owner === null ? 'a user’s' : `${owner}’s`}: it reads only that user’s watch data.`} To import another user’s, type the password of their account on the server: Polyfin signs in as them to read it. Without it, their account can still be created.`,
      ownerOnlyNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Signed in as ${owner ?? 'this user'}` : `This key belongs to ${owner ?? 'a user'}`}, who is not an administrator of this server: Polyfin imports only this account. To list the server’s other users, connect with an administrator’s account or an API key.`,
      serverPassword: (server: string) => `Password on ${server}`,
      serverPasswordHelp: (user: string) =>
        `Polyfin signs in as ${user} to read their watch data, then signs out. Leave it empty if the account has none.`,
      keepPassword: 'Keep this password in Polyfin',
      keepPasswordHelp:
        'The new account signs in to Polyfin with the same password, if it has at least 8 characters.',
      nothingToImport: 'Choose a user to create, or watch data to import.',
      start: 'Start import',
      starting: 'Starting…',
      started: (created: number, importing: boolean) =>
        created === 0
          ? 'The import has started.'
          : `${created === 1 ? '1 user created.' : `${created} users created.`}${importing ? ' The import has started.' : ''}`,
      currentTitle: 'Import in progress',
      lastTitle: 'Last import',
      startedLabel: 'Started',
      endedLabel: 'Ended',
      startedBy: (name: string, own: boolean) =>
        own ? `${name}’s own watch history, from My account` : `Started by ${name}`,
      runningHelp:
        'You can leave this page: the import goes on. If Polyfin restarts meanwhile, the import stops: what it imported stays, and importing again adds nothing twice.',
      stop: 'Stop import',
      stopping: 'Stopping…',
      states: {
        running: 'Running',
        done: 'Done',
        stopped: 'Stopped',
        failed: 'Failed',
      },
      problems: {
        jellyfin_unreachable: (server: string) =>
          `${server} could not be reached during the import. What was imported is kept.`,
        jellyfin_key_refused: (server: string) =>
          `${server} refused the API key during the import: it may have been revoked. What was imported is kept.`,
        not_jellyfin: (server: string) =>
          `The server stopped answering like ${server} during the import. What was imported is kept.`,
        internal: () =>
          'Polyfin ran into an error during the import: the server log tells more. What was imported is kept.',
        jellyfin_user_forbidden: () =>
          'The server does not let this key read this user’s data. Import it with an API key from the server’s dashboard, or with the user’s own key.',
      },
      /** The errors of connecting that name the server, in Emby's and Plex's words. */
      errors: {
        emby: {
          invalid_jellyfin_address:
            'Enter the address of the Emby server, such as http://192.168.1.10:8096.',
          jellyfin_key_refused:
            'Emby refused this API key. Create one in its settings, under API Keys, and paste it again.',
          jellyfin_sign_in_refused: 'Emby refused this name or password.',
          jellyfin_sign_in_forbidden:
            'Emby does not let this account sign in: it may be disabled, or outside its allowed hours.',
          jellyfin_password_refused:
            'Emby refused this password. Leave it empty if the account has none.',
          jellyfin_unreachable:
            'Nothing answered at this address. Check it, and that Emby is running.',
          not_jellyfin: 'A server answered at this address, but it is not Emby. Check the address.',
        },
        plex: {
          invalid_jellyfin_address:
            'Enter the address of Plex Media Server, such as http://192.168.1.10:32400.',
          jellyfin_key_refused: 'Plex refused this token. Copy the owner’s token again.',
          jellyfin_key_limited:
            'This token may not list the server’s accounts: it is not the server owner’s. Use the owner’s token.',
          jellyfin_unreachable:
            'Nothing answered at this address. Check it, and that Plex Media Server is running.',
          not_jellyfin:
            'A server answered at this address, but it is not Plex Media Server. Check the address.',
        },
      } as Record<'emby' | 'plex', Partial<Record<string, string>>>,
      userStates: {
        waiting: 'Waiting',
        reading: 'Reading',
        saving: 'Saving',
        done: 'Done',
        failed: 'Failed',
      },
      notImported: 'Not imported',
      userMapping: (jellyfinName: string, userName: string) => `${jellyfinName} → ${userName}`,
      read: (count: number) => (count === 1 ? '1 item read' : `${count} items read`),
      counts: (played: number, resumed: number, favorites: number) =>
        `${played} marked played, ${resumed === 1 ? '1 resume point' : `${resumed} resume points`}, ${favorites === 1 ? '1 favorite' : `${favorites} favorites`}`,
      unmatched: (count: number) =>
        count === 1 ? '1 title not found' : `${count} titles not found`,
      unmatchedTitle: (name: string) => `Titles not found for ${name}`,
      unmatchedHelp:
        'These titles were not imported: no Polyfin title matches them by IMDb, TMDB or TVDB.',
      titleColumn: 'Title',
      kindColumn: 'Kind',
      yearColumn: 'Year',
      reasonColumn: 'Reason',
      kinds: {
        movie: 'Movie',
        episode: 'Episode',
        series: 'Series',
      },
      reasons: {
        no_identifier: 'No IMDb, TMDB or TVDB identifier',
        not_found: 'Not in Polyfin',
      },
      more: (count: number) => `And ${count} more.`,
    },
    appImport: {
      open: 'Import from an app',
      title: 'Import from an app',
      description:
        'Bring a user’s watch data over from their account on an app built on addons: the movies and episodes they played with their dates, their resume points, and the app’s library as favorites, matched by IMDb, TMDB or Kitsu. Polyfin’s data is only added to, and nothing is written to the app. You need the user’s sign-in to the app; users can also import themselves, under My account.',
      user: 'Into the account of',
      userHelp: 'The Polyfin user whose watch data and addons the import adds to.',
    },
    letterboxdImport: {
      open: 'Import from Letterboxd',
      title: 'Import from Letterboxd',
      description:
        'Bring a user’s movies over from their Letterboxd export file: the movies they watched with their dates and play counts, their watchlist, their ratings, and the movies they liked as favorites if you choose. Polyfin’s data is only added to. Users can also import their own, under My account.',
      user: 'Into the account of',
      userHelp: 'The Polyfin user whose watch data the import adds to.',
    },
  },
  invites: {
    title: 'Invite links',
    create: 'Create an invite link',
    empty: 'No invite link yet.',
    emptyHelp:
      'An invite link lets the people you send it to create their own account, with the settings you choose.',
    maxUses: 'Accounts it may create',
    maxUsesHelp: 'From 1 to 100. 1 by default.',
    expires: 'Expires',
    after: (days: number) => (days === 1 ? 'After 1 day' : `After ${days} days`),
    never: 'Never',
    accountsEnd: 'Accounts end',
    accountsEndHelp: 'Each account the link creates ends this many days after it is created.',
    accountsEndAfter: (days: number) =>
      days === 1 ? 'Accounts end after 1 day' : `Accounts end after ${days} days`,
    model: 'Settings of',
    modelHelp:
      'New accounts copy this user’s permissions, visible libraries, blocked genres, allowed hours, parental control, playback and access limits and quality group. They are never administrators.',
    newUser: 'A new user, as with Create a user',
    submit: 'Create link',
    creating: 'Creating…',
    createdTitle: 'Invite link created',
    createdNotice: 'Copy this link now: it is shown only this once.',
    copy: 'Copy link',
    copied: 'Link copied.',
    copyFailed: 'The link could not be copied. Select it and copy it by hand.',
    done: 'Done',
    uses: (uses: number, maxUses: number) =>
      maxUses === 1 ? `${uses} of 1 account created` : `${uses} of ${maxUses} accounts created`,
    settingsOf: (name: string) => `Settings of ${name}`,
    newUserSettings: 'New user settings',
    expiresLabel: 'Expires',
    expiredLabel: 'Expired',
    neverExpires: 'Never expires',
    createdBy: (name: string) => `Created by ${name}`,
    createdByDeleted: 'Created by a deleted user',
    states: {
      active: 'Active',
      used_up: 'Used up',
      expired: 'Expired',
      revoked: 'Revoked',
    },
    revoke: 'Revoke',
    revokeTitle: 'Revoke this invite link?',
    revokeConfirm: 'The link stops working at once. Accounts already created through it are kept.',
    revoking: 'Revoking…',
    revoked: 'The invite link is revoked.',
  },
  groups: {
    tabsLabel: 'Users and groups',
    title: 'Groups',
    description:
      'A group holds the limits of a user’s page. Its members follow it: a change to the group reaches every one of them at once.',
    listTitle: 'Groups',
    empty: 'No group yet.',
    emptyHelp:
      'Create a group to set the permissions, playback limits, parental control, libraries, genres and hours of several accounts at once.',
    create: 'Create a group',
    name: 'Name',
    nameTaken: 'This name is already used by another group.',
    nameRule: '1 to 64 characters, different from the other groups’ names.',
    copyFrom: 'Settings of',
    copyFromHelp:
      'The group starts with this user’s permissions, playback and access limits, quality group, parental control, visible libraries, blocked genres and allowed hours.',
    submit: 'Create group',
    creating: 'Creating…',
    created: (name: string) => `The group ${name} has been created.`,
    memberCount: (count: number) =>
      count === 0 ? 'No member' : count === 1 ? '1 member' : `${count} members`,
    notFound: 'Group not found',
    notFoundHelp: 'It may have been deleted.',
    backToGroups: 'Back to groups',
    sectionsLabel: 'Sections of the group',
    pageHelp: 'Each section saves on its own and reaches the members who follow the group at once.',
    rename: 'Rename',
    renamed: 'The group has been renamed.',
    updated: 'The group has been updated.',
    accessHelp:
      'The permissions members take from the group. Their role, sign-in screen, Disabled and end date stay on their own page.',
    delete: 'Delete group',
    deleteTitle: (name: string) => `Delete the group ${name}?`,
    deleteConfirm:
      'Its members keep the group’s values as their own settings, which their pages can then change.',
    deleting: 'Deleting…',
    deleted: (name: string) => `The group ${name} has been deleted.`,
    membersTitle: 'Members',
    membersHelp:
      'A member who no longer follows the group keeps its values as their own settings, marked Own settings.',
    noMembers: 'No member yet.',
    noMembersHelp: 'Add members here, or choose the group in the Access section of a user’s page.',
    ownSettings: 'Own settings',
    followsHelp: 'Follows the group',
    ownSettingsHelp: 'Keeps their own settings',
    addMembers: 'Add members',
    addMembersTitle: (name: string) => `Add members to ${name}`,
    addMembersHelp:
      'The users chosen follow the group: they take its values in place of their own. Administrators cannot join a group.',
    noCandidates: 'No one to add.',
    noCandidatesHelp: 'Every user who is not an administrator is already in this group.',
    movesFrom: (name: string) => `Leaves the group ${name}`,
    add: 'Add',
    adding: 'Adding…',
    membersAdded: (count: number) =>
      count === 1 ? '1 member has been added.' : `${count} members have been added.`,
    group: 'Group',
    noGroup: 'No group',
    groupHelp:
      'A member follows the group: its values apply, and this page shows them read only. Leaving it keeps them as the user’s own.',
    administratorNoGroup: 'Administrators cannot join a group, as their permissions differ.',
    administratorLeaves: (name: string) =>
      `Making this user an administrator takes them out of the group ${name}.`,
    newAccountGroupHelp: 'The accounts created join this group and follow it.',
    newUserGroupHelp: 'The user joins this group and follows it.',
    followGroup: 'Follow the group',
    followGroupHelp:
      'Turned off, the user keeps the group’s current values as their own, and this page can change them.',
    setByGroup: (name: string) => `Set by the group ${name}.`,
    openGroup: 'Open the group',
    joinTitle: 'Join the group?',
    joinConfirm: (user: string, group: string) =>
      `${user} joins ${group} and follows it: the group’s values replace theirs in Access, Playback and access, Parental control, Visible libraries, Blocked genres and Allowed hours.`,
    join: 'Join',
    followAgainTitle: 'Follow the group again?',
    followAgainConfirm: (user: string, group: string) =>
      `${user} takes the values of ${group} back: their own settings in the group’s sections are replaced.`,
    followAgain: 'Follow the group',
    groupBadge: (name: string) => `Group ${name}`,
    groupOwnBadge: (name: string) => `Group ${name}, own settings`,
    filter: 'Show the users of',
    allGroups: 'All groups',
    noneShown: 'No user here.',
  },
}

export default users
