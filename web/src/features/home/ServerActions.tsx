import { ArrowClockwiseIcon, CloudArrowDownIcon, DownloadSimpleIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useCallback, useEffect, useState, type FormEvent } from 'react'
import {
  ApiError,
  askUpdateTool,
  fetchOwnTwoStep,
  fetchServerActions,
  installDesktopUpdate,
  queryKeys,
  restartServer,
  type DesktopUpdate,
  type ServerActions,
  type TwoStepConfirmation,
  type UpdateToolAnswer,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import { Button, Field, Modal, Notice, ProgressBar, Segmented, Spinner, TextInput } from '@/ui'

/** How often Polyfin is asked whether it answers again, in milliseconds. */
const pollEvery = 2000

/** How long Polyfin may take to answer again before the page says what to do, in seconds. */
const longWait = 120

/** How the server's actions stand, refreshed every 10 seconds: Polyfin Desktop's download moves. */
export function useServerActions() {
  return useQuery({
    queryKey: queryKeys.serverActions,
    queryFn: ({ signal }) => fetchServerActions(signal),
    refetchInterval: 10_000,
  })
}

export type ServerAction = 'restart' | 'install' | 'update'

/**
 * At the top of Home: the version Polyfin runs, Restart, in red as on Jellyfin's dashboard, and
 * Update now while an update tool is set. Each opens its confirmation through `onOpen`.
 */
export function ServerActionButtons({
  actions,
  onOpen,
}: {
  actions: ServerActions
  onOpen: (action: ServerAction) => void
}) {
  const { t } = useI18n()
  const text = t.dashboard.server
  return (
    <>
      <span className="mr-2 text-small text-ink-3">
        {t.dashboard.overview.version}{' '}
        <span className="font-mono text-ink-2">{actions.version}</span>
      </span>
      <Button
        size="sm"
        variant="danger"
        icon={ArrowClockwiseIcon}
        onClick={() => onOpen('restart')}
      >
        {text.restart}
      </Button>
      {actions.updateTool && (
        <Button
          size="sm"
          variant="secondary"
          icon={CloudArrowDownIcon}
          onClick={() => onOpen('update')}
        >
          {text.updateNow}
        </Button>
      )}
    </>
  )
}

/** Update now in the notice of a new version, beside its release notes, while an update tool is set. */
export function NoticeUpdateButton({ onOpen }: { onOpen: (action: ServerAction) => void }) {
  const { t } = useI18n()
  return (
    <Button size="sm" variant="primary" icon={CloudArrowDownIcon} onClick={() => onOpen('update')}>
      {t.dashboard.server.updateNow}
    </Button>
  )
}

/**
 * Install Polyfin <version>, beside the release notes of a new version, while Polyfin Desktop
 * offers it, as its tray's item does; its download's progress while it downloads.
 */
export function DesktopInstall({
  actions,
  update,
}: {
  actions: ServerActions
  update: DesktopUpdate
}) {
  const { t } = useI18n()
  const text = t.dashboard.server
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button
        size="sm"
        icon={DownloadSimpleIcon}
        disabled={update.installing}
        onClick={() => setOpen(true)}
      >
        {update.installing ? text.downloading(update.percent) : text.install(update.version)}
      </Button>
      {open && <ActionDialog action="install" actions={actions} onClose={() => setOpen(false)} />}
    </>
  )
}

type Phase =
  | { kind: 'confirm' }
  /** The server stops: waiting for one started after `since` to answer. */
  | { kind: 'waiting'; since: string; gone: boolean }
  | { kind: 'answered'; answer: UpdateToolAnswer }

/**
 * Confirms an action with the password or a two-step code, then shows what follows: Polyfin
 * stopping and answering again, which reloads the page, or the update tool's answer.
 */
