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
}

export default auth
