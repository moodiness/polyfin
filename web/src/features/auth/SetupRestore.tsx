import { ClockCounterClockwiseIcon, LockKeyIcon, UploadSimpleIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useRef, useState, type FormEvent } from 'react'
import {
  ApiError,
  fetchStatus,
  setupRestoreDiscard,
  setupRestoreStart,
  setupRestoreUnlock,
  setupRestoreUpload,
  type RestorePlan,
} from '@/api'
import { errorMessage, formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Field, Notice, Spinner, TextInput } from '@/ui'
import {
  backupFileTypes,
  failure,
  UnlockField,
  unreadableKinds,
  UnreadableKeys,
  useRestartWatch,
} from '@/features/system/RestoreBackup'

/**
 * "Restore a backup instead" on the setup page: the setup code and a backup file, the passphrase
 * of an encrypted one, then the restore, after which Polyfin starts again on the backup and the
 * page shows the sign-in. `onCancel` goes back to creating the first administrator.
 */
export default function SetupRestore({
  setupCode,
  onSetupCode,
  onCancel,
}: {
  setupCode: string
  onSetupCode: (code: string) => void
  onCancel: () => void
}) {
  const { language, t } = useI18n()
  const text = t.setup
  const input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [plan, setPlan] = useState<RestorePlan | null>(null)
  const [passphrase, setPassphrase] = useState('')
  const [restoring, setRestoring] = useState(false)
  const [failed, setFailed] = useState(false)
  const code = setupCode.trim()

  const send = useMutation({
    mutationFn: (chosen: File) => setupRestoreUpload(code, chosen),
    meta: { public: true },
    onSuccess: setPlan,
  })
  const unlock = useMutation({
    mutationFn: (id: string) => setupRestoreUnlock(code, id, passphrase),
    meta: { public: true },
    onSuccess: setPlan,
    // A damaged backup is dropped: it must be sent again.
    onError: (error) => {
      if (error instanceof ApiError && error.code !== 'wrong_passphrase') setPlan(null)
    },
  })
  const start = useMutation({
    mutationFn: (restore: RestorePlan) =>
      setupRestoreStart(code, restore.id, unreadableKinds(t, restore).length > 0),
    meta: { public: true },
    onSuccess: () => setRestoring(true),
  })

  function cancel() {
    if (plan !== null) void setupRestoreDiscard(code, plan.id).catch(() => undefined)
    onCancel()
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (plan === null) {
      if (file !== null) send.mutate(file)
    } else if (plan.locked) {
      if (passphrase !== '') unlock.mutate(plan.id)
    } else start.mutate(plan)
  }

  if (restoring) return <Restoring onFailed={() => setFailed(true)} failed={failed} />

  const wrong = unlock.error instanceof ApiError && unlock.error.code === 'wrong_passphrase'
  const error = [send, unlock, start].find((mutation) => mutation.isError)?.error
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <Field label={text.setupCode} help={text.setupCodeHint}>
        <TextInput
          value={setupCode}
          onValue={onSetupCode}
          autoComplete="off"
          autoCapitalize="characters"
          spellCheck={false}
          placeholder="XXXX-XXXX"
          mono
          disabled={plan !== null}
          autoFocus
          required
        />
      </Field>
      <Field label={text.restoreFile} help={text.restoreFileHelp}>
        <div className="flex flex-wrap items-center gap-3">
          <input
            ref={input}
            type="file"
            accept={backupFileTypes}
            className="sr-only"
            tabIndex={-1}
            onChange={(event) => {
              const chosen = event.target.files?.[0]
              event.target.value = ''
              if (chosen) {
                send.reset()
                setFile(chosen)
              }
            }}
          />
          <Button
            variant="secondary"
            icon={UploadSimpleIcon}
            disabled={plan !== null || send.isPending}
            onClick={() => input.current?.click()}
          >
            {text.restoreChoose}
          </Button>
          {file !== null && (
            <span className="min-w-0 text-small break-all text-ink-2">
              <code className="font-mono">{file.name}</code> · {formatBytes(file.size, language)}
            </span>
          )}
        </div>
      </Field>
      {plan?.locked && (
        <>
          <p className="text-small text-ink-2">{text.restoreUnlockHelp}</p>
          <UnlockField
            passphrase={passphrase}
            onPassphrase={(value) => {
              unlock.reset()
              setPassphrase(value)
            }}
            error={wrong ? errorMessage(t, unlock.error) : undefined}
          />
        </>
      )}
      {plan !== null && !plan.locked && (
        <div className="space-y-3">
          <Notice tone="ok">{text.restoreReady(plan.file)}</Notice>
          <ul className="list-disc space-y-1.5 pl-5 text-small text-ink-2">
            {text.restoreSteps.map((step) => (
              <li key={step}>{step}</li>
            ))}
          </ul>
          <UnreadableKeys plan={plan} />
        </div>
      )}
      {error !== undefined && !wrong && (
        <Notice tone="danger" live>
          {failure(t, error)}
        </Notice>
      )}
      {plan === null ? (
        <Button
          type="submit"
          variant="primary"
          icon={UploadSimpleIcon}
          loading={send.isPending}
          disabled={file === null || code === ''}
          className="w-full"
        >
          {send.isPending ? text.restoreSending : text.restoreSend}
        </Button>
      ) : plan.locked ? (
        <Button
          type="submit"
          variant="primary"
          icon={LockKeyIcon}
          loading={unlock.isPending}
          disabled={passphrase === ''}
          className="w-full"
        >
          {unlock.isPending ? t.backups.restore.unlocking : t.backups.restore.unlock}
        </Button>
      ) : (
        <Button
          type="submit"
          variant="danger"
          icon={ClockCounterClockwiseIcon}
          loading={start.isPending}
          className="w-full"
        >
          {unreadableKinds(t, plan).length > 0 ? t.backups.restore.anyway : text.restoreStart}
        </Button>
      )}
      <Button variant="ghost" onClick={cancel} className="w-full">
        {text.createInstead}
      </Button>
    </form>
  )
}

/**
 * While Polyfin restores the backup and starts again: once it answers, the page reloads on the
 * sign-in when an administrator exists, and tells the restore failed otherwise.
 */
function Restoring({ failed, onFailed }: { failed: boolean; onFailed: () => void }) {
  const { t } = useI18n()
  useRestartWatch(() => {
    void fetchStatus()
      .then((status) => {
        if (status.setupRequired) onFailed()
        else window.location.reload()
      })
      .catch(() => window.location.reload())
  })
  if (failed) {
    return (
      <div className="flex flex-col gap-4">
        <Notice tone="danger" live>
          {t.setup.restoreFailed}
        </Notice>
        <Button variant="secondary" onClick={() => window.location.reload()} className="w-full">
          {t.setup.createInstead}
        </Button>
      </div>
    )
  }
  return (
    <p className="flex items-center gap-3 text-small text-ink-2" aria-live="polite">
      <Spinner size={20} /> {t.setup.restoreRestoring}
    </p>
  )
}