export function ActionDialog({
  action,
  actions,
  onClose,
}: {
  action: ServerAction
  actions: ServerActions
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.dashboard.server
  const [phase, setPhase] = useState<Phase>({ kind: 'confirm' })
  const [method, setMethod] = useState<'password' | 'code'>('password')
  const [value, setValue] = useState('')
  const twoStep = useQuery({
    queryKey: queryKeys.ownTwoStep,
    queryFn: ({ signal }) => fetchOwnTwoStep(signal),
  })
  const version = actions.desktopUpdate?.version ?? ''
  const unknown = action === 'restart' && actions.restart === 'unknown'
  const mutation = useMutation({
    mutationFn: async (confirmation: TwoStepConfirmation): Promise<Phase> => {
      switch (action) {
        case 'restart': {
          const { startedAt } = await restartServer(confirmation, unknown)
          return { kind: 'waiting', since: startedAt, gone: false }
        }
        case 'install':
          await installDesktopUpdate(confirmation, version)
          return { kind: 'waiting', since: actions.startedAt, gone: false }
        case 'update':
          try {
            return { kind: 'answered', answer: await askUpdateTool(confirmation) }
          } catch (error) {
            // Polyfin stopped before the tool answered: the tool is replacing it.
            if (stopped(error)) return { kind: 'waiting', since: actions.startedAt, gone: true }
            throw error
          }
      }
    },
    onSuccess: setPhase,
  })

  if (phase.kind === 'waiting') {
    return (
      <Waiting
        title={action === 'install' ? text.installWaitingTitle(version) : text.waitingTitle}
        since={phase.since}
        gone={phase.gone}
        installing={action === 'install'}
        onClose={onClose}
      />
    )
  }
  if (phase.kind === 'answered') {
    return <Answered answer={phase.answer} since={actions.startedAt} onClose={onClose} />
  }

  const failure = mutation.isError ? actionError(t, mutation.error) : undefined
  const fieldError =
    mutation.error instanceof ApiError &&
    ['invalid_code', 'wrong_password'].includes(mutation.error.code)
      ? failure
      : undefined
  const formId = 'server-action-confirmation'
  const title =
    action === 'restart'
      ? text.restartTitle
      : action === 'install'
        ? text.installTitle(version)
        : text.updateTitle
  const confirm =
    action === 'restart'
      ? unknown
        ? text.restartAnyway
        : text.restartConfirm
      : action === 'install'
        ? text.installConfirm
        : text.updateConfirm
  const pending =
    action === 'restart' ? text.restarting : action === 'install' ? text.installing : text.sending

  return (
    <Modal
      open
      onClose={onClose}
      title={title}
      width={560}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={mutation.isPending}>
            {t.common.cancel}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant={action === 'update' ? 'primary' : 'danger'}
            loading={mutation.isPending}
            disabled={value === ''}
          >
            {mutation.isPending ? pending : confirm}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        className="space-y-4 p-5"
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault()
          mutation.mutate(method === 'code' ? { code: value.trim() } : { password: value })
        }}
      >
        <p className="text-control text-ink-2">
          {action === 'restart'
            ? `${text.restartText}${actions.restart === 'unknown' ? '' : ` ${text.restartBy[actions.restart]}`}`
            : action === 'install'
              ? text.installText
              : text.updateText}
        </p>
        {unknown && (
          <Notice tone="warn">
            <strong className="block font-semibold">{text.unknownTitle}</strong>
            {actions.system === 'container' ? text.unknown.container : text.unknown.other}{' '}
            {text.unknownHelp}
          </Notice>
        )}
        {twoStep.data?.on && (
          <Segmented
            label={text.confirmWith}
            options={[
              { value: 'password', label: text.withPassword },
              { value: 'code', label: text.withCode },
            ]}
            value={method}
            onChange={(next) => {
              mutation.reset()
              setValue('')
              setMethod(next)
            }}
          />
        )}
        <Field
          label={method === 'code' ? text.code : text.password}
          help={text.identityHelp}
          error={fieldError}
        >
          <TextInput
            key={method}
            type={method === 'code' ? 'text' : 'password'}
            value={value}
            onValue={(next) => {
              mutation.reset()
              setValue(next)
            }}
            autoComplete={method === 'code' ? 'one-time-code' : 'current-password'}
            mono={method === 'code'}
            autoFocus
            required
            invalid={fieldError !== undefined}
          />
        </Field>
        {failure !== undefined && fieldError === undefined && (
          <Notice tone="danger" live>
            {failure}
          </Notice>
        )}
      </form>
    </Modal>
  )
}

