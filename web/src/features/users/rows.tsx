import { ArrowLeftIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useId, type ReactNode } from 'react'
import { Link } from 'react-router'
import { fetchLiveSessions, queryKeys, type LiveSession } from '@/api'
import { formatClock } from '@/format'
import { useI18n } from '@/i18n'

/**
 * One section of a user's or a group's page: its title (25 px), an optional line under it, then
 * its rows 8 px apart. `id` is the target of the section list's anchor.
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
  /** On the right of the title: a button. */
  aside?: ReactNode
  children: ReactNode
}) {
  return (
    <section id={id} aria-labelledby={`${id}-title`} className="scroll-mt-8 max-md:scroll-mt-20">
      <div className="mb-[18px] flex flex-wrap items-end justify-between gap-x-4 gap-y-3 max-md:mb-3.5">
        <div className="min-w-0">
          <h2 id={`${id}-title`} className="text-group break-words text-ink">
            {title}
          </h2>
          {description !== undefined && (
            <p className="mt-1.5 max-w-[66ch] text-small text-ink-2">{description}</p>
          )}
        </div>
        {aside}
      </div>
      <div className="grid grid-cols-[minmax(0,1fr)] gap-2">{children}</div>
    </section>
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
    <header className="mb-12 max-md:mb-8">
      <Link
        to={back.to}
        className="group focus-lit mb-3 -ml-3 inline-flex items-center gap-2 rounded-control px-3 py-1.5 text-small font-medium text-ink-2 hover:bg-s1 hover:text-ink"
      >
        <ArrowLeftIcon
          size={18}
          aria-hidden="true"
          className="transition-transform duration-300 ease-nuit group-hover:-translate-x-0.5"
        />
        {back.label}
      </Link>
      <section
        aria-labelledby={titleId}
        className="relative isolate grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-7 overflow-hidden rounded-tile bg-s1 px-8 py-[30px] max-md:grid-cols-[auto_minmax(0,1fr)] max-md:gap-x-[18px] max-md:gap-y-4 max-md:rounded-panel max-md:px-5 max-md:py-[22px]"
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
          <h1 id={titleId} className="text-h1 break-words text-ink max-md:text-[34px]">
            {title}
          </h1>
          <div className="mt-2 flex flex-wrap items-center gap-x-[18px] gap-y-1.5 text-[18px] text-ink-2 max-md:text-[16px]">
            {facts}
          </div>
          {extra}
        </div>
        {actions && (
          <div className="flex max-w-full flex-wrap gap-3 self-start max-md:col-span-full">
            {actions}
          </div>
        )}
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
      className="focus-lit mt-[18px] flex w-fit max-w-full items-center gap-3.5 rounded-[20px] bg-ink/8 py-2 pr-[18px] pl-2 text-[17.5px] text-ink hover:bg-ink/13 max-md:text-[16px]"
    >
      {art !== null && (
        <img
          src={posterUrl(art, 112)}
          alt=""
          loading="lazy"
          onError={onArtError}
          className="h-14 w-10 shrink-0 rounded-[10px] bg-s2 object-cover"
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
