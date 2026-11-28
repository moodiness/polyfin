/**
 * A page that fails as it draws, to see the error page in the shell (`/admin/dev/error`,
 * development builds only).
 */
export default function FailingPage(): never {
  throw new TypeError("Cannot read properties of undefined (reading 'title')")
}