/** The words of an action's error, with what the server tells of the update tool. */
function actionError(t: Messages, error: unknown): string {
  if (error instanceof ApiError && error.code === 'too_soon') return t.dashboard.server.tooSoon
  const message = errorMessage(t, error)
  return error instanceof ApiError && error.detail ? `${message} ${error.detail}` : message
}

/**
 * Whether a request failed because Polyfin stopped: no answer, or a reverse proxy's 502, 503 or 504
 * without Polyfin's own error.
 */
function stopped(error: unknown): boolean {
  if (error instanceof TypeError) return true
  return (
    error instanceof ApiError &&
    error.status >= 502 &&
    error.status <= 504 &&
    error.code === 'unknown'
  )
}

/**
 * Asks Polyfin every 2 seconds whether one started after `since` answers, and reloads the page
 * then: the admin app may be new too. Tells the seconds waited, and calls `onState` with each
 * answer of the Polyfin still running.
 */
function useComeBack(since: string, onState?: (state: ServerActions) => void): number {
  const [waited, setWaited] = useState(0)
  useEffect(() => {
    const started = Date.now()
    const timer = window.setInterval(() => {
      setWaited(Math.round((Date.now() - started) / 1000))
      void fetchServerActions()
        .then((state) => {
          if (state.startedAt !== since) window.location.reload()
          else onState?.(state)
        })
        .catch(() => undefined)
    }, pollEvery)
    return () => window.clearInterval(timer)
  }, [since, onState])
  return waited
}

/** While Polyfin stops and starts again: Polyfin Desktop's download first, for an install. */
function Waiting({
  title,
  since,
  gone,
  installing,
  onClose,
}: {
  title: string
  since: string
  gone: boolean
  installing: boolean
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.dashboard.server
  const [update, setUpdate] = useState<DesktopUpdate | null>(null)
  // Polyfin Desktop's download, from each answer of the Polyfin still running.
  const onState = useCallback((state: ServerActions) => setUpdate(state.desktopUpdate), [])
  const waited = useComeBack(since, installing ? onState : undefined)
  const failed = installing && update?.failed === true && !update.installing
  return (
    <Modal
      open
      onClose={failed ? onClose : () => {}}
      title={title}
      width={480}
      footer={
        failed ? (
          <Button variant="ghost" onClick={onClose}>
            {text.close}
          </Button>
        ) : undefined
      }
    >
      <div className="space-y-4 p-5">
        {failed ? (
          <Notice tone="danger" live>
            {text.installFailed}
          </Notice>
        ) : installing && update?.installing ? (
          <ProgressBar
            value={update.percent / 100}
            label={text.downloading(update.percent)}
            valueText={text.downloading(update.percent)}
            live
          />
        ) : (
          <p className="flex items-center gap-3 text-small text-ink-2" role="status">
            <Spinner size={20} />{' '}
            {gone ? text.gone : installing ? text.installStopping : text.waiting}
          </p>
        )}
        {!failed && waited >= longWait && <Notice tone="warn">{text.waitingLong(waited)}</Notice>}
      </div>
    </Modal>
  )
}

/** The update tool's answer, as it gave it; the page reloads if Polyfin is replaced meanwhile. */
function Answered({
  answer,
  since,
  onClose,
}: {
  answer: UpdateToolAnswer
  since: string
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.dashboard.server
  useComeBack(since)
  const ok = answer.status >= 200 && answer.status < 300
  return (
    <Modal
      open
      onClose={onClose}
      title={text.updateTitle}
      width={560}
      footer={
        <Button variant="ghost" onClick={onClose}>
          {text.close}
        </Button>
      }
    >
      <div className="space-y-4 p-5">
        <Notice tone={ok ? 'ok' : 'warn'} live>
          {text.answer(answer.status)}
        </Notice>
        {answer.body === '' ? (
          <p className="text-small text-ink-3">{text.answerEmpty}</p>
        ) : (
          <pre className="max-h-64 overflow-auto rounded-row bg-s2 p-3 font-mono text-micro whitespace-pre-wrap text-ink-2">
            {answer.body}
          </pre>
        )}
        {ok && <p className="text-small text-ink-3">{text.watching}</p>}
      </div>
    </Modal>
  )
}
