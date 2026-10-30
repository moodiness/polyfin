/** The GPU methods, as the server names them. */
const gpuMethodNames: Record<string, string> = {
  cuda: 'NVIDIA',
  vaapi: 'VAAPI',
  videotoolbox: 'VideoToolbox',
  qsv: 'QSV',
  amf: 'AMF',
}

/** Home: the administrator overview, now playing and recent activity, plus Quick Connect and the page not found. */
const home = {
  quickConnect: {
    title: 'Quick Connect',
    description:
      'Sign in a TV or phone app without typing your password: choose Quick Connect in a Jellyfin app, or open Silo’s app on a TV, then enter the code it shows.',
    code: 'Quick Connect code',
    codeHint: 'Codes expire after 10 minutes.',
    lookingUp: 'Looking for the device…',
    requestTitle: 'Device waiting for approval',
    device: 'Device',
    app: 'App',
    requested: 'Asked',
    approve: 'Approve and sign in this device',
    approving: 'Approving…',
    approved: (device: string, user: string) => `${device} is now signed in as ${user}.`,
    another: 'Approve another device',
    steps: [
      'On the device, choose Quick Connect in a Jellyfin app, or open Silo’s app on a TV.',
      'Type the code it shows here: 6 digits from Jellyfin, 8 from Silo.',
      'Check the device, then approve it.',
    ],
    stepsLabel: 'How it works',
  },
  notFound: {
    title: 'Page not found',
    description: 'This page does not exist.',
    home: 'Back to the home page',
  },
  dashboard: {
    overview: {
      greeting: (hour: number, name: string) =>
        `${hour >= 18 || hour < 5 ? 'Good evening' : hour >= 12 ? 'Good afternoon' : 'Good morning'}, ${name}`,
      summary: (playing: number, problems: number) =>
        `${playing === 0 ? 'Nothing is playing' : playing === 1 ? 'One playback under way' : `${playing} playbacks under way`}${
          problems === 0
            ? ' and nothing to look at. Everything runs normally.'
            : problems === 1
              ? ', and one thing to look at.'
              : `, and ${problems} things to look at.`
        }`,
      description: 'What is playing on the server, and what happened lately.',
      conversions: 'Video conversions',
      conversionsOf: (count: number, limit: number) =>
        limit > 0 ? `${count} of ${limit}` : `${count}, no limit`,
      uptime: 'Running for',
      problems: 'To look at',
      problemsEmpty: 'Nothing to look at',
      problemsEmptyHelp:
        'Polyfin checks its disks, sources, guides, backups and tasks. Anything that needs you shows up here.',
      problemError: 'Problem',
      problemWarning: 'To check',
      update: (version: string, running: string) =>
        `Polyfin ${version} is available. This server runs version ${running}.`,
      releaseNotes: 'Release notes',
      open: 'Open',
      openHealth: 'Open Health',
      serverTitle: 'Server state',
      version: 'Version',
      database: 'Database',
      databaseSize: (size: string) => `${size} on disk`,
      cache: 'Cache',
      cacheOf: (limit: string) => `of ${limit}`,
      cacheNoLimit: 'no limit',
      cacheOff: 'Off',
      cacheUnit: 'Cache in use',
      graphics: 'Graphics card',
      graphicsNone: 'None',
      graphicsCpu: 'Conversions use the processor',
      toneMapping: 'tone mapping',
    },
    /** Home's Get started card: the steps of a good setup, each done or not. */
    getStarted: {
      title: 'Get started',
      lede: (done: number, total: number) =>
        `${done} of ${total} steps done. Each one opens the page where it is set.`,
      dismiss: 'Dismiss',
      done: 'Done',
      todo: 'To do',
      steps: {
        metadata: {
          title: 'Add a metadata addon',
          help: 'It gives titles their posters, summaries and episodes.',
        },
        library: {
          title: 'Choose your libraries',
          help: 'The catalogs of your sources that apps show as libraries.',
        },
        users: {
          title: 'Create users or an invite link',
          help: 'An account for each person you share Polyfin with.',
        },
        backups: {
          title: 'Back up the database every day',
          help: 'A copy of the accounts, the settings and what everyone watched.',
        },
        secretKey: {
          title: 'Set POLYFIN_SECRET_KEY',
          help: 'It encrypts the keys and passwords Polyfin stores. Set it, then restart.',
        },
        address: {
          title: 'Serve HTTPS or set a public address',
          help: 'Apps away from home connect securely, and notifications link to Polyfin.',
        },
        discovery: {
          title: 'Let apps on your network find Polyfin',
          help: 'Answer local network discovery with the address apps reach.',
        },
      },
    },
    /** Restart, the install of Polyfin Desktop's update, and Update now, on Home. */
    server: {
      restart: 'Restart',
      updateNow: 'Update now',
      install: (version: string) => `Install Polyfin ${version}`,
      downloading: (percent: number) => `Downloading: ${percent}%`,
      confirmWith: 'Confirm with',
      withPassword: 'Password',
      withCode: 'Two-step code',
      password: 'Your password',
      code: 'Code of your app, or a recovery code',
      identityHelp: 'Each of these actions asks for it again.',
      restartTitle: 'Restart Polyfin',
      restartText:
        'Polyfin stops what it is doing, playbacks and recordings included, and exits. Apps connect again once it answers.',
      restartBy: {
        desktop: 'Polyfin Desktop starts it again by itself.',
        confirmed:
          'Its container’s restart policy, or its service manager, starts it again, as confirmed under Settings › General.',
      },
      unknownTitle: 'Polyfin may not come back',
      unknown: {
        container:
          'Polyfin cannot see its container’s restart policy. Without one that starts it again, such as unless-stopped or always, Polyfin stays stopped until it is started on the host.',
        other:
          'Polyfin cannot tell whether a service manager starts it again. Without one, Polyfin stays stopped until it is started on the host.',
      },
      unknownHelp:
        'Once it is set, turn on “Polyfin is started again when it stops” under Settings › General, and this warning goes.',
      restartConfirm: 'Restart',
      restartAnyway: 'Restart anyway',
      restarting: 'Restarting…',
      waitingTitle: 'Polyfin is restarting',
      waiting: 'Waiting for Polyfin to answer again…',
      waitingLong: (seconds: number) =>
        `Polyfin has not answered for ${seconds} seconds. If nothing starts it again, start it on the host.`,
      installTitle: (version: string) => `Install Polyfin ${version}`,
      installText:
        'Polyfin Desktop downloads the new version and checks it, then stops Polyfin, installs it and starts it, as Install Polyfin does in its menu. Playbacks and recordings stop meanwhile.',
      installConfirm: 'Install',
      installing: 'Asking…',
      installWaitingTitle: (version: string) => `Installing Polyfin ${version}`,
      installStopping: 'Polyfin stops to install the new version. Waiting for it to answer again…',
      installFailed:
        'Polyfin Desktop could not install the update: the log folder tells why. Polyfin runs on as it was.',
      updateTitle: 'Update now',
      updateText:
        'Polyfin asks the update tool set under Settings › General to update now. What the tool does next is up to it: one that replaces Polyfin’s container stops Polyfin for a moment.',
      updateConfirm: 'Send the request',
      sending: 'Sending…',
      answer: (status: number) => `The update tool answered HTTP ${status}.`,
      answerEmpty: 'Its answer holds no text.',
      watching:
        'If the tool replaces Polyfin’s container, this page reloads once Polyfin answers again.',
      gone: 'Polyfin stopped before the tool answered: the tool is probably replacing it. Waiting for Polyfin to answer again…',
      tooSoon: 'This was asked less than a minute ago. Try again in a moment.',
      close: 'Close',
    },
    live: {
      title: 'Now playing',
      empty: 'Nothing is playing right now.',
      emptyHelp: 'Playbacks from Jellyfin apps show up here within a few seconds.',
      autoRefresh: 'Updates every 3 seconds.',
      paused: 'Paused',
      unknownTitle: 'Title not found',
      movie: (year: number) => (year > 0 ? `Movie, ${year}` : 'Movie'),
      liveChannel: 'Live',
      progress: (title: string) => `Playback position of ${title}`,
      remaining: (time: string) => `${time} left`,
      startedAt: (time: string) => `Started ${time}`,
      appOn: (app: string, device: string) => (app ? `${app} on ${device}` : device),
      /** A guest's playback through a share link, in place of its app and device. */
      sharedLink: 'Shared link',
      /** A converted download, in place of the app and device. */
      downloadOn: (app: string, device: string) =>
        app ? `Download, ${app} on ${device}` : 'Download',
      downloading: 'Downloading',
      downloadProgress: (title: string) => `Download progress of ${title}`,
      downloadNotControllable:
        'A download converted for offline viewing. It stops when the app cancels it.',
      howItPlays: 'How it plays',
      details: 'Details',
      delivery: {
        directPlay: 'Direct play',
        remux: 'Remux',
        conversion: 'Conversion',
        stream: 'Streaming',
      } as Record<string, string>,
      deliveryHelp: {
        directPlay: 'The app reads the file as it is.',
        remux: 'Polyfin repackages the file for the app, without re-encoding it.',
        conversion: 'Polyfin re-encodes what the app cannot play.',
        stream:
          'Polyfin streams the file to the app. Details show again once the app asks for more.',
      } as Record<string, string>,
      why: 'Why',
      source: 'Source',
      sent: 'Sent',
      encoder: 'Encoder',
      gpu: (method: string) => `GPU (${gpuMethodNames[method] ?? method})`,
      gpuMethod: (method: string) => gpuMethodNames[method] ?? method,
      cpu: 'CPU',
      copied: 'Copied',
      toneMapped: 'HDR to SDR',
      burned: 'Subtitles burned in',
      qualityGroup: 'Quality group',
      original: 'Original',
      sizeLimit: 'Smaller than the source',
      /** Shown when the app may switch between several qualities of converted video. */
      qualityPlayed: 'Quality played',
      /** The version the play went on from once its source failed. */
      switchedVersion: 'Switched version',
      qualityAmong: (played: string, offered: string) => `${played}, among ${offered}`,
      sizeLimits: {
        steppedDown: (height: string) => `The app stepped down from ${height}`,
        gpu: (height: string) => `${height} at most: converted by the graphics card`,
        processor: (height: string) => `${height} at most: converted by the processor`,
        toneMapping: (height: string) => `${height} at most: HDR converted by the processor`,
        gpuToneMapping: (height: string) =>
          `${height} at most: the graphics card converts HDR in time only up to there`,
        qualityGroup: (height: string) => `Quality group: ${height}`,
        serverMax: (height: string) => `Maximum quality of converted video: ${height}`,
        app: (height: string) => `Quality allowed by the app: ${height}`,
        appBitrate: (rate: string) => `Bitrate allowed by the app: ${rate}`,
        userBitrate: (rate: string) => `User’s maximum quality: ${rate}`,
      },
      unknown: 'Unknown',
      stop: 'Stop',
      stopFor: (user: string) => `Stop ${user}’s playback`,
      stopTitle: 'Stop this playback?',
      stopConfirm: (user: string, title: string) =>
        `Stop “${title}” on ${user}’s device? The app stops playing at once.`,
      stopped: 'The app was asked to stop.',
      message: 'Send a message',
      messageTo: (user: string) => `Send a message to ${user}`,
      messageLabel: (device: string) => `Message to show on ${device}`,
      messageHint: 'Shown on the device for 10 seconds. 500 characters at most.',
      send: 'Send',
      messageSent: 'Message sent.',
      notControllable: 'This app does not accept remote control.',
      sharedNotControllable:
        'A guest watching through a share link. Revoking the link, on the user’s page, stops it.',
    },
    reasons: {
      ContainerNotSupported: 'File format not supported',
      VideoCodecNotSupported: 'Video codec not supported',
      AudioCodecNotSupported: 'Audio codec not supported',
      SubtitleCodecNotSupported: 'Subtitle format not supported',
      AudioIsExternal: 'Audio in a separate file',
      SecondaryAudioNotSupported: 'Second audio track not supported',
      VideoProfileNotSupported: 'Video profile not supported',
      VideoLevelNotSupported: 'Video level not supported',
      VideoResolutionNotSupported: 'Resolution too high',
      VideoBitDepthNotSupported: 'Video bit depth not supported',
      VideoFramerateNotSupported: 'Frame rate not supported',
      RefFramesNotSupported: 'Too many reference frames',
      AnamorphicVideoNotSupported: 'Anamorphic video not supported',
      InterlacedVideoNotSupported: 'Interlaced video not supported',
      VideoCodecTagNotSupported: 'Video codec tag not supported',
      VideoBitrateNotSupported: 'Video bitrate too high',
      AudioChannelsNotSupported: 'Too many audio channels',
      AudioProfileNotSupported: 'Audio profile not supported',
      AudioSampleRateNotSupported: 'Audio sample rate not supported',
      AudioBitDepthNotSupported: 'Audio bit depth not supported',
      AudioBitrateNotSupported: 'Audio bitrate too high',
      ContainerBitrateExceedsLimit: 'Above the bitrate limit',
      UnknownVideoStreamInfo: 'Video not described',
      UnknownAudioStreamInfo: 'Audio not described',
      DirectPlayError: 'Direct play failed',
      VideoRangeTypeNotSupported: 'HDR not supported',
      StreamCountExceedsLimit: 'Too many tracks',
    } as Record<string, string>,
    activity: {
      title: 'Recent activity',
      openLog: 'Open the log',
      empty: 'No activity yet.',
      emptyHelp: 'Sign-ins, playbacks and changes to the server show up here as they happen.',
      warning: 'Warning',
      error: 'Error',
      filter: 'Show',
      filters: {
        all: 'All',
        problems: 'Problems',
        signIns: 'Sign-ins',
        playback: 'Playback',
        users: 'Users',
        server: 'Server and addons',
        recordings: 'Recordings',
      },
      search: 'Search the events shown',
      listLabel: 'Latest events',
      noMatch: 'No event matches.',
      noMatchHelp: 'Choose another filter or clear the search.',
      count: (shown: number, total: number) => `${shown} of ${total} events.`,
      autoRefresh: 'Updates every 15 seconds.',
    },
  },
}

export default home
