import { ArrowSquareInIcon, PlugsConnectedIcon, StopCircleIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import {
  ApiError,
  connectJellyfin,
  fetchJellyfinImport,
  fetchUsers,
  queryClient,
  queryKeys,
  startJellyfinImport,
  stopJellyfinImport,
  type JellyfinConnection,
  type JellyfinCredentials,
  type JellyfinImportEntry,
  type JellyfinImportStatus,
  type JellyfinUser,
  type JellyfinUserImport,
  type ServerKind,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage, formatEpisode } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Avatar,
  Badge,
  Button,
  Checkbox,
  Field,
  InlineError,
  Modal,
  Notice,
  Panel,
  PanelFooter,
  RelativeTime,
  Row,
  RowList,
  Segmented,
  Select,
  SkeletonRows,
  StatusPill,
  Table,
  TextInput,
  useToast,
  type SelectOption,
  type StatusTone,
} from '@/ui'

/** How often the import is read while it runs. */
const importPollMs = 2000

/** Connection codes about the address, the key or the account, shown under that field. */
export const addressCodes = ['invalid_jellyfin_address', 'jellyfin_unreachable', 'not_jellyfin']
const keyCodes = ['jellyfin_key_refused', 'jellyfin_key_limited', 'emby_user_key']
export const accountCodes = [
  'jellyfin_sign_in_refused',
  'jellyfin_sign_in_forbidden',
  'jellyfin_key_limited',
]
/** Codes about a row's name, password, or password on the server, shown under that field. */
const nameCodes = ['invalid_name', 'name_taken']
const passwordCodes = ['invalid_password']
const signInCodes = [
  'jellyfin_password_refused',
  'jellyfin_sign_in_forbidden',
  'jellyfin_other_user',
  'jellyfin_key_owner_only',
]

const importTones: Record<JellyfinImportStatus['state'], StatusTone> = {
  running: 'live',
  done: 'ok',
  stopped: 'muted',
  failed: 'danger',
}

const userTones: Record<JellyfinUserImport['state'], StatusTone> = {
  waiting: 'muted',
  reading: 'live',
  saving: 'live',
  done: 'ok',
  failed: 'danger',
}

/** Where each kind of server usually answers, as the address field suggests. */
const addressPlaceholders: Record<ServerKind, string> = {
  jellyfin: 'http://192.168.1.10:8096',
  emby: 'http://192.168.1.10:8096',
  plex: 'http://192.168.1.10:32400',
}

/** The text of a failed request about a server of `kind`, in the words of that server. */
export function serverError(t: Messages, kind: ServerKind, error: unknown): string {
  if (kind !== 'jellyfin' && error instanceof ApiError) {
    const own = t.users.jellyfinImport.errors[kind][error.code]
    if (own !== undefined) return own
  }
  return errorMessage(t, error)
}

/** What a Jellyfin user becomes: nothing, a new Polyfin user, or an existing one (`user:<id>`). */
type Target = 'skip' | 'new' | `user:${string}`

/** The choices of one Jellyfin user's row. */
type Choice = {
  target: Target
  name: string
  password: string
  isAdministrator: boolean
  watchData: boolean
  /** The user's password on the server, to read their watch data signed in as them. */
  jellyfinPassword: string
  /** Whether a new user keeps that password in Polyfin. */
  keepPassword: boolean
}

/**
 * How the page connects to the server: its kind, then an API key, or a user's name and password.
 * Plex takes its owner's token, sent as the key.
 */
type Login = {
  kind: ServerKind
  method: 'key' | 'account'
  apiKey: string
  name: string
  password: string
}

/** What a login sends: the key, or the account. */
function credentialsOf(login: Login): JellyfinCredentials {
  return login.method === 'key' || login.kind === 'plex'
    ? { apiKey: login.apiKey.trim() }
    : { account: { name: login.name.trim(), password: login.password } }
}

/**
 * `/users/jellyfin-import`: imports accounts and their watch data from a Jellyfin, Emby or Plex
 * server. The running or last import shows first, whoever started it; under it, connecting to a
 * server of the kind chosen, then choosing what each of its users becomes. The key and passwords
 * only live in this page's state: the server never keeps them.
 */
