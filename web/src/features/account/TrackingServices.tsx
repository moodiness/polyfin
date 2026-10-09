import {
  ArrowSquareOutIcon,
  ClockCounterClockwiseIcon,
  DownloadSimpleIcon,
  HourglassIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState, type FormEvent, type ReactNode } from 'react'
import {
  ApiError,
  connectTracking,
  disconnectTracking,
  fetchTracking,
  importTracking,
  queryClient,
  queryKeys,
  revealTrackingKey,
  setTrackingImport,
  type TrackingKeyServiceName,
  type TrackingService,
  type TrackingServiceName,
} from '@/api'
import { useSessionUser } from '@/app/session'
import { dateTime, errorMessage, relativeTime } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Button,
  ConfirmDialog,
  cx,
  ExternalButtonLink,
  IconTile,
  InlineError,
  Notice,
  SecretField,
  SkeletonRows,
  StatusPill,
  Switch,
  TextLink,
  useToast,
  type StatusTone,
} from '@/ui'

const serviceNames: Record<TrackingServiceName, string> = {
  trakt: 'Trakt',
  simkl: 'Simkl',
  mdblist: 'MDBList',
  publicmetadb: 'PublicMetaDB',
  lastfm: 'Last.fm',
  listenbrainz: 'ListenBrainz',
}

/** The letters in each service's tile. */
const monograms: Record<TrackingServiceName, string> = {
  trakt: 'T',
  simkl: 'Si',
  mdblist: 'M',
  publicmetadb: 'P',
  lastfm: 'L',
  listenbrainz: 'LB',
}

/** How often the services are read while a code waits to be entered on a service's site. */
const codePollMs = 5000

/** How often the services are read while a watch history is imported. */
const importPollMs = 3000

function codeExpired(code: TrackingService['code'], now: number): boolean {
  return code !== null && Date.parse(code.expiresAt) <= now
}

/** The signed-in user's tracking services: Polyfin tells the connected ones what the user watches. */
export default function TrackingServices() {
  const { t } = useI18n()
  const tracking = useQuery({
    queryKey: queryKeys.tracking,
    queryFn: ({ signal }) => fetchTracking(signal),
    // The server learns when a code is entered: it is asked again while a code is waiting. React
    // Query pauses this while the page is hidden, and stops it when the page is left.
    refetchInterval: (query) =>
      query.state.data?.some(
        (service) => service.code !== null && !codeExpired(service.code, Date.now()),
      )
        ? codePollMs
        : query.state.data?.some((service) => service.importing)
          ? importPollMs
          : false,
  })

  if (tracking.isPending) return <SkeletonRows rows={6} label={t.account.tracking.loading} />
  if (tracking.isError) {
    return (
      <InlineError onRetry={() => void tracking.refetch()} retrying={tracking.isFetching}>
        {errorMessage(t, tracking.error)}
      </InlineError>
    )
  }
  return (
    <ul className="overflow-hidden rounded-panel border border-line-2 bg-s1">
      {tracking.data.map((service) => (
        <ServiceRow key={service.service} service={service} />
      ))}
    </ul>
  )
}

/** The current time, updated once when `until` (a time in ms) passes. */
function useNowUntil(until: number | null): number {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (until === null) return
    const wait = until - Date.now()
    if (wait <= 0) return
    const timer = window.setTimeout(() => setNow(Date.now()), wait + 100)
    return () => window.clearTimeout(timer)
  }, [until])
  return Math.max(now, Date.now())
}

/** Puts the service as the server answered it into the list, without asking for the list again. */
function store(service: TrackingService) {
  queryClient.setQueryData<TrackingService[]>(queryKeys.tracking, (services) =>
    services?.map((current) => (current.service === service.service ? service : current)),
  )
}

