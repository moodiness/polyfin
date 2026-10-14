import { ClockCounterClockwiseIcon, UploadSimpleIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import {
  ApiError,
  fetchBackupFiles,
  prepareRestore,
  queryKeys,
  startRestore,
  uploadRestore,
  type BackupFile,
  type RestorePlan,
} from '@/api'
import { dateTime, errorMessage, formatBytes } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Badge,
  Button,
  Field,
  InlineError,
  Modal,
  Notice,
  SkeletonRows,
  Spinner,
  TextInput,
} from '@/ui'

/** How often Polyfin is asked whether it answers again after a restore, in milliseconds. */
const pollEvery = 2000

/** An error with what the destination or the tool said, when the server tells it. */
function failure(t: Messages, error: unknown): string {
  const message = errorMessage(t, error)
  return error instanceof ApiError && error.detail ? `${message} ${error.detail}` : message
}

/**
 * Restore in the Database backups panel: lists the backups of the folder and of each destination,
 * or takes a .dump file, checks the one chosen, then asks for the server's name, telling what
 * happens and which stored keys the current POLYFIN_SECRET_KEY cannot read.
 */
export default function RestoreBackup({ unavailable }: { unavailable: '' | 'pg_restore_missing' }) {
  const { t } = useI18n()
  const text = t.backups.restore
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button
        size="sm"
        variant="secondary"
        icon={ClockCounterClockwiseIcon}
        onClick={() => setOpen(true)}
      >
        {text.button}
      </Button>
      {open && <RestoreModal unavailable={unavailable} onClose={() => setOpen(false)} />}
    </>
  )
}

