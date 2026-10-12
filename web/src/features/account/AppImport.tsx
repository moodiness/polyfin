import {
  ArrowSquareInIcon,
  ArrowSquareOutIcon,
  ClockCounterClockwiseIcon,
  DownloadSimpleIcon,
  KeyIcon,
  SignInIcon,
} from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useEffectEvent, useState, type FormEvent } from 'react'
import {
  ApiError,
  type AppAccount,
  type AppConnection,
  type AppConnectionActions,
  type AppImportChoice,
  type AppImportStatus,
  type AppKind,
  type AppReaders,
  type NuvioSignIn,
  type StremioLink,
} from '@/api'
import { dateTime, errorMessage, formatEpisode } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Checkbox,
  ConfirmDialog,
  ExternalButtonLink,
  Field,
  Modal,
  Notice,
  Panel,
  PanelFooter,
  RelativeTime,
  Segmented,
  Select,
  StatusPill,
  Table,
  TextInput,
  useToast,
  type StatusTone,
} from '@/ui'

const importTones: Record<AppImportStatus['state'], StatusTone> = {
  running: 'live',
  done: 'ok',
  failed: 'danger',
}

/** The apps imported from, in the order offered. */
const apps: readonly AppKind[] = ['nuvio', 'stremio']

/**
 * The import of an app account's watch data into a Polyfin account: the user's own, under My
 * account, or a user's, for an administrator. `current` is the import running into that account,
 * else the last one. The app chosen signs in with a form of its own, through `readers`, and the
 * account read then has its profile chosen with its addons: the server holds what it read a few
 * minutes only. `importEveryMinutes` is how often a connection kept is imported again.
 */
