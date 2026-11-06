import type { Icon } from '@phosphor-icons/react'
import type { AnchorHTMLAttributes, ButtonHTMLAttributes, MouseEvent, ReactNode } from 'react'
import { Link, type LinkProps } from 'react-router'
import { cx, rem } from './cx'
import { Spinner } from './Spinner'

/**
 * Buttons: quiet until lit. Under the keyboard each one lights up pale with dark text and rises.
 * - `primary`: the one main action of a view (save, add), periwinkle with white text. One per view.
 * - `secondary`: other actions (replace, refresh, cancel in a form), a soft fill.
 * - `ghost`: quiet actions in rows and toolbars (disconnect, reset), no fill until hovered.
 * - `danger`: destructive actions, in red words; pair them with ConfirmDialog.
 */
export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'

/** `md` is 52 px tall, the default; `sm` is 44 px, for dense rows and docks. Both grow when a
 * label wraps on a phone. */
export type ButtonSize = 'md' | 'sm'

const variants: Record<ButtonVariant, string> = {
  primary: 'bg-accent text-on-accent hover:bg-accent-hover active:bg-accent-press',
  secondary: 'bg-s3 text-ink hover:bg-s4',
  ghost: 'text-ink-2 hover:bg-s1 hover:text-ink',
  danger: 'text-danger hover:bg-danger/14',
}

const sizes: Record<ButtonSize, string> = {
  md: 'min-h-13 gap-2.5 px-5.5 py-2 text-control',
  sm: 'min-h-11 gap-2 px-4 py-1.5 text-small',
}

/** The classes of a button, for the rare element that cannot be a Button or ButtonLink. */
export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'md'): string {
  return cx(
    // A label too long for a phone wraps inside the button rather than push the page sideways.
    'inline-flex max-w-full shrink-0 cursor-pointer items-center justify-center rounded-control text-center leading-tight font-[620] whitespace-nowrap select-none max-sm:whitespace-normal',
    'focus-lit [--lift:1.05] active:scale-[0.97] [&>svg]:shrink-0',
    'disabled:cursor-not-allowed disabled:opacity-45 disabled:active:scale-none',
    'aria-busy:cursor-progress',
    variants[variant],
    sizes[size],
  )
}

type CommonProps = {
  /** Look of the button; see ButtonVariant. Defaults to `secondary`. */
  variant?: ButtonVariant
  /** Height; defaults to `md`. */
  size?: ButtonSize
  /** A Phosphor icon before the label. */
  icon?: Icon
  /** A Phosphor icon after the label, such as an arrow. */
  iconEnd?: Icon
  children?: ReactNode
}

function Content({
  icon: Before,
  iconEnd: After,
  size,
  loading,
  children,
}: CommonProps & { loading?: boolean }) {
  const glyph = size === 'sm' ? 18 : 21
  return (
    <>
      {loading ? (
        <Spinner size={glyph} />
      ) : (
        Before && <Before size={rem(glyph)} aria-hidden="true" />
      )}
      {children}
      {After && <After size={rem(glyph)} aria-hidden="true" />}
    </>
  )
}

export type ButtonProps = CommonProps &
  ButtonHTMLAttributes<HTMLButtonElement> & {
    /**
     * Shows a spinner and ignores clicks while an action runs. The button stays focusable, so the
     * keyboard does not lose its place; pass a busy label as children if the wording changes.
     */
    loading?: boolean
  }

/** A button. `type` defaults to "button"; set `type="submit"` in forms. */
export function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  iconEnd,
  loading = false,
  type = 'button',
  className,
  onClick,
  children,
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      aria-busy={loading || undefined}
      className={cx(buttonClass(variant, size), className)}
      onClick={(event: MouseEvent<HTMLButtonElement>) => {
        if (loading) {
          event.preventDefault()
          return
        }
        onClick?.(event)
      }}
      {...rest}
    >
      <Content icon={icon} iconEnd={iconEnd} size={size} loading={loading}>
        {children}
      </Content>
    </button>
  )
}

export type ButtonLinkProps = CommonProps & LinkProps

/** A link to a page of the app that looks like a button. */
export function ButtonLink({
  variant = 'secondary',
  size = 'md',
  icon,
  iconEnd,
  className,
  children,
  ...rest
}: ButtonLinkProps) {
  return (
    <Link className={cx(buttonClass(variant, size), className as string)} {...rest}>
      <Content icon={icon} iconEnd={iconEnd} size={size}>
        {children}
      </Content>
    </Link>
  )
}

export type ExternalButtonLinkProps = CommonProps & AnchorHTMLAttributes<HTMLAnchorElement>

/** A link outside the app (the web player, a provider's site) that looks like a button. */
export function ExternalButtonLink({
  variant = 'secondary',
  size = 'md',
  icon,
  iconEnd,
  className,
  children,
  ...rest
}: ExternalButtonLinkProps) {
  return (
    <a className={cx(buttonClass(variant, size), className)} {...rest}>
      <Content icon={icon} iconEnd={iconEnd} size={size}>
        {children}
      </Content>
    </a>
  )
}

export type IconButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & {
  /** What the button does, read by screen readers and shown as a native tooltip. Required. */
  label: string
  /** The Phosphor icon. */
  icon: Icon
  /** `md` is 44 px, `sm` 36 px for rows. */
  size?: 'md' | 'sm'
  /** Red on hover, for removing. */
  danger?: boolean
  /** For toggles (show a secret, mute): sets `aria-pressed`. */
  pressed?: boolean
  /** Shows a spinner and ignores clicks, keeping the focus. */
  loading?: boolean
}

/** A square button with only an icon. Its `label` is mandatory: it is the accessible name. */
export function IconButton({
  label,
  icon: Glyph,
  size = 'md',
  danger = false,
  pressed,
  loading = false,
  type = 'button',
  className,
  onClick,
  ...rest
}: IconButtonProps) {
  return (
    <button
      type={type}
      aria-label={label}
      title={label}
      aria-pressed={pressed}
      aria-busy={loading || undefined}
      onClick={(event) => {
        if (!loading) onClick?.(event)
      }}
      className={cx(
        'inline-grid shrink-0 cursor-pointer place-items-center text-ink-2 focus-lit [--lift:1.06]',
        'hover:bg-s2 hover:text-ink aria-pressed:text-link disabled:cursor-not-allowed disabled:opacity-35 disabled:hover:bg-transparent disabled:hover:text-ink-2',
        size === 'md' ? 'size-11 rounded-control' : 'size-9 rounded-[0.875rem]',
        danger && 'hover:bg-danger/14 hover:text-danger',
        className,
      )}
      {...rest}
    >
      {loading ? (
        <Spinner size={size === 'md' ? 22 : 19} />
      ) : (
        <Glyph size={rem(size === 'md' ? 22 : 19)} aria-hidden="true" />
      )}
    </button>
  )
}
