import {
  ClockCounterClockwiseIcon,
  DownloadSimpleIcon,
  LockKeyIcon,
  UploadSimpleIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import {
  ApiError,
  backupDecryptedUrl,
  backupDownloadUrl,
  checkBackupPassphrase,
  discardRestore,
  fetchBackupFiles,
  prepareRestore,
  queryKeys,
  startRestore,
  unlockRestore,
  uploadRestore,
  type BackupFile,
  type RestorePlan,
} from '@/api'
import { dateTime, errorMessage, formatBytes } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Badge,
  Button,
  cx,
  ExternalButtonLink,
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

/** The files a restore takes: backups, plain or encrypted. */
export const backupFileTypes = '.dump,.enc,application/octet-stream'

/** An error with what the destination or the tool said, when the server tells it. */
export function failure(t: Messages, error: unknown): string {
  const message = errorMessage(t, error)
  return error instanceof ApiError && error.detail ? `${message} ${error.detail}` : message
}

/**
 * Restore in the Database backups panel: lists the backups of the folder and of each destination,
 * or takes a backup file, checks the one chosen (asking first for the passphrase of an encrypted
 * one), then asks for the server's name, telling what happens and which stored keys the current
 * POLYFIN_SECRET_KEY cannot read. The backups of the folder can be downloaded too, as they are or
 * decrypted.
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
  const [decrypting, setDecrypting] = useState<string | null>(null)
  const prepare = useMutation({
    mutationFn: (choice: { destination: string | null; file: string } | File) =>
      choice instanceof File
        ? uploadRestore(choice)
        : prepareRestore(choice.destination, choice.file),
    onSuccess: setPlan,
  })
  if (restoring) return <Restoring />
  if (decrypting !== null) {
    return <DownloadDecrypted file={decrypting} onClose={() => setDecrypting(null)} />
  }
  if (plan !== null) {
    const close = () => {
      // The file downloaded or sent for it is deleted at once.
      void discardRestore(plan.id).catch(() => undefined)
      onClose()
    }
    const back = () => {
      void discardRestore(plan.id).catch(() => undefined)
      setPlan(null)
      prepare.reset()
    }
    if (plan.locked) {
      return (
        <Unlock
          plan={plan}
          unlock={(passphrase) => unlockRestore(plan.id, passphrase)}
          onUnlocked={setPlan}
          onClose={close}
          onBack={back}
        />
      )
    }
    return (
      <Confirm plan={plan} onClose={close} onBack={back} onStarted={() => setRestoring(true)} />
    )
  }
  return (
    <Modal open onClose={onClose} title={text.title} width={680}>
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
          onDecrypted={setDecrypting}
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
  onDecrypted,
}: {
  disabled: boolean
  onChoose: (destination: string | null, file: string) => void
  onDecrypted: (file: string) => void
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
        onDecrypted={onDecrypted}
      />
      {files.data.destinations.map(({ destination, files: there, error }) => (
        <FileList
          key={destination.id}
          title={destination.name}
          mono
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
  mono = false,
  files,
  error,
  disabled,
  onChoose,
  onDecrypted,
}: {
  title: string
  /** A destination's name, an address: shown in monospace, unlike the folder's heading. */
  mono?: boolean
  files: BackupFile[]
  error?: string
  disabled: boolean
  onChoose: (file: string) => void
  /** Offers the downloads, for the backups of the folder: decrypted with this. */
  onDecrypted?: (file: string) => void
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
            className="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2.5 [&+&]:border-t [&+&]:border-line"
          >
            <span className="min-w-0 flex-1">
              <code className="font-mono text-small break-all text-ink">{file.name}</code>
              <span className="block text-small text-ink-3">
                {dateTime(file.madeAt, language)} ·{' '}
                {file.size < 0 ? '–' : formatBytes(file.size, language)}
                {file.encrypted && (
                  <>
                    {' '}
                    <Badge>
                      <LockKeyIcon size={12} aria-hidden="true" className="mr-1 inline" />
                      {text.encrypted}
                    </Badge>
                  </>
                )}
                {file.beforeRestore && (
                  <>
                    {' '}
                    <Badge>{text.beforeRestore}</Badge>
                  </>
                )}
              </span>
            </span>
            {onDecrypted && (
              <ExternalButtonLink
                size="sm"
                variant="ghost"
                icon={DownloadSimpleIcon}
                href={backupDownloadUrl(file.name)}
                download={file.name}
              >
                {text.download}
              </ExternalButtonLink>
            )}
            {onDecrypted && file.encrypted && (
              <Button size="sm" variant="ghost" onClick={() => onDecrypted(file.name)}>
                {text.downloadDecrypted}
              </Button>
            )}
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
      <h3 className={cx('mb-2 text-[14px] font-medium break-all text-ink', mono && 'font-mono')}>
        {title}
      </h3>
      {body}
    </section>
  )
}