export default function AppImport({
  current,
  addonsAllowed,
  readers,
  start,
  onStarted,
  own,
  importEveryMinutes,
}: {
  current: AppImportStatus | null
  /** Whether the account's user may add their own addons: the app's are offered only then. */
  addonsAllowed: boolean
  readers: AppReaders
  start: (body: AppImportChoice) => Promise<AppImportStatus>
  onStarted: (started: AppImportStatus) => void
  /** The user's own account, under My account. */
  own: boolean
  importEveryMinutes: number
}) {
  const { t } = useI18n()
  const text = t.account.appImport
  const every = t.account.importEvery(importEveryMinutes)
  const toast = useToast()
  const [app, setApp] = useState<AppKind>('nuvio')
  const [stayConnected, setStayConnected] = useState(false)
  const [account, setAccount] = useState<AppAccount | null>(null)
  const [profile, setProfile] = useState('')
  const [addons, setAddons] = useState<number[]>([])
  const begin = useMutation({
    mutationFn: start,
    onSuccess: (started) => {
      setAccount(null)
      onStarted(started)
      toast(text.started)
    },
    onError: (error) => {
      // What the server held is gone: sign in again.
      if (error instanceof ApiError && error.code === 'app_import_expired') setAccount(null)
    },
  })
  const running = current?.state === 'running'
  const chosen = account?.profiles.find((p) => p.id === profile)
  const notice = begin.isError ? errorMessage(t, begin.error) : undefined

  function read(read: AppAccount) {
    begin.reset()
    setAccount(read)
    setProfile(read.profiles[0]?.id ?? '')
    setAddons([])
  }

  return (
    <div className="flex flex-col gap-6">
      {current !== null && <AppImportProgress current={current} own={own} />}
      {!running && account === null && (
        <Panel as="div" flush>
          <div className="flex max-w-lg flex-col gap-2 px-6 pt-6 max-sm:px-4">
            <span aria-hidden className="text-control font-medium text-ink">
              {text.app}
            </span>
            <Segmented
              label={text.app}
              value={app}
              options={apps.map((value) => ({ value, label: text.apps[value] }))}
              onChange={(value) => {
                begin.reset()
                setApp(value)
              }}
              className="self-start"
            />
            <Checkbox
              checked={stayConnected}
              onChange={setStayConnected}
              label={text.stayConnected}
              help={text.stayConnectedHelp[app](every)}
              className="mt-3"
            />
          </div>
          {app === 'nuvio' ? (
            <NuvioSignInForm
              read={readers.nuvio}
              onRead={read}
              notice={notice}
              stayConnected={stayConnected}
            />
          ) : (
            <StremioSignIn
              readers={readers.stremio}
              onRead={read}
              notice={notice}
              stayConnected={stayConnected}
            />
          )}
        </Panel>
      )}
      {!running && account !== null && (
        <Panel
          as="div"
          flush
          title={text.chooseTitle}
          titleAs="h3"
          description={text.chooseHelp[account.app]}
        >
          <div className="flex max-w-lg flex-col gap-5 px-6 py-6 max-sm:px-4">
            <Field
              label={text.profile}
              help={
                chosen === undefined
                  ? undefined
                  : text.profileCounts(chosen.played, chosen.resumes, chosen.library)
              }
            >
              <Select
                value={profile}
                options={account.profiles.map((p) => ({
                  value: p.id,
                  label:
                    p.name !== ''
                      ? p.name
                      : account.profiles.length === 1
                        ? text.wholeAccount
                        : text.profileNumber(p.id),
                }))}
                onValue={(value) => {
                  begin.reset()
                  setProfile(value)
                  setAddons([])
                }}
              />
            </Field>
            {chosen !== undefined && (
              <div className="flex flex-col gap-3">
                <span className="text-control font-medium text-ink">{text.addons}</span>
                {!addonsAllowed ? (
                  <p className="text-small text-ink-3">{text.addonsOff}</p>
                ) : chosen.addons.length === 0 ? (
                  <p className="text-small text-ink-3">{text.noAddons}</p>
                ) : (
                  <>
                    <p className="text-small text-ink-3">{text.addonsHelp}</p>
                    {chosen.addons.map((addon) => (
                      <Checkbox
                        key={addon.index}
                        checked={addons.includes(addon.index)}
                        onChange={(checked) =>
                          setAddons((current) =>
                            checked
                              ? [...current, addon.index]
                              : current.filter((index) => index !== addon.index),
                          )
                        }
                        label={addon.name}
                        help={<span className="font-mono">{addon.address}</span>}
                      />
                    ))}
                  </>
                )}
              </div>
            )}
            {account.stayConnected && <Notice tone="info">{text.staysConnected(every)}</Notice>}
            {begin.isError && (
              <Notice tone="danger" live>
                {errorMessage(t, begin.error)}
              </Notice>
            )}
          </div>
          <PanelFooter>
            <Button
              onClick={() => {
                begin.reset()
                setAccount(null)
              }}
            >
              {text.back}
            </Button>
            <Button
              variant="primary"
              icon={ArrowSquareInIcon}
              loading={begin.isPending}
              disabled={chosen === undefined}
              onClick={() => begin.mutate({ key: account.key, profile, addons })}
            >
              {begin.isPending ? text.starting : text.start}
            </Button>
          </PanelFooter>
        </Panel>
      )}
    </div>
  )
}

/** Sign-in codes about the backend's address, or the account, shown under that field. */
const backendCodes = ['invalid_nuvio_address', 'nuvio_unreachable', 'not_nuvio']
const accountCodes = ['nuvio_sign_in_refused']

/**
 * Signs in to a Nuvio account, on Nuvio's own backend or a self-hosted one, and hands the account
 * read to `onRead`. Each app signs in its own way, with a form of its own. The password only
 * lives in this form's state. `notice` is what went wrong with the last import's start;
 * `stayConnected` asks the server to keep the session.
 */
