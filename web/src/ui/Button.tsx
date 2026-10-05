import type { Icon } from '@phosphor-icons/react'
import type { AnchorHTMLAttributes, ButtonHTMLAttributes, MouseEvent, ReactNode } from 'react'
import { Link, type LinkProps } from 'react-router'
import { cx } from './cx'
import { Spinner } from './Spinner'

/**
 * Buttons.
 * - `primary`: the one main action of a view (save, add). One per view.
 * - `secondary`: other actions (replace, refresh, cancel in a form).
 * - `ghost`: quiet actions in rows and toolbars (disconnect, reset).
 * - `danger`: destructive actions; pair them with ConfirmDialog.
 */
export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'

/** `md` is 36 px tall, the default; `sm` is 30 px, for dense rows. */
export type ButtonSize = 'md' | 'sm'

const variants: Record<ButtonVariant, string> = {
  primary: 'bg-accent text-white hover:bg-accent-press',
  secondary: 'border border-line-2 bg-s3 text-ink hover:border-line-3 hover:bg-s4',
  ghost: 'text-ink-2 hover:bg-s2 hover:text-ink',
  danger: 'text-danger hover:bg-danger/8',
}

const sizes: Record<ButtonSize, string> = {
  md: 'h-9 px-3.5 text-control',
  sm: 'h-[30px] px-2.5 text-small',
}

/** The classes of a button, for the rare element that cannot be a Button or ButtonLink. */
export function buttonClass(variant: ButtonVariant = 'secondary', size: ButtonSize = 'md'): string {
  return cx(
    'inline-flex shrink-0 cursor-pointer items-center justify-center gap-2 rounded-field font-medium whitespace-nowrap select-none',
    'transition-[background-color,border-color,color,translate] duration-160 ease-nuit active:translate-y-px',
    'disabled:cursor-not-allowed disabled:opacity-45 disabled:active:translate-y-0',
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
  loading,
  children,
}: CommonProps & { loading?: boolean }) {
  return (
    <>
      {loading ? <Spinner /> : Before && <Before size={16} aria-hidden="true" />}
      {children}
      {After && <After size={16} aria-hidden="true" />}
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
      <Content icon={icon} iconEnd={iconEnd} loading={loading}>
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
      <Content icon={icon} iconEnd={iconEnd}>
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
      <Content icon={icon} iconEnd={iconEnd}>
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
  /** `md` is 36 px, `sm` 28 px for rows. */
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
        'inline-grid shrink-0 cursor-pointer place-items-center text-ink-2 transition-[background-color,color] duration-160 ease-nuit',
        'hover:bg-ink/8 hover:text-ink aria-pressed:text-link disabled:cursor-not-allowed disabled:opacity-35 disabled:hover:bg-transparent disabled:hover:text-ink-2',
        size === 'md' ? 'size-9 rounded-field' : 'size-7 rounded-md',
        danger && 'hover:bg-danger/10 hover:text-danger',
        className,
      )}
      {...rest}
    >
      {loading ? <Spinner /> : <Glyph size={size === 'md' ? 18 : 16} aria-hidden="true" />}
    </button>
  )
}
