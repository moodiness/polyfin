import { useHealthProblems } from '@/features/system/problems'
import { useI18n } from '@/i18n'
import { cx } from '@/ui'

/**
 * The column every signed-in page and the top row share: 1440 px at most at every density, margins
 * included, with margins of 14 steps, 10 under 1400 px and 5 on a phone.
 */
export const pageFrame = 'mx-auto w-full max-w-page px-14 max-[1400px]:px-10 max-md:px-5'

/** The quiet squares and pills of the top row and the phone menu, lit when focused. */
export const topControl =
  'focus-lit inline-flex shrink-0 cursor-pointer items-center rounded-choice bg-s1 text-ink hover:bg-s2 [--lift:1.05]'

/**
 * How many things Health asks to look at, and whether one of them is an error. Only administrators
 * may read them; a member reads none.
 */
function useProblemCount(): { count: number; error: boolean } {
  const problems = useHealthProblems().data ?? []
  return {
    count: problems.length,
    error: problems.some((problem) => problem.tone === 'error'),
  }
}

/**
 * How many things Health asks to look at, beside System: red when one of them is an error, amber
 * when they are all warnings, and only when there are some. The number is read out with its words.
 */
export function ProblemCount({ className }: { className?: string }) {
  const { t } = useI18n()
  const { count, error } = useProblemCount()
  if (count === 0) return null
  return (
    <>
      <span
        aria-hidden="true"
        className={cx(
          'figures ml-1.5 inline-grid h-5 min-w-5 place-items-center rounded-full px-1 text-micro leading-none font-bold',
          error ? 'bg-danger/14 text-danger' : 'bg-warn/14 text-warn',
          className,
        )}
      >
        {count}
      </span>
      <span className="sr-only">, {t.nav.toLookAt(count)}</span>
    </>
  )
}
