import { useEffect, useId, useState, type ReactNode } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ApiError,
  fetchParentalRatings,
  queryClient,
  queryKeys,
  updateUser,
  type ParentalControl,
  type ParentalRating,
  type QualityGroup,
  type User,
  type UserPatch,
} from '@/api'
import { useSessionUser } from '@/app/session'
import { dateTime, relativeTime } from '@/format'
import { useI18n } from '@/i18n'
import { Avatar, cx, Switch } from '@/ui'

/** Puts a user returned by the server into the cached list. */
export function storeUser(updated: User) {
  queryClient.setQueryData<User[]>(queryKeys.users, (old) =>
    old?.map((item) => (item.id === updated.id ? updated : item)),
  )
}

/** A 404 means the user is gone: the list is refreshed so the page shows it. */
export function refreshIfGone(error: unknown) {
  if (error instanceof ApiError && error.status === 404) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.users })
  }
}

/** A PATCH mutation for one user that keeps the list (and our own session) up to date. */
export function useUserPatch(user: User) {
  const self = useSessionUser()
  return useMutation({
    mutationFn: (patch: UserPatch) => updateUser(user.id, patch),
    onSuccess: (updated) => {
      storeUser(updated)
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      void queryClient.invalidateQueries({ queryKey: queryKeys.userDevices(user.id) })
      if (updated.id === self.id)
        void queryClient.invalidateQueries({ queryKey: queryKeys.session })
    },
    onError: refreshIfGone,
  })
}

/** "2 hours ago", with the full date on hover. */
export function RelativeTime({ iso, className }: { iso: string; className?: string }) {
  const { language, t } = useI18n()
  return (
    <time dateTime={iso} title={dateTime(iso, language)} className={className}>
      {relativeTime(iso, language, t.time.justNow)}
    </time>
  )
}

/** "14:05" in the reader's language. */
export function clockTime(iso: string, language: string) {
  return new Intl.DateTimeFormat(language, { hour: '2-digit', minute: '2-digit' }).format(
    new Date(iso),
  )
}

/** The user's profile picture, as their Jellyfin apps show it, or their initial. */
export function UserAvatar({ user, size = 'lg' }: { user: User; size?: 'md' | 'lg' }) {
  if (user.imageTag !== null) {
    return (
      <img
        src={`/UserImage?userId=${encodeURIComponent(user.id)}&tag=${encodeURIComponent(user.imageTag)}`}
        alt=""
        className={cx(
          'shrink-0 border border-line-2 object-cover',
          size === 'lg' ? 'size-10 rounded-row' : 'size-7 rounded-[9px]',
        )}
      />
    )
  }
  return <Avatar name={user.name} size={size} />
}

/** A quality group as people name video of that height: 4K, else its lines, 1080p. */
export function qualityGroupName(group: Exclude<QualityGroup, 0>) {
  return group === 2160 ? '4K' : `${group}p`
}

/** Ratings sharing a score and sub-score, offered as one choice (like jellyfin-web). */
export type RatingGroup = { name: string; score: number; subScore: number | null }

export function groupRatings(ratings: ParentalRating[]): RatingGroup[] {
  const groups: RatingGroup[] = []
  for (const rating of ratings) {
    const last = groups.at(-1)
    if (last !== undefined && last.score === rating.score && last.subScore === rating.subScore) {
      last.name = `${last.name} / ${rating.name}`
    } else {
      groups.push({ ...rating })
    }
  }
  return groups
}

/** The group matching the limit exactly, else the last one whose score fits under it. */
export function selectedGroup(groups: RatingGroup[], control: ParentalControl): number {
  const { maxRating, maxSubRating } = control
  if (maxRating === null) return -1
  const exact = groups.findIndex((g) => g.score === maxRating && g.subScore === maxSubRating)
  if (exact !== -1) return exact
  let fallback = -1
  groups.forEach((group, index) => {
    if (group.score <= maxRating) fallback = index
  })
  return fallback
}

export function useParentalRatings(enabled = true) {
  return useQuery({
    queryKey: queryKeys.parentalRatings,
    queryFn: ({ signal }) => fetchParentalRatings(signal),
    staleTime: Infinity,
    enabled,
  })
}

/** "Up to PG-13" when the user has a rating limit, once the ratings are known. */
export function useRatingLimit(parentalControl: ParentalControl): string | null {
  const { t } = useI18n()
  const limited = parentalControl.maxRating !== null
  const ratings = useParentalRatings(limited)
  if (!limited || ratings.data === undefined) return null
  const groups = groupRatings(ratings.data)
  const group = groups[selectedGroup(groups, parentalControl)]
  return group === undefined ? null : t.users.ratingLimit(group.name)
}

/** The limits set on a user, in words: hidden, no conversion, up to PG-13, 1080p… */
export function useRestrictions(user: User): string[] {
  const { t } = useI18n()
  const rating = useRatingLimit(user.parentalControl)
  const words: string[] = []
  if (user.isHidden) words.push(t.users.hidden)
  if (rating !== null) words.push(rating)
  if (!user.transcoding) words.push(t.users.noTranscoding)
  if (!user.downloads) words.push(t.users.noDownloads)
  if (!user.personalAddons) words.push(t.users.noPersonalAddons)
  if (user.qualityGroup !== 0) words.push(qualityGroupName(user.qualityGroup))
  return words
}

/**
 * One setting of a panel: its title and help on the left, a switch on the right (like the
 * settings rows of the mockup).
 */
export function SwitchRow({
  title,
  help,
  checked,
  onChange,
  disabled,
}: {
  title: string
  help?: ReactNode
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
}) {
  const titleId = useId()
  const helpId = useId()
  return (
    <div className="flex items-start justify-between gap-6 py-4 not-first:border-t not-first:border-line">
      <div className="min-w-0">
        <p id={titleId} className="text-control font-medium text-ink">
          {title}
        </p>
        {help !== undefined && (
          <p id={helpId} className="mt-1 text-small text-ink-3">
            {help}
          </p>
        )}
      </div>
      <Switch
        checked={checked}
        onChange={onChange}
        labelledBy={titleId}
        describedById={help !== undefined ? helpId : undefined}
        disabled={disabled}
        className="mt-0.5"
      />
    </div>
  )
}

/**
 * The id of the section in view, for a SectionNav of anchors. The last section counts once the
 * page is scrolled to its end.
 */
export function useSectionInView(ids: readonly string[], ready: boolean) {
  const [current, setCurrent] = useState(ids[0])
  const key = ids.join(' ')
  useEffect(() => {
    if (!ready) return
    const sections = key
      .split(' ')
      .map((id) => document.getElementById(id))
      .filter((el): el is HTMLElement => el !== null)
    function update() {
      const line = 140
      let found = sections[0]?.id
      for (const section of sections) {
        if (section.getBoundingClientRect().top <= line) found = section.id
      }
      const end = window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 4
      if (end && sections.length > 0) found = sections[sections.length - 1].id
      if (found) setCurrent(found)
    }
    update()
    window.addEventListener('scroll', update, { passive: true })
    window.addEventListener('resize', update)
    return () => {
      window.removeEventListener('scroll', update)
      window.removeEventListener('resize', update)
    }
  }, [key, ready])
  return current
}
