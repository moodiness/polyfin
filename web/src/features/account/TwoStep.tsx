import { CheckIcon, CopyIcon, ShieldCheckIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import {
  ApiError,
  confirmOwnTwoStep,
  fetchOwnTwoStep,
  newOwnRecoveryCodes,
  queryClient,
  queryKeys,
  startOwnTwoStep,
  turnOffOwnTwoStep,
  type TwoStepConfirmation,
  type TwoStepSetup,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Field,
  InlineError,
  Modal,
  Notice,
  Segmented,
  SkeletonText,
  StatusPill,
  TextInput,
  useToast,
} from '@/ui'

/** The light modules around a QR code that scanners need, in modules. */
const quietZone = 4

/**
 * A QR code the server drew, a row a line of '1' for dark modules. It stays black on white,
 * whatever the theme: scanners read it so.
 */
function QrCode({ rows, label }: { rows: string[]; label: string }) {
  const size = rows.length + 2 * quietZone
  const path = rows
    .flatMap((row, y) =>
      [...row].map((module, x) =>
        module === '1' ? `M${x + quietZone} ${y + quietZone}h1v1h-1z` : '',
      ),
    )
    .join('')
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${size} ${size}`}
      shapeRendering="crispEdges"
      className="size-48 shrink-0 rounded-md"
    >
      <rect width={size} height={size} fill="white" />
      <path d={path} fill="black" />
    </svg>
  )
}

/**
 * What an authenticator app reads to add the account: the QR code, the key to type by hand, and
 * the otpauth:// address for an app on this device.
 */
export function SetupKey({ setup }: { setup: TwoStepSetup }) {
  const { t } = useI18n()
  const text = t.account.twoStep
  return (
    <div className="flex flex-wrap items-start gap-5">
      <QrCode rows={setup.qr} label={text.qrLabel} />
      <div className="min-w-0 flex-1 basis-56 space-y-3">
        <p className="max-w-[60ch] text-small text-ink-2">{text.scan}</p>
        <div>
          <p className="text-small text-ink-3">{text.key}</p>
          <code className="figures block font-mono text-control tracking-wider break-all text-ink select-all">
            {setup.secret.match(/.{1,4}/g)?.join(' ')}
          </code>
        </div>
        <a
          href={setup.uri}
          className="block text-small font-medium text-link transition-colors duration-160 hover:text-link-hover"
        >
          {text.openInApp}
        </a>
      </div>
    </div>
  )
}

/** The recovery codes, shown once, to copy before going on. */
export function RecoveryCodes({
  codes,
  onDone,
  doneLabel,
}: {
  codes: string[]
  onDone: () => void
  doneLabel: string
}) {
  const { t } = useI18n()
  const text = t.account.twoStep
  const [copy, setCopy] = useState<'idle' | 'copied' | 'failed'>('idle')

  async function copyCodes() {
    try {
      // The clipboard is missing outside secure contexts, such as plain http on a local network.
      await navigator.clipboard.writeText(codes.join('\n'))
      setCopy('copied')
    } catch {
      setCopy('failed')
    }
  }

  return (
    <div className="space-y-4">
      <Notice tone="warn">{text.recoveryHelp}</Notice>
      {/* Two columns where a code fits whole in each, one on a narrow screen. */}
      <ul className="figures grid max-w-md grid-cols-[repeat(auto-fit,minmax(19ch,1fr))] gap-x-6 gap-y-1.5 rounded-field border border-line-2 bg-bg px-4 py-3 font-mono text-control whitespace-nowrap text-ink select-all">
        {codes.map((code) => (
          <li key={code}>{code}</li>
        ))}
      </ul>
      <div aria-live="polite">
        {copy === 'copied' && <Notice tone="ok">{text.copied}</Notice>}
        {copy === 'failed' && <Notice tone="danger">{text.copyFailed}</Notice>}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button icon={copy === 'copied' ? CheckIcon : CopyIcon} onClick={() => void copyCodes()}>
          {text.copy}
        </Button>
        <Button variant="primary" onClick={onDone}>
          {doneLabel}
        </Button>
      </div>
    </div>
  )
}

/**
 * My account › Two-step sign-in: turning it on with a code of the app, the recovery codes left,
 * new ones, and turning it off, both confirmed with a code or the password.
 */
export default function TwoStep() {
  const { t } = useI18n()
  const text = t.account.twoStep
  const toast = useToast()
  const state = useQuery({
    queryKey: queryKeys.ownTwoStep,
    queryFn: ({ signal }) => fetchOwnTwoStep(signal),
  })
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[] | null>(null)
  const [asking, setAsking] = useState<'off' | 'codes' | null>(null)
  const refresh = () => void queryClient.invalidateQueries({ queryKey: queryKeys.ownTwoStep })

  const start = useMutation({ mutationFn: startOwnTwoStep, onSuccess: () => setCode('') })
  const confirm = useMutation({
    mutationFn: confirmOwnTwoStep,
    onSuccess: (answer) => {
      start.reset()
      setCodes(answer.recoveryCodes)
      toast(text.turnedOn, { tone: 'ok' })
      refresh()
    },
  })

  if (state.isPending) return <SkeletonText lines={2} />
  if (state.isError) {
    return (
      <InlineError onRetry={() => void state.refetch()} retrying={state.isFetching}>
        {errorMessage(t, state.error)}
      </InlineError>
    )
  }
  const current = state.data

  if (codes !== null) {
    return (
      <div className="space-y-3">
        <h3 className="text-lead font-semibold text-ink">{text.recoveryTitle}</h3>
        <RecoveryCodes codes={codes} onDone={() => setCodes(null)} doneLabel={text.saved} />
      </div>
    )
  }

  if (start.data !== undefined) {
    const codeError = confirm.isError ? errorMessage(t, confirm.error) : undefined
    return (
      <form
        noValidate
        className="space-y-5"
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault()
          confirm.mutate(code.trim())
        }}
      >
        <SetupKey setup={start.data} />
        <Field label={text.code} help={text.codeHelp} error={codeError} className="max-w-xs">
          <TextInput
            value={code}
            onValue={(value) => {
              confirm.reset()
              setCode(value)
            }}
            inputMode="numeric"
            autoComplete="one-time-code"
            mono
            required
            invalid={codeError !== undefined}
          />
        </Field>
        <div className="flex gap-2">
          <Button
            type="submit"
            variant="primary"
            loading={confirm.isPending}
            disabled={code.trim() === ''}
          >
            {confirm.isPending ? text.confirming : text.confirm}
          </Button>
          <Button variant="ghost" onClick={() => start.reset()}>
            {t.common.cancel}
          </Button>
        </div>
      </form>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        {current.on ? (
          <StatusPill tone="ok" icon={ShieldCheckIcon}>
            {text.on}
          </StatusPill>
        ) : (
          <StatusPill tone="muted">{text.off}</StatusPill>
        )}
        <span className="text-small text-ink-2">
          {current.on ? text.onText : text.offText}
          {current.on && ` ${text.codesLeft(current.recoveryCodes)}.`}
        </span>
      </div>
      {current.required && (
        <Notice tone={current.on ? 'info' : 'warn'}>
          {current.on ? text.requiredOn : text.required}
        </Notice>
      )}
      {current.unreadable && <Notice tone="danger">{text.unreadable}</Notice>}
      {start.isError && <Notice tone="danger">{errorMessage(t, start.error)}</Notice>}
      <div className="flex flex-wrap gap-2">
        {current.on ? (
          <>
            <Button onClick={() => setAsking('codes')}>{text.newCodes}</Button>
            {!current.required && (
              <Button variant="danger" onClick={() => setAsking('off')}>
                {text.turnOff}
              </Button>
            )}
          </>
        ) : (
          <Button variant="primary" loading={start.isPending} onClick={() => start.mutate()}>
            {start.isPending ? text.starting : text.turnOn}
          </Button>
        )}
      </div>
      {asking !== null && (
        <ConfirmChange
          change={asking}
          onClose={() => setAsking(null)}
          onDone={(answer) => {
            setAsking(null)
            refresh()
            if (answer === undefined) toast(text.turnedOff)
            else setCodes(answer)
          }}
        />
      )}
    </div>
  )
}

/**
 * Asks a code of the app, a recovery code, or the password before turning two-step sign-in off or
 * making new recovery codes; `onDone` gets the new codes, if any.
 */
function ConfirmChange({
  change,
  onClose,
  onDone,
}: {
  change: 'off' | 'codes'
  onClose: () => void
  onDone: (codes: string[] | undefined) => void
}) {
  const { t } = useI18n()
  const text = t.account.twoStep
  const [method, setMethod] = useState<'code' | 'password'>('code')
  const [value, setValue] = useState('')
  const mutation = useMutation({
    mutationFn: async (confirmation: TwoStepConfirmation) =>
      change === 'off'
        ? void (await turnOffOwnTwoStep(confirmation))
        : (await newOwnRecoveryCodes(confirmation)).recoveryCodes,
    onSuccess: onDone,
  })
  const failure = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const fieldError =
    mutation.error instanceof ApiError &&
    ['invalid_code', 'wrong_password'].includes(mutation.error.code)
      ? failure
      : undefined
  const formId = 'two-step-confirmation'

  return (
    <Modal
      open
      onClose={onClose}
      title={change === 'off' ? text.turnOffTitle : text.newCodesTitle}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant={change === 'off' ? 'danger' : 'primary'}
            loading={mutation.isPending}
            disabled={value === ''}
          >
            {change === 'off'
              ? mutation.isPending
                ? text.turningOff
                : text.turnOff
              : mutation.isPending
                ? text.making
                : text.newCodes}
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
          {change === 'off' ? text.turnOffText : text.newCodesText}
        </p>
        <Segmented
          label={text.confirmWith}
          options={[
            { value: 'code', label: text.withCode },
            { value: 'password', label: text.withPassword },
          ]}
          value={method}
          onChange={(next) => {
            mutation.reset()
            setValue('')
            setMethod(next)
          }}
        />
        <Field label={method === 'code' ? text.codeOrRecovery : text.password} error={fieldError}>
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
