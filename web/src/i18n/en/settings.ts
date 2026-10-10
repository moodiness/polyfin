/** The bounds and default of a setting, as its help text shows them, numbers already written. */
export type RangeText = { min: string; max: string; default: string }

/** Settings: the server-wide settings and their sections. */
const settings = {
  settings: {
    tracking: {
      description:
        'Each user can connect their own Trakt, Simkl, MDBList and PublicMetaDB accounts on their My account page, and Polyfin tells those services what they watch; Last.fm and ListenBrainz hear of the songs they play. Trakt, Simkl and Last.fm first need an app of this server’s, set up here. MDBList, PublicMetaDB and ListenBrainz need nothing.',
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
    },
    title: 'Settings',
    description: 'Options that apply to the whole server.',
    serverName: 'Server name',
    serverNameHelp: (r: RangeText) => `${r.min} to ${r.max} characters, shown in Jellyfin apps.`,
    quickConnect: 'Allow Quick Connect',
    quickConnectHelp: 'Lets TV and phone apps sign in with a 6-digit code approved here.',
    language: 'Language of generated names',
    languageHelp:
      'Polyfin names some things itself in Jellyfin apps: seasons (“Season 1”, “Specials”), episodes without a title, and the type added to libraries that share a name (“Popular (Movies)”).',
    legacyAuthorization: 'Allow legacy authorization',
    legacyAuthorizationHelp:
      'Accepts the old X-Emby-* headers, the api_key parameter and the Emby scheme for apps that still need them. Off by default, like Jellyfin 12.2.',
    updateCheck: 'Check for new versions',
    updateCheckHelp:
      'Once a day, and shortly after startup, Polyfin asks GitHub for its latest release, sending only its version. A newer one shows on Home and under System › Health, and is sent to the New version notifications. Off, Polyfin never contacts GitHub.',
    legacyWarningTitle: 'Security warning',
    legacyWarning:
      'Legacy methods can send credentials in URLs, which end up in logs, browser history and proxies. Only turn this on if an app you use cannot sign in otherwise.',
    prepareAhead: 'Prepare playback in advance',
    prepareAheadHelp:
      'Polyfin reads the file as soon as a title’s page opens, and gets the next episode ready near the end of the current one, so playback starts right away. It also lists in advance the versions of the titles in Continue Watching and Next Up, so their pages show them at once. This sends a few more requests to your sources, also for titles that are opened but not played.',
    transcoding: 'Conversion (transcoding)',
    transcodingHelp:
      'Re-encodes video and audio for apps that cannot play a file as it is. When off, apps play files as they are or simply repackaged without re-encoding, and a title an app cannot play that way will not start on that app.',
    analysisTimeout: 'Maximum time to analyze a version',
    analysisTimeoutHelp: (r: RangeText) =>
      `Before a first play, Polyfin analyzes the file or the channel to know how to play it. If the source does not answer within this many seconds, Polyfin gives up on that version and moves on to the next one. A lower number moves on sooner, but may give up on slow sources that would have worked. A channel is analyzed for 8 seconds at most. From ${r.min} to ${r.max} seconds; ${r.default} by default.`,
    versionAttempts: 'Versions tried when one does not work',
    versionAttemptsHelp: (r: RangeText) =>
      `When an app plays a title without choosing a version, Polyfin tries the versions in order until one works, but analyzes no more than this many. Versions already analyzed are tried too, as they cost nothing. A higher number finds a working version more often, but a title that does not play takes longer to say so. From ${r.min} to ${r.max}; ${r.default} by default.`,
    preferDirectPlay: 'Prefer versions the app plays without conversion',
    preferDirectPlayHelp:
      'When an app plays a title without choosing a version, Polyfin picks the first version the app plays as it is or simply repackaged, rather than the first that plays at all. The very first play of a title can be a little slower, as more versions may be analyzed; nothing changes once they are known.',
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
    publicAddress: 'Public address',
    publicAddressHelp:
      'The address people open Polyfin at, such as https://media.example.org. Links in notifications start with it; empty, messages carry no link.',
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
    maxConversions: 'Video conversions at once (0 = no limit)',
    maxConversionsHelp: (r: RangeText) =>
      `Converting video is the heaviest work the server does. Once this many playbacks have their video converted (subtitles burned into the picture included), a new playback gets a version that needs no conversion, or does not start. Playbacks already running are never cut. Live TV keeps its own limits too: 4 channels per user and 16 for the server. From ${r.min} to ${r.max}; ${r.default} by default.`,
    maxConversionHeight: 'Maximum quality of converted video',
    maxConversionHeightHelp:
      'Converted video is scaled down to this height at most, keeping its shape, so that it plays well over a slower connection. Files played as they are or simply repackaged keep their quality. A graphics card converts up to 4K; the processor stops at 1080p, and when it converts HDR to SDR, at "Maximum quality of HDR converted by the processor" (Conversion › HDR).',
    conversionHeightOriginal: 'Original',
    conversionHeight: (height: number) => `${height}p`,
    conversion: {
      description:
        'When an app cannot play a file as it is, Polyfin converts it with FFmpeg. These settings choose how, for movies, series and Live TV alike. The defaults suit most servers.',
      groups: {
        general: 'General',
        gpu: 'Graphics card',
        video: 'Video',
        hdr: 'HDR',
        interlaced: 'Interlaced video',
        audio: 'Audio',
        performance: 'Performance',
      },
      hardwareAcceleration: 'Graphics card used to convert',
      hardwareAccelerationHelp:
        'A graphics card (GPU) converts video much faster than the processor. A change applies once saved, without a restart.',
      vaapiDevice: 'Graphics card for VAAPI',
      vaapiDeviceHelp:
        'The render node VAAPI converts video on, when several graphics cards could. Each in turn by default, the first that works.',
      vaapiDeviceEach: 'Each in turn',
      vaapiDeviceMissing: (path: string) => `${path} (not found)`,
      hardware: {
        auto: 'Automatic: NVIDIA, else AMD or Intel',
        nvenc: 'NVIDIA (NVENC)',
        vaapi: 'AMD or Intel (VAAPI)',
        none: 'None: the processor only',
      },
      detected: 'Detected on this server',
      noGpu: 'No graphics card is used: the processor converts video.',
      gpu: 'Graphics card',
      gpuNames: { cuda: 'NVIDIA', vaapi: 'AMD or Intel (VAAPI)' },
      encoders: 'Encoders on the card',
      gpuToneMapping: 'HDR to SDR on the card',
      processorToneMappingHint:
        'HDR is then converted by the processor, up to "Maximum quality of HDR converted by the processor".',
      qualityFactor: 'Takes a quality number',
      processor: 'Encoders on the processor',
      processorToneMapping: 'HDR to SDR on the processor',
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
        'Slower gives a better picture at the same size, but needs more power. If converted videos stutter, choose a faster one. Automatic keeps Polyfin’s choice: very fast on the processor, medium on NVIDIA cards, the driver’s on AMD and Intel cards.',
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
      toneMapping: 'Convert HDR to SDR (tone mapping)',
      toneMappingHelp:
        'Keeps the colors and brightness of HDR videos right on screens that only show SDR. When off, converted HDR videos look pale, and Dolby Vision videos without an HDR10 layer are not converted.',
      toneMappingUnavailable:
        'Neither the graphics card nor FFmpeg can do it on this server: HDR videos are only converted when this is off.',
      toneMappingAlgorithm: 'Tone mapping method',
      toneMappingAlgorithmHelp:
        'How bright parts are brought down to SDR. Automatic uses BT.2390 on NVIDIA cards and Hable on the processor. Intel cards use their own method and ignore this choice.',
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
        'NVIDIA cards, and Intel cards through VAAPI, convert HDR to SDR themselves, up to 4K. Turn this off if their colors look wrong: the processor then does it, at a lower quality. Intel cards only take HDR10 videos that carry their mastering display information; the processor converts the others, and Dolby Vision without an HDR10 layer is only converted on NVIDIA cards. AMD cards never do it: their Linux driver failed or froze each way Polyfin tried.',
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
        `Automatic gives 192 kb/s in stereo, and 64 kb/s per channel above. 0, or from ${r.min} to ${r.max}; ${r.default} by default.`,
      encodingThreads: 'Processor threads per conversion (0 = automatic)',
      encodingThreadsHelp: (r: RangeText) =>
        `Limits how much of the processor one conversion uses, to leave room for other work. From ${r.min} to ${r.max}; 0 lets FFmpeg choose.`,
      aheadSeconds: 'Seconds prepared ahead',
      aheadSecondsHelp: (r: RangeText) =>
        `Polyfin prepares a video at most this many seconds past the part the app asked for, then waits. More helps with slow sources and keeps their connection busy, but uses more power and disk space when people stop watching early. From ${r.min} to ${r.max}; ${r.default} by default.`,
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
    publicMetaDbKeyHelp: 'Optional. A PublicMetaDB API key adds a third source of skip markers.',
    theIntroDbKey: 'TheIntroDB key (optional)',
    theIntroDbKeyHelp:
      'Raises TheIntroDB’s daily limit and includes your own submissions. Reading works without it.',
    segmentSources: {
      label: 'Skip marker sources',
      help: 'For each kind of passage (intro, recap, credits, preview), the first source turned on in this list that has it wins; the next ones fill the gaps. Sources turned off are never asked.',
      turnOn: (name: string) => `Ask ${name}`,
      needsKey: 'Needs a key',
    },
    similarTitles: 'Similar titles',
    similarTitlesHelp:
      'A title’s page lists titles close to it, found in the addons’ catalogs. When off, the list is empty and the addons get fewer requests.',
    lyrics: 'Song lyrics from LRCLIB',
    lyricsHelp:
      'Apps show the lyrics of songs, synced line by line when LRCLIB, a free lyrics database, has them. Polyfin looks a song up once, by its artist, title, album and length, when it starts playing or its details or lyrics open. When off, songs have no lyrics and LRCLIB is never asked.',
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
      'Reads every collection of the server’s collection libraries at this hour, one after the other, so that they open at once. It asks the addons for many pages: leave it off for an addon you share with others.',
    localScanHours: 'Scan local folders every (hours, 0 = never)',
    localScanHoursHelp: (r: RangeText) =>
      `How often each local folder is scanned again for files added, changed or removed. Folders are also scanned at startup and when you ask. 0 turns the schedule off. From ${r.min} to ${r.max}; ${r.default} by default.`,
    personalAddons: 'Allow users’ own addons',
    personalAddonsHelp:
      'Lets users add Stremio addons of their own, besides the server’s. When off, their addons are kept but not used, and their Jellyfin apps show the server’s addons only.',
    serverImports: 'Users may import their watch history from another server',
    serverImportsHelp:
      'Under My account, each user can import their played movies and episodes, resume points and favorites from a Jellyfin or Emby server, signing in there with their own name and password. When off, that section is hidden.',
    loginAttempts: 'Block an account after this many wrong passwords (0 = never)',
    loginAttemptsHelp: (r: RangeText) =>
      `After this many wrong passwords in a row, the account cannot sign in for 15 minutes, even with the right password. An administrator can unblock it sooner on the Users page. 0, or from ${r.min} to ${r.max}.`,
    inactiveDeviceDays: 'Sign out devices unused for (days, 0 = never)',
    inactiveDeviceDaysHelp: (r: RangeText) =>
      `Jellyfin apps not used for this many days are signed out and must sign in again. This is checked every hour. Signing in to this admin page is not affected. From ${r.min} to ${r.max}.`,
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
    recordingsTitle: 'Recordings',
    recordingsFolder: (folder: string) => `Recordings are saved in ${folder}.`,
    recording: 'Record Live TV',
    recordingHelp:
      'Lets users schedule recordings of Live TV programmes and series, written to the folder below.',
    recordingsFolderLabel: 'Recordings folder',
    recordingsFolderHelp:
      'A folder of Polyfin’s container, the one shown when empty, in its data volume. To keep recordings on another disk, mount it there, or elsewhere and enter its path here. Polyfin must be able to write to it. Changing it does not move the recordings already made.',
    recordingPrePadding: 'Start recordings early (minutes)',
    recordingPrePaddingHelp: (r: RangeText) =>
      `How many minutes before the programme a recording starts. From ${r.min} to ${r.max}; ${r.default} by default.`,
    recordingPostPadding: 'Keep recording after the end (minutes)',
    recordingPostPaddingHelp: (r: RangeText) =>
      `How many minutes after the programme a recording goes on. From ${r.min} to ${r.max}; ${r.default} by default.`,
    recordingRetentionDays: 'Keep recordings for (days, 0 = forever)',
    recordingRetentionDaysHelp: (r: RangeText) =>
      `Recordings older than this are deleted. This is checked every day. From ${r.min} to ${r.max}.`,
    liveTvRefreshHours: 'Refresh Live TV lists and guides every (hours)',
    liveTvRefreshHoursHelp: (r: RangeText) =>
      `How often the IPTV channel lists and the XMLTV programme guides are downloaded again. A download that failed is tried again sooner: after 5 minutes, 15 minutes, then every hour. From ${r.min} to ${r.max}; ${r.default} by default.`,
    backupsFolder: (folder: string) => `Backups of the database are saved in ${folder}.`,
    backups: 'Back up the database every day',
    backupsHelp:
      'Writes a copy of the database to the folder below each day, at the hour chosen, and deletes the oldest past the number kept.',
    backupFolderLabel: 'Backups folder',
    backupFolderHelp:
      'A folder of Polyfin’s container, the one shown when empty, in its data volume. Backups are safer on another disk than the database’s: mount one there, or elsewhere and enter its path here. Polyfin must be able to write to it.',
    backupHour: 'Back up every day at',
    backupHourHelp: (hour: string) => `In the server’s time zone. ${hour} by default.`,
    backupsKept: 'Backups to keep',
    backupsKeptHelp: (r: RangeText) =>
      `After each backup, Polyfin deletes its oldest backups past this number. Other files in the folder are never touched. From ${r.min} to ${r.max}; ${r.default} by default.`,
    lastBackup: 'Last backup',
    webPlayerHelp:
      'What the web player (jellyfin-web, at /web/) shows besides its own pages. The script applies the next time a page of the web player is loaded; the web player keeps the CSS and the sign-in message for up to a minute.',
    openWebPlayer: 'Open the web player',
    customCss: 'Custom CSS',
    customCssHelp:
      'Applied to every page of the web player, for every user, as Jellyfin’s custom CSS. Users can turn it off under Settings › Display. By default, it holds the LumaaGlaass theme’s stylesheet, which goes with the default Custom JavaScript: empty both to keep the web player’s own look.',
    customJs: 'Custom JavaScript',
    customJsHelp:
      'Loaded by every page of the web player, after its own scripts. Empty loads nothing. By default, it loads the LumaaGlaass theme’s script, which goes with the default Custom CSS.',
    customJsWarningTitle: 'Only paste code you trust',
    customJsWarning:
      'This script runs in the browser of every user who opens the web player on this server, with their account. It can read what they see and act for them.',
    loginDisclaimer: 'Sign-in message',
    loginDisclaimerHelp:
      'Shown under the sign-in form of the web player. Plain text, Markdown or HTML; the web player removes unsafe HTML.',
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
      general: 'The server’s name, its language, how apps sign in and the check for new versions.',
      playback: 'How Polyfin picks and prepares the versions it plays.',
      content: 'Skip intro buttons, similar titles, song lyrics and playback thresholds.',
      catalogs: 'How much Polyfin reads from the addons’ catalogs, and how often.',
      security: 'Users’ own addons and imports, blocked accounts and unused devices.',
      liveTv: 'How often the IPTV channel lists and guides are downloaded again.',
      diagnostics: 'The detailed log, and the environment variables in effect.',
      notifications:
        'Where Polyfin tells of new episodes, recordings, Health problems and new users, for every user.',
    },
    groups: {
      skip: 'Skip intro and credits',
      titlePages: 'Title pages',
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
