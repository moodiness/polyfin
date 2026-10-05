import { useState } from 'react'
import { CheckCircleIcon, DeviceMobileIcon, PlusIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
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

const codeLength = 6

/** `/me/quick-connect`: approving a device's Quick Connect code. */
export default function QuickConnectRoute() {
  const { language, t } = useI18n()
  const text = t.quickConnect
  const user = useSessionUser()
  const [code, setCode] = useState('')
  const [approvedDevice, setApprovedDevice] = useState<string | null>(null)
  const complete = code.length === codeLength

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
      <div className="grid max-w-[784px] gap-5">
        {approvedDevice !== null ? (
          <Panel as="div">
            <div className="flex flex-col items-start gap-5">
              <Notice tone="ok" live>
                {text.approved(approvedDevice, user.name)}
              </Notice>
              <Button icon={PlusIcon} onClick={reset}>
                {text.another}
              </Button>
            </div>
          </Panel>
        ) : (
          <>
            <Panel as="div">
              <div className="grid gap-8 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                <Field label={text.code} help={text.codeHint}>
                  <TextInput
                    value={code}
                    onValue={(value) => {
                      approve.reset()
                      setCode(value.replace(/\D/g, '').slice(0, codeLength))
                    }}
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    pattern="[0-9]{6}"
                    maxLength={codeLength}
                    placeholder="000000"
                    autoFocus
                    mono
                    className="h-14 text-[24px] tracking-[0.4em] [&_input]:text-[24px] [&_input]:tracking-[0.4em]"
                  />
                </Field>
                <div>
                  <p className="text-control font-medium text-ink">{text.stepsLabel}</p>
                  <ol className="mt-3 flex flex-col gap-2.5 text-small text-ink-2">
                    {text.steps.map((step, index) => (
                      <li key={step} className="flex gap-3">
                        <span
                          aria-hidden="true"
                          className="figures grid size-5 shrink-0 place-items-center rounded-md border border-line-2 bg-s2 text-micro text-ink-2"
                        >
                          {index + 1}
                        </span>
                        {step}
                      </li>
                    ))}
                  </ol>
                </div>
              </div>
            </Panel>

            {complete && lookup.isPending && (
              <div
                role="status"
                aria-label={text.lookingUp}
                className="flex items-center gap-4 rounded-panel border border-line-2 bg-s1 p-6"
              >
                <Skeleton className="size-10 rounded-row" />
                <span className="flex flex-1 flex-col gap-2">
                  <Skeleton className="h-3.5 w-40" />
                  <Skeleton className="h-3 w-56" />
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
                media={<IconTile icon={DeviceMobileIcon} />}
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
                <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2.5 text-control">
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
                  <Notice tone="danger" live className="mt-5">
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
