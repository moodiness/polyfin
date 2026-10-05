import { useParams } from 'react-router'
import UserDetail from './UserDetail'
import UsersList from './UsersList'

/** `/users` lists the users; `/users/:id` is one user's page. */
export default function UsersRoute() {
  const { id } = useParams()
  // A new address opens its user afresh.
  return id === undefined ? <UsersList /> : <UserDetail key={id} id={id} />
}
