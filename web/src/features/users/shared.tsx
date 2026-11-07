import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ADMIN_APP,
  ApiError,
  fetchParentalRatings,
  queryClient,
  queryKeys,
  updateUser,
  type Group,
  type ParentalControl,
  type ParentalRating,
  type QualityGroup,
  type User,
  type UserPatch,
} from '@/api'
import { SubAreaContext } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { useI18n } from '@/i18n'
import { RelativeTime, scaledPx, StatusPill, Tabs } from '@/ui'

/** PageLayout claims the words; there is nothing to show above a page in their place. */
const noClaim = () => () => {}

/**
 * The pages of Users › Users and Users › Groups: their PageLayout shows the sub-area words Users ·
 * Groups under its title, as Content's and System's pages show theirs.
 */
export function UsersArea({ children }: { children: ReactNode }) {
  const { t } = useI18n()
  const subArea = useMemo(
    () => ({
      words: (
        <Tabs
          label={t.groups.tabsLabel}
          items={[
            { id: 'users', label: t.users.title, to: '/users', end: true },
            { id: 'groups', label: t.groups.title, to: '/users/groups', end: true },
          ]}
        />
      ),
      claim: noClaim,
    }),
    [t],
  )
  return <SubAreaContext value={subArea}>{children}</SubAreaContext>
}

/** The state of an account, as an icon and a word: Active, Disabled, Ended or Blocked until… */
export function UserState({ user }: { user: User }) {
  const { t, language } = useI18n()
  if (user.isDisabled) return <StatusPill tone="muted">{t.users.disabled}</StatusPill>
  if (user.ended) return <StatusPill tone="muted">{t.users.ended}</StatusPill>
  if (user.blockedUntil !== null) {
    return (
      <StatusPill tone="warn">
        {t.users.blockedUntil(clockTime(user.blockedUntil, language))}
      </StatusPill>
    )
  }
  return <StatusPill tone="ok">{t.users.active}</StatusPill>
}

/**
 * A user's last activity and its app: "Last activity: 3 minutes ago · Jellyfin Android TV", or
 * "Last activity: Never". The admin app's own name reads in the language of the page.
 */
export function LastActivity({ user }: { user: User }) {
  const { t } = useI18n()
  if (user.lastActivityAt === null) return <>{`${t.users.lastActivityLabel} ${t.common.never}`}</>
  const app = user.lastActivityApp === ADMIN_APP ? t.users.adminApp : user.lastActivityApp
  return (
    <>
      {t.users.lastActivityLabel} <RelativeTime iso={user.lastActivityAt} />
      {app !== '' && ` · ${app}`}
    </>
  )
}

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

/**
 * Keeps the caches up to date after a user is changed: the list, their devices, our own session,
 * and the groups, whose members and counts a change of group moves.
 */
export function userSaved(updated: User, selfId: string) {
  storeUser(updated)
  void queryClient.invalidateQueries({ queryKey: queryKeys.users })
  void queryClient.invalidateQueries({ queryKey: queryKeys.userDevices(updated.id) })
  void queryClient.invalidateQueries({ queryKey: queryKeys.groups })
  if (updated.id === selfId) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.session })
    void queryClient.invalidateQueries({ queryKey: queryKeys.ownWebTheme })
  }
}

/** A PATCH mutation for one user that keeps the list (and our own session) up to date. */
export function useUserPatch(user: User) {
  const self = useSessionUser()
  return useMutation({
    mutationFn: (patch: UserPatch) => updateUser(user.id, patch),
    onSuccess: (updated) => userSaved(updated, self.id),
    onError: refreshIfGone,
  })
}

/**
 * Keeps the caches up to date after a group is changed: the group, the list, the users, as its
 * following members took its values, and the invite links naming it.
 */
export function groupSaved(updated: Group) {
  queryClient.setQueryData(queryKeys.group(updated.id), updated)
  void queryClient.invalidateQueries({ queryKey: queryKeys.groups })
  void queryClient.invalidateQueries({ queryKey: queryKeys.users })
  void queryClient.invalidateQueries({ queryKey: queryKeys.invites })
}

/** A 404 means the group is gone: the groups are refreshed so the page shows it. */
export function refreshGroupIfGone(error: unknown) {
  if (error instanceof ApiError && error.status === 404) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.groups })
  }
}

/** "14:05" in the reader's language. */
export function clockTime(iso: string, language: string) {
  return new Intl.DateTimeFormat(language, { hour: '2-digit', minute: '2-digit' }).format(
    new Date(iso),
  )
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
function useRatingLimit(parentalControl: ParentalControl): string | null {
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
 * The id of the section in view, for a SectionNav of anchors. The last section counts once the
 * page is scrolled to its end.
 */
export function useSectionInView(ids: readonly string[], ready: boolean) {
  const [current, setCurrent] = useState(ids[0])
  const key = ids.join(' ')
  useEffect(() => {
    if (!ready) return
    const ids = key.split(' ')
    function update() {
      // Looked up each time: a section drawn again (after a save or a reset) is a new element.
      const sections = ids
        .map((id) => document.getElementById(id))
        .filter((el): el is HTMLElement => el !== null)
      const line = scaledPx(140)
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
