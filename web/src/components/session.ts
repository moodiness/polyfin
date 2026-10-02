import { useOutletContext } from 'react-router'
import type { SessionUser } from '@/api'

/** The signed-in user, provided by the session gate to every page it renders. */
export function useSessionUser(): SessionUser {
  return useOutletContext<SessionUser>()
}
