/** System: health, schedule, logs and API keys. */
const system = {
  apiKeys: {
    title: 'API keys',
    description:
      'Keys let tools and apps, such as request managers or scripts, use the Jellyfin API of this server with administrator rights. Anyone who has a key has these rights: keep keys private and revoke the ones you no longer use.',
    listTitle: 'Keys',
    empty: 'No API key yet.',
    created: 'Created',
    lastUsed: 'Last used',
    createTitle: 'Create a key',
    app: 'App name',
    appHint: 'Who will use this key, to recognize it later. 1 to 64 characters.',
    create: 'Create key',
    creating: 'Creating…',
    newKeyTitle: (app: string) => `Key for ${app}`,
    newKeyNotice: 'Copy this key now: it will not be shown again.',
    copy: 'Copy',
    copied: 'Copied.',
    copyFailed: 'The key could not be copied. Select it and copy it by hand.',
    done: 'I have copied it',
    revoke: 'Revoke',
    revoking: 'Revoking…',
    revokeConfirm: (app: string) =>
      `Revoke the key for ${app}? Tools and apps using it will stop working. This cannot be undone.`,
    revoked: (app: string) => `The key for ${app} has been revoked.`,
    emptyHint: 'Create one below for each tool or app, so each can be revoked on its own.',
    revokeTitle: 'Revoke this key?',
    revokeLabel: (app: string) => `Revoke the key for ${app}`,
  },
  system: {
    schedule: {
      title: 'Schedule',
      description: 'The server’s tasks, the recordings ahead, and the next Live TV refreshes.',
      tasksTitle: 'Tasks',
      task: 'Task',
      lastRun: 'Last run',
      nextRun: 'Next run',
      notRunYet: 'Not run since the server started',
      byHandOnly: 'Run by hand only',
      every: (span: string) => `Interval: ${span}`,
      daily: (time: string) => `Every day at ${time}, server time`,
      took: (span: string) => `took ${span}`,
      results: {
        Completed: 'Completed',
        Failed: 'Failed',
        Cancelled: 'Stopped',
      } as Record<string, string>,
      running: 'Running',
      cancelling: 'Stopping',
      runNow: 'Run now',
      runLabel: (name: string) => `Run ${name} now`,
      stop: 'Stop',
      stopLabel: (name: string) => `Stop ${name}`,
      started: (name: string) => `${name} started.`,
      autoRefresh: 'Updates every 5 seconds.',
      recordingsTitle: 'Upcoming recordings',
      recordingsOff: 'Recording is off. Turn it on under Settings › Recordings.',
      noTimers: 'No recording is scheduled.',
      recordingNow: 'Recording',
      failed: 'Failed',
      series: 'Series',
      scheduledBy: (name: string) => `Scheduled by ${name}`,
      unknownChannel: 'Channel not found',
      tunersShort: 'Not enough tuners',
      tunersShortHelp:
        'More recordings on this tuner’s channels than it has tuners run at the same time: one of them may fail.',
      refreshesTitle: 'Live TV refreshes',
      refreshesHelp: (hours: number) =>
        `IPTV channel lists and programme guides, the server’s and users’ own, are downloaded again every ${hours} h (Settings › Live TV), by the “Refresh Live TV guides” task.`,
      noRefreshes: 'No IPTV source or programme guide.',
      channelList: 'Channel list',
      iptvLists: 'Channel, movie and series lists',
      guide: 'Programme guide',
      due: 'At the next check',
      lastDownload: 'Last download',
      nextDownload: 'Next download',
      notYet: 'Not yet',
      noTasks: 'No task.',
      noTasksHint: 'The server lists its tasks here once it has started.',
      recordingsHint: 'Recordings scheduled from an app or from the guide appear here.',
      refreshesHint:
        'Add an IPTV source under Content › Sources, or a programme guide to a TV catalog, and their downloads appear here.',
      backupsTitle: 'Database backups',
      backUpNow: 'Back up now',
    },
    health: {
      title: 'Health',
      noIptv: 'No IPTV source.',
      owner: { server: 'Server', user: (name: string) => `${name}’s addons` },
      description:
        'How the server and its sources are doing, from what Polyfin already records: this page sends no request to them.',
      problemsTitle: 'Needs attention',
      allGood: 'No problem found.',
      update: (version: string) => `Polyfin ${version} is available`,
      updateRunning: (version: string) => `This server runs version ${version}.`,
      releaseNotes: 'Release notes',
      checkedAt: (time: string) => `Checked at ${time}. Updates every 10 seconds.`,
      ok: 'OK',
      warning: 'Warning',
      error: 'Error',
      problems: {
        database: 'The database does not answer.',
        disk: (folder: string, free: string) =>
          `Little space left for the ${folder}: ${free} free.`,
        addon: (name: string, failure: string) => `${name}: ${failure}`,
        iptv: (name: string) => `${name}: the last channel list download failed.`,
        tuner: (name: string) => `${name}: the tuner failed to play a channel.`,
        tunerUnreachable: (name: string) => `${name}: the tuner does not answer.`,
        folder: (name: string) =>
          `${name}: Polyfin cannot read this local folder. Check that it is mounted and readable by user 65532.`,
        share: (name: string) =>
          `${name}: Polyfin cannot read this network share. Check its address, and that its user may read it.`,
        shareUnreachable: (name: string) => `${name}: Polyfin cannot reach this network share.`,
        shareRefused: (name: string) =>
          `${name}: the network share refuses Polyfin’s user or password.`,
        shareHostKey: (name: string) =>
          `${name}: the SFTP server offers another host key than the one saved, and Polyfin refuses it until it is accepted.`,
        folderWatchLimit: (name: string) =>
          `${name}: Polyfin cannot watch this local folder for changes: the system’s limit of watched folders is reached. It is still scanned on its schedule. Raise fs.inotify.max_user_watches on the host.`,
        folderInstanceLimit: (name: string) =>
          `${name}: Polyfin cannot watch this local folder for changes: the system’s limit of watchers or of open files is reached. It is still scanned on its schedule. Raise fs.inotify.max_user_instances on the host, or the container’s limit of open files.`,
        folderOpenFilesLimit: (name: string) =>
          `${name}: Polyfin cannot watch this local folder for changes: the system’s limit of open files is reached, as macOS watches each folder and file through one. It is still scanned on its schedule. Raise kern.maxfilesperproc and kern.maxfiles with sysctl.`,
        folderUnwatched: (name: string) =>
          `${name}: Polyfin cannot watch this local folder for changes. It is still scanned on its schedule.`,
        shareUnwatchedUnreachable: (name: string) =>
          `${name}: Polyfin cannot watch this network share for changes: it cannot be reached. It is scanned on its schedule, and again once it answers.`,
        shareUnwatchedRefused: (name: string) =>
          `${name}: Polyfin cannot watch this network share for changes: it refuses Polyfin’s user or password. Check them, and enter them again.`,
        shareUnwatched: (name: string) =>
          `${name}: Polyfin cannot watch this network share for changes. It is still scanned on its schedule.`,
        folderNoSearch: (name: string) =>
          `${name}: no addon of the server describes and searches the titles of its files, so they stay unmatched. Add one under Content › Sources: users’ own addons, under My sources, do not count.`,
        folderSearch: (name: string, addon: string, failure: string) =>
          `${name}: the titles of its files could not be searched${addon === '' ? '' : failure === '' ? ` (${addon})` : ` (${addon}: ${failure})`}. They stay unmatched until a search goes through, which Polyfin tries again on its own, after a few minutes, then less often.`,
        searchLate: (name: string) =>
          `${name}: its search did not answer in time. Searches leave it out for a few minutes, then ask it again.`,
        guide: (name: string) => `${name}: the last programme guide fetch failed.`,
        paused: (host: string) => `Thumbnails paused for ${host}: it asked to slow down.`,
        unknownProxy: (address: string) =>
          `${address} forwards requests (X-Forwarded-For) but is not a known proxy, so its clients all count as ${address}. If it is your reverse proxy, add it to Known proxies under Settings › Security.`,
        discoveryPort: (port: number) =>
          `Polyfin cannot listen on UDP port ${port}, so Jellyfin apps on the local network do not find it by themselves. Another program probably uses it, such as a Jellyfin server on the same host. Saving the settings tries again.`,
        torrentRoute: (why: string) =>
          `Torrent streams do not play: ${why}. Nothing is sent another way; check the route under Settings › Torrents.`,
        torrentWhy: {
          interface_missing: (name: string) => `the network interface ${name} is missing`,
          interface_down: (name: string) => `the network interface ${name} is down`,
          interface_no_address: (name: string) => `the network interface ${name} has no address`,
          interface_unsupported: () => 'a network interface works on Linux only',
          proxy_unreachable: (name: string) => `the SOCKS5 proxy ${name} does not answer`,
          proxy_refused: (name: string) => `the SOCKS5 proxy ${name} refused the user and password`,
          wireguard_endpoint: (name: string) => `the WireGuard server ${name} cannot be looked up`,
          wireguard_down: (name: string) => `the WireGuard tunnel to ${name} cannot start`,
          wireguard_handshake: (name: string) =>
            `the WireGuard server ${name} does not answer the handshake: it is unreachable, or a key of the configuration is wrong`,
          no_route: () => 'no route is set',
        },
        torrentUdp: (name: string) =>
          `The SOCKS5 proxy ${name} does not offer UDP ASSOCIATE: torrents play over TCP, with UDP trackers and the DHT off.`,
        torrentStart: (why: string) =>
          `A torrent did not start in time: ${why}. Check that the route reaches the internet; the log tells each tracker’s answer at debug level.`,
        torrentStartWhy: {
          no_udp:
            'no peer was found, and the route carries no UDP, which the DHT and UDP trackers need',
          refused: 'its trackers refused the announce or did not answer, and the DHT found no peer',
          no_peers: 'its trackers and the DHT found no peer',
          silent: 'peers were found, but none sent it',
        },
        torrentStartPeers: (found: number, why: string) =>
          `${found} ${found === 1 ? 'peer' : 'peers'} found: ${why}`,
        torrentPeerWhy: {
          connected: (n: number) => `${n} connected`,
          timedOut: (n: number) => `${n} timed out`,
          refused: (n: number) => `${n} refused the connection`,
          unreachable: (n: number) => `${n} unreachable`,
          proxy: (n: number) => `${n} refused by the proxy`,
          closed: (n: number) => `${n} closed the connection at the handshake`,
          left: (n: number) => `${n} connected, then left`,
          other: (n: number) => `${n} failed otherwise`,
          untried: (n: number) => `${n} not dialed yet`,
        },
        dlnaPort: (port: number) =>
          `Polyfin cannot listen on UDP port ${port}, so DLNA devices on the local network do not find it by themselves. Another program probably uses it, such as another DLNA server on the same host. Saving the settings tries again.`,
        discoveryAddress: (address: string) =>
          `Jellyfin apps that look for servers on the local network are given ${address}, an address of Polyfin’s container they cannot reach. Set Address on the local network under Settings › General.`,
        proxyNameUnresolved: (name: string) =>
          `The known proxy ${name} resolves to no address, so no request is trusted to come from it. Check its name under Settings › Security, and that Polyfin shares a network with it.`,
        task: (name: string) => `The task “${name}” failed.`,
        httpsPort: (port: number) =>
          `Polyfin cannot listen on TCP port ${port}, so HTTPS is not served. HTTP keeps working. Another program probably uses it, or it is below 1024. Choose another HTTPS port under Settings › Security.`,
        httpsFileMissing: (file: string) =>
          `The HTTPS certificate file ${file} does not exist. The certificate last read stays in use.`,
        httpsFilesMismatch:
          'The HTTPS private key does not belong to the certificate. The certificate last read stays in use.',
        httpsFileUnreadable: (file: string) =>
          `Polyfin cannot read the HTTPS certificate file ${file}, or it holds no PEM certificate or key it reads. The certificate last read stays in use.`,
        httpsRenewal: (error: string) =>
          `Let’s Encrypt did not give the HTTPS certificate: ${error}`,
        httpsExpires: (date: string) => `The HTTPS certificate expires on ${date}.`,
        httpsExpired: (date: string) => `The HTTPS certificate expired on ${date}.`,
        subtitleOcrLanguage: (name: string, file: string) =>
          `Picture subtitles in ${name} are not read into text: Tesseract has no data for this language. Add the file ${file} to the folder POLYFIN_TESSDATA_DIR names.`,
        metadataLanguages: (name: string) =>
          `${name}: every metadata addon of the server has a language, so they describe titles in their language for every user, whatever their metadata language. Keep Language of its descriptions at None on this addon, or on one more metadata addon.`,
        nvidiaDriver: (version: string) =>
          `The NVIDIA graphics card does not convert video: its driver is older than version ${version}, which FFmpeg needs for NVENC. Update the driver to use the card.`,
        backupFailed: 'The last database backup failed.',
        backupStale: 'The last database backup is more than two days old.',
        backupDestinationFailed: (name: string) =>
          `${name}: the last upload of the database backups failed.`,
        backupDestinationStale: (name: string) =>
          `${name}: the newest database backup there is more than two days old.`,
        conversionsFull: 'Video conversions are at their limit: new ones wait or play otherwise.',
        unreadableSecrets: (names: string) =>
          `Stored keys cannot be decrypted with POLYFIN_SECRET_KEY and count as not set: ${names}. Set the key they were encrypted with again, or enter them again.`,
        plaintextSecrets:
          'Keys and tokens are stored unencrypted. Set POLYFIN_SECRET_KEY to encrypt them.',
        appStopped: (app: string, user: string) =>
          `${app} refused the sign-in Polyfin kept for ${user}: nothing is imported until ${user} signs in again under My account, with Stay connected ticked.`,
        trackingStopped: (service: string, user: string) =>
          `${service} refused the token or key Polyfin kept for ${user}: nothing is sent to it or imported until ${user} presses Connect again under My account › Tracking.`,
      },
      addonsTitle: 'Addons',
      addonsHelp:
        'From the requests made for apps since the server started. “Check” asks an addon for its manifest once, at most once a minute.',
      noAddons: 'The server has no addon.',
      addonStatus: {
        ok: 'Answering',
        failing: 'Failing',
        unknown: 'No request since start',
        off: 'Turned off',
      },
      failures: {
        unreachable: 'could not be reached',
        timeout: 'did not answer in time',
        private_network: 'is on a local network',
        rate_limited: 'asked to slow down (too many requests)',
        server_error: 'answered with a server error',
        http_error: 'refused the request',
        invalid_response: 'sent an answer too large',
      } as Record<string, string>,
      lastSuccess: 'Last answer',
      lastFailure: 'Last failure',
      responseTime: 'Response time',
      requests: (count: number, failures: number) =>
        `${count} ${count === 1 ? 'request' : 'requests'}, ${failures} failed`,
      check: 'Check',
      checkLabel: (name: string) => `Check ${name} now`,
      checking: 'Checking…',
      iptvTitle: 'IPTV sources',
      guidesTitle: 'Programme guides',
      sharesTitle: 'Network shares',
      shareKinds: {
        smb: 'SMB',
        webdav: 'WebDAV',
        s3: 'S3',
        sftp: 'SFTP',
        rclone: 'rclone',
      } as Record<string, string>,
      shareStatus: {
        notified: 'Notified',
        polled: 'Polled',
        unwatched: 'Not watched',
        scans: 'At scans only',
      },
      sharePolledEvery: (share: string, minutes: number) =>
        `${share === 's3' ? 'S3 buckets' : share === 'sftp' ? 'SFTP servers' : 'rclone remotes'} do not tell of changes: the folder’s folders are listed again every ${minutes === 1 ? 'minute' : `${minutes} minutes`}, as its settings ask.`,
      shareScansOnly:
        'Its settings ask to look for changes only at its scans: the folder is read on its schedule, and when scanned by hand.',
      shareNotified:
        'The server tells Polyfin of each change: the share is scanned again a few seconds after. Its folders are also listed every 10 minutes, should a change go untold.',
      sharePartial:
        'The server tells Polyfin of the changes made through the network, but missed one in a folder of the share, such as one made on the server’s own disk: the share’s folders are also listed again every 2 minutes.',
      sharePolled: {
        webdav:
          'WebDAV servers do not tell of changes: the share’s folders are listed again every 2 minutes.',
        unsupported:
          'This SMB server does not tell of changes: the share’s folders are listed again every 2 minutes.',
        silent:
          'This SMB server did not tell of a change that a listing found: the share’s folders are listed again every 2 minutes.',
        failed:
          'Polyfin lost the server’s notifications, and asks for them again shortly. Meanwhile, the share’s folders are listed again every 2 minutes.',
        starting:
          'Polyfin is asking the server to tell of changes. Meanwhile, the share’s folders are listed again every 2 minutes.',
      } as Record<string, string>,
      shareUnwatched: {
        unreachable:
          'The share cannot be reached. It is scanned on its schedule, and again once it answers.',
        refused: 'The share refuses Polyfin’s user or password. Check them, and enter them again.',
        host_key_changed:
          'The SFTP server offers another host key than the one saved. Accept it under Edit the folder if the change is expected.',
        failed: 'Polyfin cannot watch this share. It is still scanned on its schedule.',
      } as Record<string, string>,
      sharesOff:
        'Watching for changes is turned off in the settings: shares are scanned on their schedule only.',
      transcoderTitle: 'Transcoder',
      gpu: 'GPU',
      noGpu: 'None: video is converted on the CPU',
      toneMapping: 'HDR to SDR on the GPU',
      cpuEncoders: 'CPU encoders',
      conversions: 'Video conversions',
      noLimit: 'no limit',
      remuxes: 'Remuxes',
      maxHeight: 'Converted up to',
      transcodingOff: 'Conversion is turned off in the settings.',
      databaseTitle: 'Database',
      reachable: 'Answering',
      unreachable: 'Not answering',
      size: 'Size',
      cacheTitle: 'Source cache',
      cacheHelp:
        'Parts of the files being played, kept on disk up to the size set under Settings › Playback. A full cache is normal: the oldest parts make room.',
      cacheUse: (used: string, limit: string) => `${used} of ${limit}`,
      imagesTitle: 'Image cache',
      imagesHelp:
        'The artwork relayed to the apps, its resized copies and the images Polyfin draws, kept on disk up to its limit, the least recently used going first. Clearing it has each image downloaded, resized or drawn again at its next request. The scrubbing thumbnails, uploaded images, the local folders’ artwork and the web player’s theme are not in it.',
      imagesFiles: 'Images on disk',
      imagesMemory: 'In memory',
      clearImages: 'Clear the image cache',
      imagesCleared: (images: string, bytes: string) =>
        `Image cache cleared: ${images} images, ${bytes} freed on disk.`,
      sourcesOpen: 'Files open',
      torrentPieces: 'Of which torrent pieces',
      torrentsTitle: 'Torrents',
      torrentsHelp:
        'The torrents playback reads now, over their route alone. A torrent stops when its playback ends: no more peers, no more sharing.',
      torrentRoute: {
        interface: 'Network interface',
        proxy: 'SOCKS5 proxy',
        wireguard: 'WireGuard tunnel',
        '': 'No route',
      },
      torrentAddresses: 'Tunnel addresses',
      torrentHandshake: 'Last handshake',
      torrentNoHandshake: 'None yet',
      torrentsPlaying: 'Torrents playing',
      torrentPeers: 'Peers',
      torrentUTPPeers: 'Peers over uTP',
      torrentsShared: 'Shared since the start',
      disksTitle: 'Disk space',
      folders: { cache: 'cache', recordings: 'recordings', backups: 'backups' } as Record<
        string,
        string
      >,
      folderTitle: {
        cache: 'Cache folder',
        recordings: 'Recordings folder',
        backups: 'Backups folder',
      } as Record<string, string>,
      free: (free: string) => `${free} free`,
      notMeasured: 'Cannot be measured here.',
      thumbnailsTitle: 'Thumbnails (trickplay)',
      thumbnailsOff:
        'Thumbnails and chapter images are off under Settings › Thumbnails (trickplay).',
      waiting: (count: number, length: number) => `${count} of ${length} waiting`,
      working: 'Making images now',
      idle: 'Idle',
      pausedHosts: 'Paused sources',
      noPausedHosts: 'No source is paused.',
      until: (time: string) => `until ${time}`,
      ratingsTitle: 'Ratings from MDBList',
      ratingsHelp:
        'The requests MDBList allows the server’s key for ratings each day, and those left until its daily reset. Titles whose ratings are not kept are asked about in batches, in the background.',
      requestsLeft: 'Requests left today',
      requestsLeftOf: (left: string, limit: string) => `${left} of ${limit}`,
      notToldYet: 'Known once MDBList is next asked',
      resets: 'Daily reset',
      ratingsHeld: 'Asked again',
      heldReasons: {
        limit: 'MDBList limits requests',
        refused: 'MDBList refused the key',
        failed: 'MDBList did not answer',
      } as Record<string, string>,
      notHeld: 'Now',
      titlesWaiting: 'Titles waiting',
      titlesKept: 'Titles with ratings kept',
      backupTitle: 'Backups',
      backupOff: 'Backups are off. Turn them on under Settings › Backups.',
      backup: {
        result: 'Last run',
        none: 'No backup yet',
        succeeded: 'Succeeded',
        failed: 'Failed',
        made: 'Last backup made',
        file: 'File',
        size: 'Size',
        next: 'Next backup',
        runHint: 'Run “Back up the database” on the Schedule page to back up now.',
      },
      processTitle: 'Polyfin',
      version: 'Version',
      uptime: 'Running for',
      memory: 'Memory',
      heap: 'In use',
      goroutines: 'Goroutines',
      goroutinesDownload: 'Download the goroutines',
      goroutinesHelp:
        'When a request never answers, download what the server is doing and send it with your report. It holds no address or secret.',
      secretsTitle: 'Stored keys',
      secretsHelp:
        'The server’s keys, the users’ tracking tokens and keys, and the network shares’ passwords, encrypted in the database with POLYFIN_SECRET_KEY (openssl rand -base64 32).',
      encryption: 'Encryption',
      encrypted: 'On',
      notEncrypted: 'Off: POLYFIN_SECRET_KEY is not set',
      storedPlain: 'Stored unencrypted',
      unreadable: 'Cannot be decrypted',
      noneUnreadable: 'None',
      proxiesTitle: 'Known proxies',
      proxiesHelp:
        'The host names listed in Known proxies, looked up at each change of the settings, every minute, and as soon as an unknown address forwards requests. Every address a name resolves to is trusted as a proxy.',
      resolvesToNothing: 'Resolves to nothing',
      secretNames: {
        publicMetaDbKey: 'PublicMetaDB key',
        theIntroDbKey: 'TheIntroDB key',
        mdblistRatingsKey: 'MDBList API key for ratings',
        tmdbKey: 'TMDB API key',
        traktClientSecret: 'Trakt client secret',
        lastFmSecret: 'Last.fm shared secret',
        aniListClientSecret: 'AniList client secret',
        myAnimeListClientSecret: 'MyAnimeList client secret',
        smtpPassword: 'SMTP password',
        torrentProxyPassword: 'Torrents’ SOCKS5 proxy password',
        torrentWireGuard: 'Torrents’ WireGuard configuration',
        metricsToken: 'Metrics token',
        updateToolToken: 'Update tool token',
      },
      connectionOf: (service: string, user: string) => `${service} connection of ${user}`,
      targetOf: (target: string, user: string) => `notification target “${target}” of ${user}`,
      serverTargetOf: (target: string) => `server notification target “${target}”`,
      sharePasswordOf: (folder: string) => `password or key of the share “${folder}”`,
      destinationSecretOf: (name: string) => `password or key of the backup destination “${name}”`,
      certificateKeys: 'keys of the Let’s Encrypt certificate',
      twoStepOf: (user: string) => `key of the two-step sign-in of ${user}`,
      noAddonsHint: 'Add a Stremio addon under Content › Sources.',
      noIptvHint: 'Add an IPTV account under Content › Sources.',
      noGuides: 'No programme guide.',
      noGuidesHint: 'Add a programme guide to a TV catalog under Content › Live TV.',
      serverTitle: 'Server',
    },
    logs: {
      title: 'Logs',
      description:
        'The latest lines of the server log, kept in memory. Addresses keep only their host, and passwords, tokens and keys are hidden.',
      follow: 'Follow',
      pause: 'Pause',
      level: 'Level',
      levels: {
        all: 'All',
        info: 'Info and above',
        warn: 'Warnings and errors',
        error: 'Errors only',
      },
      search: 'Search',
      download: 'Download',
      clear: 'Clear the view',
      empty: 'No line yet.',
      noMatch: 'No line matches.',
      shown: (shown: number, total: number) => `${shown} of ${total} lines shown.`,
      following: 'Following: new lines appear every 2 seconds.',
      paused: 'Paused: the lines written meanwhile appear when you follow again.',
      detailedHint: 'Debug lines appear once “Detailed log” is on, under Settings › Diagnostics.',
      region: 'Server log',
      emptyHint: 'Lines appear here as the server writes them.',
      noMatchHint: 'Choose another level or change the search.',
      filters: 'Filter the log',
    },
  },
}

export default system
