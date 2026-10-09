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
  type JellyfinImportEntry,
  type JellyfinImportStatus,
  type JellyfinUser,
  type JellyfinUserImport,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage, formatEpisode } from '@/format'
import { useI18n } from '@/i18n'
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

/** Connection codes about the address or the key, shown under that field rather than above. */
const addressCodes = ['invalid_jellyfin_address', 'jellyfin_unreachable', 'not_jellyfin']
const keyCodes = ['jellyfin_key_refused']
/** Codes about a new user's name or password, shown under that field of its row. */
const nameCodes = ['invalid_name', 'name_taken']
const passwordCodes = ['invalid_password']

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

/** What a Jellyfin user becomes: nothing, a new Polyfin user, or an existing one (`user:<id>`). */
type Target = 'skip' | 'new' | `user:${string}`

/** The choices of one Jellyfin user's row. */
type Choice = {
  target: Target
  name: string
  password: string
  isAdministrator: boolean
  watchData: boolean
}

/**
 * `/users/jellyfin-import`: imports accounts and their watch data from a Jellyfin server. The
 * running or last import shows first; under it, connecting to a server, then choosing what each of
 * its users becomes. The API key only lives in this page's state: the server never keeps it.
 */
export default function JellyfinImportRoute() {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const [address, setAddress] = useState('')
  const [apiKey, setApiKey] = useState('')
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
          apiKey={apiKey}
          onAddress={setAddress}
          onApiKey={setApiKey}
          onConnected={setConnection}
        />
      ) : (
        <ChooseUsers
          connection={connection}
          apiKey={apiKey}
          onBack={() => setConnection(null)}
          onStarted={() => {
            setConnection(null)
            setApiKey('')
          }}
        />
      )}
    </PageLayout>
  )
}

