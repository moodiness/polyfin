import { useParams } from 'react-router'
import UsersPage from '@/pages/UsersPage'

/**
 * `/users` and `/users/:id`, which opens that user's editor.
 * Temporary: renders the page from before « Nuit » inside the new shell, until the users area
 * replaces it.
 */
export default function UsersRoute() {
  const { id } = useParams()
  // A new address opens its user afresh.
  return <UsersPage key={id ?? ''} openId={id} />
}
