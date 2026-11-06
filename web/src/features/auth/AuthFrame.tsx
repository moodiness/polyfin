import type { ReactNode } from 'react'
import { cx } from '@/ui'

/** The form of a signed-out page: its wells 22 px apart. */
export const authForm = 'flex flex-col gap-[1.375rem]'

/** The big button of a signed-out page: as wide as the wells, 60 px tall. */
export const wideButton =
  'min-h-[3.75rem] w-full rounded-[1.25rem] text-[1.25rem] max-md:text-[1.125rem]'

/**
 * The frame of the pages shown before anyone is signed in: the logo, a big title and a lede, then
 * the form, if any, in big wells straight on the room, with no panel around it. The public shell
 * around it has the brand and the language at the top right.
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
        className="size-14 max-md:size-12"
      />
      <h1 className="mt-[1.625rem] text-[3.25rem] leading-[1.05] font-[680] tracking-[-0.02em] break-words text-ink max-md:mt-5 max-md:text-[2.5rem]">
        {title}
      </h1>
      <p className="mt-2.5 max-w-[46ch] text-[1.3125rem] text-ink-2 max-md:text-[1.125rem]">
        {lede}
      </p>
      {children !== undefined && <div className="mt-10 max-md:mt-8">{children}</div>}
    </div>
  )
}
