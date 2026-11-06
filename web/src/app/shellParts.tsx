import { useHealthProblems } from '@/features/system/problems'
import { useI18n } from '@/i18n'
import { cx } from '@/ui'

/**
 * The column every signed-in page and the top row share: 1280 px at most, with 80 px margins,
 * 56 under 1400 px and 20 on a phone.
 */
export const pageFrame = 'mx-auto w-full max-w-[1440px] px-20 max-[1400px]:px-14 max-md:px-5'

/** The quiet squares and pills of the top row and the phone menu, lit when focused. */
export const topControl =
  'focus-lit inline-flex shrink-0 cursor-pointer items-center rounded-choice bg-s1 text-ink hover:bg-s2 [--lift:1.05]'

/**
 * How many things Health asks to look at, in red beside System: only for administrators, who
 * alone may read them, and only when there are some. The number is read out with its words.
 */
export function ProblemCount({ className }: { className?: string }) {
  const { t } = useI18n()
  const count = useHealthProblems().data?.length ?? 0
  if (count === 0) return null
  return (
    <>
      <span
        aria-hidden="true"
        className={cx(
          'figures ml-2 inline-grid h-6 min-w-6 place-items-center rounded-xl bg-danger/14 px-1.5 text-[15px] leading-none font-bold text-danger',
          className,
        )}
      >
        {count}
      </span>
      <span className="sr-only">, {t.nav.toLookAt(count)}</span>
    </>
  )
}
