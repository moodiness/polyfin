import { CheckCircleIcon } from '@phosphor-icons/react'
import type { ReactNode } from 'react'
import { useI18n } from '@/i18n'
import { Button } from './Button'
import { cx } from './cx'
import { FieldError } from './Field'

/**
 * The bar that saves a whole form page (a settings section, a user): it sticks to the bottom of
 * the screen while there are changes, and says "All changes saved" otherwise. One per page.
 * Put it inside the `<form>`: its save button submits it.
 */
export function SaveBar({
  dirty,
  saving,
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
  /** Puts the form back as saved; the "Discard changes" button shows only with it. */
  onDiscard?: () => void
  /** A save error not tied to one field. Field errors go under their field. */
  error?: string
  /** Defaults to the shared "Save". */
  saveLabel?: string
  /** Defaults to the shared "Saving…". */
  savingLabel?: string
  /** A line on the left when there are changes: "Applied to the next playback." */
  note?: ReactNode
}) {
  const { t } = useI18n()
  return (
    <div
      className={cx(
        'sticky bottom-0 z-20 -mx-1 mt-10 px-1 pt-3 pb-4',
        dirty && 'bg-gradient-to-t from-bg via-bg/95 to-transparent',
      )}
    >
      <div
        className={cx(
          'flex flex-wrap items-center gap-3 rounded-row border px-4 py-3 transition-[background-color,border-color] duration-220 ease-nuit',
          dirty ? 'border-line-2 bg-s2 shadow-pop' : 'border-transparent',
        )}
      >
        <div aria-live="polite" className="mr-auto min-w-0 text-small">
          {dirty ? (
            <span className="font-medium text-warn">{note ?? t.ui.unsaved}</span>
          ) : (
            <span className="inline-flex items-center gap-2 text-ink-3">
              <CheckCircleIcon size={16} aria-hidden="true" className="text-ok" />
              {t.ui.allSaved}
            </span>
          )}
        </div>
        {dirty && onDiscard && (
          <Button variant="ghost" onClick={onDiscard} disabled={saving}>
            {t.ui.discard}
          </Button>
        )}
        <Button type="submit" variant="primary" loading={saving} disabled={!dirty && !saving}>
          {saving ? (savingLabel ?? t.common.saving) : (saveLabel ?? t.common.save)}
        </Button>
        {error && (
          <div className="basis-full">
            <FieldError>{error}</FieldError>
          </div>
        )}
      </div>
    </div>
  )
}
