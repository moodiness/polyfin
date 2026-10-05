/** Settings: the server-wide settings and their sections. */
const settings = {
  settings: {
    tracking: {
      description:
        'Each user can connect their own Trakt, Simkl, MDBList and PublicMetaDB accounts on their My account page, and Polyfin tells those services what they watch. Trakt and Simkl first need an app of this server’s, set up here. MDBList and PublicMetaDB need nothing.',
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
    },
    title: 'Settings',
    description: 'Options that apply to the whole server.',
    serverName: 'Server name',
    serverNameHelp: '1 to 64 characters, shown in Jellyfin apps.',
    quickConnect: 'Allow Quick Connect',
    quickConnectHelp: 'Lets TV and phone apps sign in with a 6-digit code approved here.',
    language: 'Language of generated names',
    languageHelp:
      'Polyfin names some things itself in Jellyfin apps: seasons (“Season 1”, “Specials”), episodes without a title, and the type added to libraries that share a name (“Popular (Movies)”).',
    legacyAuthorization: 'Allow legacy authorization',
    legacyAuthorizationHelp:
      'Accepts the old X-Emby-* headers, the api_key parameter and the Emby scheme for apps that still need them. Off by default, like Jellyfin 12.1.',
    legacyWarningTitle: 'Security warning',
    legacyWarning:
      'Legacy methods can send credentials in URLs, which end up in logs, browser history and proxies. Only turn this on if an app you use cannot sign in otherwise.',
    chapters: 'Show chapters',
    chaptersHelp:
      'Chapters are read along with the file analysis Polyfin does anyway before a first play, so they never delay playback. Turning this off only hides them from apps.',
    prepareAhead: 'Prepare playback in advance',
    prepareAheadHelp:
      'Polyfin reads the file as soon as a title’s page opens, and gets the next episode ready near the end of the current one, so playback starts right away. This sends a few more requests to your sources, also for titles that are opened but not played.',
    transcoding: 'Conversion (transcoding)',
    transcodingHelp:
      'Re-encodes video and audio for apps that cannot play a file as it is. When off, apps play files as they are or simply repackaged without re-encoding, and a title an app cannot play that way will not start on that app.',
    downloads: 'Downloads',
    downloadsHelp:
      'Lets users save titles in Jellyfin apps to watch offline. When off, nobody can download, whatever their own permission.',
    analysisTimeout: 'Maximum time to analyze a version',
    analysisTimeoutHelp:
      'Before a first play, Polyfin analyzes the file or the channel to know how to play it. If the source does not answer within this many seconds, Polyfin gives up on that version and moves on to the next one. A lower number moves on sooner, but may give up on slow sources that would have worked. From 5 to 120 seconds; 45 by default.',
    versionAttempts: 'Versions tried when one does not work',
    versionAttemptsHelp:
      'When an app plays a title without choosing a version, Polyfin tries the versions in order until one works, but analyzes no more than this many. Versions already analyzed are tried too, as they cost nothing. A higher number finds a working version more often, but a title that does not play takes longer to say so. From 1 to 10; 3 by default.',
    preferDirectPlay: 'Prefer versions the app plays without conversion',
    preferDirectPlayHelp:
      'When an app plays a title without choosing a version, Polyfin picks the first version the app plays as it is or simply repackaged, rather than the first that plays at all. The very first play of a title can be a little slower, as more versions may be analyzed; nothing changes once they are known.',
    maxConversions: 'Video conversions at once (0 = no limit)',
    maxConversionsHelp:
      'Converting video is the heaviest work the server does. Once this many playbacks have their video converted (subtitles burned into the picture included), a new playback gets a version that needs no conversion, or does not start. Playbacks already running are never cut. Live TV keeps its own limits too: 4 channels per user and 16 for the server. From 0 to 32; 0 by default.',
    maxConversionHeight: 'Maximum quality of converted video',
    maxConversionHeightHelp:
      'Converted video is scaled down to this height at most, keeping its shape, so that it plays well over a slower connection. Files played as they are or simply repackaged keep their quality. Polyfin never converts above 1080p, so higher choices change nothing for now.',
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
        'A graphics card (GPU) converts video much faster than the processor. The POLYFIN_HWACCEL environment variable still sets the default; a choice here overrides it once saved, without a restart.',
      hardwareDefault: (value: string) => `Default from POLYFIN_HWACCEL (${value})`,
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
      qualityHelp:
        'A lower number gives a better picture and more data; the bitrate still sets the maximum. 0 aims for the bitrate alone. Jellyfin uses 23 for H.264 and 28 for HEVC. 0, or from 1 to 51; 0 by default.',
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
        'How bright parts are brought down to SDR. Automatic uses BT.2390 on the graphics card and Hable on the processor.',
      algorithms: {
        auto: 'Automatic',
        bt2390: 'BT.2390 (graphics card only)',
        hable: 'Hable',
        reinhard: 'Reinhard',
        mobius: 'Möbius',
        clip: 'Clip',
        linear: 'Linear',
      },
      toneMappingPeak: 'Peak brightness in nits (0 = from the video)',
      toneMappingDesat: 'Highlight desaturation (0 = off)',
      toneMappingPeakHelp:
        'On the processor only. The peak replaces the brightest level the video says it reaches: 0, or from 100 to 10,000. Desaturation fades the color of very bright parts: from 0 to 10. Both are 0 by default.',
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
      downmixBoostHelp:
        'Stereo mixes often sound quieter: the volume is multiplied by this number. From 0.5 to 3; 1 (no change) by default. Jellyfin uses 2.',
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
      audioBitratePerChannelHelp:
        'Automatic gives 192 kb/s in stereo, and 64 kb/s per channel above. 0, or from 32 to 320; 0 by default.',
      encodingThreads: 'Processor threads per conversion (0 = automatic)',
      encodingThreadsHelp:
        'Limits how much of the processor one conversion uses, to leave room for other work. From 0 to 64; 0 lets FFmpeg choose.',
      aheadSegments: 'Segments prepared ahead',
      aheadSegmentsHelp:
        'Polyfin prepares a video at most this many segments, of about 6 seconds each, past the part the app asked for, then waits. More helps with slow sources, but uses more power and disk space when people stop watching early. From 1 to 60; 10 by default.',
    },
    catalogsTitle: 'Catalogs',
    catalogLimit: 'Titles read per movie and series catalog',
    catalogLimitHelp:
      'Some catalogs are nearly endless, so Polyfin stops reading a catalog after this many titles. A higher number shows more titles, but lists load more slowly and the addon gets more requests. From 100 to 20,000; 2,000 by default.',
    channelLimit: 'Channels read per Live TV catalog',
    channelLimitHelp:
      'Polyfin stops reading a Live TV catalog after this many channels, and reads at most this many programmes per day for the guide. A higher number shows more, but loads more slowly and the addon gets more requests. From 100 to 50,000; 10,000 by default.',
    skipButtons: 'Skip intro and credits buttons',
    skipButtonsHelp:
      'Apps offer to skip intros, recaps and credits, found in community databases. When off, apps show no skip buttons and the databases are not asked.',
    publicMetaDbKey: 'PublicMetaDB key',
    publicMetaDbKeyHelp: 'Optional. A PublicMetaDB API key adds a third source of skip markers.',
    theIntroDbKey: 'TheIntroDB key (optional)',
    theIntroDbKeyHelp:
      'Raises TheIntroDB’s daily limit and includes your own submissions. Reading works without it.',
    segmentSources: {
      label: 'Order of the skip marker sources',
      help: 'For each kind of passage (intro, recap, credits, preview), the first source in this list that has it wins; the next ones fill the gaps.',
      offHelp: 'Sources turned off by POLYFIN_SEGMENTS are never asked, wherever they are.',
      on: 'On',
      needsKey: 'Needs a key',
      off: 'Off (POLYFIN_SEGMENTS)',
      reset: 'Reset to the default order',
      resetDone: 'Default order restored. Save to apply it.',
    },
    similarTitles: 'Similar titles',
    similarTitlesHelp:
      'A title’s page lists titles close to it, found in the addons’ catalogs. When off, the list is empty and the addons get fewer requests.',
    playedPercent: 'Marked played after (%)',
    playedPercentHelp:
      'A title is marked played once playback goes past this share of its length. From 50 to 100; 90 by default, like Jellyfin.',
    resumePercent: 'Resume point kept after (%)',
    resumePercentHelp:
      'Where playback stopped is kept, to resume from there, once past this share of a title’s length. It must be lower than the played threshold. From 0 to 50; 5 by default, like Jellyfin. A title shorter than 5 minutes is marked played as soon as it is past this point.',
    versionListMinutes: 'Keep version lists for (minutes)',
    versionListMinutesHelp:
      'How long Polyfin keeps the versions and subtitles the addons list for a title. Keeping lists longer sends fewer requests to the stream addon, which helps with providers that refuse too many requests, but new versions show up later. From 1 to 360; 10 by default.',
    catalogRefreshMinutes: 'Refresh catalogs every (minutes)',
    catalogRefreshMinutesHelp:
      'How long Polyfin keeps the catalog pages it reads from addons, the Live TV guide included, before reading them again. A longer time sends fewer requests to the addons, but new titles show up later. From 1 to 1,440 (one day); 10 by default.',
    personalAddons: 'Allow users’ own addons',
    personalAddonsHelp:
      'Lets users add Stremio addons of their own, besides the server’s. When off, their addons are kept but not used, and their Jellyfin apps show the server’s addons only.',
    loginAttempts: 'Block an account after this many wrong passwords (0 = never)',
    loginAttemptsHelp:
      'After this many wrong passwords in a row, the account cannot sign in for 15 minutes, even with the right password. An administrator can unblock it sooner on the Users page. 0, or from 3 to 20.',
    inactiveDeviceDays: 'Sign out devices unused for (days, 0 = never)',
    inactiveDeviceDaysHelp:
      'Jellyfin apps not used for this many days are signed out and must sign in again. This is checked every hour. Signing in to this admin page is not affected. From 0 to 365.',
    detailedLog: 'Detailed log (to diagnose a problem)',
    detailedLogHelp:
      'Polyfin writes much more to its log, at once and without a restart. Turn it off once the problem is found.',
    saved: 'Settings saved.',
    thumbnailsHelp:
      'Images made from the titles themselves, after they are watched. To make them, Polyfin reads small parts of the file from the source, at a gentle pace, once nobody is playing from that source: at most 60 requests per title, one every 3 seconds, and 120 an hour per source, never the whole file. A source that asks Polyfin to slow down gets no request for images for 2 hours. Off by default.',
    trickplay: 'Thumbnails when moving through a title',
    trickplayHelp:
      'Apps show a small image of the moment the user moves to in the playback bar. Polyfin makes them once a title has been watched, and for the next episode when playback is prepared in advance. Long movies get coarser thumbnails: a few minutes apart rather than a few seconds.',
    trickplayInterval: 'One thumbnail every (seconds)',
    trickplayIntervalHelp:
      'How often the playback bar changes image. Polyfin reads at most 60 images per title, so on long titles the same image covers several steps. From 5 to 60; 10 by default, like Jellyfin.',
    trickplayWidth: 'Thumbnail width',
    trickplayWidthHelp:
      'Wider thumbnails look sharper on large screens, but take more space. Changing it makes new thumbnails the next time a title is played.',
    pixels: (width: number) => `${width} pixels`,
    chapterImages: 'Chapter images',
    chapterImagesHelp:
      'Apps show an image for each chapter of a title, in its list of scenes. Polyfin uses the image it read nearest each chapter’s start, from the same reads as the thumbnails, once the title has been watched.',
    thumbnailStorage: 'Space for images (GB)',
    thumbnailStorageHelp:
      'Thumbnails and chapter images are kept in the database. Past this size, the images of the titles watched longest ago are removed; they are made again if the title is played again. From 1 to 50; 2 by default.',
    recordingsTitle: 'Recordings',
    recordingsFolder: (folder: string) => `Recordings are saved in ${folder}.`,
    recordingsOff: 'Recording is off. Set POLYFIN_RECORDINGS_DIR to a folder to turn it on.',
    recordingPrePadding: 'Start recordings early (minutes)',
    recordingPrePaddingHelp:
      'How many minutes before the programme a recording starts. From 0 to 60; 0 by default.',
    recordingPostPadding: 'Keep recording after the end (minutes)',
    recordingPostPaddingHelp:
      'How many minutes after the programme a recording goes on. From 0 to 60; 0 by default.',
    recordingRetentionDays: 'Keep recordings for (days, 0 = forever)',
    recordingRetentionDaysHelp:
      'Recordings older than this are deleted. This is checked every day. From 0 to 3,650.',
    liveTvRefreshHours: 'Refresh Live TV lists and guides every (hours)',
    liveTvRefreshHoursHelp:
      'How often the IPTV channel lists and the XMLTV programme guides are downloaded again. From 1 to 168; 12 by default.',
    backupsFolder: (folder: string) => `Backups of the database are saved in ${folder}.`,
    backupsOff:
      'Backups are off. Set POLYFIN_BACKUP_DIR to a folder on the container, mounted from the server, then restart Polyfin to turn them on.',
    backupHour: 'Back up every day at',
    backupHourHelp: 'In the server’s time zone. 4:00 AM by default.',
    backupsKept: 'Backups to keep',
    backupsKeptHelp:
      'After each backup, Polyfin deletes its oldest backups past this number. Other files in the folder are never touched. From 1 to 90; 7 by default.',
    lastBackup: 'Last backup',
    webPlayerHelp:
      'What the web player (jellyfin-web, at /web/) shows besides its own pages. The script applies the next time a page of the web player is loaded; the web player keeps the CSS and the sign-in message for up to a minute.',
    openWebPlayer: 'Open the web player',
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
    codeSize: (used: number, max: number) => `${used} KB of ${max} KB`,
    codeKeys: 'Tab inserts spaces; press Escape, then Tab, to leave the field.',
  },
  settingsPage: {
    searchHint: 'By name or description.',
    noMatch: 'No setting matches this search.',
    sections: {
      variables: 'Environment variables',
    },
    variablesHelp:
      'Read only. These are set on the container (Docker environment, compose file or Unraid template) and apply when Polyfin starts: change them there, then restart the container. Secrets are hidden, and the database address shows its host and database only.',
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
      general: 'The server’s name, its language and how apps sign in.',
      playback: 'How Polyfin picks and prepares the versions it plays.',
      content: 'Skip intro buttons, similar titles and playback thresholds.',
      catalogs: 'How much Polyfin reads from the addons’ catalogs, and how often.',
      security: 'Users’ own addons, blocked accounts and unused devices.',
      liveTv: 'How often the IPTV channel lists and guides are downloaded again.',
      diagnostics: 'The detailed log, and the environment variables in effect.',
    },
    groups: {
      skip: 'Skip intro and credits',
      titlePages: 'Title pages',
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