export default function JellyfinImportRoute() {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const [address, setAddress] = useState('')
  const [login, setLogin] = useState<Login>({
    kind: 'jellyfin',
    method: 'key',
    apiKey: '',
    name: '',
    password: '',
  })
  const [connection, setConnection] = useState<JellyfinConnection | null>(null)
  const status = useQuery({
    queryKey: queryKeys.jellyfinImport,
    queryFn: ({ signal }) => fetchJellyfinImport(signal),
    // React Query pauses this while the page is hidden, and stops it when the page is left.
    refetchInterval: (query) => (query.state.data?.state === 'running' ? importPollMs : false),
  })

  return (
    <PageLayout
      title={text.title}
      lede={text.description}
      back={{ to: '/users', label: t.users.title }}
    >
      {status.isPending ? (
        <SkeletonRows rows={2} boxed label={t.common.loading} />
      ) : status.isError ? (
        <InlineError onRetry={() => void status.refetch()} retrying={status.isFetching}>
          {errorMessage(t, status.error)}
        </InlineError>
      ) : (
        status.data !== null && <ImportProgress current={status.data} />
      )}
      {/* One import at a time: the server refuses another while one runs. */}
      {status.data?.state === 'running' ? null : connection === null ? (
        <ConnectForm
          address={address}
          login={login}
          onAddress={setAddress}
          onLogin={(patch) => setLogin((current) => ({ ...current, ...patch }))}
          onConnected={setConnection}
        />
      ) : (
        <ChooseUsers
          connection={connection}
          credentials={credentialsOf(login)}
          onBack={() => setConnection(null)}
          onStarted={() => {
            setConnection(null)
            setLogin((current) => ({ ...current, apiKey: '', password: '' }))
          }}
        />
      )}
    </PageLayout>
  )
}

/**
 * An import running, or the last one: its state, then each user's. The administrator's page names
 * who started it, and can stop it; `own` is a user's own import, under My account.
 */