/**
 * Downloads the dump an encrypted backup of the folder holds: the passphrase is checked, then a
 * link with the ticket the server answers lets the browser save the dump as it comes.
 */
function DownloadDecrypted({ file, onClose }: { file: string; onClose: () => void }) {
  const { t } = useI18n()
  const text = t.backups.restore
  const [passphrase, setPassphrase] = useState('')
  const check = useMutation({
    mutationFn: () => checkBackupPassphrase(file, passphrase),
    onSuccess: ({ ticket }) => {
      const link = document.createElement('a')
      link.href = backupDecryptedUrl(file, ticket)
      link.download = file.replace(/\.enc$/, '')
      link.click()
      onClose()
    },
  })
  const wrong = check.error instanceof ApiError && check.error.code === 'wrong_passphrase'
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (passphrase !== '') check.mutate()
  }
  return (
    <Modal
      open
      onClose={onClose}
      title={text.downloadTitle(file)}
      width={540}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={check.isPending}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            icon={DownloadSimpleIcon}
            loading={check.isPending}
            disabled={passphrase === ''}
            onClick={() => check.mutate()}
          >
            {check.isPending ? text.downloading : text.download}
          </Button>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-4 p-5">
        <p className="text-small text-ink-2">{text.downloadBody}</p>
        <Field label={text.passphrase} error={wrong ? errorMessage(t, check.error) : undefined}>
          <TextInput
            type="password"
            revealable
            value={passphrase}
            onValue={(value) => {
              check.reset()
              setPassphrase(value)
            }}
            autoComplete="off"
            invalid={wrong}
            autoFocus
          />
        </Field>
        {check.isError && !wrong && <Notice tone="danger">{failure(t, check.error)}</Notice>}
      </form>
    </Modal>
  )
}

/**
 * The passphrase of an encrypted backup prepared: `unlock` opens it, and the backup, then checked,
 * goes to `onUnlocked`. A wrong passphrase may be typed again; a damaged backup is dropped.
 */
export function Unlock({
  plan,
  unlock,
  onUnlocked,
  onClose,
  onBack,
}: {
  plan: RestorePlan
  unlock: (passphrase: string) => Promise<RestorePlan>
  onUnlocked: (plan: RestorePlan) => void
  onClose: () => void
  onBack: () => void
}) {
  const { t } = useI18n()
  const text = t.backups.restore
  const [passphrase, setPassphrase] = useState('')
  const open = useMutation({ mutationFn: () => unlock(passphrase), onSuccess: onUnlocked })
  const wrong = open.error instanceof ApiError && open.error.code === 'wrong_passphrase'
  return (
    <Modal
      open
      onClose={onClose}
      title={text.unlockTitle(plan.file)}
      width={540}
      footer={
        <>
          <Button variant="ghost" onClick={onBack} disabled={open.isPending}>
            {text.back}
          </Button>
          <Button variant="ghost" onClick={onClose} disabled={open.isPending}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            icon={LockKeyIcon}
            loading={open.isPending}
            disabled={passphrase === ''}
            onClick={() => open.mutate()}
          >
            {open.isPending ? text.unlocking : text.unlock}
          </Button>
        </>
      }
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          if (passphrase !== '') open.mutate()
        }}
        className="space-y-4 p-5"
      >
        <p className="text-small text-ink-2">{text.unlockBody}</p>
        <UnlockField
          passphrase={passphrase}
          onPassphrase={(value) => {
            open.reset()
            setPassphrase(value)
          }}
          error={wrong ? errorMessage(t, open.error) : undefined}
        />
        {open.isError && !wrong && <Notice tone="danger">{failure(t, open.error)}</Notice>}
      </form>
    </Modal>
  )
}

