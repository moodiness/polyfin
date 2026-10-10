/** Sign-in and the first administrator. */
const auth = {
  setup: {
    title: 'Welcome to Polyfin',
    description:
      'No administrator exists yet. Create the first account to manage this server and sign in from Jellyfin apps.',
    codeHelp:
      'For security, Polyfin printed a one-time setup code in its log when it started. Run docker logs <container> (on Unraid, open the container log) and look for the line with the setup code.',
    setupCode: 'Setup code',
    setupCodeHint: 'Format XXXX-XXXX. Case and dash do not matter.',
    name: 'Administrator name',
    password: 'Password',
    confirmPassword: 'Confirm password',
    submit: 'Create administrator',
    submitting: 'Creating…',
  },
  login: {
    title: 'Sign in',
    description: 'Sign in with your Polyfin account to continue.',
    name: 'Name',
    password: 'Password',
    submit: 'Sign in',
    submitting: 'Signing in…',
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
