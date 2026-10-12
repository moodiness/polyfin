import { ArrowSquareInIcon, SignInIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import {
  ApiError,
  type AppAccount,
  type AppImportChoice,
  type AppImportStatus,
  type NuvioSignIn,
} from '@/api'
import { errorMessage, formatEpisode } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Checkbox,
  Field,
  Modal,
  Notice,
  Panel,
  PanelFooter,
  RelativeTime,
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

/**
 * The import of an app account's watch data into a Polyfin account: the user's own, under My
 * account, or a user's, for an administrator. `current` is the import running into that account,
 * else the last one. The app's own sign-in form reads the account, whose profile is then chosen
 * with its addons: the server holds what it read a few minutes only.
 */
export default function AppImport({
  current,
  addonsAllowed,
  readNuvio,
  start,
  onStarted,
  own,
}: {
  current: AppImportStatus | null
  /** Whether the account's user may add their own addons: the app's are offered only then. */
  addonsAllowed: boolean
  readNuvio: (body: NuvioSignIn) => Promise<AppAccount>
  start: (body: AppImportChoice) => Promise<AppImportStatus>
  onStarted: (started: AppImportStatus) => void
  /** The user's own account, under My account. */
  own: boolean
}) {
  const { t } = useI18n()
  const text = t.account.appImport
  const toast = useToast()
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

  return (
    <div className="flex flex-col gap-6">
      {current !== null && <AppImportProgress current={current} own={own} />}
      {!running && account === null && (
        <NuvioSignInForm
          read={readNuvio}
          onRead={(read) => {
            begin.reset()
            setAccount(read)
            setProfile(read.profiles[0]?.id ?? '')
            setAddons([])
          }}
          notice={begin.isError ? errorMessage(t, begin.error) : undefined}
        />
      )}
      {!running && account !== null && (
        <Panel as="div" flush title={text.chooseTitle} titleAs="h3" description={text.chooseHelp}>
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
                  label: p.name === '' ? text.profileNumber(p.id) : p.name,
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
 * lives in this form's state. `notice` is what went wrong with the last import's start.
 */
function NuvioSignInForm({
  read,
  onRead,
  notice,
}: {
  read: (body: NuvioSignIn) => Promise<AppAccount>
  onRead: (account: AppAccount) => void
  notice: string | undefined
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
    signIn.mutate({ backend: backend.trim(), email: email.trim(), password })
  }

  return (
    <Panel as="div" flush>
      <form onSubmit={submit} noValidate>
        <div className="flex max-w-lg flex-col gap-5 px-6 py-6 max-sm:px-4">
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
    </Panel>
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
