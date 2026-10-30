/** Sign-in and the first administrator. */
const auth = {
  setup: {
    title: 'Welcome to Polyfin',
    description:
      'No administrator exists yet. Create the first account to manage this server and sign in from Jellyfin apps.',
    codeHelp: {
      container:
        'For security, Polyfin printed a one-time setup code in its log when it started. Run docker logs <container> (on Unraid, open the container log) and look for the line with the setup code.',
      computer:
        'For security, Polyfin printed a one-time setup code in its log when it started. With Polyfin Desktop, choose Setup code in its menu to copy it; otherwise, look for the line with the setup code in Polyfin’s log.',
    },
    setupCode: 'Setup code',
    setupCodeHint: 'Format XXXX-XXXX. Case and dash do not matter.',
    name: 'Administrator name',
    password: 'Password',
    confirmPassword: 'Confirm password',
    submit: 'Create administrator',
    submitting: 'Creating…',
    restoreInstead: 'Restore a backup instead',
    createInstead: 'Create an administrator instead',
    restoreTitle: 'Restore a backup',
    restoreLede:
      'Bring this Polyfin back from a backup of its database: its administrators, users, settings and watch history.',
    restoreFile: 'Backup file',
    restoreFileHelp:
      'A .dump or .dump.enc file Polyfin made, from its backups folder or a destination.',
    restoreChoose: 'Choose a file',
    restoreSend: 'Send and check',
    restoreSending: 'Sending and checking…',
    restoreUnlockHelp: 'This backup is encrypted: type the passphrase it was made with.',
    restoreReady: (file: string) =>
      `${file} is a backup of Polyfin’s database that this Polyfin can restore.`,
    restoreSteps: [
      'Polyfin stops, restores the backup into its database in one go, then starts again on it. If anything fails, the database stays as it is now.',
      'Afterwards, sign in with an administrator of the backup.',
    ],
    restoreStart: 'Restore',
    restoreRestoring:
      'Polyfin is restoring the backup, then starts again. This page shows the sign-in once it answers.',
    restoreFailed:
      'The backup could not be restored: Polyfin started again without it. Its log tells why, and holds a new setup code.',
  },
  login: {
    title: 'Sign in',
    description: 'Sign in with your Polyfin account to continue.',
    name: 'Name',
    password: 'Password',
    submit: 'Sign in',
    submitting: 'Signing in…',
    /** The second step, after a right password. */
    codeTitle: 'Two-step sign-in',
    codeDescription: 'Enter the 6-digit code your authenticator app shows for Polyfin.',
    code: 'Code',
    codeHelp: 'Lost your phone? Enter one of your recovery codes instead.',
    verify: 'Verify',
    verifying: 'Verifying…',
    restart: 'Sign in as someone else',
    setupTitle: 'Set up two-step sign-in',
    setupDescription:
      'Administrators must use two-step sign-in on this server. Add your account to an authenticator app on your phone, then enter the code it shows.',
    recoveryTitle: 'Keep your recovery codes',
    continue: 'Continue',
    /** The identity provider's button, and what its sign-in can come back with. */
    providerSignIn: (name: string) => `Sign in with ${name}`,
    providerOutcomes: {
      provider_refused: 'The sign-in with the identity provider was cancelled.',
      provider_no_account:
        'No account on this server matches your account at the identity provider.',
      account_disabled: 'This account is disabled.',
      outside_allowed_hours: 'This account may not sign in at this time.',
      outside_local_network: 'This account may only sign in from the local network.',
      too_many_attempts: 'Too many attempts. Try again later.',
      provider_failed: 'The sign-in with the identity provider failed. Try again.',
    },
  },
  invite: {
    title: (server: string) => `Join ${server}`,
    description:
      'Choose the name and password of your account. You will use them in Jellyfin apps too.',
    name: 'Your name',
    password: 'Password',
    confirmPassword: 'Confirm password',
    submit: 'Create my account',
    submitting: 'Creating…',
    gone: {
      invite_unknown: {
        title: 'This invite link does not work',
        description:
          'Check that the whole link was copied, or ask the person who sent it for a new one.',
      },
      invite_used_up: {
        title: 'This invite link is used up',
        description:
          'It has created all the accounts it could. Ask the person who sent it for a new one.',
      },
      invite_expired: {
        title: 'This invite link has expired',
        description: 'Ask the person who sent it for a new one.',
      },
      invite_revoked: {
        title: 'This invite link was revoked',
        description: 'It no longer works. Ask the person who sent it for a new one.',
      },
    },
  },
}

export default auth
