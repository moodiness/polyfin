import { ArrowLeftIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { createContext, use, useEffect, useId, useSyncExternalStore, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router'
import { fetchLiveSessions, queryKeys, type LiveSession } from '@/api'
import { formatClock } from '@/format'
import { useI18n } from '@/i18n'
import { Row, RowList, SectionNav } from '@/ui'
import { useSectionInView } from './shared'

/**
 * On a phone, the one section a person's or a group's page shows: its id, '' for none (the list
 * of sections), or null for every one, as on a wider screen.
 */
const ShownSection = createContext<string | null>(null)

/**
 * One section of a user's or a group's page: its title (25 px), an optional line under it, then
 * its rows 8 px apart. `id` is the target of the section list's anchor. On a phone, it hides while
 * another section is open (see useSectionPages), staying drawn so that its changes wait.
 */
export function PageSection({
  id,
  title,
  description,
  aside,
  children,
}: {
  id: string
  title: string
  /** What the section is for, or how it saves, under the title. */
  description?: ReactNode
  /** On the right of the title: a button, or where its values come from. */
  aside?: ReactNode
  children: ReactNode
}) {
  const shown = use(ShownSection)
  return (
    <section
      id={id}
      aria-labelledby={`${id}-title`}
      hidden={shown !== null && shown !== id}
      className="scroll-mt-8 max-md:scroll-mt-20"
    >
      <div className="mb-3 flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <h2 id={`${id}-title`} className="text-h2 break-words text-ink">
            {title}
          </h2>
          {description !== undefined && (
            <p className="mt-1 max-w-[72ch] text-small text-ink-2">{description}</p>
          )}
        </div>
        {aside}
      </div>
      <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">{children}</div>
    </section>
  )
}

/** A section in the lists of a page's sections, with what to say beside it on a phone. */
type SectionEntry = { id: string; label: string; note?: string }

/** The sections of a page under a heading word ("Limits"). */
export type SectionGroup = { id: string; label: string; items: readonly SectionEntry[] }

/** Below 768 px, where the section list of a split page would be a strip. */
const phoneQuery = '(max-width: 767.98px)'

function subscribePhone(onChange: () => void) {
  const media = window.matchMedia(phoneQuery)
  media.addEventListener('change', onChange)
  return () => media.removeEventListener('change', onChange)
}

/**
 * The sections of a person's or a group's page, and how they show: PageLayout's `head` and `nav`,
 * and `shown`, for a `ShownSections` around the sections.
 *
 * Wider than a phone, the band, then the sections one after the other beside a SectionNav of
 * anchors under their heading words; an address with a #section scrolls to it. On a phone, a long
 * page of every section would be many screens tall, so each is a page of its own: without a
 * #section the band and the list of the sections, each with its note; with one, that section
 * alone under a link back to the list. Every section stays drawn, hidden, so that what one holds
 * unsaved waits in the dock while another is open.
 */
export function useSectionPages({
  label,
  name,
  groups,
  band,
}: {
  /** The accessible name of the lists: "Sections of the user". */
  label: string
  /** The person's or the group's name, which the link back to the list reads on a phone. */
  name: string
  groups: readonly SectionGroup[]
  band: ReactNode
}): { head: ReactNode; nav: ReactNode; shown: string | null } {
  const phone = useSyncExternalStore(subscribePhone, () => window.matchMedia(phoneQuery).matches)
  const { pathname, hash } = useLocation()
  const ids = groups.flatMap((group) => group.items.map((item) => item.id))
  const current = useSectionInView(ids, !phone)
  const open = ids.find((id) => `#${id}` === hash) ?? ''

  // The content arrives after the first render: a #section address scrolls once it is drawn. On
  // a phone each section is a page, which opens at its top.
  useEffect(() => {
    if (phone) window.scrollTo(0, 0)
    else if (hash) document.getElementById(hash.slice(1))?.scrollIntoView({ block: 'start' })
  }, [hash, phone])

  if (!phone) {
    return {
      head: band,
      nav: (
        <SectionNav
          label={label}
          current={current}
          items={groups.map((group) => ({
            ...group,
            items: group.items.map((item) => ({
              id: item.id,
              label: item.label,
              to: `#${item.id}`,
            })),
          }))}
        />
      ),
      shown: null,
    }
  }
  if (open === '') {
    return {
      head: band,
      nav: (
        <nav aria-label={label} className="space-y-6">
          {groups.map((group) => (
            <div key={group.id}>
              <h2 className="mb-2 px-1 text-small font-semibold text-ink-2">{group.label}</h2>
              <RowList aria-label={group.label}>
                {group.items.map((item) => (
                  <Row key={item.id} to={`#${item.id}`} title={item.label} meta={item.note} />
                ))}
              </RowList>
            </div>
          ))}
        </nav>
      ),
      shown: '',
    }
  }
  return {
    head: (
      <header>
        <BackLink to={pathname} label={name} />
        <h1 className="sr-only">{name}</h1>
      </header>
    ),
    // Never empty: the page keeps its split, so that the sections are not drawn anew.
    nav: <></>,
    shown: open,
  }
}

/** Around the sections of a page of useSectionPages: on a phone, shows only the open one. */
export function ShownSections({ shown, children }: { shown: string | null; children: ReactNode }) {
  return <ShownSection value={shown}>{children}</ShownSection>
}

/** The link back above a page's band: "← Users". */
function BackLink({ to, label }: { to: string; label: string }) {
  return (
    <Link
      to={to}
      className="group focus-lit mb-2 -ml-2.5 inline-flex items-center gap-1.5 rounded-control px-2.5 py-1 text-small font-medium text-ink-2 hover:bg-s1 hover:text-ink"
    >
      <ArrowLeftIcon
        size="1rem"
        aria-hidden="true"
        className="transition-transform duration-300 ease-nuit group-hover:-translate-x-0.5"
      />
      {label}
    </Link>
  )
}

/**
 * The head of a person's or a group's page: the link back to the list, then a tile with the
 * avatar or icon, the name (the h1), its facts, what they watch now, and the page's actions. With
 * `art`, the tile takes the colour of that poster, blurred under a scrim.
 */
export function Band({
  back,
  leading,
  title,
  facts,
  extra,
  actions,
  art,
}: {
  back: { to: string; label: string }
  leading: ReactNode
  title: string
  facts: ReactNode
  extra?: ReactNode
  actions?: ReactNode
  art?: string | null
}) {
  const titleId = useId()
  return (
    <header className="mb-7 max-md:mb-6">
      <BackLink to={back.to} label={back.label} />
      <section
        aria-labelledby={titleId}
        className="relative isolate grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-5 overflow-hidden rounded-panel bg-s1 px-5 py-4 max-md:items-start max-md:gap-x-3.5 max-md:gap-y-3 max-md:px-4"
      >
        {art && (
          <>
            <img
              src={art}
              alt=""
              loading="lazy"
              className="absolute -inset-[60px] -z-20 size-[calc(100%+120px)] max-w-none object-cover opacity-55 blur-[60px] brightness-[0.55] saturate-[1.2]"
            />
            <div
              aria-hidden="true"
              className="absolute inset-0 -z-10 bg-linear-to-r from-room/90 from-30% to-room/55"
            />
          </>
        )}
        {leading}
        <div className="min-w-0">
          <h1 id={titleId} className="text-h1 break-words text-ink">
            {title}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-x-3.5 gap-y-1 text-control text-ink-2">
            {facts}
          </div>
          {extra}
        </div>
        {/* The page's "…" menu, at the top right at every width. */}
        {actions && <div className="flex gap-2 self-start">{actions}</div>}
      </section>
    </header>
  )
}

/** A poster through Polyfin, as apps load it, at a height in pixels. */
export function posterUrl(posterId: string, height: number) {
  return `/Items/${encodeURIComponent(posterId)}/Images/Primary?maxHeight=${height}&quality=85`
}

/**
 * What the user plays now, from the playbacks Home's Now playing reads: the first that is theirs,
 * leaving out downloads being converted and guests of their share links. A member is answered
 * their own playbacks only.
 */
export function useWatching(userId: string): LiveSession | null {
  const sessions = useQuery({
    queryKey: queryKeys.liveSessions,
    queryFn: ({ signal }) => fetchLiveSessions(signal),
    refetchInterval: 10_000,
  })
  return (
    sessions.data?.find(
      (session) =>
        session.user.id === userId && session.item !== null && !session.download && !session.shared,
    ) ?? null
  )
}

/**
 * What a person watches now, in their band: the poster, "Watching …" or "Paused on …", the device
 * and the position, as a link to Home's Now playing. `art` is the poster's id, null without one.
 */
export function WatchingLink({
  session,
  art,
  onArtError,
}: {
  session: LiveSession
  art: string | null
  onArtError: () => void
}) {
  const { t } = useI18n()
  const item = session.item
  if (item === null) return null
  return (
    <Link
      to="/"
      className="focus-lit mt-3 flex w-fit max-w-full items-center gap-2.5 rounded-[0.75rem] bg-ink/8 py-1.5 pr-3.5 pl-1.5 text-control text-ink hover:bg-ink/13"
    >
      {art !== null && (
        <img
          src={posterUrl(art, 112)}
          alt=""
          loading="lazy"
          onError={onArtError}
          className="h-11 w-8 shrink-0 rounded-[0.375rem] bg-s2 object-cover"
        />
      )}
      <span className="min-w-0">
        <b className="block font-[650] break-words">
          {(session.paused ? t.users.pausedOn : t.users.watching)(
            item.kind === 'episode' && item.seriesName ? item.seriesName : item.name,
          )}
        </b>
        <small className="block text-micro text-ink-3">
          {[
            session.device.name,
            item.runtime > 0
              ? t.users.positionOf(
                  formatClock(Math.min(session.position, item.runtime)),
                  formatClock(item.runtime),
                )
              : '',
          ]
            .filter(Boolean)
            .join(' · ')}
        </small>
      </span>
    </Link>
  )
}
