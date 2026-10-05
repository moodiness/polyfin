import { CheckCircleIcon, InfoIcon, WarningCircleIcon, XIcon } from '@phosphor-icons/react'
import { createContext, use, useCallback, useRef, useState, type ReactNode } from 'react'
import { useI18n } from '@/i18n'
import { cx } from './cx'

export type ToastTone = 'ok' | 'info' | 'danger'

export type ToastOptions = {
  /** `ok` for a saved change (the default), `info` for a neutral note, `danger` for a failure. */
  tone?: ToastTone
  /** How long it stays, in ms; 4 s by default, 8 s for `danger`. */
  duration?: number
}

type ToastItem = { id: number; message: ReactNode; tone: ToastTone }

type ToastContextValue = (message: ReactNode, options?: ToastOptions) => void

const ToastContext = createContext<ToastContextValue | null>(null)

/**
 * Shows a quiet confirmation at the bottom of the screen: `toast(t.users.saved)`. Use it after
 * saving or for an action whose result is not visible where the user is; never `window.alert`.
 * Errors that need an answer stay inline, next to what failed.
 */
export function useToast(): ToastContextValue {
  const value = use(ToastContext)
  if (!value) throw new Error('useToast must be used inside ToastProvider')
  return value
}

const icons = { ok: CheckCircleIcon, info: InfoIcon, danger: WarningCircleIcon }
const iconColors = { ok: 'text-ok', info: 'text-link', danger: 'text-danger' }

/** Holds the toasts of the app; `main.tsx` mounts it once around the router. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const { t } = useI18n()
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const next = useRef(0)

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
  }, [])

  const show = useCallback<ToastContextValue>(
    (message, { tone = 'ok', duration } = {}) => {
      const id = ++next.current
      // Three at most: the oldest goes first.
      setToasts((current) => [...current.slice(-2), { id, message, tone }])
      window.setTimeout(() => dismiss(id), duration ?? (tone === 'danger' ? 8000 : 4000))
    },
    [dismiss],
  )

  return (
    <ToastContext value={show}>
      {children}
      <section
        aria-label={t.ui.notifications}
        aria-live="polite"
        className="pointer-events-none fixed inset-x-0 bottom-6 z-[60] flex flex-col items-center gap-2 px-4"
      >
        {toasts.map((toast) => {
          const Glyph = icons[toast.tone]
          return (
            <div
              key={toast.id}
              className="pointer-events-auto flex max-w-[min(520px,100%)] animate-rise items-center gap-3 rounded-row bg-s3 py-2.5 pr-2 pl-3.5 text-control text-ink shadow-pop"
            >
              <Glyph
                size={18}
                aria-hidden="true"
                className={cx('shrink-0', iconColors[toast.tone])}
              />
              <span className="min-w-0 flex-1">{toast.message}</span>
              <button
                type="button"
                aria-label={t.ui.dismiss}
                title={t.ui.dismiss}
                onClick={() => dismiss(toast.id)}
                className="inline-grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-ink-3 transition-colors duration-160 hover:bg-ink/8 hover:text-ink"
              >
                <XIcon size={14} aria-hidden="true" />
              </button>
            </div>
          )
        })}
      </section>
    </ToastContext>
  )
}
