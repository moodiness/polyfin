import { useMutation } from '@tanstack/react-query'
import { createContext, use } from 'react'
import { queryClient, queryKeys, signOut, type SessionUser, type Status } from '@/api'
import { declineWebClientToken } from '@/webClientSession'

/** The signed-in user; the session gate provides it around every signed-in page. */
export const SessionContext = createContext<SessionUser | null>(null)

/** The signed-in user. Only pages behind the session gate may call it. */
export function useSessionUser(): SessionUser {
  const user = use(SessionContext)
  if (!user) throw new Error('useSessionUser must be used behind the session gate')
  return user
}

/**
 * Signs out, then shows the sign-in page in place of the current one. The web client's sign-in in
 * this browser does not sign the admin app in again, until the web client signs in anew.
 */
export function useSignOut() {
  return useMutation({
    mutationFn: signOut,
    onSettled: () => {
      declineWebClientToken(queryClient.getQueryData<Status>(queryKeys.status)?.serverId)
      // Update the mounted session query in place: removing it would detach its observer and keep
      // the signed-in screen. The answers kept for the user go with it (see answersFor in api.ts).
      queryClient.setQueryData(queryKeys.session, null)
    },
  })
}
