import { useMutation } from '@tanstack/react-query'
import { createContext, use } from 'react'
import { queryClient, queryKeys, signOut, type SessionUser } from '@/api'

/** The signed-in user; the session gate provides it around every signed-in page. */
export const SessionContext = createContext<SessionUser | null>(null)

/** The signed-in user. Only pages behind the session gate may call it. */
export function useSessionUser(): SessionUser {
  const user = use(SessionContext)
  if (!user) throw new Error('useSessionUser must be used behind the session gate')
  return user
}

/** Signs out, then shows the sign-in page in place of the current one. */
export function useSignOut() {
  return useMutation({
    mutationFn: signOut,
    onSettled: () => {
      // Update the mounted session query in place (removing it would detach its observer and
      // keep the signed-in screen), then forget the rest; the status query stays public.
      queryClient.setQueryData(queryKeys.session, null)
      queryClient.removeQueries({
        predicate: (query) => query.queryKey[0] !== 'status' && query.queryKey[0] !== 'session',
      })
    },
  })
}
