/** The bounds and default of a setting, as its help text shows them, numbers already written. */
export type RangeText = { min: string; max: string; default: string }

/** Settings: the server-wide settings and their sections. */
const settings = {
  settings: {
    tracking: {
      description:
        'Each user can connect their own Trakt, Simkl, MDBList, PublicMetaDB, AniList and MyAnimeList accounts on their My account page, and Polyfin tells those services what they watch; Last.fm and ListenBrainz hear of the songs they play. Trakt, Simkl, AniList, MyAnimeList and Last.fm first need an app of this server’s, set up here. MDBList, PublicMetaDB and ListenBrainz need nothing.',
      traktSetup:
        'Create an app on trakt.tv, in Settings, Your API Apps, New application. Give it any name, then copy its client ID and client secret here.',
      redirectUri: 'Redirect URI to enter in the app',
      traktClientId: 'Trakt client ID',
      traktClientIdHelp:
        'Leave empty to turn Trakt off. Users can connect once both the client ID and the client secret are saved.',
      traktClientSecret: 'Trakt client secret',
      traktClientSecretHelp: 'Shown on the app’s page on trakt.tv, under the client ID.',
      simklSetup:
        'Create an app on simkl.com, in Settings, Developer, Create new app, of the type “TV, devices & command line” (Simkl’s AUTH V2; it needs no redirect URI, and client IDs of older AUTH V1 apps are refused). Give it any name, then copy its client ID here.',
      simklClientId: 'Simkl client ID',
      simklClientIdHelp: 'Leave empty to turn Simkl off. No client secret is needed.',
      lastFmSetup:
        'Create an API account on last.fm, at last.fm/api/account/create. Give it any name and leave its callback URL empty: users allow Polyfin on Last.fm, and Polyfin asks Last.fm for their session itself. Then copy its API key and shared secret here.',
      lastFmApiKey: 'Last.fm API key',
      lastFmApiKeyHelp:
        'Leave empty to turn Last.fm off. Users can connect once both the API key and the shared secret are saved.',
      lastFmSecret: 'Last.fm shared secret',
      lastFmSecretHelp: 'Shown on your API account’s page on last.fm, under the API key.',
      aniListSetup:
        'Create an app on anilist.co, in Settings, Developer, Create New Client. Give it any name, and as its redirect URL the address below, where AniList shows users the code they paste in Polyfin. Then copy its client ID and client secret here.',
      aniListClientId: 'AniList client ID',
      aniListClientIdHelp:
        'Leave empty to turn AniList off. Users can connect once both the client ID and the client secret are saved.',
      aniListClientSecret: 'AniList client secret',
      aniListClientSecretHelp: 'Shown on the app’s page on anilist.co, under the client ID.',
      myAnimeListSetup:
        'Create an app on myanimelist.net, in Account Settings, API, Create ID, of the type “web”. Give it any name, and as its redirect URL the address below: MyAnimeList sends users’ browsers back there once they allow Polyfin. Only their browsers need to reach it. Then copy its client ID and client secret here.',
      myAnimeListRedirectHelp:
        'This server’s public address under Settings › General, else the address you use now.',
      myAnimeListClientId: 'MyAnimeList client ID',
      myAnimeListClientIdHelp:
        'Leave empty to turn MyAnimeList off. Users can connect once both the client ID and the client secret are saved.',
      myAnimeListClientSecret: 'MyAnimeList client secret',
      myAnimeListClientSecretHelp:
        'Shown on the app’s page on myanimelist.net, under the client ID.',
    },
    title: 'Settings',
    description: 'Options that apply to the whole server.',
    serverName: 'Server name',
    serverNameHelp: (r: RangeText) => `${r.min} to ${r.max} characters, shown in Jellyfin apps.`,
    quickConnect: 'Allow Quick Connect',
    quickConnectHelp:
      'Lets TV and phone apps sign in with a code approved here: 6 digits from Jellyfin apps, 8 from Silo’s TV apps while Silo’s apps are allowed.',
    language: 'Language of generated names',
    languageHelp:
      'Polyfin names some things itself in Jellyfin apps: seasons (“Season 1”, “Specials”), episodes without a title, and the type added to libraries that share a name (“Popular (Movies)”).',
    legacyAuthorization: 'Allow legacy authorization',
    legacyAuthorizationHelp:
      'Accepts the old X-Emby-* headers, the api_key parameter and the Emby scheme for apps that still need them. Off by default, like Jellyfin 12.4.',
    siloApps: 'Allow Silo’s apps',
    siloAppsHelp:
      'Lets Silo’s apps for iPhone, iPad, Apple TV, Mac and Android sign in and play, under Silo’s API at /api/v2, as Jellyfin apps do, within each user’s limits. Off, that API answers nothing.',
    updateCheck: 'Check for new versions',
    updateCheckHelp:
      'Once a day, and shortly after startup, Polyfin asks GitHub for its latest release, sending only its version. A newer one shows on Home and under System › Health, and is sent to the New version notifications. Off, Polyfin never contacts GitHub.',
    restartConfirmed: 'Polyfin is started again when it stops',
    restartConfirmedHelp:
      'Turn on once Polyfin’s container has a restart policy that starts it again, such as unless-stopped, always or on-failure, or once its service manager does. Polyfin cannot see its container’s restart policy without the Docker socket, which it is never given: while this is off, Restart on Home warns that Polyfin may not come back.',
    updateToolUrl: 'Update tool address',
    updateToolUrlHelp:
      'The update API of a tool you run beside Polyfin to update its container, such as http://updater:8080/v1/update. Update now, on Home, sends it a POST request without a body. Empty hides Update now.',
    updateToolToken: 'Update tool token',
    updateToolTokenHelp:
      'Sent as an Authorization: Bearer header with each request, when the tool asks for one. Stored encrypted, as the other secrets.',
    localDiscovery: 'Answer local network discovery',
    localDiscoveryHelp:
      'Jellyfin and Emby apps that look for servers on the local network find Polyfin and list it, so its address does not have to be typed. They ask on UDP port 7359, and Polyfin answers only addresses of the local network. Applies at once, without a restart. In Docker, this needs host networking or the published port 7359/udp: see the installation docs.',
    localAddress: 'Address on the local network',
    localAddressHelp:
      'The address apps on the local network reach Polyfin at, such as http://192.168.1.10:8096. Discovery answers give it, and Jellyfin apps read it as the server’s local address. Empty, the answers give the address of the network interface that reaches the app, which is right unless Polyfin runs in a Docker bridge network: there, enter the host’s address and Polyfin’s port.',
    torrents: {
      description:
        'Some addons list torrents rather than links. Polyfin can play them: it joins a torrent’s swarm only while a playback reads it, and keeps all of its traffic to one route, a network interface such as a VPN’s, a SOCKS5 proxy, or a WireGuard tunnel Polyfin runs itself. Nothing is ever sent another way. The pieces are kept within Disk space for files being read, under Settings › Playback.',
      play: 'Play torrent streams',
      playHelp:
        'Off by default. Playing a torrent downloads it from other peers, and shares what was downloaded while the upload limit allows: you answer for what this server downloads and shares. Users also need Can play torrent streams, on their page under Users. Needs a route.',
      route: 'Route',
      routeHelp:
        'Every torrent socket, for peers, trackers and the DHT, goes this way alone. When it is down, torrent versions are skipped, also during a playback, and System › Health says why.',
      routeHelpNoInterface:
        'Every torrent socket, for peers, trackers and the DHT, goes this way alone. This system binds no socket to a network interface, which only Linux does, as on Polyfin Desktop for Windows and macOS: a SOCKS5 proxy or a WireGuard tunnel carries the torrents. When it is down, torrent versions are skipped, also during a playback, and System › Health says why.',
      routes: {
        interface: 'Network interface',
        proxy: 'SOCKS5 proxy',
        wireguard: 'WireGuard tunnel',
      },
      interface: 'Network interface for torrents',
      interfaceHelp:
        'Its name, such as wg0. Every torrent socket is bound to it, and tracker names are looked up over it. Linux only. In Docker, the container must see the interface: see the torrents docs.',
      proxy: 'SOCKS5 proxy for torrents',
      proxyHelp:
        'Its address and port, such as 10.0.0.2:1080. Peers and HTTP trackers go through its CONNECT. UDP trackers, the DHT and uTP need its UDP ASSOCIATE, and stay off without it. Through a proxy, no peer can connect in.',
      proxyUser: 'Proxy user',
      proxyUserHelp: 'Empty when the proxy asks for none.',
      proxyPassword: 'Proxy password',
      proxyPasswordHelp: 'Stored encrypted when POLYFIN_SECRET_KEY is set.',
      wireGuard: 'WireGuard configuration',
      wireGuardHelp:
        'Paste the .conf your VPN provider gives for WireGuard. Polyfin runs the tunnel itself, for torrents alone, with nothing to set up on this system. Tracker names are looked up through the tunnel by its DNS servers, never another way. No peer can connect in. Stored encrypted when POLYFIN_SECRET_KEY is set, and never shown again.',
      wireGuardSaved: (endpoint: string, addresses: string) =>
        `A configuration is saved: server ${endpoint}, addresses ${addresses}.`,
      wireGuardDns: (servers: string) =>
        `Tracker names are looked up by ${servers}, through the tunnel.`,
      wireGuardNoDns:
        'It names no DNS server: trackers named by a host name are not reached, those named by an address are.',
      wireGuardReplace: 'Paste another configuration to replace the saved one.',
      wireGuardInvalid: (problem: { line: number; key: string; reason: string }) => {
        const where = problem.line > 0 ? `Line ${problem.line}: ` : ''
        const expected: Record<string, string> = {
          PrivateKey: 'a key of 32 bytes in base64',
          PublicKey: 'a key of 32 bytes in base64',
          PresharedKey: 'a key of 32 bytes in base64',
          Address: 'IP addresses, such as 10.2.0.2/32, separated by commas',
          DNS: 'IP addresses separated by commas',
          MTU: 'a number from 576 to 65535',
          ListenPort: 'a port from 0 to 65535',
          Endpoint: 'a host name or an IP address with a port, such as vpn.example.com:51820',
          AllowedIPs:
            'IP addresses with their prefix length, such as 0.0.0.0/0, separated by commas',
          PersistentKeepalive: 'off, or a number of seconds from 0 to 65535',
        }
        switch (problem.reason) {
          case 'too_long':
            return 'The WireGuard configuration must be text of at most 8192 bytes.'
          case 'syntax':
            return `${where}each line must be a [section], a Key = Value, or a # comment.`
          case 'outside':
            return `${where}${problem.key} comes before [Interface] or [Peer].`
          case 'unknown':
            return `${where}${problem.key} is not part of a WireGuard configuration.`
          case 'repeated':
            return problem.key === 'Peer'
              ? `${where}only one [Peer] is supported.`
              : `${where}${problem.key} is given twice.`
          case 'missing':
            return `${where}${problem.key} is missing from the WireGuard configuration.`
          case 'no_ipv4':
            return `${where}Address needs an IPv4 address, which UDP trackers and the DHT use.`
          case 'not_all':
            return `${where}AllowedIPs needs 0.0.0.0/0: the peers of a torrent are anywhere.`
          default:
            return `${where}${problem.key} must be ${expected[problem.key] ?? 'valid'}.`
        }
      },
      downloadLimit: 'Download limit (Mbps, 0 = none)',
      downloadLimitHelp: (r: RangeText) => `From 0 to ${r.max}; ${r.default} by default.`,
      uploadLimit: 'Upload limit (Mbps, 0 = no sharing)',
      uploadLimitHelp: (r: RangeText) =>
        `Peers favour those that share back, so a start may be slower at 0. Sharing stops when the playback ends. From 0 to ${r.max}; ${r.default} by default.`,
      atOnce: 'Torrents at once (0 = no limit)',
      atOnceHelp: (r: RangeText) =>
        `How many torrents may run at once, for every user together. At the limit, a title plays another of its versions that is not a torrent, or is said to be unplayable for now; a place frees as a torrent stops, 30 seconds after its playback, or at once when the same device or user plays another title. From 0 to ${r.max}; ${r.default} by default.`,
      trackers: 'Trackers added to every torrent',
      trackersHelp: (r: RangeText) =>
        `Announce URLs, one a line, such as udp://tracker.example:1337/announce. Polyfin asks them for the peers of every torrent, besides the trackers its addon names: a torrent the addon lists without any is then not left to the DHT alone. HTTP, HTTPS and UDP; ${r.max} at most. A private torrent asks its own trackers only. Empty by default: Polyfin comes with no list.`,
    },
    dlna: {
      title: 'DLNA',
      description:
        'TVs, receivers, consoles and players of the local network that have no Jellyfin app can browse and play the libraries over DLNA. A device cannot sign in: each shows the libraries of the user chosen for it, under that user’s limits.',
      share: 'Share libraries over DLNA',
      shareHelp:
        'Announces Polyfin to the devices of the local network on UDP port 1900, and answers them only. Applies at once, without a restart. In Docker, this needs host networking or a network that gives the container its own address: see the DLNA docs.',
      name: 'Name on the network',
      nameHelp: 'The name devices list Polyfin under. Empty, the server’s name.',
      newDevicesUser: 'Show new devices the libraries of',
      newDevicesUserHelp:
        'The user whose libraries a device seen for the first time shows. With No one, a new device sees an empty server until you choose a user for it below.',
      noOne: 'No one',
      markPlayed: 'Mark played what DLNA devices play',
      markPlayedHelp:
        'Devices report no position: a title counts as played once the device read past Marked played after (%) of it. Devices read ahead, so a title can be marked early.',
      devices: 'Devices',
      devicesEmpty: 'No device has asked for anything yet.',
      device: 'Device',
      address: 'Address',
      model: 'Model',
      lastSeen: 'Last seen',
      showsLibrariesOf: 'Shows the libraries of',
      blocked: 'Blocked',
      profile: 'Profile',
      automaticProfile: (name: string) => `Automatic (${name})`,
      forget: 'Forget',
      profiles: 'Profiles',
      profilesHelp:
        'Profiles use the XML format of Jellyfin’s DLNA plugin. A device gets the first that matches what it sends, added profiles first, else the generic one.',
      builtin: 'Built-in',
      addProfile: 'Add a profile',
      replaceProfile: 'Replace',
      removeProfile: 'Remove',
      profileXml: 'Profile XML',
      devicesHelp:
        'The devices of the local network that asked Polyfin for anything, the latest seen first, each known by its address. Changes apply at once.',
      devicesLabel: 'DLNA devices',
      automatic: 'Automatic',
      deviceSaved: (name: string) => `${name} saved.`,
      forgetTitle: (name: string) => `Forget ${name}?`,
      forgetBody:
        'Polyfin forgets the user, the block and the profile chosen for it. If it asks again, it is a new device, and shows the libraries of the user chosen for new devices.',
      forgotten: (name: string) => `${name} forgotten.`,
      profilesLabel: 'DLNA profiles',
      added: 'Added',
      viewXml: 'View XML',
      profileXmlHelp:
        'Paste the XML of a profile made for Jellyfin’s DLNA plugin, or open its file.',
      openFile: 'Open a file',
      add: 'Add',
      adding: 'Adding…',
      replacing: 'Replacing…',
      profileAdded: (name: string) => `Profile ${name} added.`,
      profileReplaced: (name: string) => `Profile ${name} replaced.`,
      removeTitle: (name: string) => `Remove the profile ${name}?`,
      removeBody: 'The devices it is forced on get their profile chosen again from what they send.',
      profileRemoved: (name: string) => `Profile ${name} removed.`,
    },
    metrics: 'Expose metrics',
    metricsHelp:
      'Serves Polyfin’s metrics to Prometheus at /metrics: playbacks, conversions, addon requests, imports, notifications, HTTP requests, scans, the database, disk space and the process. They name no user, title, address or device. Prometheus sends the metrics token with each request; without it, /metrics answers 401, and while this is off, 404.',
    metricsToken: 'Metrics token',
    metricsTokenHelp:
      'Made by Polyfin when metrics are first turned on. Prometheus sends it as a bearer token: see the administration docs for a scrape configuration.',
    metricsTokenPending: 'Polyfin makes the metrics token when you save.',
    newMetricsToken: 'Make a new token',
    newMetricsTokenTitle: 'Make a new metrics token?',
    newMetricsTokenConfirm:
      'The current token stops working at once: Prometheus needs the new one to read the metrics.',
    newMetricsTokenMade: 'New metrics token made',
    legacyWarningTitle: 'Security warning',
    legacyWarning:
      'Legacy methods can send credentials in URLs, which end up in logs, browser history and proxies. Only turn this on if an app you use cannot sign in otherwise.',
    prepareAhead: 'Prepare playback in advance',
    prepareAheadHelp:
      'Polyfin reads the file as soon as a title’s page opens, and gets the next episode ready near the end of the current one, so playback starts right away. It also lists in advance the versions of the titles in Continue Watching and Next Up, so their pages show them at once. This sends a few more requests to your sources, also for titles that are opened but not played.',
    transcoding: 'Conversion (transcoding)',
    transcodingHelp:
      'Re-encodes video and audio for apps that cannot play a file as it is. When off, apps play files as they are or simply repackaged without re-encoding, and a title an app cannot play that way will not start on that app. This includes recordings converted to HEVC, on apps that do not play HEVC.',
    analysisTimeout: 'Maximum time to analyze a version',
    analysisTimeoutHelp: (r: RangeText) =>
      `Before a first play, Polyfin analyzes the file or the channel to know how to play it. If the source does not answer within this many seconds, Polyfin gives up on that version and moves on to the next one. A lower number moves on sooner, but may give up on slow sources that would have worked. A channel is analyzed for 8 seconds at most. From ${r.min} to ${r.max} seconds; ${r.default} by default.`,
    versionAttempts: 'Versions tried when one does not work',
    versionAttemptsHelp: (r: RangeText) =>
      `When an app plays a title without choosing a version, Polyfin tries the versions in order until one works, but analyzes no more than this many. Versions already analyzed are tried too, as they cost nothing. A higher number finds a working version more often, but a title that does not play takes longer to say so. From ${r.min} to ${r.max}; ${r.default} by default.`,
    preferDirectPlay: 'Prefer versions the app plays without conversion',
    preferDirectPlayHelp:
      'When an app plays a title without choosing a version, Polyfin picks the first version the app plays as it is or simply repackaged, rather than the first that plays at all. The very first play of a title can be a little slower, as more versions may be analyzed; nothing changes once they are known.',
    nameHowVersionsPlay: 'Say in version names how they play',
    nameHowVersionsPlayHelp:
      'Version names end with how the app plays each: “(direct play)”, “(direct stream)” when copied without conversion, “(converted: reason)” with the app’s request, HDR, the quality limit, subtitles, video or audio as the reason, or “(cannot play here)”. Polyfin decides it for the device profile the app sent at its last playback, so names say nothing before a device’s first play or after a restart, and nothing for a version not analyzed or described by RemuxDB yet. Names are in the language of generated names. Nothing is read for them.',
    versionOrderGroup: 'Version order',
    versionOrder: {
      label: 'Order versions by',
      help: 'Each criterion groups the versions of a movie or an episode, and the next one sorts them within each group; versions that tie keep the addons’ order. The first version is the one Play starts, and the versions tried when it does not work come next. Polyfin goes by what it knows of a version without reading the file: its analysis once played, else RemuxDB’s description, else the words of its labels. A version a criterion knows nothing of comes after the others. The preferred audio language is each user’s own, set in their app, else Default audio language: versions known in that language come first, then those labeled MULTI or DUAL, then those of unknown language. Under My account, a user may also prefer an audio region, such as French for Quebec: among the versions in its language, those of that region come first, then those of no known region. While a title’s page is open, the versions it shows keep their places.',
      none: 'None: versions keep the addons’ order.',
      add: 'Add a criterion',
      remove: (name: string) => `Remove ${name}`,
      criterion: (position: number) => `Criterion ${position}`,
      criteria: {
        language: 'Preferred audio language',
        'resolution-high': 'Resolution, highest first',
        'resolution-low': 'Resolution, lowest first',
        'size-large': 'File size, largest first',
        'size-small': 'File size, smallest first',
        codec: 'Video codec, in the order below',
        'hdr-first': 'HDR and Dolby Vision first',
        'hdr-last': 'HDR and Dolby Vision last',
        source: 'Source, in the order below',
      },
    },
    versionCodecOrder: {
      label: 'Order of video codecs',
      help: 'The order the Video codec criterion puts versions in.',
    },
    versionSourceOrder: {
      label: 'Order of sources',
      help: 'The order the Source criterion puts versions in: the server’s addons and folders that give versions of movies and episodes. A source added later comes last until you move it, and users’ own addons come after the server’s sources. This changes nothing but the order of versions: descriptions, subtitles and search results follow the order of Content › Sources.',
      none: 'No source of the server gives versions of movies or episodes yet.',
      dragHandle: (name: string) => `Drag ${name}`,
    },
    versionCodecs: {
      av1: 'AV1',
      hevc: 'HEVC (H.265)',
      vp9: 'VP9',
      h264: 'H.264 (AVC)',
      vc1: 'VC-1',
      mpeg4: 'MPEG-4 Part 2 (Xvid, DivX)',
      mpeg2video: 'MPEG-2',
    },
    hideVersionsLarger: 'Hide versions larger than (GB, 0 = no limit)',
    hideVersionsLargerHelp: (r: RangeText) =>
      `Versions known to be larger are listed nowhere: not on title pages, in playback or in downloads. When the rules below and this one would hide every version of a title, none is hidden. Up to ${r.max} GB; ${r.default} by default.`,
    hiddenVersionCodecs: 'Hide these video codecs',
    hiddenVersionCodecsHelp: 'Versions known to be in these codecs are listed nowhere.',
    hiddenVersionWords: 'Hide versions whose labels contain',
    hiddenVersionWordsHelp: (r: RangeText) =>
      `One word or phrase a line, matched in any letter case in a version’s name, description and file name: for addons that say in their labels that a file is not ready to play yet. Up to ${r.max} lines; none by default.`,
    hiddenReleases: 'Hide low-quality releases',
    hiddenReleasesHelp:
      'Versions whose name or file name tags them as one of these kinds are listed nowhere. A tag counts as a whole word after the title’s own words, in any letter case: a movie named Cam is no camera copy, and a .ts file no telesync. None by default.',
    releaseKinds: {
      cam: 'Camera copies (CAM, HDCAM)',
      telesync: 'Telesyncs (TS, HDTS, TELESYNC)',
      telecine: 'Telecines (TC, TELECINE)',
      screener: 'Screeners (SCR, DVDSCR)',
      early: 'Early copies (R5, WORKPRINT)',
    },
    syncAddonSubtitles: 'Sync addon subtitles',
    syncAddonSubtitlesHelp:
      'Times the addons’ subtitles to the version played, when they run early, late, or faster or slower than it. At a version’s first play, Polyfin finds in the background when it has speech: from a text subtitle inside the file, else from three stretches of its audio, reading at most 300 MB of an addon’s file. Torrent versions are left out, as that would download them from their swarm. From then on, a subtitle that fits clearly better shifted or sped up comes synced, its name ending with “(synced)”.',
    remuxDb: 'Describe versions from RemuxDB',
    remuxDbHelp:
      'Shows the audio, subtitle and video tracks of a version before it is first played, as RemuxDB found them in the same file. When a title’s details open, Polyfin asks RemuxDB, a community database, by the title’s IMDb identifier, and finds the versions by their file names. Playback still analyzes each version.',
    remuxDbUrl: 'RemuxDB address',
    remuxDbUrlHelp: 'The RemuxDB server Polyfin asks.',
    playbackHistoryGroup: 'Playback history',
    playbackHistory: 'Keep a playback history',
    playbackHistoryHelp:
      'Keeps each video played, with who played it, on which app and how, for System › Statistics and each user’s statistics under My account. Turned off, nothing more is kept; what is kept stays until it is too old.',
    playbackHistoryDays: 'Days to keep playback history',
    playbackHistoryDaysHelp: (r: RangeText) =>
      `Older playbacks are deleted. This is checked every day. From ${r.min} to ${r.max}, ${r.default} by default.`,
    defaultLanguagesGroup: 'Default languages',
    defaultAudioLanguage: 'Default audio language',
    defaultAudioLanguageHelp:
      'For the users who chose no audio language in their app, Polyfin plays the audio track in this language when a version has one, and puts the versions known in this language first when versions are ordered by language. A user’s own choice always wins, and their app keeps showing it as they left it. This language comes before the track the file flags as default, even for users whose app plays the default track whatever its language, a setting on until they turn it off; a file without it plays its flagged track. None by default.',
    defaultSubtitleLanguage: 'Default subtitle language',
    defaultSubtitleLanguageHelp:
      'For the users who chose no subtitle language in their app, Polyfin shows subtitles in this language as each user’s subtitle mode says. A user’s own choice always wins, and their app keeps showing it as they left it. None by default.',
    noDefaultLanguage: 'None',
    publicAddress: 'Public address',
    publicAddressHelp:
      'The address people open Polyfin at, such as https://media.example.org. Links in notifications start with it; empty, messages carry no link. When it starts with https://, Redirect HTTP to HTTPS, under Settings › Security, goes to its port, 443 when it names none.',
    email: 'Email',
    emailHelp:
      'Email targets go through this SMTP server. They can be added once a server and a sender address are saved.',
    smtpHost: 'SMTP server',
    smtpHostHelp: 'Its host name, such as smtp.example.org. Empty, email targets cannot be added.',
    smtpPort: 'SMTP port',
    smtpPortHelp: 'Usually 587 with STARTTLS, and 465 with TLS from the start.',
    smtpSecurity: 'SMTP encryption',
    smtpSecurityHelp:
      'How the connection is encrypted. With STARTTLS, nothing is sent if the server does not offer it. Without encryption, the password is only sent to this machine.',
    smtpSecurities: { starttls: 'STARTTLS', tls: 'TLS from the start', none: 'None' },
    smtpUser: 'SMTP user',
    smtpUserHelp: 'Empty to send without signing in.',
    smtpPassword: 'SMTP password',
    smtpPasswordHelp: 'It is kept encrypted and never shown again.',
    smtpFrom: 'Sender address',
    smtpFromHelp: 'The address messages come from, such as polyfin@example.org.',
    smtpFromName: 'Sender name',
    smtpFromNameHelp: 'Shown with the address. Empty, the server name is.',
    weeklySummary: 'Weekly summary',
    weeklySummaryHelp:
      'Targets that chose "Weekly summary" in their Events receive it once a week: the server’s and administrators’ targets the server’s week, the others their own week. Nothing is sent for a week with nothing to tell. System › Schedule sends it now.',
    weeklySummaryDay: 'Send the weekly summary on',
    weeklySummaryDayHelp: (day: string) => `${day} by default.`,
    weeklySummaryHour: 'Send the weekly summary at',
    weeklySummaryHourHelp: (hour: string) =>
      `In the server’s time zone. The month and year in review are sent at this hour too, on the 1st. ${hour} by default.`,
    maxConversions: 'Video conversions at once (0 = no limit)',
    maxConversionsHelp: (r: RangeText) =>
      `Converting video is the heaviest work the server does. Once this many playbacks have their video converted (subtitles burned into the picture included), a new playback gets a version that needs no conversion, or does not start. Playbacks already running are never cut. Live TV keeps its own limits too: 4 channels per user and 16 for the server. From ${r.min} to ${r.max}; ${r.default} by default.`,
    maxConversionHeight: 'Maximum quality of converted video',
    maxConversionHeightHelp:
      'Converted video is scaled down to this height at most, keeping its shape, so that it plays well over a slower connection. Files played as they are or simply repackaged keep their quality. A graphics card converts up to 4K; the processor stops at 1080p, and when it converts HDR to SDR, at "Maximum quality of HDR converted by the processor", under Conversion (transcoding) › HDR.',
    conversionHeightOriginal: 'Original',
    conversionHeight: (height: number) => `${height}p`,
    convertedDownloads: 'Converted downloads',
    convertedDownloadsHelp:
      'Apps that ask for it, such as Streamyfin, save a title at the quality the user picks: Polyfin sends a smaller copy, made as the app downloads it and never kept on the server. Without it, they save the file as it is. Each download counts toward "Video conversions at once" while it converts, and as one of the user’s "Playbacks at once". The user needs "Can download", and "Can use conversion (transcoding)" when the video or the audio is converted. An interrupted download starts again. Off by default.',
    conversion: {
      description:
        'When an app cannot play a file as it is, Polyfin converts it with FFmpeg. These settings choose how, for movies, series and Live TV alike. The defaults suit most servers.',
      groups: {
        general: 'General',
        subtitles: 'Picture subtitles',
        gpu: 'Graphics card',
        video: 'Video',
        hdr: 'HDR',
        interlaced: 'Interlaced video',
        audio: 'Audio',
        performance: 'Performance',
      },
      subtitleOcr: 'Turn picture subtitles into text',
      subtitleOcrHelp:
        'PGS and DVD (VobSub) subtitles are pictures, and many apps take only text: for them, Polyfin burns the track into the video, which needs a conversion. With this on, Tesseract reads each picture track once, in the background, into a text copy listed as one more track marked "(OCR)", which plays without conversion. Until a copy is ready, the track is burned in as before. The picture track stays: italics, colors and positions are lost, and stylized fonts can be misread. "When played" reads a version’s tracks the first time an app that cannot draw them plays it; "Also after library scans" also reads the tracks of new local files. Off by default.',
      subtitleOcrChoices: {
        off: 'Off',
        played: 'When played',
        scans: 'Also after library scans',
      },
      subtitleOcrTracks: 'Picture subtitle tracks read at once',
      subtitleOcrTracksHelp: (r: RangeText) =>
        `Each track is read by one Tesseract process using one processor core. From ${r.min} to ${r.max}; ${r.default} by default.`,
      hardwareAcceleration: 'Graphics card used to convert',
      hardwareAccelerationHelp:
        'A graphics card (GPU) converts video much faster than the processor. Each system offers its own: NVIDIA and VAAPI on Linux, VideoToolbox on macOS, NVIDIA, Intel and AMD on Windows. Without a card that works, the processor converts. A change applies once saved, without a restart.',
      vaapiDevice: 'Graphics card for VAAPI',
      vaapiDeviceHelp:
        'The render node VAAPI converts video on, when several graphics cards could. Each in turn by default, the first that works.',
      vaapiDeviceEach: 'Each in turn',
      vaapiDeviceMissing: (path: string) => `${path} (not found)`,
      hardware: {
        auto: 'Automatic: the first that works, in the order below',
        nvenc: 'NVIDIA (NVENC)',
        vaapi: 'AMD or Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
        none: 'None: the processor only',
      },
      detected: 'Detected on this server',
      noGpu: 'No graphics card is used: the processor converts video.',
      gpu: 'Graphics card',
      gpuNames: {
        cuda: 'NVIDIA',
        vaapi: 'AMD or Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
      } as Record<string, string>,
      encoders: 'Encoders on the card',
      gpuToneMapping: 'HDR to SDR on the card',
      processorToneMappingHint:
        'HDR is then converted by the processor, up to "Maximum quality of HDR converted by the processor".',
      qualityFactor: 'Takes a quality number',
      processor: 'Encoders on the processor',
      processorToneMapping: 'HDR to SDR on the processor',
      /** Where HDR titles are converted to SDR, and why, as the timings at startup chose it. */
      hdrPath: {
        term: 'HDR titles',
        gpu: 'Converted to SDR on the card',
        gpuUpTo: (height: number) => `Converted to SDR on the card, up to ${height}p`,
        processor: 'Converted to SDR on the processor, encoded on the card',
        notTimed: 'The card’s speed is not measured yet, or could not be.',
        keepsUp: (gpu: string) => `The card converts HDR at ${gpu} real time.`,
        processorFaster: (gpu: string, processor: string) =>
          `The card converts HDR at ${gpu} real time, too slow for playback; the processor keeps up, at ${processor}.`,
        /** `processor` is null when the processor was not measured. */
        lowerHeight: (gpu: string, low: string, processor: string | null) =>
          `The card converts HDR to 1080p at ${gpu} real time, too slow for playback, and to 720p at ${low}${processor ? `; the processor at ${processor}, too slow as well` : ''}.`,
        noneKeepsUp: (gpu: string, low: string, processor: string | null) =>
          `Neither keeps up with playback: the card converts HDR to 1080p at ${gpu} real time and to 720p at ${low}${processor ? `, the processor at ${processor}` : ''}. HDR titles may pause while they convert.`,
        turnedOff: 'HDR to SDR on the card is turned off in the settings.',
      },
      ffmpeg: 'FFmpeg',
      ffmpegShipped: 'The one Polyfin ships',
      ffmpegOlderDrivers: 'For NVIDIA drivers older than 610',
      ffmpegOlderDownloading:
        'Downloading the FFmpeg for NVIDIA drivers older than 610, which NVENC needs here: the processor converts video meanwhile.',
      ffmpegOlderFailed: (error: string) =>
        `The FFmpeg for NVIDIA drivers older than 610, which NVENC needs here, could not be downloaded: ${error}`,
      ffmpegOlderUnusable: (error: string) =>
        `NVENC does not encode with the FFmpeg for NVIDIA drivers older than 610 either: ${error}`,
      yes: 'Yes',
      no: 'No',
      none: 'None',
      hardwareDecoding: 'Read these formats on the graphics card',
      hardwareDecodingHelp:
        'The card reads (decodes) these formats itself, which leaves the processor free. Uncheck one if its videos fail to convert or look wrong: the processor then reads it. Other formats are tried on the card. HEVC 10-bit needs HEVC.',
      hardwareDecodingNoGpu: 'No graphics card is used, so the processor reads every format.',
      codecs: {
        h264: 'H.264',
        hevc: 'HEVC',
        hevc_10bit: 'HEVC 10-bit',
        vp9: 'VP9',
        av1: 'AV1',
        mpeg2video: 'MPEG-2',
        vc1: 'VC-1',
      },
      encoderPreset: 'Encoding speed',
      encoderPresetHelp:
        'Slower gives a better picture at the same size, but needs more power. If converted videos stutter, choose a faster one. Automatic keeps Polyfin’s choice: very fast on the processor, medium on NVIDIA cards, the card’s own on the others.',
      presets: {
        auto: 'Automatic',
        veryslow: 'Very slow (best picture)',
        slower: 'Slower',
        slow: 'Slow',
        medium: 'Medium',
        fast: 'Fast',
        faster: 'Faster',
        veryfast: 'Very fast',
        superfast: 'Super fast',
        ultrafast: 'Ultra fast (lightest work)',
      },
      h264Quality: 'H.264 quality (0 = by bitrate)',
      hevcQuality: 'HEVC quality (0 = by bitrate)',
      qualityHelp: (r: RangeText) =>
        `A lower number gives a better picture and more data; the bitrate still sets the maximum. 0 aims for the bitrate alone. Jellyfin uses 23 for H.264 and 28 for HEVC. 0, or from ${r.min} to ${r.max}; ${r.default} by default.`,
      qualityIgnored:
        'This graphics card cannot take a quality number: it keeps aiming for the bitrate.',
      allowHevcEncoding: 'Allow converting to HEVC',
      allowHevcEncodingHelp:
        'HEVC needs less data than H.264 for the same picture, but takes more power to make. When on, apps that list HEVC first get it. When off, only apps that cannot play H.264 get HEVC.',
      noHevcEncoder: 'FFmpeg cannot make HEVC on this server.',
      adaptiveQuality: 'Several qualities when converting',
      adaptiveQualityHelp:
        'Converted videos also offer up to two lower qualities, such as 720p and 480p below 1080p. Apps that can switch step down to them when their connection slows, instead of stopping to load. A quality the app leaves keeps converting for 10 seconds, so that switching back does not start over, and all of them count as one conversion. Copied videos, Live TV and music keep one quality. “Local network” means what it means for password reset PINs, through the known proxies. Off by default.',
      adaptiveQualities: { off: 'Off', remote: 'Outside the local network', always: 'Always' },
      toneMapping: 'Convert HDR to SDR (tone mapping)',
      toneMappingHelp:
        'Keeps the colors and brightness of HDR videos right on screens that only show SDR. When off, converted HDR videos look pale, and Dolby Vision videos without an HDR10 layer are not converted.',
      toneMappingUnavailable:
        'Neither the graphics card nor FFmpeg can do it on this server: HDR videos are only converted when this is off.',
      toneMappingAlgorithm: 'Tone mapping method',
      toneMappingAlgorithmHelp:
        'How bright parts are brought down to SDR. Automatic uses BT.2390 on NVIDIA cards and Hable on the processor. Intel cards and VideoToolbox use their own method and ignore this choice.',
      algorithms: {
        auto: 'Automatic',
        bt2390: 'BT.2390 (NVIDIA cards only)',
        hable: 'Hable',
        reinhard: 'Reinhard',
        mobius: 'Möbius',
        clip: 'Clip',
        linear: 'Linear',
      },
      gpuToneMappingSetting: 'Tone map HDR on the graphics card',
      gpuToneMappingSettingHelp:
        'NVIDIA cards, Intel cards (through VAAPI on Linux, QSV on Windows) and VideoToolbox on macOS convert HDR to SDR themselves, up to 4K. Turn this off if their colors look wrong: the processor then does it, at a lower quality. Intel cards only take HDR10 videos that carry their mastering display information, and VideoToolbox HDR10 and HLG; the processor converts the others, and Dolby Vision without an HDR10 layer is only converted on NVIDIA cards. AMD cards never do it: their Linux driver failed or froze each way Polyfin tried, and FFmpeg’s AMF filter on Windows does not tone map.',
      gpuToneMappingUnavailable:
        'The graphics card on this server cannot do it: the processor converts HDR to SDR.',
      processorToneMappingHeight: 'Maximum quality of HDR converted by the processor',
      processorToneMappingHeightHelp:
        'Converting HDR to SDR is heavy work for the processor: about four times the encoding itself at 1080p. Automatic times this server at startup: 1080p when it converts at least one and a half times as fast as the video plays, else 720p, which is also used until the timing ends. Above 1080p only counts when a graphics card encodes the video: on its own, the processor stops at 1080p.',
      toneMappingHeightAuto: 'Automatic (measured at startup)',
      height4k: '4K',
      measuredHeight: (height: number) => `Measured: ${height}p`,
      upToHeight: (height: number) => `up to ${height}p`,
      toneMappingPeak: 'Peak brightness in nits (0 = from the video)',
      toneMappingDesat: 'Highlight desaturation (0 = off)',
      toneMappingPeakHelp: (peak: RangeText, desat: RangeText) =>
        `Only when the processor converts HDR to SDR: a graphics card that does it ignores them. The peak replaces the brightest level the video says it reaches: 0, or from ${peak.min} to ${peak.max}; ${peak.default} by default. Desaturation fades the color of very bright parts: from ${desat.min} to ${desat.max}; ${desat.default} by default.`,
      processorCannotToneMap: 'FFmpeg cannot do tone mapping on the processor on this server.',
      deinterlaceMethod: 'Deinterlacing method',
      deinterlaceMethodHelp:
        'TV broadcasts and DVDs are often interlaced, which shows comb-like lines once converted. Yadif is fast; Bwdif is a little sharper.',
      noBwdif: 'FFmpeg on this server has no Bwdif.',
      deinterlacers: { yadif: 'Yadif', bwdif: 'Bwdif' },
      deinterlaceDoubleRate: 'Double the frame rate',
      deinterlaceDoubleRateHelp:
        'Makes a picture from each half-frame, for smoother motion in sports and TV shows. Only for videos up to 30 frames per second.',
      downmixAlgorithm: 'Mix to stereo',
      downmixAlgorithmHelp:
        'How surround sound is mixed down to two speakers. Dave750 and Night mode keep voices clear; RFC 7845 and AC-4 follow standards.',
      downmixes: {
        None: 'FFmpeg’s own mix',
        Dave750: 'Dave750',
        NightmodeDialogue: 'Night mode (clearer voices)',
        Rfc7845: 'RFC 7845',
        Ac4: 'AC-4',
      },
      downmixBoost: 'Volume when mixing to stereo',
      downmixBoostHelp: (r: RangeText) =>
        `Stereo mixes often sound quieter: the volume is multiplied by this number. From ${r.min} to ${r.max}; ${r.default} (no change) by default. Jellyfin uses 2.`,
      maxAudioChannels: 'Most audio channels',
      maxAudioChannelsHelp:
        'Converted sound keeps at most this many channels, even if the app takes more.',
      audioChannels: (channels: number): string =>
        channels === 0
          ? 'As many as the app takes'
          : channels === 1
            ? 'Mono'
            : channels === 2
              ? 'Stereo'
              : '5.1',
      audioBitratePerChannel: 'Audio bitrate per channel in kb/s (0 = automatic)',
      audioBitratePerChannelHelp: (r: RangeText) =>
        `Automatic gives Jellyfin’s bitrates: 128 kb/s in mono, 384 kb/s in stereo and 640 kb/s in 5.1, less under a low bitrate limit. 0, or from ${r.min} to ${r.max}; ${r.default} by default.`,
      encodingThreads: 'Processor threads per conversion (0 = automatic)',
      encodingThreadsHelp: (r: RangeText) =>
        `Limits how much of the processor one conversion uses, to leave room for other work. From ${r.min} to ${r.max}; 0 lets FFmpeg choose.`,
      aheadSeconds: 'Seconds prepared ahead',
      aheadSecondsHelp: (r: RangeText) =>
        `Polyfin prepares a video at most this many seconds past the part the app asked for, then waits. More helps with slow sources and keeps their connection busy, but uses more power and disk space when people stop watching early. From ${r.min} to ${r.max}; ${r.default} by default.`,
      keepConvertedVideo: 'Keep the converted video when the audio track changes',
      keepConvertedVideoHelp:
        'Converts the picture and the sound apart, so that choosing another audio track in the player only prepares the new sound: the picture already converted plays on, and the new track starts sooner. The picture is kept 30 seconds after a player stops it, and counts as one conversion meanwhile.',
    },
    catalogsTitle: 'Catalogs',
    catalogLimit: 'Titles read per movie and series catalog',
    catalogLimitHelp: (r: RangeText) =>
      `Some catalogs are nearly endless, so Polyfin stops reading a catalog after this many titles. A higher number shows more titles, but lists load more slowly and the addon gets more requests. From ${r.min} to ${r.max}; ${r.default} by default.`,
    channelLimit: 'Channels read per Live TV catalog',
    channelLimitHelp: (r: RangeText) =>
      `Polyfin stops reading a Live TV catalog after this many channels, and reads at most this many programmes per day for the guide. A higher number shows more, but loads more slowly and the addon gets more requests. From ${r.min} to ${r.max}; ${r.default} by default.`,
    skipButtons: 'Skip intro and credits buttons',
    skipButtonsHelp:
      'Apps offer to skip intros, recaps and credits, found in community databases. When off, apps show no skip buttons and the databases are not asked.',
    publicMetaDbKey: 'PublicMetaDB key',
    publicMetaDbKeyHelp:
      'Optional. A PublicMetaDB API key adds PublicMetaDB as a source of skip markers.',
    theIntroDbKey: 'TheIntroDB key (optional)',
    theIntroDbKeyHelp:
      'Raises TheIntroDB’s daily limit and includes your own submissions. Reading works without it.',
    segmentSources: {
      label: 'Skip marker sources',
      help: 'For each kind of passage (intro, recap, credits, preview), the first source turned on in this list that has it wins; the next ones fill the gaps. Sources turned off are never asked.',
      turnOn: (name: string) => `Ask ${name}`,
      needsKey: 'Needs a key',
    },
    audioIntros: 'Find intros and credits in local episodes’ audio',
    audioIntrosHelp:
      'When an episode of a local folder plays without an intro or credits from its chapters or the sources above, Polyfin compares the audio of its season’s episodes in the background, and gives each the stretch they share near their start as its intro, and the one they share near their end as its credits. The season’s other episodes then have theirs, and the one played has them from its next play. Needs the skip buttons on.',
    similarTitles: 'Similar titles',
    similarTitlesHelp:
      'A title’s page lists titles close to it, found in the addons’ catalogs. When off, the list is empty and the addons get fewer requests.',
    generateImages: 'Generate images for collections without one',
    generateImagesHelp:
      'An addon’s collection without a poster, and a smart list shown as a collection, show the posters of their first titles with their name, drawn by Polyfin once their titles have been read. Users under parental control or blocking genres do not see them. When off, they show no image. Libraries set to a generated image keep it.',
    tmdbKey: 'TMDB API key',
    tmdbKeyHelp:
      'Optional. With an API key or a read access token from a TMDB account, jellyfin-web’s Edit images also searches TMDB for a title’s posters, backdrops and logos, an episode’s stills, a season’s posters and a person’s photos, which Polyfin relays. People are also looked up on TMDB once: those without a photo get theirs, and their pages show their biography, birth and death dates and birthplace. A local folder’s files whose search every metadata addon fails are searched on TMDB too. Without a key, Edit images lists the addons’ images only and TMDB is never asked.',
    mdblistRatingsKey: 'MDBList API key for ratings',
    mdblistRatingsKeyHelp:
      'Optional. With a key from an MDBList account, movies and series show their IMDb rating, critics’ score and audience score from MDBList, looked up in the background and kept for every user. Users’ own MDBList keys for tracking are never used for this. Without a key, MDBList is never asked.',
    criticRatingShows: 'Critic rating shows',
    criticRatingShowsHelp:
      'Jellyfin apps have a single field for a critic rating, from 0 to 100: it shows the critics’ score or the audience score. On a title’s page, the web player shows both, with the IMDb rating.',
    criticRatingChoices: { critics: 'Critics’ score', audience: 'Audience score' },
    posterBadges: 'Badges on posters',
    posterBadgesHelp:
      'Draws over a movie’s or a series’ poster its star rating, naming IMDb when the rating is IMDb’s from MDBList, its critics’ and audience scores while an MDBList API key for ratings is set, and a series’ new episode or new season of the last 30 days. Every app shows them, as they are part of the poster, with icons and numbers only, for any language. While on, the web player shows no ratings on hover. When off, apps get the posters as they are.',
    posterBadgeSize: 'Badge size',
    posterBadgeSizeHelp:
      'How large the badges are for the poster’s width. They keep a least size on small cards, where fewer of them fit.',
    posterBadgeSizeChoices: { medium: 'Medium', small: 'Small', large: 'Large' },
    posterPreview: {
      title: 'Preview',
      help: 'Titles of your libraries as apps show them, at the badge size chosen, before it is saved, whether the badges are on or not.',
      original: 'As it is',
      badged: 'With badges',
      card: 'On a small card',
      another: 'Another title',
      none: 'No movie or series of your libraries is known yet: open a library in the web player first.',
      nothing: 'This title has no rating and no new episode: its poster shows no badge.',
    },
    posterTemplate: 'Poster address template',
    posterTemplateHelp:
      'Optional. The address movies’ and series’ posters are downloaded from in place of the addons’ ones, with {imdb_id}, {tmdb_id}, {type}, movie or series, and {tmdb_type}, movie or tv, filled in. A title lacking an identifier the address names, or whose poster cannot be downloaded there, keeps its addon’s. Local folders’ posters and uploaded ones still come first.',
    lyrics: 'Song lyrics from LRCLIB',
    lyricsHelp:
      'Apps show the lyrics of songs, synced line by line when LRCLIB, a free lyrics database, has them. Polyfin looks a song up once, by its artist, title, album and length, when it starts playing or its details or lyrics open. When off, songs have no lyrics and LRCLIB is never asked.',
    evenVolume: 'Even the volume between songs',
    evenVolumeHelp:
      'Apps raise or lower each song, or each album, to the same loudness, from the ReplayGain tags of local songs or their measured loudness. The songs themselves are not changed. When off, apps get no gain and nothing is measured.',
    measureLoudness: 'Measure the loudness of songs without a gain',
    measureLoudnessHelp:
      'FFmpeg measures the loudness of the songs that have no ReplayGain tag, two at a time at the lowest priority, once each: local songs after a scan, addon songs after their first play. Songs longer than 15 minutes and audiobooks are never measured.',
    measureLoudnessChoices: { off: 'Off', local: 'Local songs', all: 'Local and addon songs' },
    playedPercent: 'Marked played after (%)',
    playedPercentHelp: (r: RangeText) =>
      `A title is marked played once playback goes past this share of its length. From ${r.min} to ${r.max}; ${r.default} by default, like Jellyfin.`,
    resumePercent: 'Resume point kept after (%)',
    resumePercentHelp: (r: RangeText) =>
      `Where playback stopped is kept, to resume from there, once past this share of a title’s length. It must be lower than the played threshold. From ${r.min} to ${r.max}; ${r.default} by default, like Jellyfin. A title shorter than 5 minutes is marked played as soon as it is past this point.`,
    versionListMinutes: 'Keep version lists for (minutes)',
    versionListMinutesHelp: (r: RangeText) =>
      `How long Polyfin uses the versions and subtitles the addons list for a title before asking them again. An older list still shows at once when the title is opened again, while Polyfin asks. Keeping lists longer sends fewer requests to the stream addon, which helps with providers that refuse too many requests, but new versions show up later. From ${r.min} to ${r.max}; ${r.default} by default.`,
    catalogRefreshMinutes: 'Refresh catalogs after (minutes)',
    catalogRefreshMinutesHelp: (r: RangeText) =>
      `How old a catalog page Polyfin read from an addon, the Live TV guide included, may get before Polyfin reads it again. Apps never wait for it: an older page still shows at once while Polyfin reads it again in the background, and it is kept a day more. A longer time sends fewer requests to the addons, but new titles show up later. From ${r.min} to ${r.max} (one day); ${r.default} by default.`,
    collectionReadHour: 'Read every collection each day',
    collectionReadHourNever: 'Never',
    collectionReadHourHelp:
      'Reads every collection of the server’s collection libraries at this hour, one after the other, then the catalogs of the server’s smart lists, so that they open at once. It asks the addons for many pages: leave it off for an addon you share with others.',
    collectionLibraryApps: 'Apps that show collection libraries by collection',
    collectionLibraryAppsHelp:
      'Collection libraries are told to apps as libraries of mixed content, so that jellyfin-web and Streamyfin give them a row on the home screen. The apps ticked are told they hold collections, as Jellyfin’s Collections view does: apps that list a library by its type then show its collections, and a collection’s titles once it is opened, rather than the titles of all its collections together. jellyfin-web and Streamyfin leave such libraries out of their home screen rows, so do not tick them. The list shows the apps that have signed in to this server.',
    collectionLibraryAppsNone: 'No app has signed in to this server yet.',
    localScanHours: 'Scan local folders every (hours, 0 = never)',
    localScanHoursHelp: (r: RangeText) =>
      `How often each local folder is scanned again for files added, changed or removed. Folders are also scanned at startup, when you ask, and when watched ones change. 0 turns the schedule off. From ${r.min} to ${r.max}; ${r.default} by default.`,
    watchlistCheckHours: 'Look for versions of watchlist titles every (hours, 0 = never)',
    watchlistCheckHoursHelp: (r: RangeText) =>
      `How often each title of a user’s Watchlist they have not started is checked again for versions, for the Now available and Still waiting home rows and the message telling a user of a title found available. Each hour, up to 20 titles of each user are checked, asking their stream addons. 0 turns the checks off. From ${r.min} to ${r.max}; ${r.default} by default.`,
    watchLocalFolders: 'Watch folders and shares for changes',
    watchLocalFoldersHelp: {
      container:
        'Scans a folder in the container again a few seconds after its files stop changing, so that files added, renamed or removed show within a minute.',
      computer:
        'Scans a folder of the computer Polyfin runs on again a few seconds after its files stop changing, so that files added, renamed or removed show within a minute.',
    },
    watchSharesHelp:
      'An SMB share is scanned again a few seconds after its server tells of a change. WebDAV folders are listed again every 2 minutes, S3 buckets, SFTP folders and rclone remotes as often as each folder asks, a share of many folders over several rounds, and scanned again when a listing changed. System › Health tells how each share is watched, and which folders or shares cannot be, and why.',
    personalAddons: 'Allow users’ own addons',
    personalAddonsHelp:
      'Lets users add Stremio addons of their own, besides the server’s. When off, their addons are kept but not used, and their Jellyfin apps show the server’s addons only.',
    serverImports: 'Users may import their watch history from another server or app',
    serverImportsHelp:
      'Under My account, each user can import their played movies and episodes, resume points and favorites from a Jellyfin or Emby server, or from their Stremio or Nuvio account, signing in there as themselves. When off, those sections are hidden. An administrator can still import for a user, under Users.',
    importEveryMinutes: 'Import connected accounts every',
    importEveryMinutesHelp:
      'How often Polyfin imports again the Stremio and Nuvio accounts users stay connected to, and the watch histories they import from tracking services. A change applies at once: each next import comes this long after the last one. Each import reads the whole account again, so a short interval sends more requests to those services, and, for Stremio, whose terms forbid automated extraction, makes that grey area more pronounced. 6 hours by default.',
    shareLinks: 'Allow share links',
    shareLinksHelp:
      'Users with Can share titles, on their page under Users, may give a movie or an episode to someone without an account, through a link that expires after 1 hour to 30 days. The guest watches in a browser, at 1080p at most and within the user’s quality group, and counts among the user’s playbacks at once. When off, no link can be made and every link is refused, without being deleted. Off by default.',
    importEveryChoice: (minutes: number) => (minutes < 60 ? `${minutes} min` : `${minutes / 60} h`),
    loginAttempts: 'Block an account after this many wrong passwords (0 = never)',
    loginAttemptsHelp: (r: RangeText) =>
      `After this many wrong passwords in a row, the account cannot sign in for 15 minutes, even with the right password. An administrator can unblock it sooner on the Users page. 0, or from ${r.min} to ${r.max}.`,
    twoStepAdministrators: 'Require two-step sign-in for administrators',
    twoStepAdministratorsHelp:
      'Administrators sign in to this admin app with a code from an authenticator app after their password. One who has not set it up does so right after their password, before getting a session. Their password alone then signs in to no Jellyfin app: they use Quick Connect, approved here. The sessions and devices signed in already stay signed in. Off by default.',
    inactiveDeviceDays: 'Sign out devices unused for (days, 0 = never)',
    inactiveDeviceDaysHelp: (r: RangeText) =>
      `Jellyfin apps not used for this many days are signed out and must sign in again. This is checked every hour. Signing in to this admin page is not affected. From ${r.min} to ${r.max}.`,
    knownProxies: 'Known proxies',
    knownProxiesHelp: (r: RangeText) =>
      `The reverse proxies in front of Polyfin, one a line: an IP address, a network such as 172.18.0.0/16, or a host name such as a container’s name on a Docker network (traefik), up to ${r.max}. A name is looked up every minute and each address it resolves to is trusted, so list only names your own network resolves. A request from one of them comes from the client its X-Forwarded-For or X-Real-IP header names: that address limits failed sign-ins, shows on devices and tells the local network. Forwarded headers from any other address are ignored. Leave it empty when nothing stands in front of Polyfin.`,
    /** Settings › Security › Identity provider: signing in through an OpenID Connect provider. */
    provider: {
      title: 'Identity provider',
      intro:
        'People can sign in to this admin app and to the web player with an OpenID Connect provider, which then asks for its own two-step sign-in: Polyfin does not ask for its code again. Jellyfin apps that cannot open the provider sign in with Quick Connect, approved from a session opened through it. Register Polyfin at the provider as a confidential client, or a public one without a secret, using the redirect URI below.',
      redirectUri: 'Redirect URI',
      redirectUriHelp:
        'The address the provider sends browsers back to, built from the public address. Register it at the provider exactly as shown.',
      noPublicAddress:
        'Set the public address under Settings › Notifications first: the sign-in starts and ends there, and the provider sends browsers back to it.',
      issuer: 'Issuer',
      issuerHelp:
        'The provider’s issuer, exactly as its discovery document gives it, such as https://id.example.org or https://id.example.org/realms/home. Polyfin reads its /.well-known/openid-configuration. Empty turns the sign-in off.',
      name: 'Button name',
      nameHelp:
        'What the sign-in buttons say after “Sign in with”. Empty shows the issuer’s host name.',
      clientId: 'Client ID',
      clientIdHelp: 'The ID the provider gave Polyfin’s client. Empty turns the sign-in off.',
      clientSecret: 'Client secret',
      clientSecretHelp:
        'The secret of a confidential client; leave it empty for a public client. It is kept encrypted with POLYFIN_SECRET_KEY and never shown again.',
      userClaim: 'Claim naming the user',
      userClaimHelp:
        'The claim of the ID token compared with account names, without regard to case, the first time someone signs in: preferred_username by default. The verified email comes next. Later sign-ins find the same account whatever the claim says.',
      createAccounts: 'Create accounts',
      createAccountsHelp:
        'Someone no account matches gets one, named after the claim, else the email, with the permissions of a new account, signing in through the provider only. Off, they are refused. Off by default.',
      adminClaim: 'Administrators’ claim',
      adminClaimHelp:
        'The claim listing someone’s groups or roles, such as groups. With a value below, someone whose claim holds it becomes an administrator as they sign in. Polyfin asks for the groups scope when the claim is groups and the provider offers it.',
      adminValue: 'Administrators’ value',
      adminValueHelp:
        'The group or role that makes administrators, such as polyfin-admins. Leaving it later does not make an administrator a member again: change that on the Users page.',
    },
    https: {
      title: 'HTTPS',
      intro:
        'Polyfin can serve HTTPS itself, on a port of its own, without a reverse proxy in front of it. HTTP keeps working on its port whatever happens to HTTPS: a port that cannot be opened, or a certificate that cannot be read or renewed, shows on System › Health. Changes apply at once, without a restart.',
      mode: 'Serve HTTPS',
      modeHelp:
        'Certificate files: a certificate another tool obtains, read again when it changes. Let’s Encrypt: a certificate Polyfin obtains and renews itself, for public host names that point at this server. Off by default.',
      modes: { off: 'Off', files: 'Certificate files', letsencrypt: 'Let’s Encrypt' },
      port: 'HTTPS port',
      portHelp: (r: RangeText) =>
        `The TCP port HTTPS listens on in the container: publish it, as 8096 is for HTTP. The container’s user cannot listen below 1024: to reach Polyfin on 443, forward 443 to this port on the router or in the container’s ports, and set the Public address, under Settings › Notifications, to https:// and the host name, without a port. From ${r.min} to ${r.max}; ${r.default} by default, as Jellyfin.`,
      redirect: 'Redirect HTTP to HTTPS',
      redirectHelp:
        'Requests from the internet that come over HTTP get a permanent redirect (308) to the same address over HTTPS, on the port the internet reaches HTTPS at: the port of the Public address, under Settings › Notifications, when it starts with https://, 443 when it names none, else the HTTPS port. The local network, Let’s Encrypt’s checks, and requests a known proxy received over HTTPS stay on HTTP. It applies only while HTTPS is served. Off by default.',
      certificate: 'Certificate',
      certificateHelp:
        'The path in the container of the PEM file of the certificate with its chain, such as /certs/fullchain.pem. Saving checks that it can be read. Polyfin reads it again within a minute of a change, and keeps the last good certificate while a change cannot be read.',
      privateKey: 'Private key',
      privateKeyHelp:
        'The path in the container of the certificate’s RSA or ECDSA private key, as a PEM file, such as /certs/privkey.pem. Saving checks that it belongs to the certificate.',
      hosts: 'Host names',
      hostsHelp: (r: RangeText) =>
        `The public names that point at this server, one a line, such as media.example.org, up to ${r.max}. One certificate holds them all. Names that resolve only on the local network, and wildcards, need the DNS-01 challenge, which Polyfin does not answer: use certificate files from another tool for them.`,
      email: 'Email',
      emailHelp: 'Optional. Let’s Encrypt may write to this address about the account.',
      terms: 'I agree to Let’s Encrypt’s terms of service',
      termsHelp:
        'Let’s Encrypt issues certificates only once its terms of service, at letsencrypt.org/repository, are agreed to.',
      challenge: 'Challenge',
      challengeHelp:
        'How Let’s Encrypt checks that the names point at this server. HTTP-01: Polyfin answers on its HTTP port, which the internet’s port 80 must reach. TLS-ALPN-01: Polyfin answers on its HTTPS port, which the internet’s port 443 must reach. The certificate, its key and the account’s key are kept in the database, the keys encrypted with POLYFIN_SECRET_KEY, and the certificate is renewed once a third of its lifetime remains.',
      challenges: {
        'http-01': 'HTTP-01 (port 80 to the HTTP port)',
        'tls-alpn-01': 'TLS-ALPN-01 (port 443 to the HTTPS port)',
      },
      useAddress: (address: string) => `Use ${address}`,
      offeredAddress:
        'The addresses Let’s Encrypt serves HTTPS at, under Settings › Security: without a port once the router forwards 443 to the HTTPS port. Redirect HTTP to HTTPS goes to the port of the address chosen.',
    },
    detailedLog: 'Detailed log (to diagnose a problem)',
    detailedLogHelp:
      'Polyfin writes much more to its log, at once and without a restart. Turn it off once the problem is found.',
    saved: 'Settings saved.',
    thumbnailsHelp:
      'Images made from the titles themselves, after they are watched. To make them, Polyfin reads small parts of the file from the source, at a gentle pace, once nobody is playing from that source: at most 60 requests per title, one every 3 seconds, and 120 an hour per source, never the whole file. A source that asks Polyfin to slow down gets no request for images for 2 hours. Off by default.',
    trickplay: 'Thumbnails when moving through a title',
    trickplayHelp:
      'Apps show a small image of the moment the user moves to in the playback bar. Polyfin makes them once a title has been watched, and for the next episode when playback is prepared in advance. Long movies get coarser thumbnails: a few minutes apart rather than a few seconds.',
    trickplayInterval: 'One thumbnail every (seconds, at least)',
    trickplayIntervalHelp: (r: RangeText) =>
      `How often the playback bar changes image. Polyfin reads at most 60 images per title, so a title longer than about 10 minutes gets one image per step, its steps spread evenly over its runtime: about 1 minute apart for a 1-hour episode, 2 minutes for a 2-hour movie. From ${r.min} to ${r.max}; ${r.default} by default, like Jellyfin.`,
    trickplayWidth: 'Thumbnail width',
    trickplayWidthHelp:
      'Wider thumbnails look sharper on large screens, but take more space. Changing it makes new thumbnails the next time a title is played.',
    pixels: (width: number) => `${width} pixels`,
    chapterImages: 'Chapter images',
    chapterImagesHelp:
      'Apps show an image for each chapter of a title, in its list of scenes. Polyfin uses the image it read nearest each chapter’s start, from the same reads as the thumbnails, once the title has been watched.',
    thumbnailStorage: 'Space for images (GB)',
    thumbnailStorageHelp: (r: RangeText) =>
      `Thumbnails and chapter images are kept in the database. Past this size, the images of the titles watched longest ago are removed; they are made again if the title is played again. From ${r.min} to ${r.max}; ${r.default} by default.`,
    cacheSize: 'Disk space for files being read (GB)',
    cacheSizeHelp: (r: RangeText) =>
      `Space Polyfin may use to keep the parts of the files being played, so that seeks and restarts do not download them again. Parts read in the last 30 seconds are kept even above it. From ${r.min} to ${r.max} GB; ${r.default} by default.`,
    workFolders: {
      conversionFolder: 'Conversion folder',
      conversionFolderHelp:
        'Where the segments of the files converted or repackaged are written while they play: the busiest writes, which a fast disk or a memory folder suits. It needs room for the segments of every conversion running.',
      readingFolder: 'Folder for files being read',
      readingFolderHelp:
        'Where the parts of the addons’ files and the torrents’ pieces being played are kept, within the disk space above: often several GB, which a large disk suits.',
      cacheFolder: 'Cache folder',
      cacheFolderHelp:
        'Where the relayed images are kept, up to 1 GB, from one start to the next, with the guides and the picture subtitles being read.',
      where: {
        container:
          'A folder of Polyfin’s container, where a disk or a memory folder is mounted; empty, the cache in its data volume, shown. Polyfin must be able to write to it. A new folder applies when Polyfin starts again; a former one is left as it is.',
        computer:
          'A folder of the computer Polyfin runs on; empty, the cache in its data folder, shown. Polyfin must be able to write to it. A new folder applies when Polyfin starts again; a former one is left as it is.',
      },
    },
    recordingsTitle: 'Recordings',
    recordingsFolder: (folder: string) => `Recordings are saved in ${folder}.`,
    recording: 'Record Live TV',
    recordingHelp:
      'Lets users schedule recordings of Live TV programmes and series, written to the folder below.',
    recordingsFolderLabel: 'Recordings folder',
    recordingsFolderHelp: {
      container:
        'A folder of Polyfin’s container, the one shown when empty, in its data volume. To keep recordings on another disk, mount it there, or elsewhere and enter its path here. Polyfin must be able to write to it. Changing it does not move the recordings already made.',
      computer:
        'A folder of the computer Polyfin runs on, the one shown when empty, in its data folder. To keep recordings on another disk, enter a folder of that disk here. Polyfin must be able to write to it. Changing it does not move the recordings already made.',
    },
    recordingPrePadding: 'Start recordings early (minutes)',
    recordingPrePaddingHelp: (r: RangeText) =>
      `How many minutes before the programme a recording starts. From ${r.min} to ${r.max}; ${r.default} by default.`,
    recordingPostPadding: 'Keep recording after the end (minutes)',
    recordingPostPaddingHelp: (r: RangeText) =>
      `How many minutes after the programme a recording goes on. From ${r.min} to ${r.max}; ${r.default} by default.`,
    recordingRetentionDays: 'Keep recordings for (days, 0 = forever)',
    recordingRetentionDaysHelp: (r: RangeText) =>
      `Recordings older than this are deleted. This is checked every day. From ${r.min} to ${r.max}.`,
    recordingCommercials: 'Find commercials in recordings',
    recordingCommercialsHelp:
      'Once a recording ends, Polyfin reads it in the background, one recording at a time, for the moments that are both black and silent, and takes the short pieces between them as commercial breaks. Apps then offer to skip them, as each user chose in their app’s playback settings, while the skip intro and credits buttons are on. Recordings longer than 4 hours are not read.',
    convertRecordings: 'Convert finished recordings to HEVC',
    convertRecordingsHelp:
      'Finished recordings are converted to HEVC, one at a time, in the hours below, so that they take less space. The picture keeps its size; the audio and subtitles are copied as they are. The conversion uses the "Graphics card used to convert", with the "HEVC quality" and the "Deinterlacing method" of Conversion (transcoding), and counts as one of the "Video conversions at once": it waits while playbacks fill them. It starts only with free space for a copy as large as the recording. The copy replaces the recording only if it lasts as long, holds the same tracks, decodes to its end and is smaller; otherwise it is deleted, and the recording is kept as it is and not tried again. A recording someone is playing is replaced once they stop; its played state, resume points and commercials stay. Apps that do not play HEVC get the recording converted when they play it: with "Conversion (transcoding)" off, they cannot play it. Off by default.',
    convertRecordingsBetween: 'Convert between',
    convertRecordingsAnd: 'and',
    convertRecordingsBetweenHelp: (from: string, until: string) =>
      `In the server’s time zone. A conversion still running at the end stops, and starts over the next night. ${from} to ${until} by default.`,
    convertRecordingsFrom: 'Start of the conversions',
    convertRecordingsUntil: 'End of the conversions',
    convertRecordingsOnProcessor: 'Use the processor when there is no graphics card',
    convertRecordingsOnProcessorHelp:
      'Without a graphics card that encodes HEVC, recordings are converted on the processor with x265, at its lowest priority. It is slow, and keeps the processor busy through the hours above. Without a graphics card and with this off, nothing is converted. Off by default.',
    liveTvRefreshHours: 'Refresh Live TV lists and guides every (hours)',
    liveTvRefreshHoursHelp: (r: RangeText) =>
      `How often the IPTV channel lists and the XMLTV programme guides are downloaded again. A download that failed is tried again sooner: after 5 minutes, 15 minutes, then every hour. From ${r.min} to ${r.max}; ${r.default} by default.`,
    backupsFolder: (folder: string) => `Backups of the database are saved in ${folder}.`,
    backups: 'Back up the database every day',
    backupsHelp:
      'Writes a copy of the database to the folder below each day, at the hour chosen, and deletes the oldest past the number kept.',
    backupFolderLabel: 'Backups folder',
    backupFolderHelp: {
      container:
        'A folder of Polyfin’s container, the one shown when empty, in its data volume. Backups are safer on another disk than the database’s: mount one there, or elsewhere and enter its path here. Polyfin must be able to write to it.',
      computer:
        'A folder of the computer Polyfin runs on, the one shown when empty, in its data folder. Backups are safer on another disk than the database’s: enter a folder of one here. Polyfin must be able to write to it.',
    },
    backupHour: 'Back up every day at',
    backupHourHelp: (hour: string) => `In the server’s time zone. ${hour} by default.`,
    backupsKept: 'Backups to keep',
    backupsKeptHelp: (r: RangeText) =>
      `After each backup, Polyfin deletes its oldest backups past this number. Other files in the folder are never touched. From ${r.min} to ${r.max}; ${r.default} by default.`,
    backupPassphrase: 'Backup passphrase',
    backupPassphraseHelp: (min: number) =>
      `Encrypts each backup as it is written, in the folder and at every destination, which then cannot read it. At least ${min} characters. It is kept encrypted and never shown again.`,
    backupPassphraseNew:
      'Keep this passphrase somewhere other than this server, such as a password manager: a lost passphrase means lost backups. Nobody can restore them without it, Polyfin included. Check it with the eye before saving. Backups already made stay as they are.',
    backupPassphraseRemoved:
      'Backups made from now on are not encrypted. Those already made still need this passphrase to be restored.',
    lastBackup: 'Last backup',
    backupDestinations: 'Destinations',
    backupDestinationsHelp:
      'Each backup is also sent to these places, under the same file name, once written to the folder: S3-compatible storage, SFTP or WebDAV servers. A failed upload is tried again at the next backup.',
    webPlayerHelp:
      'What the web player (jellyfin-web, at /web/) shows besides its own pages. The theme and the script apply the next time a page of the web player is loaded; the web player keeps the sign-in message for up to a minute.',
    openWebPlayer: 'Open the web player',
    webTheme: 'Server theme',
    webThemeHelp:
      'The web player’s look for the users who keep the server’s; each user can have their own, on their page under Users. LumaaGlaass is the theme Polyfin bundles. jellyfin-web’s own look has no custom CSS or script, and each user picks its display themes under Settings › Display. Custom uses your own CSS and JavaScript below, which are kept while another theme is chosen.',
    webThemeChoices: {
      lumaaglaass: 'LumaaGlaass',
      none: 'jellyfin-web’s own look',
      custom: 'Custom',
    },
    webThemeReload: 'After a change, reload the web player’s open pages.',
    webThemeTexts: {
      lumaaglaass:
        'What Polyfin serves for LumaaGlaass, shown here and not editable. Choose Custom, or Edit as Custom, to write your own.',
      none: 'jellyfin-web’s own look serves no custom CSS or JavaScript. Choose Custom to write your own.',
    },
    editAsCustom: 'Edit as Custom',
    editAsCustomTitle: 'Replace your Custom texts?',
    editAsCustomConfirm:
      'Your Custom CSS and JavaScript are replaced with LumaaGlaass’s, and the theme becomes Custom. Saving the page keeps the change.',
    themeCopy: 'Keep a copy in Polyfin',
    themeCopyHelp: (depth: number, file: string, total: string, files: number) =>
      `Saves the stylesheets, scripts, fonts and images the Custom texts load from elsewhere, such as a theme on a CDN, in Polyfin’s data folder, and has the texts load them from Polyfin: the theme then works without reaching its CDN. It follows @import, url() and the addresses scripts name, ${depth} levels down, from addresses on the internet only, up to ${file} a file, and ${total} and ${files} files in all.`,
    themeCopySaved: (files: number, size: string, date: string) =>
      `A copy of ${files === 1 ? 'one file' : `${files} files`}, ${size}, saved ${date}.`,
    themeCopyFiles: 'Files saved',
    themeCopyUnsaved: (count: number) =>
      count === 1
        ? 'One address could not be saved; it is still loaded from elsewhere:'
        : `${count} addresses could not be saved; they are still loaded from elsewhere:`,
    themeCopyMissing: (count: number) =>
      `${count === 1 ? 'One file' : `${count} files`} of the copy ${count === 1 ? 'is' : 'are'} missing from Polyfin’s data folder, as after restoring a backup on another system: browsers load ${count === 1 ? 'it' : 'them'} from ${count === 1 ? 'its' : 'their'} original address meanwhile, which needs the internet. Update the copy to download ${count === 1 ? 'it' : 'them'} again.`,
    themeCopyOutside: (count: number) =>
      count === 1
        ? 'The Custom texts load one address from elsewhere:'
        : `The Custom texts load ${count} addresses from elsewhere:`,
    themeCopyStillOutside: (count: number) =>
      count === 1
        ? 'One address added since the copy is loaded from elsewhere; update the copy to save it too:'
        : `${count} addresses added since the copy are loaded from elsewhere; update the copy to save them too:`,
    themeCopyNothing: 'The Custom texts load nothing from elsewhere.',
    themeCopyKeep: 'Keep a copy in Polyfin',
    themeCopyUpdate: 'Update the copy',
    themeCopyRemove: 'Remove the copy',
    themeCopyRemoveTitle: 'Remove the copy?',
    themeCopyRemoveConfirm:
      'Its files are deleted, and the Custom texts load them from their original addresses again.',
    themeCopyKept: 'Copy saved. Reload the web player’s open pages a minute from now.',
    themeCopyRemoved: 'Copy removed. Reload the web player’s open pages a minute from now.',
    themeCopyFailed: 'No file could be saved, so no copy was made:',
    themeCopySaveFirst: 'Save your changes first: the copy is made from the saved texts.',
    themeCopyKinds: { stylesheet: 'stylesheet', script: 'script', font: 'font', image: 'image' },
    themeCopyReasons: {
      private_address: 'on a local network, or redirected there',
      not_http: 'redirected to an address neither http nor https',
      unavailable: 'not available',
      timeout: 'took too long',
      wrong_type: 'not a file of the kind it is loaded as',
      too_large: 'larger than the limit for one file',
      total_too_large: 'past the limit for the whole copy',
      too_many_files: 'past the limit of files',
      too_deep: 'named too many levels down',
      built_at_run_time: 'completed by a script when it runs, which no copy can follow',
    },
    customCss: 'Custom CSS',
    customCssHelp:
      'Applied to every page of the web player, for every user, as Jellyfin’s custom CSS. Users can turn it off under Settings › Display.',
    customJs: 'Custom JavaScript',
    customJsHelp:
      'Loaded by every page of the web player, after its own scripts. Empty loads nothing.',
    customJsWarningTitle: 'Only paste code you trust',
    customJsWarning:
      'This script runs in the browser of every user who opens the web player on this server, with their account. It can read what they see and act for them.',
    loginDisclaimer: 'Sign-in message',
    loginDisclaimerHelp:
      'Shown under the sign-in form of the web player. Plain text, Markdown or HTML; the web player removes unsafe HTML.',
    addVersionsToOpenPage: 'Add versions to an open title’s page',
    addVersionsToOpenPageHelp:
      'Adds the versions the addons find to the version menu of a movie’s or an episode’s page while it is open, keeping the version picked. When off, a page opened before any version was found still lists the first ones as they come, and its play buttons still wait for a version; the others show when the page is opened again.',
    posterRatings: 'Show ratings on posters',
    posterRatingsHelp:
      'Shows a movie’s or a series’ star rating over its poster in the web player as the pointer or the focus comes to it, with its critics’ and audience scores while an MDBList API key for ratings is set under Settings › Content. For every user. While Badges on posters is on under Settings › Content, the badges show instead.',
    homeRows: 'Home rows',
    homeRowsHelp:
      'Rows the web player adds to its home page, at their place, for every user or for chosen users. Each user can turn off a row under My account › Home rows; the web player’s own sections stay in its Settings › Home. Every row leaves out what the user may not see: titles above their parental control, blocked genres, hidden titles, and the libraries hidden from them.',
    homeRow: {
      loading: 'Loading the home rows…',
      empty: 'No home row',
      emptyHint: 'The web player’s home page shows its own sections only.',
      add: 'Add a row',
      addTitle: 'Add a home row',
      editTitle: (name: string) => `Edit ${name}`,
      edit: 'Edit',
      delete: 'Delete',
      deleteTitle: (name: string) => `Delete ${name}?`,
      deleteBody: 'The row leaves every user’s home page, and its library leaves other apps.',
      deleted: (name: string) => `${name} is deleted.`,
      saved: (name: string) => `${name} is saved.`,
      create: 'Add',
      creating: 'Adding…',
      save: 'Save',
      saving: 'Saving…',
      name: 'Name',
      nameHelp: 'The row’s title on the home page.',
      becauseNameHelp:
        'Each row is named after it, followed by the title it follows: “Because you watched Example Movie”.',
      kind: 'Kind',
      kinds: {
        catalog: 'A catalog',
        collection: 'A collection',
        because: 'Because you watched',
        new: 'New in your libraries',
        available: 'Now available',
        waiting: 'Still waiting',
        trakt: 'Recommended on Trakt',
        picks: 'Top picks for you',
      },
      kindHelp: {
        catalog:
          'The first page of a catalog of the server’s addons, a library or not, such as a trending one. Users who do not use the server’s addons, or from whom its library is hidden, do not see it.',
        collection:
          'The titles of a collection: one made by users, a smart list shown as a collection, or an addon’s collection.',
        because:
          'One row for each of the last movies and series the user played, of the titles similar to it, as movie recommendations give them. None while Similar titles is off.',
        new: 'The titles that came into the catalogs of the user’s libraries in the last 7 days, newest first. Polyfin counts them from the time it first saw each title in each catalog.',
        available:
          'The titles of the user’s Watchlist they have not started that the last check found versions for, the latest found first. A version found has not been tried yet: it may still fail to play.',
        waiting:
          'The titles of the user’s Watchlist they have not started that the last check found no version for, or that are not out yet, the latest added first. A title not checked yet is in neither row.',
        trakt:
          'The movies and series Trakt recommends to the user, read with their Trakt connection every 12 hours, without those in their Trakt collection or Watchlist and those they played. Only users who connected Trakt in My account › Tracking see it; the first time, it shows once Trakt has answered.',
        picks:
          'The movies and series of the user’s libraries, smart lists included, that their taste profile ranks first, without those they played or of a genre they dislike. Until the profile knows enough of them, the libraries’ first titles, the most popular first. Users see their profile in My account.',
      },
      defaultNames: {
        because: 'Because you watched',
        new: 'New in your libraries',
        available: 'Now available',
        waiting: 'Still waiting',
        trakt: 'Recommended on Trakt',
        picks: 'Top picks for you',
      },
      catalog: 'Catalog',
      noCatalog: 'The server’s addons have no catalog to show.',
      collection: 'Collection',
      noCollection: 'There is no collection yet.',
      collectionSources: {
        users: 'Made by users',
        smart: 'Smart list',
        addon: 'Addon',
      },
      choose: 'Choose…',
      unknownCatalog: 'A catalog no longer offered',
      unknownCollection: 'A collection no longer offered',
      count: 'Rows',
      countHelp: 'How many of the last movies and series played get a row, from 1 to 10.',
      place: 'Place',
      places: {
        top: 'At the top',
        resume: 'After Continue Watching',
        nextup: 'After Next Up',
        bottom: 'At the bottom',
      },
      placeHelp:
        'Where the web player shows it. When a user’s home page does not show that section, the row takes the place before.',
      shownTo: 'Shown to',
      everyone: 'Every user',
      chosenUsers: 'Chosen users',
      users: 'Users',
      noUser: 'Choose at least one user.',
      shownToUsers: (count: number) => (count === 1 ? '1 user' : `${count} users`),
      rowsCount: (count: number) => (count === 1 ? '1 row' : `${count} rows`),
      library: 'Also a library in other apps',
      libraryHelp:
        'Other apps cannot be given a new kind of home section: this makes the row a library too, without type, which they list among the user’s libraries, with a row of its latest titles on their home.',
      libraryBadge: 'Library',
      moveLabel: 'Move',
    },
    codeSize: (used: number, max: number) =>
      `${used} KB of ${max >= 1024 ? `${max / 1024} MB` : `${max} KB`}`,
    codeKeys: 'Tab inserts spaces; press Escape, then Tab, to leave the field.',
  },
  settingsPage: {
    searchHint: 'By name or description.',
    noMatch: 'No setting matches this search.',
    sections: {
      variables: 'Environment variables',
    },
    variablesHelp:
      'Read only. These are set on the container (Docker environment, compose file or Unraid template) and apply when Polyfin starts: change them there, then restart the container. The variables that used to set other options are copied into the settings once, at the first start of this version, and are no longer read: change those in the settings. Secrets are hidden, and the database address shows its host and database only.',
    value: 'Value in effect',
    defaultValue: 'Default',
    setValue: 'Set',
    hidden: 'Hidden',
    empty: 'Empty',
    notRead: 'Not read by Polyfin',
    searchPlaceholder: 'Search a setting',
    results: (count: number) => (count === 1 ? '1 setting found' : `${count} settings found`),
    noMatchHelp: 'Try another word, such as the name of a variable or a service.',
    variableHint: 'Environment variable',
    ledes: {
      general:
        'The server’s name, its language, how apps sign in and find it, and the check for new versions.',
      playback: 'How Polyfin picks and prepares the versions it plays.',
      content:
        'Skip intro buttons, similar titles, generated images, ratings, song lyrics, even volume between songs and playback thresholds.',
      catalogs: 'How much Polyfin reads from the addons’ catalogs, and how often.',
      security:
        'Users’ own addons and imports, blocked accounts, unused devices and reverse proxies.',
      liveTv: 'How often the IPTV channel lists and guides are downloaded again.',
      diagnostics: 'The detailed log, and the environment variables in effect.',
      notifications:
        'Where Polyfin tells, for every user, of new episodes, recordings, playbacks and connections that stop, and of Health problems, new users, new versions and blocked accounts, and sends the weekly summary and the month and year in review.',
    },
    groups: {
      skip: 'Skip intro and credits',
      titlePages: 'Title pages',
      images: 'Images',
      ratings: 'Ratings',
      posters: 'Posters',
      music: 'Music',
      thresholds: 'Playback thresholds',
    },
    leaveTitle: 'Leave without saving?',
    leaveBody: 'The changes made in this section are not saved yet. They are lost if you leave.',
    leave: 'Leave without saving',
    stay: 'Stay',
    variablesEmpty: 'No variable to show',
    variablesEmptyHelp: 'Set POLYFIN_ variables on the container, then restart it.',
  },
}

export default settings
