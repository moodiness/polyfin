import LoginPage from '@/pages/LoginPage'

/**
 * Sign-in, shown by the session gate at any address while no one is signed in.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the auth area
 * replaces it.
 */
export default function LoginRoute() {
  return <LoginPage />
}
