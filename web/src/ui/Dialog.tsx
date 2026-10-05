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
      className="m-auto w-[min(440px,calc(100vw-24px))] rounded-panel bg-s1 p-0 text-ink shadow-pop backdrop:bg-scrim backdrop:backdrop-blur-[6px] open:animate-modal"
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

export type ModalProps = {
  open: boolean
  onClose: () => void
  /** The heading of the panel and its accessible name. */
  title: ReactNode
  /** Hides the title visually, when the content starts with its own header (the phone menu). */
  hideTitle?: boolean
  /** Width in px on screens wider than it; a phone gets the full width less a 12 px margin. */
  width?: number
  /** Buttons at the bottom (save, cancel). They stay put while the body scrolls. */
  footer?: ReactNode
  children: ReactNode
}

/**
 * A panel floating at the center of the page over a lightly blurred backdrop: the phone menu, a
 * form or an editor too large for a row. Modal like ConfirmDialog; the close button is always
 * there. The header and footer stay put while the body scrolls.
 */
export function Modal({
  open,
  onClose,
  title,
  hideTitle = false,
  width = 480,
  footer,
  children,
}: ModalProps) {
  const { t } = useI18n()
  const dialog = useModal(open, onClose)
  const titleId = useId()
  return createPortal(
    <dialog
      ref={dialog}
      aria-labelledby={titleId}
      style={{ width: `min(${width}px, calc(100vw - 24px))` }}
      className={cx(
        'm-auto max-h-[min(85dvh,calc(100dvh-24px))] max-w-none flex-col overflow-hidden rounded-panel bg-s1 p-0 text-ink shadow-pop open:flex open:animate-modal',
        'backdrop:bg-scrim backdrop:backdrop-blur-[6px]',
      )}
    >
      <div className="flex h-topbar shrink-0 items-center justify-between gap-3 border-b border-line px-5">
        <h2 id={titleId} className={cx('text-h3 text-ink', hideTitle && 'sr-only')}>
          {title}
        </h2>
        <IconButton label={t.ui.close} icon={XIcon} onClick={onClose} className="ml-auto" />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">{children}</div>
      {footer && (
        <div className="flex shrink-0 justify-end gap-2 border-t border-line bg-bg/40 px-5 py-3.5">
          {footer}
        </div>
      )}
    </dialog>,
    document.body,
  )
}
