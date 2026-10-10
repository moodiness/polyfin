/** Notification targets: Settings › Notifications and My account › Notifications. */
const notifications = {
  notifications: {
    title: 'Notifications',
    loading: 'Loading the targets…',
    empty: 'No target yet.',
    emptyHint: {
      server:
        'Add a target, such as an email address, a Telegram chat or an ntfy topic, to be told of new episodes, recordings and Health problems.',
      own: 'Add a target, such as an email address, a Telegram chat or an ntfy topic, to be told when a new episode of a series you follow is out, or when your recordings end.',
    },
    serverTargets: 'Server targets',
    serverTargetsHelp:
      'They receive the events of every user: one message per new episode, whoever follows its series, every recording and every playback. Each user can add their own targets under My account › Notifications.',
    add: 'Add a target',
    addTitle: 'Add a target',
    editTitle: (name: string) => `Edit ${name}`,
    kind: 'Kind',
    kinds: {
      webhook: 'Webhook',
      discord: 'Discord',
      ntfy: 'ntfy',
      email: 'Email',
      telegram: 'Telegram',
      gotify: 'Gotify',
      pushover: 'Pushover',
    },
    kindHelp: {
      webhook:
        'Polyfin posts each event to this address as JSON, described in the documentation for app developers.',
      discord: 'Polyfin posts each event to a Discord channel, as a message with a link.',
      ntfy: 'Polyfin publishes each event to an ntfy topic, which the ntfy app shows on your phone.',
      email: 'Polyfin emails each event to this address, in plain text and HTML.',
      telegram: 'Polyfin posts each event to a Telegram chat through your bot, with a link.',
      gotify: 'Polyfin posts each event to an application of your Gotify server.',
      pushover:
        'Polyfin sends each event to your Pushover devices, through one of your applications.',
    },
    emailUnavailable: {
      server:
        'Email needs an SMTP server: save one, with a sender address, under Email above, then add the target.',
      own: 'Email needs an SMTP server, which an administrator sets under Settings › Notifications.',
    },
    name: 'Name',
    nameHelp: 'Shown here only, 1 to 64 characters.',
    webhookAddress: 'Webhook address',
    discordAddress: 'Discord webhook address',
    addressHelp: {
      webhook: 'It is kept encrypted and never shown again: only its host is.',
      discord:
        'In Discord: the channel’s settings, Integrations, Webhooks, Copy Webhook URL. It is kept encrypted and never shown again.',
    },
    savedAddress: (host: string) => `Saved, at ${host}.`,
    ntfyServer: 'ntfy server',
    ntfyServerHelp: 'Leave empty for https://ntfy.sh.',
    topic: 'Topic',
    topicHelp:
      'The topic you subscribe to in the ntfy app. Anyone who knows a topic on a public server can read it: choose one that is hard to guess.',
    token: 'Access token',
    tokenHelp: 'For a topic that needs signing in. It is kept encrypted and never shown again.',
    emailAddress: 'Email address',
    emailAddressHelp: 'Where the messages go.',
    chat: 'Chat',
    chatHelp:
      'The chat’s number, such as -1001234567890, or a public channel’s name, such as @news_example. Add the bot to the chat first.',
    botToken: 'Bot token',
    botTokenHelp:
      'BotFather gives it when the bot is created. It is kept encrypted and never shown again.',
    gotifyServer: 'Gotify server',
    gotifyServerHelp: 'Its address, such as https://gotify.example.org.',
    appToken: 'Application token',
    appTokenHelp: {
      gotify:
        'In Gotify: Apps, Create Application, then copy its token. It is kept encrypted and never shown again.',
      pushover:
        'The API token of an application created on Pushover. It is kept encrypted and never shown again.',
    },
    userKey: 'User key',
    userKeyHelp:
      'Shown on Pushover’s dashboard once signed in, or a group key. It is kept encrypted and never shown again.',
    events: 'Events',
    eventsHelp: {
      server: 'What it is told of, for every user.',
      own: 'What it is told of, about you.',
    },
    eventNames: {
      new_episode: 'New episodes',
      recording_finished: 'Recording finished',
      recording_failed: 'Recording failed',
      health_problem: 'Health problem found',
      health_solved: 'Health problem solved',
      user_joined: 'User joined',
      new_version: 'New version',
      playback_started: 'Playback started',
      playback_paused: 'Playback paused',
      playback_resumed: 'Playback resumed',
      playback_stopped: 'Playback stopped',
    },
    eventHelp: {
      new_episode:
        'An episode of a series played or marked favorite is out: once per episode, never for those already out.',
      recording_finished: 'A Live TV recording ended, whole or with part of it missing.',
      recording_failed: 'A Live TV recording recorded nothing.',
      health_problem: 'System › Health found a problem, in two checks in a row.',
      health_solved: 'A problem System › Health found is gone.',
      user_joined: 'Someone created their account through an invite link.',
      new_version:
        'A new version of Polyfin is out, as the daily check found it: once per version, with its release notes.',
      playback_started:
        'A video or a song starts playing in a Jellyfin app, with who plays it, on which device, and whether it is converted.',
      playback_paused: 'A playback is paused. Positions as it plays are never sent.',
      playback_resumed: 'A paused playback plays again.',
      playback_stopped: 'A playback stops, or its app stopped reporting it for five minutes.',
    },
    enabled: 'Send messages to this target',
    create: 'Add',
    creating: 'Adding…',
    save: 'Save',
    saving: 'Saving…',
    created: (name: string) => `${name} added.`,
    saved: (name: string) => `${name} saved.`,
    test: 'Send a test',
    testing: 'Sending…',
    testDelivered: (name: string) => `${name} received the test message.`,
    testFailed: (name: string) => `${name} did not accept the test message.`,
    edit: 'Edit',
    delete: 'Delete',
    deleteTitle: (name: string) => `Delete ${name}?`,
    deleteBody: 'Messages stop going to it, and those waiting for it are dropped.',
    deleted: (name: string) => `${name} deleted.`,
    status: {
      working: 'Working',
      waiting: 'Nothing sent yet',
      off: 'Off',
      refused: 'Refused',
      rejected: 'Message refused',
      unreachable: 'Not reached',
      unreadable: 'Enter it again',
    },
    /** The code a target answered with: an HTTP status, or an SMTP server's code for email. */
    answer: (code: string, email: boolean) => (email ? `SMTP ${code}` : `HTTP ${code}`),
    problem: {
      refused: (answer: string) =>
        `The target refused Polyfin (${answer}): check its address, token or key.`,
      refusedEmail: (answer: string) =>
        `The SMTP server refused Polyfin (${answer}): check its user and password, the sender address and this address.`,
      rejected: (answer: string) => `The target refused the last message (${answer}).`,
      unreachable:
        'Recent messages could not be delivered: the target did not answer, or answered with errors. Each message is tried again for an hour.',
      unreadable:
        'Its address, token or key cannot be decrypted with POLYFIN_SECRET_KEY: nothing is sent to it until it is entered again.',
    },
    lastSent: 'Last message ',
    nothingSent: 'No message sent yet',
    noEvents: 'No event chosen',
  },
}

export default notifications
