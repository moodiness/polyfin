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
 * `PageLayout` provides. From 768 px up, a SaveBar moves into it; without it, the SaveBar draws
 * itself where it is, and on a phone it is a bar along the bottom (see SaveBar).
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
 * - On a phone it is a bar along the bottom; the form keeps room under its last row for it. The
 *   bar moves to the end of the page's `main` (or of its dialog): an ancestor's transform, such as
 *   the one an entrance animation leaves, would otherwise hold it at that ancestor's foot.
 *
 * Put it inside the `<form>`, once per page: its Save button submits that form, even from the
 * dock or the bar (`form` attribute).
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
  const [phoneHost, setPhoneHost] = useState<HTMLElement | null>(null)
  const shown = dirty || saving
  const docked = slot !== null && isWide
  const host = docked ? slot : isWide ? null : phoneHost

  // The dock may leave the form's DOM: its Save button names the form by id.
  useLayoutEffect(() => {
    const at = anchor.current
    setPhoneHost(at?.closest('dialog') ?? at?.closest('main') ?? document.body)
    const form = at?.closest('form')
    if (!form) return
    if (form.id === '') form.id = `form-${ownId.replace(/[^a-zA-Z0-9_-]/g, '')}`
    setFormId(form.id)
  }, [ownId])

  const count = changes === undefined ? t.ui.unsaved : t.ui.changes(changes)

  const discard = onDiscard && (
    <Button
      size="sm"
      onClick={onDiscard}
      disabled={saving}
      className={cx('max-md:ml-auto', docked ? 'md:w-full' : 'md:justify-self-start')}
    >
      {t.ui.discard}
    </Button>
  )

  const save = (
    <Button
      type="submit"
      form={host ? formId : undefined}
      variant="primary"
      size="sm"
      icon={CheckIcon}
      loading={saving}
      disabled={!shown}
      // When the bar wraps on a narrow phone, Save stays at the right.
      className={cx('max-md:ml-auto', docked && 'md:w-full')}
    >
      {saving ? (savingLabel ?? t.common.saving) : (saveLabel ?? t.common.save)}
    </Button>
  )

  const dock = (
    <div
      inert={!shown}
      className={cx(
        'z-30 rounded-panel raised transition-[translate,opacity] duration-500 ease-nuit motion-reduce:transition-none',
        // The slot of a split page takes no clicks: the shown dock takes its own.
        shown
          ? 'pointer-events-auto translate-y-0 opacity-100'
          : 'pointer-events-none translate-y-8 opacity-0',
        // A bar along the bottom of a phone.
        'max-md:fixed max-md:inset-x-2.5 max-md:bottom-2.5 max-md:flex max-md:flex-wrap max-md:items-center max-md:gap-1.5 max-md:rounded-[1.125rem] max-md:py-1.5 max-md:pr-1.5 max-md:pl-3.5',
        // A card at the end of the form: Discard and Save side by side. Docked under the section
        // list, the column is too narrow for both in most languages: one column, Save first and
        // full width, Discard under it.
        'md:gap-x-2 md:gap-y-2.5 md:px-2.5 md:pt-3 md:pb-2.5',
        docked ? 'md:grid-cols-1' : 'md:grid-cols-[1fr_auto]',
        docked || shown ? 'md:grid' : 'md:hidden',
        docked ? 'md:w-full' : 'md:sticky md:bottom-7 md:mt-8 md:ml-auto md:w-max md:min-w-[16rem]',
      )}
    >
      <div className="flex min-w-0 items-center gap-2.5 font-semibold text-control text-ink max-md:mr-auto max-md:gap-2 md:col-span-full md:pl-2">
        <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-link" />
        <span className="min-w-0 max-md:whitespace-nowrap">{count}</span>
      </div>
      {note !== undefined && (
        <p className="text-small text-ink-2 max-md:hidden md:col-span-full md:-mt-1.5 md:pl-6.5">
          {note}
        </p>
      )}
      {docked ? (
        <>
          {save}
          {discard}
        </>
      ) : (
        <>
          {discard ?? <span className="max-md:hidden" />}
          {save}
        </>
      )}
      {error && (
        <div className="basis-full pt-1 md:col-span-full md:px-2">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )

  return (
    <>
      <div ref={anchor} aria-hidden="true" className={cx(shown && 'max-md:h-[4.5rem]')} />
      {/* Says the count as it changes, and "All changes saved" once a save went through. */}
      <p aria-live="polite" className="sr-only">
        {shown ? count : t.ui.allSaved}
      </p>
      {host ? createPortal(dock, host) : dock}
    </>
  )
}
