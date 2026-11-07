import { useEffect, useState } from 'react'
import { CheckCircleIcon, DeviceMobileIcon, PlusIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { approveQuickConnect, lookupQuickConnect, queryClient, queryKeys } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { dateTime, errorMessage, relativeTime } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Field,
  IconTile,
  InlineError,
  Notice,
  Panel,
  PanelFooter,
  Skeleton,
  TextInput,
} from '@/ui'

/** Jellyfin apps show 6 digits, Silo’s TV apps 8. */
const shortLength = 6
const longLength = 8
/** How long 6 digits wait for more before they are looked up: they may begin 8. */
const typingPause = 700

/**
 * `/me/quick-connect`: approving a device's Quick Connect code. `?code=` fills it in, as `/activate`
 * does for the QR code of Silo’s TV apps.
 */
export default function QuickConnectRoute() {
  const { language, t } = useI18n()
  const text = t.quickConnect
  const user = useSessionUser()
  const [searchParams] = useSearchParams()
  const [code, setCode] = useState(() =>
    (searchParams.get('code') ?? '').replace(/\D/g, '').slice(0, longLength),
  )
  const [settled, setSettled] = useState(code)
  const [approvedDevice, setApprovedDevice] = useState<string | null>(null)
  const complete = code.length === longLength || (code.length === shortLength && settled === code)

  useEffect(() => {
    const timer = setTimeout(() => setSettled(code), typingPause)
    return () => clearTimeout(timer)
  }, [code])

  const lookup = useQuery({
    queryKey: ['quick-connect', code],
    queryFn: ({ signal }) => lookupQuickConnect(code, signal),
    enabled: complete && approvedDevice === null,
    retry: false,
    gcTime: 0,
  })

  const approve = useMutation({
    mutationFn: approveQuickConnect,
    onSuccess: () => {
      setApprovedDevice(lookup.data?.deviceName ?? '')
      void queryClient.invalidateQueries({ queryKey: queryKeys.myDevices })
    },
  })

  function reset() {
    setCode('')
    setApprovedDevice(null)
    approve.reset()
  }

  return (
    <PageLayout title={text.title} lede={text.description}>
      <div className="grid max-w-[48rem] gap-4">
        {approvedDevice !== null ? (
          <Panel as="div">
            <div className="flex flex-col items-start gap-4">
              <Notice tone="ok" live>
                {text.approved(approvedDevice, user.name)}
              </Notice>
              <Button variant="primary" icon={PlusIcon} onClick={reset}>
                {text.another}
              </Button>
            </div>
          </Panel>
        ) : (
          <>
            <Panel as="div">
              {/* The code in one big well, its figures spaced like the device shows them. */}
              <Field label={text.code} help={text.codeHint} className="max-w-[22rem]">
                <TextInput
                  value={code}
                  onValue={(value) => {
                    approve.reset()
                    setCode(value.replace(/\D/g, '').slice(0, longLength))
                  }}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}([0-9]{2})?"
                  maxLength={longLength}
                  placeholder="000000"
                  autoFocus
                  mono
                  className="h-15 max-sm:h-14 [&_input]:figures [&_input]:text-[1.75rem] [&_input]:tracking-[0.32em] max-sm:[&_input]:text-[1.5rem] max-sm:[&_input]:tracking-[0.24em]"
                />
              </Field>
              <ol
                aria-label={text.stepsLabel}
                className="mt-5 grid gap-2 md:grid-cols-3 max-md:mt-4"
              >
                {text.steps.map((step, index) => (
                  <li
                    key={step}
                    className="rounded-row bg-s1 px-3.5 pt-3.5 pb-4 text-small text-ink-2"
                  >
                    <span
                      aria-hidden="true"
                      className="figures mb-2 grid size-7 place-items-center rounded-full bg-s3 text-small font-bold text-ink"
                    >
                      {index + 1}
                    </span>
                    {step}
                  </li>
                ))}
              </ol>
            </Panel>

            {complete && lookup.isPending && (
              <div
                role="status"
                aria-label={text.lookingUp}
                className="flex items-center gap-3.5 rounded-panel bg-s1 p-4.5"
              >
                <Skeleton className="size-11 rounded-row" />
                <span className="flex flex-1 flex-col gap-2">
                  <Skeleton className="h-4 w-48" />
                  <Skeleton className="h-3.5 w-64" />
                </span>
              </div>
            )}
            {complete && lookup.isError && (
              <InlineError onRetry={() => void lookup.refetch()} retrying={lookup.isFetching}>
                {errorMessage(t, lookup.error)}
              </InlineError>
            )}
            {complete && lookup.data && (
              <Panel
                title={text.requestTitle}
                media={<IconTile icon={DeviceMobileIcon} size="lg" />}
                footer={
                  <PanelFooter>
                    <Button
                      variant="primary"
                      icon={CheckCircleIcon}
                      loading={approve.isPending}
                      onClick={() => approve.mutate(code)}
                    >
                      {approve.isPending ? text.approving : text.approve}
                    </Button>
                  </PanelFooter>
                }
              >
                <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-5 gap-y-1.5 text-body">
                  <dt className="text-ink-3">{text.device}</dt>
                  <dd className="font-medium text-ink">{lookup.data.deviceName}</dd>
                  <dt className="text-ink-3">{text.app}</dt>
                  <dd className="text-ink">
                    {lookup.data.appName}{' '}
                    <span className="figures text-ink-2">{lookup.data.appVersion}</span>
                  </dd>
                  <dt className="text-ink-3">{text.requested}</dt>
                  <dd className="text-ink">
                    <time
                      dateTime={lookup.data.requestedAt}
                      title={dateTime(lookup.data.requestedAt, language)}
                    >
                      {relativeTime(lookup.data.requestedAt, language, t.time.justNow)}
                    </time>
                  </dd>
                </dl>
                {approve.isError && (
                  <Notice tone="danger" live className="mt-3.5">
                    {errorMessage(t, approve.error)}
                  </Notice>
                )}
              </Panel>
            )}
          </>
        )}
      </div>
    </PageLayout>
  )
}