export function ImportProgress({
  current,
  own = false,
}: {
  current: JellyfinImportStatus
  own?: boolean
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const [shownId, setShownId] = useState<string | null>(null)
  const running = current.state === 'running'
  const product = text.servers[current.server.kind]
  const title = running
    ? text.currentTitle
    : own
      ? t.account.serverImport.lastTitle
      : text.lastTitle
  const stop = useMutation({
    mutationFn: stopJellyfinImport,
    onSuccess: (stopped) => {
      queryClient.setQueryData(queryKeys.jellyfinImport, stopped)
    },
  })
  // Looked up in each poll's answer, so an open list follows the import.
  const shown = current.users.find((user) => user.jellyfinId === shownId)

  return (
    <Panel
      title={title}
      titleAs={own ? 'h3' : 'h2'}
      titleAside={
        <StatusPill tone={importTones[current.state]}>{text.states[current.state]}</StatusPill>
      }
      description={
        <>
          {current.server.name} ({product}) · {text.startedLabel}{' '}
          <RelativeTime iso={current.startedAt} />
          {current.endedAt !== null && (
            <>
              {' '}
              · {text.endedLabel} <RelativeTime iso={current.endedAt} />
            </>
          )}
          {!own && <> · {text.startedBy(current.startedBy.name, current.own)}</>}
        </>
      }
      actions={
        running && !own ? (
          <Button icon={StopCircleIcon} loading={stop.isPending} onClick={() => stop.mutate()}>
            {stop.isPending ? text.stopping : text.stop}
          </Button>
        ) : undefined
      }
    >
      <div className="flex flex-col gap-4">
        {running && <p className="text-small text-ink-3">{text.runningHelp}</p>}
        {current.problem !== null && (
          <Notice tone="danger">{text.problems[current.problem](product)}</Notice>
        )}
        {stop.isError && (
          <Notice tone="danger" live>
            {errorMessage(t, stop.error)}
          </Notice>
        )}
        <RowList variant="plain" aria-label={title}>
          {current.users.map((user) => (
            <ImportedUserRow
              key={user.jellyfinId}
              user={user}
              product={product}
              finished={!running}
              onShowUnmatched={() => setShownId(user.jellyfinId)}
            />
          ))}
        </RowList>
        <Modal
          open={shown !== undefined}
          onClose={() => setShownId(null)}
          title={shown === undefined ? '' : text.unmatchedTitle(shown.jellyfinName)}
          width={640}
          footer={<Button onClick={() => setShownId(null)}>{t.users.close}</Button>}
        >
          {shown !== undefined && <UnmatchedList user={shown} />}
        </Modal>
      </div>
    </Panel>
  )
}

/**
 * One user of an import: the Polyfin user it went into, its state, what it read while it reads,
 * then what it added. A finished import's users still waiting were never reached.
 */
function ImportedUserRow({
  user,
  product,
  finished,
  onShowUnmatched,
}: {
  user: JellyfinUserImport
  /** The kind of server's name, such as Emby. */
  product: string
  finished: boolean
  onShowUnmatched: () => void
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const notReached = finished && user.state === 'waiting'
  return (
    <Row
      leading={<Avatar name={user.userName} size="lg" />}
      title={text.userMapping(user.jellyfinName, user.userName)}
      meta={
        user.state === 'reading'
          ? text.read(user.read)
          : user.state === 'waiting'
            ? undefined
            : text.counts(user.played, user.resumed, user.favorites)
      }
      trailing={
        <>
          <StatusPill tone={notReached ? 'muted' : userTones[user.state]}>
            {notReached ? text.notImported : text.userStates[user.state]}
          </StatusPill>
          {user.unmatchedCount > 0 && (
            <Button variant="ghost" size="sm" onClick={onShowUnmatched}>
              {text.unmatched(user.unmatchedCount)}
            </Button>
          )}
        </>
      }
    >
      {user.problem !== null ? (
        <Notice tone="danger">{text.problems[user.problem](product)}</Notice>
      ) : undefined}
    </Row>
  )
}

/** The titles of a user's watch data that no Polyfin title matched, and why. */
function UnmatchedList({ user }: { user: JellyfinUserImport }) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  // The server lists the first 500 only.
  const more = user.unmatchedCount - user.unmatched.length
  return (
    <div className="space-y-4 p-5">
      <p className="text-small text-ink-3">{text.unmatchedHelp}</p>
      <Table label={text.unmatchedTitle(user.jellyfinName)} minWidth={520}>
        <thead>
          <tr>
            <th scope="col">{text.titleColumn}</th>
            <th scope="col">{text.kindColumn}</th>
            <th scope="col" className="figures text-right">
              {text.yearColumn}
            </th>
            <th scope="col">{text.reasonColumn}</th>
          </tr>
        </thead>
        <tbody>
          {user.unmatched.map((entry, index) => {
            // "Series · S01 · E03", with what Jellyfin knows of the episode's place.
            const place =
              entry.type !== 'episode' || entry.series === null
                ? ''
                : entry.season === null || entry.episode === null
                  ? entry.series
                  : `${entry.series} · ${formatEpisode(entry.season, entry.episode)}`
            return (
              <tr key={index}>
                <td>
                  <span className="block text-ink">{entry.name}</span>
                  {place !== '' && <span className="block text-small text-ink-3">{place}</span>}
                </td>
                <td>{text.kinds[entry.type]}</td>
                <td className="figures text-right">{entry.year}</td>
                <td>{text.reasons[entry.reason]}</td>
              </tr>
            )
          })}
        </tbody>
      </Table>
      {more > 0 && <p className="text-small text-ink-3">{text.more(more)}</p>}
    </div>
  )
}

/**
 * Step 1: the kind of server, its address, and an API key or a user's account, which read its
 * users; Plex takes its owner's token.
 */
function ConnectForm({
  address,
  login,
  onAddress,
  onLogin,
  onConnected,
}: {
  address: string
  login: Login
  onAddress: (address: string) => void
  onLogin: (patch: Partial<Login>) => void
  onConnected: (connection: JellyfinConnection) => void
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const mutation = useMutation({ mutationFn: connectJellyfin, onSuccess: onConnected })
  const plex = login.kind === 'plex'
  const byKey = login.method === 'key' || plex
  const product = text.servers[login.kind]

  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  const message = mutation.isError ? serverError(t, login.kind, mutation.error) : undefined
  const addressError = code !== null && addressCodes.includes(code) ? message : undefined
  const loginError =
    code !== null && (byKey ? keyCodes : accountCodes).includes(code) ? message : undefined
  const otherError = mutation.isError && !addressError && !loginError ? message : undefined

  function change(patch: Partial<Login>) {
    mutation.reset()
    onLogin(patch)
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({ kind: login.kind, address: address.trim(), ...credentialsOf(login) })
  }

  return (
    <Panel title={text.connectTitle(product)} description={text.connectHelp} flush>
      <form onSubmit={submit} noValidate>
        <div className="flex max-w-lg flex-col gap-5 px-6 pb-6 max-sm:px-4">
          <div className="flex flex-col gap-2">
            <span aria-hidden className="text-control font-medium text-ink">
              {text.serverKind}
            </span>
            <Segmented
              label={text.serverKind}
              value={login.kind}
              options={(['jellyfin', 'emby', 'plex'] as const).map((kind) => ({
                value: kind,
                label: text.servers[kind],
              }))}
              onChange={(kind) => change({ kind })}
              className="self-start"
            />
            <span className="text-small text-ink-3">{text.serverKindHelp[login.kind]}</span>
          </div>
          <Field
            label={text.address}
            help={plex ? text.plexAddressHelp : text.addressHelp(product)}
            error={addressError}
          >
            <TextInput
              type="url"
              inputMode="url"
              value={address}
              onValue={(value) => {
                mutation.reset()
                onAddress(value)
              }}
              placeholder={addressPlaceholders[login.kind]}
              autoComplete="off"
              spellCheck={false}
              mono
              required
            />
          </Field>
          {!plex && (
            <div className="flex flex-col gap-2">
              <span aria-hidden className="text-control font-medium text-ink">
                {text.connectWith}
              </span>
              <Segmented
                label={text.connectWith}
                value={login.method}
                options={[
                  { value: 'key', label: text.withApiKey },
                  { value: 'account', label: text.withAccount },
                ]}
                onChange={(method) => change({ method })}
                className="self-start"
              />
            </div>
          )}
          {byKey ? (
            <Field
              label={plex ? text.plexToken : text.apiKey}
              help={
                plex
                  ? text.plexTokenHelp
                  : login.kind === 'emby'
                    ? text.embyApiKeyHelp
                    : text.apiKeyHelp
              }
              error={loginError}
            >
              <TextInput
                type="password"
                revealable
                value={login.apiKey}
                onValue={(apiKey) => change({ apiKey })}
                autoComplete="off"
                spellCheck={false}
                required
              />
            </Field>
          ) : (
            <>
              <Field label={text.accountName} help={text.accountNameHelp(product)}>
                <TextInput
                  value={login.name}
                  onValue={(name) => change({ name })}
                  autoComplete="off"
                  spellCheck={false}
                  required
                />
              </Field>
              <Field
                label={text.accountPassword}
                help={text.accountPasswordHelp}
                error={loginError}
              >
                <TextInput
                  type="password"
                  revealable
                  value={login.password}
                  onValue={(password) => change({ password })}
                  autoComplete="off"
                />
              </Field>
            </>
          )}
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
            icon={PlugsConnectedIcon}
            loading={mutation.isPending}
            disabled={address.trim() === '' || (byKey ? login.apiKey : login.name).trim() === ''}
          >
            {mutation.isPending ? text.connecting : text.connect}
          </Button>
        </PanelFooter>
      </form>
    </Panel>
  )
}

/**
 * Step 2: what each user of the server becomes. A user with the same name as a Polyfin user goes
 * into it, another becomes a new user, and a disabled one is left out, until changed here.
 */
function ChooseUsers({
  connection,
  credentials,
  onBack,
  onStarted,
}: {
  connection: JellyfinConnection
  credentials: JellyfinCredentials
  onBack: () => void
  onStarted: () => void
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const kind = connection.server.kind
  const product = text.servers[kind]
  const toast = useToast()
  const formId = useId()
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  // A user's key or account reads its owner's watch data only: another user's is read signed in
  // as them, with their password on the server.
  const owner = connection.keyOwner
  const ownerName =
    owner === null ? null : (connection.users.find((user) => user.id === owner)?.name ?? null)
  const signsIn = (user: JellyfinUser) => owner !== null && user.id !== owner
  const [choices, setChoices] = useState<Record<string, Choice>>(() =>
    Object.fromEntries(
      connection.users.map((user): [string, Choice] => [
        user.id,
        {
          target: user.isDisabled ? 'skip' : user.userId === null ? 'new' : `user:${user.userId}`,
          name: user.name,
          password: '',
          isAdministrator: user.isAdministrator,
          watchData: true,
          jellyfinPassword: '',
          keepPassword: true,
        },
      ]),
    ),
  )

  const start = useMutation({
    mutationFn: startJellyfinImport,
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      // Without watch data to import there is no import: the last one stays the one shown.
      if (result.import !== null) queryClient.setQueryData(queryKeys.jellyfinImport, result.import)
      toast(text.started(result.created.length, result.import !== null))
      onStarted()
    },
    onError: (error) => {
      // A Polyfin user chosen here is gone: the list of users to import into is refreshed.
      if (error instanceof ApiError && error.code === 'not_found') {
        void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      }
    },
  })

  const failed = start.error instanceof ApiError ? start.error : null
  const message = start.isError ? serverError(t, kind, start.error) : undefined
  // A field of a row the server refused shows the error under it.
  const field: RowError['field'] | null =
    failed === null || failed.jellyfinId === null
      ? null
      : nameCodes.includes(failed.code)
        ? 'name'
        : passwordCodes.includes(failed.code)
          ? 'password'
          : signInCodes.includes(failed.code)
            ? 'jellyfinPassword'
            : null
  const otherError = field === null ? message : undefined

  const options: SelectOption<Target>[] = [
    { value: 'skip', label: text.skip },
    { value: 'new', label: text.newUser },
    ...(users.data ?? []).map((user) => ({ value: `user:${user.id}` as const, label: user.name })),
  ]

  // Users are created all or none; an existing user without watch data has nothing to import.
  const entries = connection.users.flatMap((user): JellyfinImportEntry[] => {
    const choice = choices[user.id]
    if (choice.target === 'skip' || (choice.target !== 'new' && !choice.watchData)) return []
    const signIn = signsIn(user) && choice.watchData
    const entry: JellyfinImportEntry = {
      jellyfinId: user.id,
      watchData: choice.watchData,
      ...(signIn && { jellyfinPassword: choice.jellyfinPassword }),
    }
    if (choice.target !== 'new') return [{ ...entry, userId: choice.target.slice('user:'.length) }]
    const create = {
      name: choice.name.trim(),
      password: signIn && choice.keepPassword ? choice.jellyfinPassword : choice.password,
      isAdministrator: choice.isAdministrator,
      isHidden: user.isHidden,
    }
    return [{ ...entry, create }]
  })

  function update(id: string, patch: Partial<Choice>) {
    start.reset()
    setChoices((current) => ({ ...current, [id]: { ...current[id], ...patch } }))
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    start.mutate({ kind, address: connection.server.address, ...credentials, users: entries })
  }

  return (
    <Panel
      title={text.chooseTitle}
      description={text.chooseHelp(connection.server.name, product, connection.server.version)}
      actions={
        <Button variant="ghost" onClick={onBack}>
          {text.changeServer}
        </Button>
      }
      footer={
        <PanelFooter note={entries.length === 0 ? text.nothingToImport : undefined}>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            icon={ArrowSquareInIcon}
            loading={start.isPending}
            disabled={!users.isSuccess || entries.length === 0}
          >
            {start.isPending ? text.starting : text.start}
          </Button>
        </PanelFooter>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="flex flex-col gap-4">
        {users.isPending ? (
          <SkeletonRows rows={3} label={t.common.loading} />
        ) : users.isError ? (
          <InlineError onRetry={() => void users.refetch()} retrying={users.isFetching}>
            {errorMessage(t, users.error)}
          </InlineError>
        ) : connection.users.length === 0 ? (
          <p className="text-small text-ink-3">{text.noUsers(product)}</p>
        ) : (
          <>
            {owner !== null && (
              <Notice>{text.ownerNotice(ownerName, 'account' in credentials)}</Notice>
            )}
            {/* Plex's owner's token reads the other accounts' played history, not their resume points. */}
            {kind === 'plex' && <Notice>{text.plexNotice}</Notice>}
            <RowList variant="plain" aria-label={text.chooseTitle}>
              {connection.users.map((user) => (
                <ChoiceRow
                  key={user.id}
                  user={user}
                  server={connection.server.name}
                  activity={kind !== 'plex'}
                  dataHelp={
                    kind !== 'plex'
                      ? text.watchDataHelp
                      : user.isAdministrator
                        ? text.plexOwnerWatchDataHelp
                        : text.plexWatchDataHelp
                  }
                  choice={choices[user.id]}
                  options={options}
                  signsIn={signsIn(user)}
                  error={
                    field !== null && message !== undefined && failed?.jellyfinId === user.id
                      ? { field, message }
                      : undefined
                  }
                  onChange={(patch) => update(user.id, patch)}
                />
              ))}
            </RowList>
          </>
        )}
        {otherError && (
          <Notice tone="danger" live>
            {otherError}
          </Notice>
        )}
      </form>
    </Panel>
  )
}

/** An error the server answered about a field of a row. */
type RowError = { field: 'name' | 'password' | 'jellyfinPassword'; message: string }

/** One user of the server: who it is, what it becomes in Polyfin, and whether its data comes along. */
function ChoiceRow({
  user,
  server,
  dataHelp,
  activity,
  choice,
  options,
  signsIn,
  error,
  onChange,
}: {
  user: JellyfinUser
  /** The server's name. */
  server: string
  /** What importing this user's watch data brings. */
  dataHelp: string
  /** Whether the server tells when its users last used it: Plex does not. */
  activity: boolean
  choice: Choice
  options: SelectOption<Target>[]
  /** The connection is another user's: this user's watch data is read signed in as them. */
  signsIn: boolean
  error: RowError | undefined
  onChange: (patch: Partial<Choice>) => void
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  // An existing user gets nothing without its watch data.
  const existing = choice.target !== 'skip' && choice.target !== 'new'
  const signIn = signsIn && choice.watchData
  // A new user keeping their password on the server has no other.
  const keeps = signIn && choice.target === 'new' && choice.keepPassword
  const lastActivity =
    user.lastActivityAt === null ? t.common.never : <RelativeTime iso={user.lastActivityAt} />
  return (
    <Row
      leading={<Avatar name={user.name} size="lg" />}
      title={user.name}
      muted={choice.target === 'skip'}
      titleAside={
        <>
          {user.isAdministrator && <Badge tone="accent">{t.users.administrator}</Badge>}
          {user.isDisabled && <Badge>{t.users.disabled}</Badge>}
        </>
      }
      meta={
        activity ? (
          <span className="truncate">
            {text.lastActivityLabel} {lastActivity}
          </span>
        ) : undefined
      }
    >
      <div className="flex flex-col gap-4">
        <Field label={text.importAs} className="max-w-sm">
          <Select
            value={choice.target}
            options={options}
            onValue={(target) => onChange({ target })}
          />
        </Field>
        {choice.target === 'new' && (
          <>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                label={t.users.name}
                help={t.common.nameRule}
                error={error?.field === 'name' ? error.message : undefined}
              >
                <TextInput
                  value={choice.name}
                  onValue={(name) => onChange({ name })}
                  autoComplete="off"
                  maxLength={64}
                  required
                />
              </Field>
              {!keeps && (
                <Field
                  label={t.users.password}
                  help={t.common.passwordRule}
                  error={error?.field === 'password' ? error.message : undefined}
                >
                  <TextInput
                    type="password"
                    revealable
                    value={choice.password}
                    onValue={(password) => onChange({ password })}
                    autoComplete="new-password"
                    required
                  />
                </Field>
              )}
            </div>
            <Checkbox
              label={t.users.isAdministrator}
              help={t.users.isAdministratorHelp}
              checked={choice.isAdministrator}
              onChange={(isAdministrator) => onChange({ isAdministrator })}
            />
          </>
        )}
        {choice.target !== 'skip' && (
          <Checkbox
            label={text.watchData}
            help={existing && !choice.watchData ? text.watchDataNeeded : dataHelp}
            checked={choice.watchData}
            onChange={(watchData) => onChange({ watchData })}
          />
        )}
        {choice.target !== 'skip' && signIn && (
          <>
            <Field
              label={text.serverPassword(server)}
              help={text.serverPasswordHelp(user.name)}
              error={
                error?.field === 'jellyfinPassword' || (keeps && error?.field === 'password')
                  ? error.message
                  : undefined
              }
              className="max-w-sm"
            >
              <TextInput
                type="password"
                revealable
                value={choice.jellyfinPassword}
                onValue={(jellyfinPassword) => onChange({ jellyfinPassword })}
                autoComplete="off"
              />
            </Field>
            {choice.target === 'new' && (
              <Checkbox
                label={text.keepPassword}
                help={text.keepPasswordHelp}
                checked={choice.keepPassword}
                onChange={(keepPassword) => onChange({ keepPassword })}
              />
            )}
          </>
        )}
      </div>
    </Row>
  )
}
