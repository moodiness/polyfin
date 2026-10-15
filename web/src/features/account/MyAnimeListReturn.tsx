import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { ApiError, exchangeTrackingCode, queryClient, queryKeys } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Notice, Spinner, TextLink, useToast } from '@/ui'

const name = 'MyAnimeList'

/**
 * The page MyAnimeList sends the browser back to once the user allowed Polyfin there: it connects
 * the account with the code the address brings, then opens My account.
 */
export default function MyAnimeListReturn() {
  const { t } = useI18n()
  const text = t.account.tracking
  const navigate = useNavigate()
  const toast = useToast()
  const [failure, setFailure] = useState<unknown>(null)
  const started = useRef(false)

  useEffect(() => {
    // The code works once: a second render must not send it again.
    if (started.current) return
    started.current = true
    exchangeTrackingCode('myanimelist', window.location.href)
      .then(() => {
        toast(text.connectedToast(name))
        void navigate('/me/account', { replace: true })
      })
      .catch((error: unknown) => setFailure(error))
      .finally(() => void queryClient.invalidateQueries({ queryKey: queryKeys.tracking }))
  }, [navigate, text, toast])

  const message =
    failure instanceof ApiError && failure.code === 'invalid_code'
      ? text.invalidCode(name)
      : failure instanceof ApiError && failure.code === 'app_refused'
        ? text.appRefused(name)
        : failure instanceof ApiError && failure.code === 'service_unreachable'
          ? text.serviceUnreachable(name)
          : failure !== null && errorMessage(t, failure)
  return (
    <PageLayout title={text.title}>
      {failure === null ? (
        <p role="status" className="flex items-center gap-2 text-control text-ink-2">
          <Spinner />
          {text.returning(name)}
        </p>
      ) : (
        <Notice
          tone="danger"
          live
          action={
            <TextLink to="/me/account" arrow>
              {t.nav.myAccount}
            </TextLink>
          }
        >
          {message}
        </Notice>
      )}
    </PageLayout>
  )
}
