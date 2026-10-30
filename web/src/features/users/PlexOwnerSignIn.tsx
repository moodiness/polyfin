import { SignInIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import {
  fetchPlexOwnerAccess,
  type JellyfinCredentials,
  type PlexAccess,
  type PlexAccount,
} from '@/api'
import { PlexSignInWaiting, usePlexSignIn } from '@/features/account/PlexImport'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Notice } from '@/ui'

/**
 * The administrators' import of a Plex server may sign in to Plex as the server's owner, on Plex's
 * own page: the server then tells what it reads of each account, all of it with the token Plex
 * gives the owner for it, or its plays only, and hands it to `onAccess`; null until then, or once
 * the sign-in is dropped. The tokens stay on the server: this page never sees them.
 */
export function PlexOwnerSignIn({
  address,
  credentials,
  onAccess,
}: {
  /** The Plex server's address. */
  address: string
  credentials: JellyfinCredentials
  onAccess: (access: Record<string, PlexAccess> | null) => void
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport.plexOwner
  const [account, setAccount] = useState<PlexAccount | null>(null)
  const access = useMutation({
    mutationFn: () => fetchPlexOwnerAccess({ kind: 'plex', address, ...credentials }),
    onSuccess: onAccess,
  })
  const { link, failure, begin, cancel } = usePlexSignIn('owner', (read) => {
    setAccount(read)
    access.mutate()
  })

  const error = begin.isError
    ? errorMessage(t, begin.error)
    : failure !== null
      ? errorMessage(t, failure)
      : access.isError
        ? errorMessage(t, access.error)
        : undefined

  return (
    <section
      aria-label={text.title}
      className="flex flex-col gap-4 rounded-row border border-line bg-bg px-[18px] py-4 max-sm:p-3.5"
    >
      <div className="flex flex-col gap-1">
        <h3 className="text-control font-medium text-ink">{text.title}</h3>
        {account === null && <p className="text-small text-ink-3">{text.help}</p>}
      </div>
      {link !== null && <PlexSignInWaiting link={link} />}
      {account !== null && (
        <p role="status" className="text-small text-ink">
          {access.isPending
            ? text.checking
            : access.isSuccess
              ? text.signedInAs(account.name)
              : t.account.plexImport.signedInAs(account.name)}
        </p>
      )}
      {access.isSuccess && <Notice>{text.notice}</Notice>}
      {error && (
        <Notice tone="danger" live>
          {error}
        </Notice>
      )}
      <div className="flex flex-wrap gap-2">
        {account === null && link === null ? (
          <Button
            icon={SignInIcon}
            loading={begin.isPending}
            onClick={() => {
              access.reset()
              begin.mutate()
            }}
          >
            {begin.isPending ? t.account.plexImport.signingIn : text.signIn}
          </Button>
        ) : (
          <Button
            variant="ghost"
            loading={cancel.isPending}
            onClick={() =>
              cancel.mutate(undefined, {
                onSettled: () => {
                  setAccount(null)
                  access.reset()
                  onAccess(null)
                },
              })
            }
          >
            {account === null ? t.account.plexImport.cancel : text.signOut}
          </Button>
        )}
      </div>
    </section>
  )
}
