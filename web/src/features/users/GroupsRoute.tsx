import { useParams } from 'react-router'
import GroupDetail from './GroupDetail'
import GroupsList from './GroupsList'

/** `/users/groups` lists the groups; `/users/groups/:id` is one group's page. */
export default function GroupsRoute() {
  const { id } = useParams()
  // A new address opens its group afresh.
  return id === undefined ? <GroupsList /> : <GroupDetail key={id} id={id} />
}