function NuvioSignInForm({
  read,
  onRead,
  notice,
  stayConnected,
}: {
  read: (body: NuvioSignIn) => Promise<AppAccount>
  onRead: (account: AppAccount) => void
  notice: string | undefined
  stayConnected: boolean
}) {
  const { t } = useI18n()
  const text = t.account.appImport
  const [backend, setBackend] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const signIn = useMutation({
    mutationFn: read,
    onSuccess: (account) => {
      setPassword('')
      onRead(account)
    },
  })

  const code = signIn.error instanceof ApiError ? signIn.error.code : null
  const message = signIn.isError ? errorMessage(t, signIn.error) : undefined
  const backendError = code !== null && backendCodes.includes(code) ? message : undefined
  const passwordError = code !== null && accountCodes.includes(code) ? message : undefined
  const otherError = signIn.isError
    ? (backendError ?? passwordError)
      ? undefined
      : message
    : notice

  function change(apply: () => void) {
    signIn.reset()
    apply()
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    signIn.mutate({ backend: backend.trim(), email: email.trim(), password, stayConnected })
  }

  return (
    <form onSubmit={submit} noValidate>
      <div className="flex max-w-lg flex-col gap-5 px-6 pt-5 pb-6 max-sm:px-4">
        <Field label={text.backend} help={text.backendHelp} error={backendError}>
          <TextInput
            type="url"
            inputMode="url"
            value={backend}
            onValue={(value) => change(() => setBackend(value))}
            placeholder="https://api.nuvio.tv"
            autoComplete="off"
            spellCheck={false}
            mono
          />
        </Field>
        <Field label={text.email}>
          <TextInput
            type="email"
            value={email}
            onValue={(value) => change(() => setEmail(value))}
            autoComplete="off"
            spellCheck={false}
            required
          />
        </Field>
        <Field label={text.password} help={text.passwordHelp} error={passwordError}>
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={(value) => change(() => setPassword(value))}
            autoComplete="off"
            required
          />
        </Field>
        {otherError && (
          <Notice tone="danger" live>
            {otherError}
          </Notice>
        )}
      </div>
      <PanelFooter>
        <Button
          type="submit"
          variant="primary"
          icon={SignInIcon}
          loading={signIn.isPending}
          disabled={email.trim() === '' || password === ''}
        >
          {signIn.isPending ? text.signingIn : text.signIn}
        </Button>
      </PanelFooter>
    </form>
  )
}

/** The ways to sign in to Stremio, in the order offered: the link code works for every account. */
type StremioWay = 'link' | 'password' | 'key'
const stremioWays: readonly StremioWay[] = ['link', 'password', 'key']

/** How often a Stremio link code is asked about while the user approves it. */
const linkPollMs = 3000

/**
 * Signs in to a Stremio account one of three ways, and hands the account read to `onRead`. The
 * password and the auth key only live in their form's state; the server never answers a key back,
 * and keeps it only when `stayConnected` asks for it.
 */
function StremioSignIn({
  readers,
  onRead,
  notice,
  stayConnected,
}: {
  readers: AppReaders['stremio']
  onRead: (account: AppAccount) => void
  notice: string | undefined
  stayConnected: boolean
}) {
  const { t } = useI18n()
  const text = t.account.appImport.stremio
  const [way, setWay] = useState<StremioWay>('link')

  return (
    <>
      <div className="flex max-w-lg flex-col gap-2 px-6 pt-5 max-sm:px-4">
        <span aria-hidden className="text-control font-medium text-ink">
          {text.way}
        </span>
        <Segmented
          label={text.way}
          value={way}
          options={stremioWays.map((value) => ({ value, label: text.ways[value] }))}
          onChange={setWay}
          className="self-start"
        />
      </div>
      {way === 'link' ? (
        <StremioLinkForm
          readers={readers}
          onRead={onRead}
          notice={notice}
          stayConnected={stayConnected}
        />
      ) : way === 'password' ? (
        <StremioPasswordForm
          read={readers.password}
          onRead={onRead}
          notice={notice}
          stayConnected={stayConnected}
        />
      ) : (
        <StremioKeyForm
          read={readers.key}
          onRead={onRead}
          notice={notice}
          stayConnected={stayConnected}
        />
      )}
    </>
  )
}

