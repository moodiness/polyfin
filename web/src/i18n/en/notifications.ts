/** Notification targets: Settings › Notifications and My account › Notifications. */
const notifications = {
  notifications: {
    title: 'Notifications',
    loading: 'Loading the targets…',
    empty: 'No target yet.',
    emptyHint: {
      server:
        'Add a webhook, a Discord channel or an ntfy topic to be told of new episodes, recordings and Health problems.',
      own: 'Add a webhook, a Discord channel or an ntfy topic to be told when a new episode of a series you follow is out, or when your recordings end.',
    },
    serverTargets: 'Server targets',
    serverTargetsHelp:
      'They receive the events of every user: one message per new episode, whoever follows its series, and every recording. Each user can add their own targets under My account › Notifications.',
    add: 'Add a target',
    addTitle: 'Add a target',
    editTitle: (name: string) => `Edit ${name}`,
    kind: 'Kind',
    kinds: { webhook: 'Webhook', discord: 'Discord', ntfy: 'ntfy' },
    kindHelp: {
      webhook:
        'Polyfin posts each event to this address as JSON, described in the documentation for app developers.',
      discord: 'Polyfin posts each event to a Discord channel, as a message with a link.',
      ntfy: 'Polyfin publishes each event to an ntfy topic, which the ntfy app shows on your phone.',
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
    },
    eventHelp: {
      new_episode:
        'An episode of a series played or marked favorite is out: once per episode, never for those already out.',
      recording_finished: 'A Live TV recording ended, whole or with part of it missing.',
      recording_failed: 'A Live TV recording recorded nothing.',
      health_problem: 'System › Health found a problem, in two checks in a row.',
      health_solved: 'A problem System › Health found is gone.',
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
    problem: {
      refused: (status: string) =>
        `The target refused Polyfin (HTTP ${status}): check its address or token.`,
      rejected: (status: string) => `The target refused the last message (HTTP ${status}).`,
      unreachable:
        'Recent messages could not be delivered: the target did not answer, or answered with errors. Each message is tried again for an hour.',
      unreadable:
        'Its address or token cannot be decrypted with POLYFIN_SECRET_KEY: nothing is sent to it until it is entered again.',
    },
    lastSent: 'Last message ',
    nothingSent: 'No message sent yet',
    noEvents: 'No event chosen',
  },
}

export default notifications
