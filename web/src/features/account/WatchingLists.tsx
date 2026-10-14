import {
  ClockCounterClockwiseIcon,
  EyeIcon,
  EyeSlashIcon,
  FilmSlateIcon,
  TelevisionSimpleIcon,
  XIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
  fetchOwnHidden,
  fetchOwnResume,
  queryClient,
  queryKeys,
  removeOwnResume,
  showOwnHidden,
  type AccountTitle,
} from '@/api'
import { errorMessage, formatClock, formatEpisode } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  EmptyState,
  InlineError,
  Notice,
  RelativeTime,
  Row,
  RowList,
  SkeletonRows,
  useToast,
} from '@/ui'

/** The poster of a title, or the icon of its kind when it has none or fails to load. */
function TitlePoster({ title }: { title: AccountTitle }) {
  const [failed, setFailed] = useState(false)
  const KindIcon =
    title.kind === 'series' || title.kind === 'episode' ? TelevisionSimpleIcon : FilmSlateIcon
  if (failed) {
    return (
      <span
        aria-hidden="true"
        className="grid aspect-[2/3] w-8 shrink-0 place-items-center rounded-check border border-line-2 bg-s2 text-ink-3"
      >
        <KindIcon size={14} />
      </span>
    )
  }
  return (
    <img
      src={`/Items/${encodeURIComponent(title.posterId)}/Images/Primary?maxHeight=96&quality=85`}
      alt=""
      loading="lazy"
      onError={() => setFailed(true)}
      className="aspect-[2/3] w-8 shrink-0 rounded-check object-cover ring-1 ring-ink/8"
    />
  )
}

/** A title's name for its row: an episode under its series, with its number. */
function titleName(title: AccountTitle) {
  return title.kind === 'episode' && title.seriesName
    ? `${title.seriesName} ${formatEpisode(title.season, title.episode)} · ${title.name}`
    : title.name
}

/**
 * My account › Continue Watching: the titles under way, each with Remove, which takes it off
 * without changing whether it is played.
 */
export function ResumeList() {
  const { t } = useI18n()
  const text = t.account.watching
  const toast = useToast()
  const titles = useQuery({
    queryKey: queryKeys.ownResume,
    queryFn: ({ signal }) => fetchOwnResume(signal),
  })
  const removal = useMutation({
    mutationFn: (title: AccountTitle) => removeOwnResume(title.id),
    onSuccess: (_data, title) => toast(text.removed(titleName(title))),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.ownResume }),
  })

  if (titles.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (titles.isError) {
    return (
      <InlineError onRetry={() => void titles.refetch()} retrying={titles.isFetching}>
        {errorMessage(t, titles.error)}
      </InlineError>
    )
  }
  return (
    <div className="space-y-3">
      {removal.isError && <Notice tone="danger">{errorMessage(t, removal.error)}</Notice>}
      {titles.data.length === 0 ? (
        <EmptyState icon={ClockCounterClockwiseIcon} title={text.resumeEmpty}>
          {text.resumeEmptyHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={t.account.sections.resume}>
          {titles.data.map((entry) => (
            <Row
              key={entry.item.id}
              leading={<TitlePoster title={entry.item} />}
              title={titleName(entry.item)}
              meta={
                <span className="truncate">
                  {text.kinds[entry.item.kind] ?? entry.item.kind}
                  {entry.runtime > 0 &&
                    ` · ${text.progress(formatClock(entry.position), formatClock(entry.runtime))}`}
                  {entry.lastPlayedAt !== null && (
                    <>
                      {' · '}
                      <RelativeTime iso={entry.lastPlayedAt} />
                    </>
                  )}
                </span>
              }
              trailing={
                <Button
                  variant="ghost"
                  size="sm"
                  icon={XIcon}
                  aria-label={text.removeLabel(titleName(entry.item))}
                  loading={removal.isPending && removal.variables.id === entry.item.id}
                  onClick={() => removal.mutate(entry.item)}
                >
                  {text.remove}
                </Button>
              }
            />
          ))}
        </RowList>
      )}
    </div>
  )
}

/** My account › Hidden titles: the titles hidden from the user's lists, each with Show again. */
export function HiddenList() {
  const { t } = useI18n()
  const text = t.account.watching
  const toast = useToast()
  const titles = useQuery({
    queryKey: queryKeys.ownHidden,
    queryFn: ({ signal }) => fetchOwnHidden(signal),
  })
  const showing = useMutation({
    mutationFn: (title: AccountTitle) => showOwnHidden(title.id),
    onSuccess: (_data, title) => toast(text.shown(title.name)),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownHidden })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownResume })
    },
  })

  if (titles.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (titles.isError) {
    return (
      <InlineError onRetry={() => void titles.refetch()} retrying={titles.isFetching}>
        {errorMessage(t, titles.error)}
      </InlineError>
    )
  }
  return (
    <div className="space-y-3">
      {showing.isError && <Notice tone="danger">{errorMessage(t, showing.error)}</Notice>}
      {titles.data.length === 0 ? (
        <EmptyState icon={EyeSlashIcon} title={text.hiddenEmpty}>
          {text.hiddenEmptyHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={t.account.sections.hidden}>
          {titles.data.map((entry) => (
            <Row
              key={entry.item.id}
              leading={<TitlePoster title={entry.item} />}
              title={entry.item.name}
              meta={
                <span className="truncate">
                  {[text.kinds[entry.item.kind] ?? entry.item.kind, entry.item.year || '']
                    .filter(Boolean)
                    .join(' · ')}
                  {' · '}
                  {text.hiddenOn} <RelativeTime iso={entry.hiddenAt} />
                </span>
              }
              trailing={
                <Button
                  variant="ghost"
                  size="sm"
                  icon={EyeIcon}
                  aria-label={text.showLabel(entry.item.name)}
                  loading={showing.isPending && showing.variables.id === entry.item.id}
                  onClick={() => showing.mutate(entry.item)}
                >
                  {text.showAgain}
                </Button>
              }
            />
          ))}
        </RowList>
      )}
    </div>
  )
}