/**
 * Asks Stremio for a link code, shows it with the link to approve it at, and asks the server every
 * few seconds whether it is approved, until the account is read or the code expires.
 */
function StremioLinkForm({
  readers,
  onRead,
  notice,
  stayConnected,
}: {
  readers: AppReaders['stremio']
  onRead: (account: AppAccount) => void
  notice: string | undefined
  stayConnected: boolean
}) {
  const { t } = useI18n()
  const text = t.account.appImport.stremio
  const [link, setLink] = useState<StremioLink | null>(null)
  const [failure, setFailure] = useState<unknown>(null)
  const ask = useMutation({
    mutationFn: readers.startLink,
    onSuccess: (created) => {
      setFailure(null)
      setLink(created)
    },
  })
  const poll = useEffectEvent(() => readers.pollLink(stayConnected))
  const received = useEffectEvent((account: AppAccount) => onRead(account))

  useEffect(() => {
    if (link === null) return
    let stopped = false
    let timer = 0
    const next = () => {
      timer = window.setTimeout(() => void once(), linkPollMs)
    }
    const once = async () => {
      try {
        const polled = await poll()
        if (stopped) return
        if ('pending' in polled) next()
        else received(polled)
      } catch (error) {
        if (stopped) return
        setLink(null)
        setFailure(error)
      }
    }
    next()
    return () => {
      stopped = true
      window.clearTimeout(timer)
    }
  }, [link])

  const error = ask.isError
    ? errorMessage(t, ask.error)
    : failure !== null
      ? errorMessage(t, failure)
      : notice

  return (
    <>
      <div className="flex max-w-lg flex-col gap-4 px-6 pt-5 pb-6 max-sm:px-4">
        {link === null ? (
          <p className="text-small text-ink-3">{text.linkHelp}</p>
        ) : (
          <div className="flex flex-col gap-4 rounded-row border border-line bg-bg px-[18px] py-4 max-sm:p-3.5">
            <p className="text-control text-ink">{text.approve}</p>
            <div className="flex flex-wrap items-center gap-3">
              <p
                aria-label={text.codeLabel}
                className="rounded-field border border-accent/50 bg-s2 px-5 py-2 font-mono text-[26px] font-semibold tracking-[0.2em] text-ink select-all"
              >
                {link.code}
              </p>
              <ExternalButtonLink
                variant="primary"
                href={link.link}
                target="_blank"
                rel="noopener noreferrer"
                iconEnd={ArrowSquareOutIcon}
              >
                {text.openLink}
              </ExternalButtonLink>
            </div>
            <p role="status" className="text-small text-ink-3">
              {text.waiting}
            </p>
          </div>
        )}
        {link === null && error && (
          <Notice tone="danger" live>
            {error}
          </Notice>
        )}
      </div>
      <PanelFooter>
        {link === null ? (
          <Button
            variant="primary"
            icon={SignInIcon}
            loading={ask.isPending}
            onClick={() => {
              setFailure(null)
              ask.mutate()
            }}
          >
            {ask.isPending ? text.gettingCode : text.getCode}
          </Button>
        ) : (
          <Button onClick={() => setLink(null)}>{text.cancel}</Button>
        )}
      </PanelFooter>
    </>
  )
}

/**
 * Signs in to Stremio with the account's email and password, then reads it and signs out, unless
 * `stayConnected` asks the server to keep the session.
 */