/** The passphrase field of an encrypted backup. */
export function UnlockField({
  passphrase,
  onPassphrase,
  error,
}: {
  passphrase: string
  onPassphrase: (value: string) => void
  error?: string
}) {
  const { t } = useI18n()
  return (
    <Field label={t.backups.restore.passphrase} error={error}>
      <TextInput
        type="password"
        revealable
        value={passphrase}
        onValue={onPassphrase}
        autoComplete="off"
        invalid={error !== undefined}
        autoFocus
      />
    </Field>
  )
}

/** Sends a backup file of this computer. */
function Upload({ disabled, onFile }: { disabled: boolean; onFile: (file: File) => void }) {
  const { t } = useI18n()
  const text = t.backups.restore
  const input = useRef<HTMLInputElement>(null)
  return (
    <div className="border-t border-line pt-4">
      <input
        ref={input}
        type="file"
        accept={backupFileTypes}
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

/** The stored keys of a backup that POLYFIN_SECRET_KEY cannot read, by kind. */
export function unreadableKinds(t: Messages, plan: RestorePlan) {
  const text = t.backups.restore
  return Object.entries(plan.unreadable).filter(([, count]) => (count ?? 0) > 0) as [
    keyof typeof text.kinds,
    number,
  ][]
}

/** Tells which stored keys of a backup cannot be read, which a restore must then confirm. */
export function UnreadableKeys({ plan }: { plan: RestorePlan }) {
  const { t } = useI18n()
  const text = t.backups.restore
  const unreadable = unreadableKinds(t, plan)
  if (unreadable.length === 0) return null
  return (
    <Notice tone="warn">
      <strong className="block font-semibold">{text.keyTitle}</strong>
      {text.keyBody}
      <ul className="mt-1.5 list-disc pl-5">
        {unreadable.map(([kind, count]) => (
          <li key={kind}>{text.kinds[kind](count)}</li>
        ))}
      </ul>
    </Notice>
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
  const anyway = unreadableKinds(t, plan).length > 0
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
        <UnreadableKeys plan={plan} />
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
 * Watches Polyfin stop, restore and start again: `onBack` is called once it answered that it is
 * busy, then that it is ready again, or after a minute of a restore too quick to be seen down.
 */
export function useRestartWatch(onBack: () => void) {
  const callback = useRef(onBack)
  callback.current = onBack
  useEffect(() => {
    let down = false
    let done = false
    const started = Date.now()
    const timer = window.setInterval(() => {
      void fetch(`${import.meta.env.BASE_URL}../readyz`, { cache: 'no-store' })
        .then((response) => {
          if (!response.ok) down = true
          else if (!done && (down || Date.now() - started > 60_000)) {
            done = true
            window.clearInterval(timer)
            callback.current()
          }
        })
        .catch(() => {
          down = true
        })
    }, pollEvery)
    return () => window.clearInterval(timer)
  }, [])
}

/** While Polyfin stops, restores and starts again: the page reloads once it is back. */
function Restoring() {
  const { t } = useI18n()
  const text = t.backups.restore
  useRestartWatch(() => window.location.reload())
  return (
    <Modal open onClose={() => {}} title={text.restoringTitle} width={480}>
      <p className="flex items-center gap-3 p-5 text-small text-ink-2">
        <Spinner size={20} /> {text.restoringBody}
      </p>
    </Modal>
  )
}
