import type { ReactNode } from 'react'
import { cx } from '@/ui'

/** The form of a signed-out page: its wells 16 px apart. */
export const authForm = 'flex flex-col gap-4'

/** The main button of a signed-out page: as wide as the wells, 44 px tall (37 in Compact). */
export const wideButton = 'min-h-11 w-full text-lead'

/**
 * The frame of the pages shown before anyone is signed in: the logo, the title (the greeting's
 * size) and a lede, then the form, if any, in wells straight on the room, with no panel around it.
 * The public shell around it has the brand and the language at the top right.
 */
export function AuthFrame({
  title,
  lede,
  wide = false,
  children,
}: {
  title: string
  lede: string
  /** 600 px instead of 520, for the setup key and the recovery codes. */
  wide?: boolean
  /** The form; without one, the page is its heading and lede. */
  children?: ReactNode
}) {
  return (
    <div
      className={cx(
        'mx-auto w-full pt-[min(9vh,6rem)] max-md:pt-2',
        wide ? 'max-w-[37.5rem]' : 'max-w-[32.5rem]',
      )}
    >
      <img
        src={`${import.meta.env.BASE_URL}polyfin.svg`}
        alt=""
        width={56}
        height={56}
        className="size-12 max-md:size-11"
      />
      <h1 className="mt-5 text-display break-words text-ink max-md:mt-4">{title}</h1>
      <p className="mt-2 max-w-[46ch] text-lead text-ink-2">{lede}</p>
      {children !== undefined && <div className="mt-8 max-md:mt-6">{children}</div>}
    </div>
  )
}
