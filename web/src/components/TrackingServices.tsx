import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { Link } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ApiError,
  connectTracking,
  disconnectTracking,
  fetchTracking,
  importTracking,
  revealTrackingKey,
  setTrackingImport,
  queryClient,
  queryKeys,
  type TrackingService,
  type TrackingServiceName,
} from '@/api'
import { SecretField } from '@/components/settings'
import { useSessionUser } from '@/app/session'
import {
  Badge,
  buttonPrimary,
  buttonSecondary,
  Checkbox,
  ConfirmButton,
  Notice,
  RelativeTime,
} from '@/components/ui'
import { dateTime, errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'

const serviceNames: Record<TrackingServiceName, string> = {
  trakt: 'Trakt',
  simkl: 'Simkl',
  mdblist: 'MDBList',
  publicmetadb: 'PublicMetaDB',
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

  if (tracking.isPending) return <p className="text-muted">{t.common.loading}</p>
  if (tracking.isError) return <Notice kind="error">{errorMessage(t, tracking.error)}</Notice>
  return (
    <ul className="divide-y divide-line">
      {tracking.data.map((service) => (
        <li key={service.service} className="py-5 first:pt-0 last:pb-0">
          <ServiceRow service={service} />
        </li>
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
function connectError(t: Messages, name: string, error: unknown): string {
  const text = t.account.tracking
  if (error instanceof ApiError) {
    if (error.code === 'invalid_key') return text.invalidKey(name)
    if (error.code === 'service_unreachable') return text.serviceUnreachable(name)
    if (error.code === 'not_available') return text.notAvailable(name)
    if (error.code === 'app_refused') return text.appRefused(name)
  }
  return errorMessage(t, error)
}

function ServiceRow({ service }: { service: TrackingService }) {
  const { language, t } = useI18n()
  const text = t.account.tracking
  const user = useSessionUser()
  const name = serviceNames[service.service]
  const code = service.code
  const now = useNowUntil(code === null ? null : Date.parse(code.expiresAt))
  const expired = codeExpired(code, now)

  // A code the server dropped without connecting the service expired or was refused on its site.
  const codeKey = code?.userCode ?? null
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
    },
  })
  const disconnect = useMutation({
    mutationFn: () => disconnectTracking(service.service),
    onSuccess: () => {
      connect.reset()
      // Cancelling a code is not its expiry: nothing to tell.
      setLastCodeKey(null)
      setCodeEnded(false)
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

  const status: { tone: 'ok' | 'muted' | 'danger' | 'warning' | 'fin'; label: string } =
    !service.available
      ? { tone: 'muted', label: text.status.unavailable }
      : code !== null && !expired
        ? { tone: 'fin', label: text.status.waiting }
        : service.problem === 'app_refused'
          ? { tone: 'danger', label: text.status.appRefused }
          : service.problem === 'reconnect'
            ? { tone: 'danger', label: text.status.reconnect }
            : service.problem === 'unreachable'
              ? { tone: 'warning', label: text.status.unreachable }
              : service.connected
                ? { tone: 'ok', label: text.status.connected }
                : { tone: 'muted', label: text.status.notConnected }

  // The administrator's page where the app is set up, for administrators.
  const setUpLink = user.isAdministrator && (
    <Link
      to="/settings/tracking"
      className="inline-flex text-sm font-medium text-fin-5 underline-offset-4 hover:underline"
    >
      {text.setUpApp(name)}
    </Link>
  )

  let body: ReactNode
  if (!service.available) {
    body = (
      <div className="space-y-2 text-sm">
        <p className="text-muted">{text.unavailable(name)}</p>
        {setUpLink}
      </div>
    )
  } else if (code !== null && !expired) {
    const host = new URL(code.verificationUrl).host
    body = (
      <div className="space-y-4">
        <p className="text-sm text-zinc-200">{text.enterCode(host)}</p>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
          <p
            aria-label={text.codeLabel}
            className="w-fit rounded-xl border border-fin-4/50 bg-bg px-5 py-3 font-mono text-3xl font-semibold tracking-[0.2em] text-white select-all"
          >
            {code.userCode}
          </p>
          <a
            href={code.verificationUrl}
            target="_blank"
            rel="noopener noreferrer"
            className={`${buttonPrimary} w-fit`}
          >
            {text.openSite(host)}
            <span aria-hidden="true">↗</span>
          </a>
        </div>
        <p role="status" className="text-sm text-muted">
          {text.waiting} {text.expires}
          <RelativeTime iso={code.expiresAt} />.
        </p>
        <button
          type="button"
          className={buttonSecondary}
          disabled={disconnect.isPending}
          onClick={() => disconnect.mutate()}
        >
          {t.common.cancel}
        </button>
      </div>
    )
  } else {
    const codeConnect = (label: string) => (
      <button
        type="button"
        className={buttonPrimary}
        disabled={connect.isPending}
        onClick={() => connect.mutate(undefined)}
      >
        {connect.isPending ? text.connecting : label}
      </button>
    )
    const keyForm = (label: string) => (
      <KeyForm
        name={name}
        hint={text.keyHelp[service.service as 'mdblist' | 'publicmetadb']}
        label={label}
        pending={connect.isPending}
        error={connect.isError ? connectError(t, name, connect.error) : undefined}
        onChange={() => connect.reset()}
        onConnect={(key) => connect.mutate(key)}
      />
    )
    const appRefused =
      connect.isError && connect.error instanceof ApiError && connect.error.code === 'app_refused'
    const codeError =
      service.connection === 'code' && connect.isError ? (
        <div className="space-y-2">
          <Notice kind="error">{connectError(t, name, connect.error)}</Notice>
          {appRefused && setUpLink}
        </div>
      ) : null
    const ended = (expired || codeEnded) && <Callout tone="warning">{text.codeEnded}</Callout>
    // The server's app was refused while the code waited; another attempt clears it.
    const refusedApp = service.problem === 'app_refused' && !connect.isError && (
      <div className="space-y-2">
        <Callout tone="danger">{text.appRefused(name)}</Callout>
        {setUpLink}
      </div>
    )

    body = service.connected ? (
      <div className="space-y-4">
        {service.connection === 'key' && (
          <div className="max-w-md">
            <SecretField
              label={text.keyLabel(name)}
              hint={text.savedKeyHelp}
              saved
              value={undefined}
              reveal={() => revealTrackingKey(service.service as 'mdblist' | 'publicmetadb')}
              replaceable={false}
              removable={false}
            />
          </div>
        )}
        <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
          {service.account !== null && (
            <>
              <dt className="text-muted">{text.account}</dt>
              <dd className="text-zinc-100">{service.account}</dd>
            </>
          )}
          {service.connectedAt !== null && (
            <>
              <dt className="text-muted">{text.connectedAt}</dt>
              <dd className="text-zinc-100">{dateTime(service.connectedAt, language)}</dd>
            </>
          )}
          <dt className="text-muted">{text.lastSent}</dt>
          <dd className="text-zinc-100">
            {service.lastSentAt === null ? (
              text.nothingSent
            ) : (
              <RelativeTime iso={service.lastSentAt} />
            )}
          </dd>
        </dl>
        {service.problem === 'reconnect' && (
          <Callout tone="danger">{text.problemReconnect(name)}</Callout>
        )}
        {service.problem === 'unreachable' && (
          <Callout tone="warning">{text.problemUnreachable(name)}</Callout>
        )}
        {refusedApp}
        {ended}
        {codeError}
        {service.problem === 'reconnect' && service.connection === 'key' && keyForm(text.reconnect)}
        {disconnect.isError && <Notice kind="error">{errorMessage(t, disconnect.error)}</Notice>}
        <div className="flex flex-wrap gap-2">
          {(service.problem === 'reconnect' || service.problem === 'app_refused') &&
            service.connection === 'code' &&
            codeConnect(ended || service.problem === 'app_refused' ? text.newCode : text.reconnect)}
          <ConfirmButton
            label={text.disconnect}
            busyLabel={text.disconnecting}
            message={text.disconnectConfirm(name)}
            busy={disconnect.isPending}
            onConfirm={() => disconnect.mutate()}
          />
        </div>
        <HistoryImport service={service} name={name} />
      </div>
    ) : (
      <div className="space-y-4">
        <p className="text-sm text-muted">
          {service.connection === 'code' ? text.codeIntro(name) : text.keyIntro(name)}
        </p>
        {refusedApp}
        {ended}
        {codeError}
        {service.connection === 'code'
          ? codeConnect(ended ? text.newCode : text.connect)
          : keyForm(text.connect)}
      </div>
    )
  }

  return (
    <section aria-label={name} className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-base font-semibold text-white">{name}</h3>
        <Badge tone={status.tone}>{status.label}</Badge>
      </div>
      {body}
    </section>
  )
}

function Callout({ tone, children }: { tone: 'danger' | 'warning'; children: ReactNode }) {
  const tones = {
    danger: 'border-rose-400/40 bg-rose-400/5 text-rose-200',
    warning: 'border-amber-400/40 bg-amber-400/5 text-amber-100',
  }
  return (
    <div className={`flex gap-2 rounded-lg border p-3 text-sm ${tones[tone]}`}>
      <span aria-hidden="true">⚠</span>
      <p>{children}</p>
    </div>
  )
}

function KeyForm({
  name,
  hint,
  label,
  pending,
  error,
  onChange,
  onConnect,
}: {
  name: string
  hint: string
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
    <form onSubmit={submit} noValidate className="max-w-md space-y-3">
      <SecretField
        label={text.keyLabel(name)}
        hint={hint}
        saved={false}
        showStatus={false}
        value={key === '' ? undefined : key}
        onValue={(value) => {
          setKey(value ?? '')
          onChange()
        }}
        error={error}
      />
      <button type="submit" className={buttonPrimary} disabled={pending || key.trim() === ''}>
        {pending ? text.checking : label}
      </button>
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
    <div className="space-y-3 rounded-lg border border-line p-4">
      <Checkbox
        label={text.toggle(name)}
        help={text.toggleHelp(name)}
        checked={service.importHistory}
        disabled={busy}
        onChange={(on) => {
          now.reset()
          toggle.mutate(on)
        }}
      />
      {service.importHistory && (
        <div className="flex flex-wrap items-center gap-3">
          <button
            type="button"
            className={buttonSecondary}
            disabled={busy || service.importing}
            onClick={() => now.mutate()}
          >
            {service.importing ? text.importing : text.importNow}
          </button>
          <p role="status" className="text-sm text-muted">
            {service.importing ? (
              text.importingStatus
            ) : last === null ? (
              text.never
            ) : (
              <>
                {text.lastImport} <RelativeTime iso={last.at} />
                {text.colon}
                {text.counts(last.played, last.resumed, last.unmapped)}
              </>
            )}
          </p>
        </div>
      )}
      {service.importHistory && !service.importing && last?.problem && (
        <Callout tone="warning">{text.problem[last.problem](name)}</Callout>
      )}
      {failed && <Notice kind="error">{errorMessage(t, failed)}</Notice>}
    </div>
  )
}
