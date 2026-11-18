import { IdentificationCardIcon, LinkIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { ApiError, fetchOwnProvider, queryKeys, startOwnProviderLink, type Status } from '@/api'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import { Button, InlineError, Notice, SkeletonText, StatusPill } from '@/ui'

/**
 * What a link asked here came back with, as the server sends the browser back with
 * ?providerLink=: `linked`, or what went wrong. It is taken out of the address as the page loads,
 * so that a reload does not show it again, and shown by the first section drawn.
 */
let returned = takeReturned()

function takeReturned(): string | null {
  const address = new URL(window.location.href)
  const outcome = address.searchParams.get('providerLink')
  if (outcome === null) return null
  address.searchParams.delete('providerLink')
  window.history.replaceState(
    window.history.state,
    '',
    address.pathname + address.search + address.hash,
  )
  return outcome
}

/** Why a link was not made, in the words of the sign-in page where they say it. */
function refusal(t: Messages, outcome: string): string {
  if (outcome === 'provider_link_taken') return t.account.provider.taken
  if (outcome === 'provider_refused' || outcome === 'too_many_attempts')
    return t.login.providerOutcomes[outcome]
  return t.login.providerOutcomes.provider_failed
}

/**
 * My account › Identity provider, shown while the server offers one: whether the account is
 * linked to its owner's account at the provider, and, while it is not, the button that links it.
 * The button asks the server for the provider's page and opens it; the provider sends the browser
 * back here. The link is made for this session's account, so it starts where the session is: at
 * the server's public address, which the provider answers to. Opened at another address, the page
 * names that one instead of the button.
 */
export default function ProviderLink({ provider }: { provider: NonNullable<Status['provider']> }) {
  const { t } = useI18n()
  const text = t.account.provider
  const [outcome] = useState(() => {
    const taken = returned
    returned = null
    return taken
  })
  const state = useQuery({
    queryKey: queryKeys.ownProvider,
    queryFn: ({ signal }) => fetchOwnProvider(signal),
  })
  const start = useMutation({
    mutationFn: startOwnProviderLink,
    onSuccess: ({ address }) => window.location.assign(address),
  })

  if (state.isPending) return <SkeletonText lines={1} />
  if (state.isError) {
    return (
      <InlineError onRetry={() => void state.refetch()} retrying={state.isFetching}>
        {errorMessage(t, state.error)}
      </InlineError>
    )
  }
  const { linked } = state.data
  const publicAddress = new URL(provider.start).origin
  const failed = start.isError
    ? start.error instanceof ApiError
      ? refusal(t, start.error.code)
      : errorMessage(t, start.error)
    : outcome !== null && outcome !== 'linked' && !linked
      ? refusal(t, outcome)
      : null

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        {linked ? (
          <StatusPill tone="ok" icon={LinkIcon}>
            {text.linked}
          </StatusPill>
        ) : (
          <StatusPill tone="muted">{text.notLinked}</StatusPill>
        )}
      </div>
      {failed !== null && (
        <Notice tone="danger" live>
          {failed}
        </Notice>
      )}
      {!linked &&
        (publicAddress === window.location.origin ? (
          <Button
            icon={IdentificationCardIcon}
            loading={start.isPending || start.isSuccess}
            onClick={() => start.mutate()}
          >
            {text.link(provider.name)}
          </Button>
        ) : (
          <p className="max-w-[72ch] text-body text-ink-2">
            {text.otherAddress}{' '}
            <a
              href={`${publicAddress}/admin/me/account#provider`}
              className="font-medium break-all text-link transition-colors duration-160 hover:text-link-hover"
            >
              {publicAddress}
            </a>
          </p>
        ))}
    </div>
  )
}
