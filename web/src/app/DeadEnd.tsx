import { HouseIcon } from '@phosphor-icons/react'
import { Link } from 'react-router'
import { PageLayout } from './PageLayout'

/**
 * A page that leads nowhere (Not allowed, Page not found): the room, the 48 px title, what
 * happened, and one large button back to Home. It lights up under the pointer or the keyboard
 * only, as every control does: on arrival, nothing is lit yet.
 */
export function DeadEnd({
  title,
  description,
  home,
}: {
  title: string
  description: string
  /** The button's words: "Back to Home". */
  home: string
}) {
  return (
    <PageLayout title={title} lede={description} className="pt-10 max-md:pt-4">
      <div>
        <Link
          to="/"
          className="focus-lit inline-flex h-13 items-center gap-2.5 rounded-control bg-s3 px-5.5 text-control font-[620] whitespace-nowrap text-ink [--lift:1.05] hover:lit hover:lift"
        >
          <HouseIcon size="1.3125rem" aria-hidden="true" />
          {home}
        </Link>
      </div>
    </PageLayout>
  )
}
