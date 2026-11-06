import { CheckIcon } from '@phosphor-icons/react'
import {
  createContext,
  use,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { useI18n } from '@/i18n'
import { Button } from './Button'
import { cx } from './cx'
import { FieldError } from './Field'

/**
 * The place of the unsaved dock on a split page: an element under the section list, which
 * `PageLayout` provides. From 768 px up, a SaveBar moves into it; without it, or on a phone, the
 * SaveBar draws itself where it is.
 */
export const DockSlotContext = createContext<HTMLElement | null>(null)

const wide = '(min-width: 48rem)'

function subscribeWide(onChange: () => void) {
  const query = matchMedia(wide)
  query.addEventListener('change', onChange)
  return () => query.removeEventListener('change', onChange)
}

/**
 * The unsaved dock of a form page (a settings section, a user, a source's options): "2 changes",
 * Discard changes and Save. It shows while there are changes and stays out of the way otherwise.
 * - On a split page it docks under the section list (see DockSlotContext), clear of every row.
 * - Elsewhere it sticks to the bottom of the form, at its end, so the last row scrolls clear.
 * - On a phone it is a bar along the bottom; the form keeps room under its last row for it.
 *
 * Put it inside the `<form>`, once per page: its Save button submits that form, even from the
 * dock (`form` attribute).
 */
export function SaveBar({
  dirty,
  saving,
  changes,
  onDiscard,
  error,
  saveLabel,
  savingLabel,
  note,
}: {
  /** Whether the form differs from what is saved. */
  dirty: boolean
  /** While the save runs. */
  saving: boolean
  /** How many settings differ from what is saved: "2 changes". Without it, "Unsaved changes". */
  changes?: number
  /** Puts the form back as saved; the "Discard changes" button shows only with it. */
  onDiscard?: () => void
  /** A save error not tied to one setting. Setting errors go under their row. */
  error?: string
  /** Defaults to the shared "Save". */
  saveLabel?: string
  /** Defaults to the shared "Saving…". */
  savingLabel?: string
  /** A line under the count while there are changes: "Applied to the next playback." */
  note?: ReactNode
}) {
  const { t } = useI18n()
  const slot = use(DockSlotContext)
  const anchor = useRef<HTMLDivElement>(null)
  const isWide = useSyncExternalStore(subscribeWide, () => matchMedia(wide).matches)
  const ownId = useId()
  const [formId, setFormId] = useState<string | undefined>(undefined)
  const shown = dirty || saving
  const docked = slot !== null && isWide

  // The dock may leave the form's DOM for the slot: its Save button names the form by id.
  useLayoutEffect(() => {
    const form = anchor.current?.closest('form')
    if (!form) return
    if (form.id === '') form.id = `form-${ownId.replace(/[^a-zA-Z0-9_-]/g, '')}`
    setFormId(form.id)
  }, [ownId])

  const count = changes === undefined ? t.ui.unsaved : t.ui.changes(changes)

  const dock = (
    <div
      inert={!shown}
      className={cx(
        'z-30 rounded-panel raised transition-[translate,opacity] duration-500 ease-nuit motion-reduce:transition-none',
        shown ? 'translate-y-0 opacity-100' : 'pointer-events-none translate-y-8 opacity-0',
        // A bar along the bottom of a phone.
        'max-md:fixed max-md:inset-x-2.5 max-md:bottom-2.5 max-md:flex max-md:flex-wrap max-md:items-center max-md:gap-1.5 max-md:rounded-[24px] max-md:py-2 max-md:pr-2 max-md:pl-4',
        // A card under the section list, or at the end of the form.
        'md:grid md:grid-cols-[1fr_auto] md:gap-x-2 md:gap-y-3.5 md:px-3.5 md:pt-[18px] md:pb-3.5',
        docked
          ? 'md:w-full'
          : 'md:sticky md:bottom-7 md:mt-10 md:ml-auto md:w-max md:min-w-[300px]',
      )}
    >
      <div className="flex min-w-0 items-center gap-3 font-semibold text-ink max-md:mr-auto max-md:gap-2 max-md:text-[16px] md:col-span-full md:pl-2.5 md:text-body">
        <span aria-hidden="true" className="size-2.5 shrink-0 rounded-full bg-link" />
        <span className="min-w-0 max-md:whitespace-nowrap">{count}</span>
      </div>
      {note !== undefined && (
        <p className="text-small text-ink-2 max-md:hidden md:col-span-full md:-mt-2 md:pl-[34px]">
          {note}
        </p>
      )}
      {onDiscard ? (
        <Button
          size="sm"
          onClick={onDiscard}
          disabled={saving}
          className="max-md:ml-auto md:justify-self-start"
        >
          {t.ui.discard}
        </Button>
      ) : (
        <span className="max-md:hidden" />
      )}
      <Button
        type="submit"
        form={docked ? formId : undefined}
        variant="primary"
        size="sm"
        icon={CheckIcon}
        loading={saving}
        disabled={!shown}
        // When the bar wraps on a narrow phone, Save stays at the right.
        className="max-md:ml-auto"
      >
        {saving ? (savingLabel ?? t.common.saving) : (saveLabel ?? t.common.save)}
      </Button>
      {error && (
        <div className="basis-full pt-1 md:col-span-full md:px-2.5">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )

  return (
    <>
      <div ref={anchor} aria-hidden="true" className={cx(shown && 'max-md:h-24')} />
      {/* Says the count as it changes, and "All changes saved" once a save went through. */}
      <p aria-live="polite" className="sr-only">
        {shown ? count : t.ui.allSaved}
      </p>
      {docked ? createPortal(dock, slot) : dock}
    </>
  )
}