function StremioPasswordForm({
  read,
  onRead,
  notice,
  stayConnected,
}: {
  read: AppReaders['stremio']['password']
  onRead: (account: AppAccount) => void
  notice: string | undefined
  stayConnected: boolean
}) {
  const { t } = useI18n()
  const text = t.account.appImport.stremio
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const signIn = useMutation({
    mutationFn: read,
    onSuccess: (account) => {
      setPassword('')
      onRead(account)
    },
  })

  const refused =
    signIn.error instanceof ApiError && signIn.error.code === 'stremio_sign_in_refused'
  const message = signIn.isError ? errorMessage(t, signIn.error) : undefined

  function change(apply: () => void) {
    signIn.reset()
    apply()
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    signIn.mutate({ email: email.trim(), password, stayConnected })
  }

  return (
    <form onSubmit={submit} noValidate>
      <div className="flex max-w-lg flex-col gap-5 px-6 pt-5 pb-6 max-sm:px-4">
        <Field label={text.email}>
          <TextInput
            type="email"
            value={email}
            onValue={(value) => change(() => setEmail(value))}
            autoComplete="off"
            spellCheck={false}
            required
          />
        </Field>
        <Field label={text.password} help={text.passwordHelp} error={refused ? message : undefined}>
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={(value) => change(() => setPassword(value))}
            autoComplete="off"
            required
          />
        </Field>
        {(refused ? undefined : (message ?? notice)) && (
          <Notice tone="danger" live>
            {message ?? notice}
          </Notice>
        )}
      </div>
      <PanelFooter>
        <Button
          type="submit"
          variant="primary"
          icon={SignInIcon}
          loading={signIn.isPending}
          disabled={email.trim() === '' || password === ''}
        >
          {signIn.isPending ? text.signingIn : text.signIn}
        </Button>
      </PanelFooter>
    </form>
  )
}

/**
 * Reads the Stremio account with the auth key the user pasted, which is never signed out, and kept
 * only when `stayConnected` asks for it.
 */
function StremioKeyForm({
  read,
  onRead,
  notice,
  stayConnected,
}: {
  read: AppReaders['stremio']['key']
  onRead: (account: AppAccount) => void
  notice: string | undefined
  stayConnected: boolean
}) {
  const { t } = useI18n()
  const text = t.account.appImport.stremio
  const [key, setKey] = useState('')
  const reading = useMutation({
    mutationFn: (pasted: string) => read(pasted, stayConnected),
    onSuccess: (account) => {
      setKey('')
      onRead(account)
    },
  })

  const refused = reading.error instanceof ApiError && reading.error.code === 'stremio_key_refused'
  const message = reading.isError ? errorMessage(t, reading.error) : undefined

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    reading.mutate(key.trim())
  }

  return (
    <form onSubmit={submit} noValidate>
      <div className="flex max-w-lg flex-col gap-5 px-6 pt-5 pb-6 max-sm:px-4">
        <Field label={text.key} help={text.keyHelp} error={refused ? message : undefined}>
          <TextInput
            type="password"
            revealable
            value={key}
            onValue={(value) => {
              reading.reset()
              setKey(value)
            }}
            autoComplete="off"
            spellCheck={false}
            mono
            required
          />
        </Field>
        {(refused ? undefined : (message ?? notice)) && (
          <Notice tone="danger" live>
            {message ?? notice}
          </Notice>
        )}
      </div>
      <PanelFooter>
        <Button
          type="submit"
          variant="primary"
          icon={KeyIcon}
          loading={reading.isPending}
          disabled={key.trim() === ''}
        >
          {reading.isPending ? text.reading : text.readKey}
        </Button>
      </PanelFooter>
    </form>
  )
}

/**
 * An import from an app, running or the last one: its state, then what it added, the titles it did
 * not find, and the addons it installed. `own` is the user's own account, under My account.
 */
