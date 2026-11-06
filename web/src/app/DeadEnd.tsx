import { HouseIcon } from '@phosphor-icons/react'
import { Link } from 'react-router'
import { PageLayout } from './PageLayout'

/**
 * A page that leads nowhere (Not allowed, Page not found): the room, the 48 px title, what
 * happened, and one lit button back to Home.
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
          className="lit focus-lit inline-flex h-13 items-center gap-2.5 rounded-control px-5.5 text-control font-[620] whitespace-nowrap [--lift:1.05] hover:lift"
        >
          <HouseIcon size={21} aria-hidden="true" />
          {home}
        </Link>
      </div>
    </PageLayout>
  )
}