function RestoreModal({
  unavailable,
  onClose,
}: {
  unavailable: '' | 'pg_restore_missing'
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.backups.restore
  const [plan, setPlan] = useState<RestorePlan | null>(null)
  const [restoring, setRestoring] = useState(false)
  const prepare = useMutation({
    mutationFn: (choice: { destination: string | null; file: string } | File) =>
      choice instanceof File
        ? uploadRestore(choice)
        : prepareRestore(choice.destination, choice.file),
    onSuccess: setPlan,
  })
  if (restoring) return <Restoring />
  if (plan !== null) {
    return (
      <Confirm
        plan={plan}
        onClose={onClose}
        onBack={() => {
          setPlan(null)
          prepare.reset()
        }}
        onStarted={() => setRestoring(true)}
      />
    )
  }
  return (
    <Modal open onClose={onClose} title={text.title} width={640}>
      <div className="space-y-5 p-5">
        <p className="text-small text-ink-2">{text.intro}</p>
        {unavailable !== '' && <Notice tone="danger">{text.unavailable[unavailable]}</Notice>}
        {prepare.isPending && (
          <p className="flex items-center gap-2 text-small text-ink-2">
            <Spinner /> {text.checking}
          </p>
        )}
        {prepare.isError && <Notice tone="danger">{failure(t, prepare.error)}</Notice>}
        <Choices
          disabled={unavailable !== '' || prepare.isPending}
          onChoose={(destination, file) => prepare.mutate({ destination, file })}
        />
        <Upload
          disabled={unavailable !== '' || prepare.isPending}
          onFile={(file) => prepare.mutate(file)}
        />
      </div>
    </Modal>
  )
}

/** The backups of the folder, then of each destination. */
function Choices({
  disabled,
  onChoose,
}: {
  disabled: boolean
  onChoose: (destination: string | null, file: string) => void
}) {
  const { t } = useI18n()
  const text = t.backups.restore
  const files = useQuery({
    queryKey: queryKeys.backupFiles,
    queryFn: ({ signal }) => fetchBackupFiles(signal),
  })
  if (files.isPending) return <SkeletonRows rows={3} label={text.loading} />
  if (files.isError) {
    return (
      <InlineError onRetry={() => void files.refetch()} retrying={files.isFetching}>
        {failure(t, files.error)}
      </InlineError>
    )
  }
  return (
    <div className="space-y-5">
      <FileList
        title={text.folder}
        files={files.data.folder}
        disabled={disabled}
        onChoose={(file) => onChoose(null, file)}
      />
      {files.data.destinations.map(({ destination, files: there, error }) => (
        <FileList
          key={destination.id}
          title={destination.name}
          files={there}
          // A password the key cannot decrypt is told in the interface's language.
          error={destination.unreadable ? t.backups.destinations.unreadable : error}
          disabled={disabled}
          onChoose={(file) => onChoose(destination.id, file)}
        />
      ))}
    </div>
  )
}

function FileList({
  title,
  files,
  error,
  disabled,
  onChoose,
}: {
  title: string
  files: BackupFile[]
  error?: string
  disabled: boolean
  onChoose: (file: string) => void
}) {
  const { language, t } = useI18n()
  const text = t.backups.restore
  let body: ReactNode
  if (error) body = <Notice tone="danger">{error}</Notice>
  else if (files.length === 0) body = <p className="text-small text-ink-3">{text.none}</p>
  else {
    body = (
      <ul className="overflow-hidden rounded-row border border-line-2 bg-s1">
        {files.map((file) => (
          <li
            key={file.name}
            className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2.5 [&+&]:border-t [&+&]:border-line"
          >
            <span className="min-w-0 flex-1">
              <code className="font-mono text-small break-all text-ink">{file.name}</code>
              <span className="block text-small text-ink-3">
                {dateTime(file.madeAt, language)} · {formatBytes(file.size, language)}
                {file.beforeRestore && (
                  <>
                    {' '}
                    <Badge>{text.beforeRestore}</Badge>
                  </>
                )}
              </span>
            </span>
            <Button
              size="sm"
              variant="secondary"
              disabled={disabled}
              onClick={() => onChoose(file.name)}
            >
              {text.choose}
            </Button>
          </li>
        ))}
      </ul>
    )
  }
  return (
    <section>
      <h3 className="mb-2 font-mono text-[14px] font-medium break-all text-ink">{title}</h3>
      {body}
    </section>
  )
}

/** Sends a .dump file of this computer. */
function Upload({ disabled, onFile }: { disabled: boolean; onFile: (file: File) => void }) {
  const { t } = useI18n()
  const text = t.backups.restore
  const input = useRef<HTMLInputElement>(null)
  return (
    <div className="border-t border-line pt-4">
      <input
        ref={input}
        type="file"
        accept=".dump,application/octet-stream"
        className="sr-only"
        tabIndex={-1}
        onChange={(event) => {
          const file = event.target.files?.[0]
          event.target.value = ''
          if (file) onFile(file)
        }}
      />
      <Button
        variant="secondary"
        icon={UploadSimpleIcon}
        disabled={disabled}
        onClick={() => input.current?.click()}
      >
        {text.upload}
      </Button>
      <p className="mt-2 text-small text-ink-3">{text.uploadHelp}</p>
    </div>
  )
}

/** What happens, the keys that cannot be read, and the server's name to type. */
function Confirm({
  plan,
  onClose,
  onBack,
  onStarted,
}: {
  plan: RestorePlan
  onClose: () => void
  onBack: () => void
  onStarted: () => void
}) {
  const { language, t } = useI18n()
  const text = t.backups.restore
  const [name, setName] = useState('')
  const unreadable = Object.entries(plan.unreadable).filter(([, count]) => (count ?? 0) > 0) as [
    keyof typeof text.kinds,
    number,
  ][]
  const anyway = unreadable.length > 0
  const start = useMutation({
    mutationFn: () => startRestore(plan.id, name.trim(), anyway),
    onSuccess: onStarted,
  })
  const code = start.error instanceof ApiError ? start.error.code : null
  const nameError = code === 'wrong_server_name' ? errorMessage(t, start.error) : undefined
  return (
    <Modal
      open
      onClose={onClose}
      title={text.confirmTitle(plan.file)}
      width={600}
      footer={
        <>
          <Button variant="ghost" onClick={onBack} disabled={start.isPending}>
            {text.back}
          </Button>
          <Button variant="ghost" onClick={onClose} disabled={start.isPending}>
            {t.common.cancel}
          </Button>
          <Button
            variant="danger"
            loading={start.isPending}
            disabled={name.trim() !== plan.serverName}
            onClick={() => start.mutate()}
          >
            {start.isPending ? text.starting : anyway ? text.anyway : text.confirm}
          </Button>
        </>
      }
    >
      <div className="space-y-5 p-5">
        <p className="text-small text-ink-2">
          {plan.madeAt !== null && text.made(dateTime(plan.madeAt, language))}{' '}
          {plan.destination !== null && text.from(plan.destination)}
          {plan.uploaded && text.uploaded} {formatBytes(plan.size, language)}
        </p>
        <ul className="list-disc space-y-1.5 pl-5 text-small text-ink-2">
          {text.steps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ul>
        {anyway && (
          <Notice tone="warn">
            <strong className="block font-semibold">{text.keyTitle}</strong>
            {text.keyBody}
            <ul className="mt-1.5 list-disc pl-5">
              {unreadable.map(([kind, count]) => (
                <li key={kind}>{text.kinds[kind](count)}</li>
              ))}
            </ul>
          </Notice>
        )}
        <Field label={text.serverName(plan.serverName)} error={nameError}>
          <TextInput
            value={name}
            onValue={(value) => {
              start.reset()
              setName(value)
            }}
            autoComplete="off"
            spellCheck={false}
            invalid={nameError !== undefined}
          />
        </Field>
        {start.isError && nameError === undefined && (
          <Notice tone="danger">{failure(t, start.error)}</Notice>
        )}
      </div>
    </Modal>
  )
}

/**
 * While Polyfin stops, restores and starts again: reloads the page once Polyfin answered that it
 * is busy, then that it is ready again.
 */
function Restoring() {
  const { t } = useI18n()
  const text = t.backups.restore
  useEffect(() => {
    let down = false
    const started = Date.now()
    const timer = window.setInterval(() => {
      void fetch(`${import.meta.env.BASE_URL}../readyz`, { cache: 'no-store' })
        .then((response) => {
          if (!response.ok) down = true
          // A restore too quick to be seen down is over after a minute.
          else if (down || Date.now() - started > 60_000) window.location.reload()
        })
        .catch(() => {
          down = true
        })
    }, pollEvery)
    return () => window.clearInterval(timer)
  }, [])
  return (
    <Modal open onClose={() => {}} title={text.restoringTitle} width={480}>
      <p className="flex items-center gap-3 p-5 text-small text-ink-2">
        <Spinner size={20} /> {text.restoringBody}
      </p>
    </Modal>
  )
}
