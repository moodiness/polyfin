import { XIcon } from '@phosphor-icons/react'
import { useEffect, useId, useRef, type ReactNode, type RefObject } from 'react'
import { createPortal } from 'react-dom'
import { useI18n } from '@/i18n'
import { Button, IconButton } from './Button'
import { cx } from './cx'
import { FieldError } from './Field'

/**
 * Opens a native `<dialog>` as a modal while `open` is true: the page behind is inert, the focus
 * stays inside, and Escape or a click on the backdrop calls `onClose`. The focus returns to where
 * it was when it closes.
 */
function useModal(open: boolean, onClose: () => void): RefObject<HTMLDialogElement | null> {
  const dialog = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const element = dialog.current
    if (!element) return
    if (open && !element.open) element.showModal()
    if (!open && element.open) element.close()
  }, [open])
  useEffect(() => {
    const element = dialog.current
    if (!element) return
    function onCancel(event: Event) {
      event.preventDefault()
      onClose()
    }
    function onClick(event: MouseEvent) {
      if (event.target === element) onClose()
    }
    element.addEventListener('cancel', onCancel)
    element.addEventListener('click', onClick)
    return () => {
      element.removeEventListener('cancel', onCancel)
      element.removeEventListener('click', onClick)
    }
  }, [onClose])
  return dialog
}

export type ConfirmDialogProps = {
  open: boolean
  /** Called on cancel, Escape, a click outside, and after nothing else: close it here. */
  onClose: () => void
  /** Called on the confirm button. Close the dialog once the action succeeds. */
  onConfirm: () => void
  /** A question naming the thing: "Delete the user sam?" */
  title: ReactNode
  /** What happens, in plain words: "Their devices are signed out. This cannot be undone." */
  children?: ReactNode
  /** The action, as a verb: "Delete". */
  confirmLabel: string
  /** Defaults to the shared "Cancel". */
  cancelLabel?: string
  /** `danger` (red button) for destructive actions, `primary` otherwise. */
  tone?: 'danger' | 'primary'
  /** While the action runs: the confirm button spins and Escape still cancels. */
  busy?: boolean
  /** The error of the action, shown in the dialog so it can be tried again. */
  error?: string
}

/**
 * Asks before an action that cannot be undone or that disconnects someone. The focus starts on
 * Cancel, the safe choice.
 */
export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  children,
  confirmLabel,
  cancelLabel,
  tone = 'danger',
  busy = false,
  error,
}: ConfirmDialogProps) {
  const { t } = useI18n()
  const dialog = useModal(open, onClose)
  const titleId = useId()
  const bodyId = useId()
  // In the body, wherever it is declared: the page's spacing and entrance motion stay off it, and
  // a list or a table never holds a dialog.
  return createPortal(
    <dialog
      ref={dialog}
      aria-labelledby={titleId}
      aria-describedby={children ? bodyId : undefined}
      className="m-auto w-[min(440px,calc(100vw-32px))] rounded-panel bg-s2 p-0 text-ink shadow-pop backdrop:bg-scrim backdrop:backdrop-blur-[3px] open:animate-pop"
    >
      <div className="p-6">
        <h2 id={titleId} className="text-h3 text-ink">
          {title}
        </h2>
        {children && (
          <div id={bodyId} className="mt-2 text-control text-ink-2">
            {children}
          </div>
        )}
        {error && (
          <div className="mt-4">
            <FieldError>{error}</FieldError>
          </div>
        )}
      </div>
      <div className="flex justify-end gap-2 border-t border-line bg-bg/40 px-6 py-3.5">
        <Button variant="ghost" onClick={onClose} autoFocus>
          {cancelLabel ?? t.common.cancel}
        </Button>
        <Button
          variant={tone === 'danger' ? 'danger' : 'primary'}
          className={tone === 'danger' ? 'border border-danger/40 bg-danger/10' : undefined}
          loading={busy}
          onClick={onConfirm}
        >
          {confirmLabel}
        </Button>
      </div>
    </dialog>,
    document.body,
  )
}

export type DrawerProps = {
  open: boolean
  onClose: () => void
  /** The heading of the drawer and its accessible name. */
  title: ReactNode
  /** Hides the title visually, when the content starts with its own header (the phone menu). */
  hideTitle?: boolean
  /** The side it slides from. */
  side?: 'right' | 'left'
  /** Width in px on screens wider than it; a phone gets the full width less a margin. */
  width?: number
  /** Buttons at the bottom (save, cancel). */
  footer?: ReactNode
  children: ReactNode
}

/**
 * A panel sliding over the page from a side: the phone menu, an editor too large for a row
 * (a channel, a user's devices). Modal like ConfirmDialog; the close button is always there.
 */
export function Drawer({
  open,
  onClose,
  title,
  hideTitle = false,
  side = 'right',
  width = 420,
  footer,
  children,
}: DrawerProps) {
  const { t } = useI18n()
  const dialog = useModal(open, onClose)
  const titleId = useId()
  return createPortal(
    <dialog
      ref={dialog}
      aria-labelledby={titleId}
      style={{ width: `min(${width}px, calc(100vw - 32px))` }}
      className={cx(
        'm-0 h-dvh max-h-dvh max-w-none flex-col border-line-2 bg-s1 p-0 text-ink shadow-pop open:flex',
        'backdrop:bg-scrim backdrop:backdrop-blur-[3px]',
        side === 'right'
          ? 'ml-auto border-l open:animate-drawer-right'
          : 'mr-auto border-r open:animate-drawer-left',
      )}
    >
      <div className="flex h-topbar shrink-0 items-center justify-between gap-3 border-b border-line px-5">
        <h2 id={titleId} className={cx('text-h3 text-ink', hideTitle && 'sr-only')}>
          {title}
        </h2>
        <IconButton label={t.ui.close} icon={XIcon} onClick={onClose} className="ml-auto" />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
      {footer && (
        <div className="flex shrink-0 justify-end gap-2 border-t border-line px-5 py-3.5">
          {footer}
        </div>
      )}
    </dialog>,
    document.body,
  )
}