/** The import running, or the last one: its state, then each user's. */
function ImportProgress({ current }: { current: JellyfinImportStatus }) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const [shownId, setShownId] = useState<string | null>(null)
  const running = current.state === 'running'
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
      title={running ? text.currentTitle : text.lastTitle}
      titleAside={
        <StatusPill tone={importTones[current.state]}>{text.states[current.state]}</StatusPill>
      }
      description={
        <>
          {current.server.name} · {text.startedLabel} <RelativeTime iso={current.startedAt} />
          {current.endedAt !== null && (
            <>
              {' '}
              · {text.endedLabel} <RelativeTime iso={current.endedAt} />
            </>
          )}
        </>
      }
      actions={
        running ? (
          <Button icon={StopCircleIcon} loading={stop.isPending} onClick={() => stop.mutate()}>
            {stop.isPending ? text.stopping : text.stop}
          </Button>
        ) : undefined
      }
    >
      <div className="flex flex-col gap-4">
        {running && <p className="text-small text-ink-3">{text.runningHelp}</p>}
        {current.problem !== null && (
          <Notice tone="danger">{text.problems[current.problem]}</Notice>
        )}
        {stop.isError && (
          <Notice tone="danger" live>
            {errorMessage(t, stop.error)}
          </Notice>
        )}
        <RowList variant="plain" aria-label={running ? text.currentTitle : text.lastTitle}>
          {current.users.map((user) => (
            <ImportedUserRow
              key={user.jellyfinId}
              user={user}
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
 * One Jellyfin user of an import: the Polyfin user it went into, its state, what it read while it
 * reads, then what it added. A finished import's users still waiting were never reached.
 */
function ImportedUserRow({
  user,
  finished,
  onShowUnmatched,
}: {
  user: JellyfinUserImport
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
        <Notice tone="danger">{text.problems[user.problem]}</Notice>
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

/** Step 1: the server's address and an API key, which read its users. */
function ConnectForm({
  address,
  apiKey,
  onAddress,
  onApiKey,
  onConnected,
}: {
  address: string
  apiKey: string
  onAddress: (address: string) => void
  onApiKey: (apiKey: string) => void
  onConnected: (connection: JellyfinConnection) => void
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const mutation = useMutation({ mutationFn: connectJellyfin, onSuccess: onConnected })

  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  const message = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const addressError = code !== null && addressCodes.includes(code) ? message : undefined
  const keyError = code !== null && keyCodes.includes(code) ? message : undefined
  const otherError = mutation.isError && !addressError && !keyError ? message : undefined

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({ address: address.trim(), apiKey: apiKey.trim() })
  }

  return (
    <Panel title={text.connectTitle} description={text.connectHelp} flush>
      <form onSubmit={submit} noValidate>
        <div className="flex max-w-lg flex-col gap-5 px-6 pb-6 max-sm:px-4">
          <Field label={text.address} help={text.addressHelp} error={addressError}>
            <TextInput
              type="url"
              inputMode="url"
              value={address}
              onValue={(value) => {
                mutation.reset()
                onAddress(value)
              }}
              placeholder="http://192.168.1.10:8096"
              autoComplete="off"
              spellCheck={false}
              mono
              required
            />
          </Field>
          <Field label={text.apiKey} help={text.apiKeyHelp} error={keyError}>
            <TextInput
              type="password"
              revealable
              value={apiKey}
              onValue={(value) => {
                mutation.reset()
                onApiKey(value)
              }}
              autoComplete="off"
              spellCheck={false}
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
            icon={PlugsConnectedIcon}
            loading={mutation.isPending}
            disabled={address.trim() === '' || apiKey.trim() === ''}
          >
            {mutation.isPending ? text.connecting : text.connect}
          </Button>
        </PanelFooter>
      </form>
    </Panel>
  )
}

/**
 * Step 2: what each Jellyfin user becomes. A user with the same name as a Polyfin user goes into
 * it, another becomes a new user, and a disabled one is left out, until changed here.
 */
function ChooseUsers({
  connection,
  apiKey,
  onBack,
  onStarted,
}: {
  connection: JellyfinConnection
  apiKey: string
  onBack: () => void
  onStarted: () => void
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  const toast = useToast()
  const formId = useId()
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
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
  const message = start.isError ? errorMessage(t, start.error) : undefined
  // A new user's name or password the server refused shows under that field of its row.
  const field =
    failed === null || failed.jellyfinId === null
      ? null
      : nameCodes.includes(failed.code)
        ? 'name'
        : passwordCodes.includes(failed.code)
          ? 'password'
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
    if (choice.target === 'skip') return []
    if (choice.target === 'new') {
      const create = {
        name: choice.name.trim(),
        password: choice.password,
        isAdministrator: choice.isAdministrator,
        isHidden: user.isHidden,
      }
      return [{ jellyfinId: user.id, create, watchData: choice.watchData }]
    }
    if (!choice.watchData) return []
    return [{ jellyfinId: user.id, userId: choice.target.slice('user:'.length), watchData: true }]
  })

  function update(id: string, patch: Partial<Choice>) {
    start.reset()
    setChoices((current) => ({ ...current, [id]: { ...current[id], ...patch } }))
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    start.mutate({ address: connection.server.address, apiKey, users: entries })
  }

  return (
    <Panel
      title={text.chooseTitle}
      description={text.chooseHelp(connection.server.name, connection.server.version)}
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
          <p className="text-small text-ink-3">{text.noUsers}</p>
        ) : (
          <RowList variant="plain" aria-label={text.chooseTitle}>
            {connection.users.map((user) => (
              <ChoiceRow
                key={user.id}
                user={user}
                choice={choices[user.id]}
                options={options}
                nameError={field === 'name' && failed?.jellyfinId === user.id ? message : undefined}
                passwordError={
                  field === 'password' && failed?.jellyfinId === user.id ? message : undefined
                }
                onChange={(patch) => update(user.id, patch)}
              />
            ))}
          </RowList>
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

/** One Jellyfin user: who it is, what it becomes in Polyfin, and whether its data comes along. */
function ChoiceRow({
  user,
  choice,
  options,
  nameError,
  passwordError,
  onChange,
}: {
  user: JellyfinUser
  choice: Choice
  options: SelectOption<Target>[]
  nameError: string | undefined
  passwordError: string | undefined
  onChange: (patch: Partial<Choice>) => void
}) {
  const { t } = useI18n()
  const text = t.users.jellyfinImport
  // An existing user gets nothing without its watch data.
  const existing = choice.target !== 'skip' && choice.target !== 'new'
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
        <span className="truncate">
          {text.lastActivityLabel} {lastActivity}
        </span>
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
              <Field label={t.users.name} help={t.common.nameRule} error={nameError}>
                <TextInput
                  value={choice.name}
                  onValue={(name) => onChange({ name })}
                  autoComplete="off"
                  maxLength={64}
                  required
                />
              </Field>
              <Field label={t.users.password} help={t.common.passwordRule} error={passwordError}>
                <TextInput
                  type="password"
                  revealable
                  value={choice.password}
                  onValue={(password) => onChange({ password })}
                  autoComplete="new-password"
                  required
                />
              </Field>
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
            help={existing && !choice.watchData ? text.watchDataNeeded : text.watchDataHelp}
            checked={choice.watchData}
            onChange={(watchData) => onChange({ watchData })}
          />
        )}
      </div>
    </Row>
  )
}