/** The text of a failed connection, naming the service it is about. */
function connectError(t: Messages, name: string, error: unknown, music = false): string {
  const text = t.account.tracking
  if (error instanceof ApiError) {
    // The music service connected with a key takes a user token.
    if (error.code === 'invalid_key') return music ? text.invalidToken(name) : text.invalidKey(name)
    if (error.code === 'service_unreachable') return text.serviceUnreachable(name)
    if (error.code === 'not_available') return text.notAvailable(name)
    if (error.code === 'app_refused') return text.appRefused(name)
  }
  return errorMessage(t, error)
}

/** A time as "5 minutes ago", with the full date on hover. */
function Ago({ iso }: { iso: string }) {
  const { language, t } = useI18n()
  return (
    <time dateTime={iso} title={dateTime(iso, language)}>
      {relativeTime(iso, language, t.time.justNow)}
    </time>
  )
}

function ServiceRow({ service }: { service: TrackingService }) {
  const { language, t } = useI18n()
  const text = t.account.tracking
  const user = useSessionUser()
  const toast = useToast()
  const name = serviceNames[service.service]
  const code = service.code
  const now = useNowUntil(code === null ? null : Date.parse(code.expiresAt))
  const expired = codeExpired(code, now)
  const [confirming, setConfirming] = useState(false)

  // A code the server dropped without connecting the service expired or was refused on its site.
  // A sign-in has no code to show: its page names it.
  const codeKey = code === null ? null : code.userCode || code.verificationUrl
  const [lastCodeKey, setLastCodeKey] = useState(codeKey)
  const [codeEnded, setCodeEnded] = useState(false)
  if (codeKey !== lastCodeKey) {
    setLastCodeKey(codeKey)
    setCodeEnded(
      codeKey === null &&
        lastCodeKey !== null &&
        service.problem !== 'app_refused' &&
        (!service.connected || service.problem !== null),
    )
  }

  const connect = useMutation({
    mutationFn: (key?: string) => connectTracking(service.service, key),
    onSuccess: (result) => {
      setCodeEnded(false)
      store(result)
      if (result.connected && result.problem === null) toast(text.connectedToast(name))
    },
  })
  const disconnect = useMutation({
    mutationFn: () => disconnectTracking(service.service),
    onSuccess: () => {
      connect.reset()
      // Cancelling a code is not its expiry: nothing to tell.
      setLastCodeKey(null)
      setCodeEnded(false)
      if (service.connected) toast(text.disconnectedToast(name))
      setConfirming(false)
      store({
        ...service,
        connected: false,
        account: null,
        connectedAt: null,
        lastSentAt: null,
        problem: null,
        code: null,
      })
    },
  })

  const signIn = service.connection === 'signin'
  const waiting = code !== null && !expired
  const status: { tone: StatusTone; label: string; icon?: typeof HourglassIcon } =
    !service.available
      ? { tone: 'muted', label: text.status.unavailable }
      : waiting
        ? {
            tone: 'muted',
            label: signIn ? text.status.waitingSignIn : text.status.waiting,
            icon: HourglassIcon,
          }
        : service.problem === 'app_refused'
          ? { tone: 'danger', label: text.status.appRefused }
          : service.problem === 'reconnect'
            ? { tone: 'danger', label: text.status.reconnect }
            : service.problem === 'unreachable'
              ? { tone: 'warn', label: text.status.unreachable }
              : service.connected
                ? { tone: 'ok', label: text.status.connected }
                : { tone: 'muted', label: text.status.notConnected }

  // The administrator's page where the app is set up, for administrators.
  const setUpLink = user.isAdministrator && (
    <TextLink to="/settings/tracking" arrow>
      {text.setUpApp(name)}
    </TextLink>
  )

  const codeConnect = (label: string, variant: 'primary' | 'secondary') => (
    <Button variant={variant} loading={connect.isPending} onClick={() => connect.mutate(undefined)}>
      {connect.isPending ? text.connecting : label}
    </Button>
  )
  const keyForm = (label: string) => (
    <KeyForm
      keyLabel={service.music ? text.tokenLabel(name) : text.keyLabel(name)}
      help={text.keyHelp[service.service as TrackingKeyServiceName]}
      label={label}
      pending={connect.isPending}
      error={connect.isError ? connectError(t, name, connect.error, service.music) : undefined}
      onChange={() => connect.reset()}
      onConnect={(key) => connect.mutate(key)}
    />
  )
  const appRefused =
    connect.isError && connect.error instanceof ApiError && connect.error.code === 'app_refused'
  const codeError = service.connection !== 'key' && connect.isError && (
    <Notice tone="danger" live action={appRefused ? setUpLink : undefined}>
      {connectError(t, name, connect.error)}
    </Notice>
  )
  const ended = (expired || codeEnded) && (
    <Notice tone="warn" live>
      {signIn ? text.signInEnded : text.codeEnded}
    </Notice>
  )
  const startAgain = signIn ? text.signInAgain : text.newCode
  // The server's app was refused while the code waited; another attempt clears it.
  const refusedApp = service.problem === 'app_refused' && !connect.isError && (
    <Notice tone="danger" action={setUpLink || undefined}>
      {text.appRefused(name)}
    </Notice>
  )
  const cancelError = !service.connected && disconnect.isError && (
    <Notice tone="danger" live>
      {errorMessage(t, disconnect.error)}
    </Notice>
  )

  let sub: ReactNode
  let actions: ReactNode = null
  let body: ReactNode = null
  if (!service.available) {
    sub = text.unavailable(name)
    actions = <Button disabled>{text.connect}</Button>
    body = setUpLink || null
  } else if (waiting) {
    const host = new URL(code.verificationUrl).host
    sub = signIn ? text.signInIntro(name) : text.codeIntro(name)
    actions = (
      <Button variant="ghost" loading={disconnect.isPending} onClick={() => disconnect.mutate()}>
        {t.common.cancel}
      </Button>
    )
    const open = (
      <ExternalButtonLink
        variant="primary"
        href={code.verificationUrl}
        target="_blank"
        rel="noopener noreferrer"
        iconEnd={ArrowSquareOutIcon}
      >
        {text.openSite(host)}
      </ExternalButtonLink>
    )
    const expiry = relativeTime(code.expiresAt, language, t.time.justNow)
    body = (
      <>
        <div className="flex flex-col gap-4 rounded-row border border-line bg-bg px-[18px] py-4 max-sm:p-3.5">
          {signIn ? (
            <>
              <p className="text-control text-ink">{text.signInStep(host)}</p>
              <div className="flex flex-wrap items-center gap-3">{open}</div>
              <p role="status" className="text-small text-ink-3">
                {text.signInWaiting} {text.signInExpires(expiry)}
              </p>
            </>
          ) : (
            <>
              <p className="text-control text-ink">{text.enterCode(host)}</p>
              <div className="flex flex-wrap items-center gap-3">
                <p
                  aria-label={text.codeLabel}
                  className="rounded-field border border-accent/50 bg-s2 px-5 py-2 font-mono text-[26px] font-semibold tracking-[0.2em] text-ink select-all"
                >
                  {code.userCode}
                </p>
                {open}
              </div>
              <p role="status" className="text-small text-ink-3">
                {text.waiting} {text.expires(expiry)}
              </p>
            </>
          )}
        </div>
        {cancelError}
      </>
    )
  } else if (service.connected) {
    sub = (
      <>
        {service.account !== null ? (
          <>
            {text.connectedAs}
            <span className="font-mono text-ink">{service.account}</span>
          </>
        ) : service.connection === 'key' ? (
          text.connectedWithKey
        ) : (
          text.connected
        )}
        {service.connectedAt !== null && text.connectedOn(dateTime(service.connectedAt, language))}
        {service.lastSentAt === null ? (
          text.nothingSent
        ) : (
          <>
            {text.lastSent}
            <Ago iso={service.lastSentAt} />
          </>
        )}
      </>
    )
    const reconnectByCode =
      (service.problem === 'reconnect' || service.problem === 'app_refused') &&
      service.connection !== 'key'
    actions = (
      <>
        {reconnectByCode &&
          codeConnect(
            ended || service.problem === 'app_refused' ? startAgain : text.reconnect,
            'secondary',
          )}
        <Button variant="ghost" onClick={() => setConfirming(true)}>
          {text.disconnect}
        </Button>
      </>
    )
    body = (
      <>
        {service.connection === 'key' && (
          <SecretField
            label={service.music ? text.tokenLabel(name) : text.keyLabel(name)}
            help={text.savedKeyHelp}
            saved
            value={undefined}
            reveal={() => revealTrackingKey(service.service as TrackingKeyServiceName)}
            replaceable={false}
            removable={false}
          />
        )}
        {service.problem === 'reconnect' && (
          <Notice tone="danger">{text.problemReconnect(name)}</Notice>
        )}
        {service.problem === 'unreachable' && (
          <Notice tone="warn">{text.problemUnreachable(name)}</Notice>
        )}
        {refusedApp}
        {ended}
        {codeError}
        {service.problem === 'reconnect' && service.connection === 'key' && keyForm(text.reconnect)}
        {!service.music && <HistoryImport service={service} name={name} />}
      </>
    )
  } else {
    sub =
      service.connection === 'code'
        ? text.codeIntro(name)
        : signIn
          ? text.signInIntro(name)
          : service.music
            ? text.tokenIntro(name)
            : text.keyIntro(name)
    if (service.connection !== 'key')
      actions = codeConnect(ended ? startAgain : text.connect, 'primary')
    const notes = [refusedApp, ended, codeError, cancelError].some(Boolean)
    body =
      service.connection === 'key' || notes ? (
        <>
          {refusedApp}
          {ended}
          {codeError}
          {cancelError}
          {service.connection === 'key' && keyForm(text.connect)}
        </>
      ) : null
  }

  const headingId = useId()
  return (
    <li
      aria-labelledby={headingId}
      className={cx(
        'grid grid-cols-[40px_minmax(0,1fr)_auto] items-start gap-x-4 px-[22px] py-5 [&+&]:border-t [&+&]:border-line',
        'max-sm:grid-cols-[40px_minmax(0,1fr)] max-sm:gap-x-3.5 max-sm:px-4 max-sm:py-[18px]',
      )}
    >
      {service.available ? (
        <IconTile letters={monograms[service.service]} />
      ) : (
        <span
          aria-hidden="true"
          className="inline-grid size-10 place-items-center rounded-row border border-line-2 bg-s2 text-[15px] font-semibold text-ink-3"
        >
          {monograms[service.service]}
        </span>
      )}
      <div className="min-w-0">
        <h3
          id={headingId}
          className={cx(
            'flex h-10 items-center text-[15px] font-semibold tracking-[-0.01em]',
            service.available ? 'text-ink' : 'text-ink-2',
          )}
        >
          {name}
        </h3>
        <p className={cx('-mt-1.5 text-control', service.available ? 'text-ink-2' : 'text-ink-3')}>
          {sub}
        </p>
      </div>
      <div className="flex h-10 items-center justify-end gap-1.5 max-sm:col-span-full max-sm:mt-3 max-sm:h-auto max-sm:flex-wrap">
        <StatusPill tone={status.tone} icon={status.icon} className="max-sm:mr-auto">
          {status.label}
        </StatusPill>
        {actions}
      </div>
      {body !== null && (
        <div className="col-start-2 -col-end-1 mt-4 flex min-w-0 flex-col gap-4 max-sm:col-span-full">
          {body}
        </div>
      )}
      <ConfirmDialog
        open={confirming}
        onClose={() => {
          setConfirming(false)
          disconnect.reset()
        }}
        onConfirm={() => disconnect.mutate()}
        title={text.disconnectTitle(name)}
        confirmLabel={text.disconnect}
        tone="danger"
        busy={disconnect.isPending}
        error={disconnect.isError ? errorMessage(t, disconnect.error) : undefined}
      >
        {text.disconnectBody(name)}
      </ConfirmDialog>
    </li>
  )
}

