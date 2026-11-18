import { useEffect, useState, type FormEvent } from 'react'
import { CheckCircleIcon, DeviceMobileIcon, PlusIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { ApiError, approveQuickConnect, lookupQuickConnect, queryClient, queryKeys } from '@/api'
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
  Segmented,
  Skeleton,
  StatusPill,
  TextInput,
} from '@/ui'

/** Jellyfin apps show 6 digits, Silo’s TV apps 8. */
const shortLength = 6
const longLength = 8
/** How long 6 digits wait for more before they are looked up: they may begin 8. */
const typingPause = 700

/**
 * `/me/quick-connect`: approving a device's Quick Connect code. `?code=`, as `/activate` adds for
 * the QR code of Silo’s TV apps, finds the device and shows it, and approves nothing: Approve comes
 * once the person typed the code their device shows. An account with two-step sign-in then
 * confirms who they are, since the device signs in without the second step.
 */
export default function QuickConnectRoute() {
  const { language, t } = useI18n()
  const text = t.quickConnect
  const confirmation = t.dashboard.server
  const user = useSessionUser()
  const [searchParams] = useSearchParams()
  /** The code the link brought, when it is whole. */
  const [linked, setLinked] = useState(() => {
    const code = (searchParams.get('code') ?? '').replace(/\D/g, '').slice(0, longLength)
    return code.length === shortLength || code.length === longLength ? code : ''
  })
  const [code, setCode] = useState('')
  const [settled, setSettled] = useState('')
  const [approvedDevice, setApprovedDevice] = useState<string | null>(null)
  const [method, setMethod] = useState<'code' | 'password'>('code')
  const [proof, setProof] = useState('')
  /** Typed whole: 8 digits, or 6 after a pause, unless they begin the 8 the link brought. */
  const typed =
    code.length === longLength ||
    (code.length === shortLength &&
      settled === code &&
      !(linked.length === longLength && linked.startsWith(code)))
  /** The code looked up: the one typed, else the link's. Only a typed one is approved. */
  const shown = typed ? code : linked

  useEffect(() => {
    const timer = setTimeout(() => setSettled(code), typingPause)
    return () => clearTimeout(timer)
  }, [code])

  const lookup = useQuery({
    queryKey: ['quick-connect', shown],
    queryFn: ({ signal }) => lookupQuickConnect(shown, signal),
    enabled: shown !== '' && approvedDevice === null,
    retry: false,
    gcTime: 0,
    // An unknown code counts toward the sign-in attempts: it is asked once, not at each return to
    // the page.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
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
    setLinked('')
    setProof('')
    setApprovedDevice(null)
    approve.reset()
  }

  const request = lookup.data
  const fieldError =
    approve.error instanceof ApiError &&
    ['invalid_code', 'wrong_password'].includes(approve.error.code)
      ? errorMessage(t, approve.error)
      : undefined
  const formId = 'quick-connect-approval'

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
              <Field
                label={text.code}
                help={linked !== '' && !typed ? text.linkedHint : text.codeHint}
                className="max-w-[22rem]"
              >
                <TextInput
                  value={code}
                  onValue={(value) => {
                    approve.reset()
                    setProof('')
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

            {shown !== '' && lookup.isPending && (
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
            {shown !== '' && lookup.isError && (
              <InlineError onRetry={() => void lookup.refetch()} retrying={lookup.isFetching}>
                {errorMessage(t, lookup.error)}
              </InlineError>
            )}
            {shown !== '' && request && (
              <Panel
                title={text.requestTitle}
                media={<IconTile icon={DeviceMobileIcon} size="lg" />}
                footer={
                  // A code a link brought shows the device; only a typed one is approved.
                  typed && (
                    <PanelFooter>
                      <Button
                        type="submit"
                        form={formId}
                        variant="primary"
                        icon={CheckCircleIcon}
                        loading={approve.isPending}
                        disabled={request.identity && proof === ''}
                      >
                        {approve.isPending ? text.approving : text.approve}
                      </Button>
                    </PanelFooter>
                  )
                }
              >
                <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-5 gap-y-1.5 text-body">
                  <dt className="text-ink-3">{text.device}</dt>
                  <dd className="font-medium break-words text-ink">{request.deviceName}</dd>
                  <dt className="text-ink-3">{text.app}</dt>
                  <dd className="break-words text-ink">
                    {request.appName}{' '}
                    <span className="figures text-ink-2">{request.appVersion}</span>
                  </dd>
                  <dt className="text-ink-3">{text.address}</dt>
                  <dd className="flex flex-wrap items-center gap-x-3 gap-y-1 text-ink">
                    <span className="figures break-all">{request.address}</span>
                    <StatusPill tone={request.network === 'other' ? 'warn' : 'ok'}>
                      {text.network[request.network]}
                    </StatusPill>
                  </dd>
                  <dt className="text-ink-3">{text.requested}</dt>
                  <dd className="text-ink">
                    <time
                      dateTime={request.requestedAt}
                      title={dateTime(request.requestedAt, language)}
                    >
                      {relativeTime(request.requestedAt, language, t.time.justNow)}
                    </time>
                  </dd>
                </dl>
                <p className="mt-3 text-small text-ink-2">{text.claims}</p>
                {request.network === 'other' && (
                  <Notice tone="warn" className="mt-3.5">
                    {text.otherNetwork}
                  </Notice>
                )}
                {!typed && (
                  <Notice tone="info" live className="mt-3.5">
                    {text.linked}
                  </Notice>
                )}
                {typed && (
                  <form
                    id={formId}
                    noValidate
                    className={request.identity ? 'mt-4 space-y-4' : undefined}
                    onSubmit={(event: FormEvent<HTMLFormElement>) => {
                      event.preventDefault()
                      approve.mutate({
                        code: shown,
                        confirmation: code,
                        ...(request.identity && {
                          identity:
                            method === 'code' ? { code: proof.trim() } : { password: proof },
                        }),
                      })
                    }}
                  >
                    {request.identity && (
                      <>
                        <p className="text-control text-ink-2">{text.identity}</p>
                        <Segmented
                          label={confirmation.confirmWith}
                          options={[
                            { value: 'code', label: confirmation.withCode },
                            { value: 'password', label: confirmation.withPassword },
                          ]}
                          value={method}
                          onChange={(next) => {
                            approve.reset()
                            setProof('')
                            setMethod(next)
                          }}
                        />
                        <Field
                          label={method === 'code' ? confirmation.code : confirmation.password}
                          error={fieldError}
                          className="max-w-[22rem]"
                        >
                          <TextInput
                            key={method}
                            type={method === 'code' ? 'text' : 'password'}
                            value={proof}
                            onValue={(next) => {
                              approve.reset()
                              setProof(next)
                            }}
                            autoComplete={method === 'code' ? 'one-time-code' : 'current-password'}
                            mono={method === 'code'}
                            autoFocus
                            required
                            invalid={fieldError !== undefined}
                          />
                        </Field>
                      </>
                    )}
                  </form>
                )}
                {approve.isError && fieldError === undefined && (
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
