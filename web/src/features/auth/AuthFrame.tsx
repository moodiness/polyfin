import type { ReactNode } from 'react'
import { cx } from '@/ui'

/**
 * The frame of the pages shown before anyone is signed in: the logo, a heading and a lede, then a
 * panel holding the form. The public shell around it has the brand and the FR/EN choice.
 */
export function AuthFrame({
  title,
  lede,
  wide = false,
  children,
}: {
  title: string
  lede: string
  /** 520 px instead of 400, for the setup form. */
  wide?: boolean
  children: ReactNode
}) {
  return (
    <div className={cx('mx-auto w-full', wide ? 'max-w-[520px]' : 'max-w-[400px]')}>
      <img
        src={`${import.meta.env.BASE_URL}polyfin.svg`}
        alt=""
        width={48}
        height={48}
        className="mx-auto size-12"
      />
      <h1 className="mt-6 text-center text-h1 text-ink max-md:text-[26px]">{title}</h1>
      <p className="mx-auto mt-2 max-w-[44ch] text-center text-lead text-ink-2 max-md:text-[14px]">
        {lede}
      </p>
      <div className="mt-8 rounded-panel border border-line-2 bg-s1 p-6 max-sm:p-5">{children}</div>
    </div>
  )
}