function KeyForm({
  keyLabel,
  help,
  label,
  pending,
  error,
  onChange,
  onConnect,
}: {
  keyLabel: string
  help: string
  label: string
  pending: boolean
  error: string | undefined
  onChange: () => void
  onConnect: (key: string) => void
}) {
  const { t } = useI18n()
  const text = t.account.tracking
  const [key, setKey] = useState('')

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const trimmed = key.trim()
    if (trimmed !== '') onConnect(trimmed)
  }

  return (
    <form onSubmit={submit} noValidate>
      <SecretField
        label={keyLabel}
        help={help}
        saved={false}
        status={null}
        value={key === '' ? undefined : key}
        onValue={(value) => {
          setKey(value ?? '')
          onChange()
        }}
        error={error}
        actions={
          <Button
            type="submit"
            variant="primary"
            loading={pending}
            disabled={!pending && key.trim() === ''}
          >
            {pending ? text.checking : label}
          </Button>
        }
      />
    </form>
  )
}

/**
 * Whether a connected service's watch history is imported into Polyfin, Import now, and how the
 * last import went.
 */
function HistoryImport({ service, name }: { service: TrackingService; name: string }) {
  const { t } = useI18n()
  const text = t.account.tracking.history
  const labelId = useId()
  const helpId = useId()
  const toggle = useMutation({
    mutationFn: (on: boolean) => setTrackingImport(service.service, on),
    onSuccess: store,
  })
  const now = useMutation({
    mutationFn: () => importTracking(service.service),
    onSuccess: store,
  })
  const last = service.lastImport
  const busy = toggle.isPending || now.isPending
  const failed = toggle.error ?? now.error
  return (
    <div className="rounded-row border border-line bg-bg px-[18px] py-4 max-sm:p-3.5">
      <div className="flex items-start justify-between gap-6 max-sm:gap-4">
        <div className="min-w-0">
          <p id={labelId} className="text-control font-medium text-ink">
            {text.toggle(name)}
          </p>
          <p id={helpId} className="mt-0.5 text-small text-ink-3">
            {text.toggleHelp(name)}
          </p>
        </div>
        <Switch
          checked={service.importHistory}
          disabled={busy}
          labelledBy={labelId}
          describedById={helpId}
          onChange={(on) => {
            now.reset()
            toggle.mutate(on)
          }}
          className="mt-0.5"
        />
      </div>
      {service.importHistory && (
        <div className="mt-3 flex items-center justify-between gap-4 border-t border-line pt-3 max-sm:flex-col max-sm:items-start">
          <p role="status" className="flex min-w-0 items-start gap-2 text-control text-ink-2">
            <ClockCounterClockwiseIcon
              size={16}
              aria-hidden="true"
              className="mt-0.5 shrink-0 text-ink-3"
            />
            <span>
              {service.importing ? (
                text.importingStatus
              ) : last === null ? (
                text.never
              ) : (
                <>
                  {text.lastImport} <Ago iso={last.at} />
                  {text.colon}
                  {text.counts(last.played, last.resumed, last.unmapped)}
                </>
              )}
            </span>
          </p>
          <Button
            icon={DownloadSimpleIcon}
            loading={now.isPending || service.importing}
            disabled={toggle.isPending}
            onClick={() => now.mutate()}
            className="shrink-0"
          >
            {service.importing ? text.importing : text.importNow}
          </Button>
        </div>
      )}
      {service.importHistory && !service.importing && last?.problem && (
        <Notice tone="warn" className="mt-3">
          {text.problem[last.problem](name)}
        </Notice>
      )}
      {failed && (
        <Notice tone="danger" live className="mt-3">
          {errorMessage(t, failed)}
        </Notice>
      )}
    </div>
  )
}