function AppImportProgress({ current, own }: { current: AppImportStatus; own: boolean }) {
  const { t } = useI18n()
  const text = t.account.appImport
  const [showUnmatched, setShowUnmatched] = useState(false)
  const running = current.state === 'running'
  const title = running ? text.currentTitle : own ? text.ownLastTitle : text.lastTitle
  const app = text.apps[current.app]

  return (
    <Panel
      title={title}
      titleAs="h3"
      titleAside={
        <StatusPill tone={importTones[current.state]}>{text.states[current.state]}</StatusPill>
      }
      description={
        <>
          {text.from(app, current.profile)} · {text.startedLabel}{' '}
          <RelativeTime iso={current.startedAt} />
          {current.endedAt !== null && (
            <>
              {' '}
              · {text.endedLabel} <RelativeTime iso={current.endedAt} />
            </>
          )}
          {current.startedBy.id !== current.user.id && (
            <> · {text.startedBy(current.startedBy.name)}</>
          )}
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {running && <p className="text-small text-ink-3">{text.runningHelp}</p>}
        {current.problem !== null && <Notice tone="danger">{text.problem}</Notice>}
        <p className="text-body text-ink">
          {running
            ? text.read(current.read)
            : text.counts(current.read, current.played, current.resumed, current.favorites)}
        </p>
        {current.unmatchedCount > 0 && (
          <Button
            variant="ghost"
            size="sm"
            className="self-start"
            onClick={() => setShowUnmatched(true)}
          >
            {text.unmatched(current.unmatchedCount)}
          </Button>
        )}
        {current.addons.length > 0 && (
          <ul className="flex flex-col gap-1 text-small">
            {current.addons.map((addon, index) => (
              <li key={index} className="text-ink-2">
                <span className="text-ink">{addon.name}</span>{' '}
                <span className="font-mono text-ink-3">{addon.address}</span> ·{' '}
                {addon.installed
                  ? text.addonInstalled
                  : addon.problem === null
                    ? text.addonWaiting
                    : text.addonProblems[addon.problem]}
              </li>
            ))}
          </ul>
        )}
        <Modal
          open={showUnmatched}
          onClose={() => setShowUnmatched(false)}
          title={text.unmatchedTitle}
          width={640}
          footer={<Button onClick={() => setShowUnmatched(false)}>{t.users.close}</Button>}
        >
          <div className="space-y-4 p-5">
            <p className="text-small text-ink-3">{text.unmatchedHelp}</p>
            <Table label={text.unmatchedTitle} minWidth={520}>
              <thead>
                <tr>
                  <th scope="col">{text.titleColumn}</th>
                  <th scope="col">{text.kindColumn}</th>
                  <th scope="col">{text.reasonColumn}</th>
                </tr>
              </thead>
              <tbody>
                {current.unmatched.map((entry, index) => (
                  <tr key={index}>
                    <td>
                      <span className="block text-ink">
                        {entry.name === '' ? entry.id : entry.name}
                      </span>
                      <span className="block font-mono text-small text-ink-3">
                        {entry.id}
                        {entry.season !== null && entry.episode !== null
                          ? ` · ${formatEpisode(entry.season, entry.episode)}`
                          : ''}
                      </span>
                    </td>
                    <td>{t.users.jellyfinImport.kinds[entry.type]}</td>
                    <td>{text.reasons[entry.reason]}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
            {current.unmatchedCount > current.unmatched.length && (
              <p className="text-small text-ink-3">
                {t.users.jellyfinImport.more(current.unmatchedCount - current.unmatched.length)}
              </p>
            )}
          </div>
        </Modal>
      </div>
    </Panel>
  )
}

/**
 * A user's connections to apps, imported every `importEveryMinutes`: under My account, the user's
 * own, or a user's, for an administrator. Each shows its account, its last and next import, and why
 * it stopped or is paused, with Import now and Disconnect. `own` is the user's own account, where a
 * paused connection cannot be imported. `onChanged` tells that the connections changed.
 */
export function AppConnections({
  connections,
  actions,
  onChanged,
  own,
  importEveryMinutes,
}: {
  connections: AppConnection[]
  actions: AppConnectionActions
  onChanged: () => void
  own: boolean
  importEveryMinutes: number
}) {
  const { t } = useI18n()
  const text = t.account.appImport.connections
  if (connections.length === 0) return null
  return (
    <Panel
      title={text.title}
      titleAs="h3"
      description={text.help(t.account.importEvery(importEveryMinutes))}
      flush
    >
      <ul>
        {connections.map((connection) => (
          <AppConnectionRow
            key={connection.app}
            connection={connection}
            actions={actions}
            onChanged={onChanged}
            own={own}
          />
        ))}
      </ul>
    </Panel>
  )
}

function AppConnectionRow({
  connection,
  actions,
  onChanged,
  own,
}: {
  connection: AppConnection
  actions: AppConnectionActions
  onChanged: () => void
  own: boolean
}) {
  const { language, t } = useI18n()
  const text = t.account.appImport.connections
  const toast = useToast()
  const name = t.account.appImport.apps[connection.app]
  const [confirming, setConfirming] = useState(false)
  const importNow = useMutation({
    mutationFn: () => actions.importNow(connection.app),
    onSettled: onChanged,
  })
  const disconnect = useMutation({
    mutationFn: () => actions.disconnect(connection.app),
    onSuccess: () => {
      setConfirming(false)
      toast(text.disconnected(name))
      onChanged()
    },
  })
  const stopped = connection.problem !== null
  const last = connection.lastImport
  const status: { tone: StatusTone; label: string } = stopped
    ? { tone: 'danger', label: text.status.stopped }
    : connection.importing
      ? { tone: 'live', label: text.status.importing }
      : connection.paused
        ? { tone: 'muted', label: text.status.paused }
        : { tone: 'ok', label: text.status.connected }

  return (
    <li className="flex flex-col gap-3 px-6 py-5 max-sm:px-4 [&+&]:border-t [&+&]:border-line">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h4 className="text-[15px] font-semibold tracking-[-0.01em] text-ink">{name}</h4>
          <p className="text-control text-ink-2">
            {connection.account !== null ? (
              <>
                {text.connectedAs}
                <span className="font-mono text-ink">{connection.account}</span>
              </>
            ) : (
              text.connected
            )}
            {text.connectedOn(dateTime(connection.connectedAt, language))}
            {connection.connectedBy !== null && text.connectedBy(connection.connectedBy.name)}
          </p>
        </div>
        <div className="flex items-center gap-1.5">
          <StatusPill tone={status.tone}>{status.label}</StatusPill>
          <Button variant="ghost" onClick={() => setConfirming(true)}>
            {text.disconnect}
          </Button>
        </div>
      </div>
      <div className="flex items-center justify-between gap-4 rounded-row border border-line bg-bg px-[18px] py-3 max-sm:flex-col max-sm:items-start max-sm:p-3.5">
        <p role="status" className="flex min-w-0 items-start gap-2 text-control text-ink-2">
          <ClockCounterClockwiseIcon
            size={16}
            aria-hidden="true"
            className="mt-0.5 shrink-0 text-ink-3"
          />
          <span>
            {last === null ? (
              text.never
            ) : (
              <>
                {text.lastImport}
                <RelativeTime iso={last.at} />
                {text.colon}
                {t.account.appImport.counts(last.read, last.played, last.resumed, last.favorites)}
              </>
            )}
            {connection.nextImportAt !== null && !connection.importing && (
              <>
                {' '}
                {text.nextImport}
                <RelativeTime iso={connection.nextImportAt} />.
              </>
            )}
          </span>
        </p>
        <Button
          icon={DownloadSimpleIcon}
          loading={importNow.isPending || connection.importing}
          disabled={stopped || (own && connection.paused)}
          onClick={() => importNow.mutate()}
          className="shrink-0"
        >
          {connection.importing ? text.importing : text.importNow}
        </Button>
      </div>
      {stopped ? (
        <Notice tone="danger">{text.stopped(name)}</Notice>
      ) : (
        connection.paused && <Notice tone="warn">{text.paused}</Notice>
      )}
      {!stopped && !connection.importing && last?.problem && (
        <Notice tone="warn">{text.problems[last.problem](name)}</Notice>
      )}
      {importNow.isError && (
        <Notice tone="danger" live>
          {errorMessage(t, importNow.error)}
        </Notice>
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
