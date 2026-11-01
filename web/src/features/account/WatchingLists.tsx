import {
  ClockCounterClockwiseIcon,
  EyeIcon,
  EyeSlashIcon,
  FilmSlateIcon,
  ListPlusIcon,
  RowsIcon,
  TelevisionSimpleIcon,
  XIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import {
  fetchOwnHidden,
  fetchOwnHomeRows,
  fetchOwnNextUp,
  fetchOwnResume,
  putBackOwnNextUp,
  queryClient,
  queryKeys,
  removeOwnNextUp,
  removeOwnResume,
  setOwnNextUpWithResume,
  setOwnHomeRow,
  showOwnHidden,
  type AccountTitle,
  type OwnHomeRow,
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
  Switch,
  useToast,
} from '@/ui'

/** The poster of a title, or the icon of its kind when it has none or fails to load. */
export function TitlePoster({ title }: { title: AccountTitle }) {
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
export function titleName(title: AccountTitle) {
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
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownResume })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownNextUp })
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

/**
 * My account › Next Up: the episodes to watch next, each with Remove, which takes its series off
 * Next Up without changing what was watched; the series taken off, each with Show again; and
 * whether Remove from Continue Watching also takes a series off Next Up.
 */
export function NextUpList() {
  const { t } = useI18n()
  const text = t.account.watching
  const toast = useToast()
  const labelId = useId()
  const helpId = useId()
  const nextUp = useQuery({
    queryKey: queryKeys.ownNextUp,
    queryFn: ({ signal }) => fetchOwnNextUp(signal),
  })
  const removal = useMutation({
    mutationFn: (title: AccountTitle) => removeOwnNextUp(title.id),
    onSuccess: (_data, title) => toast(text.nextUpRemoved(title.seriesName || title.name)),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.ownNextUp }),
  })
  const putBack = useMutation({
    mutationFn: (title: AccountTitle) => putBackOwnNextUp(title.id),
    onSuccess: (_data, title) => toast(text.nextUpShown(title.name)),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.ownNextUp }),
  })
  const withResume = useMutation({
    mutationFn: (on: boolean) => setOwnNextUpWithResume(on),
    onSuccess: (data) => toast(data.withResume ? text.withResumeOn : text.withResumeOff),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.ownNextUp }),
  })

  if (nextUp.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (nextUp.isError) {
    return (
      <InlineError onRetry={() => void nextUp.refetch()} retrying={nextUp.isFetching}>
        {errorMessage(t, nextUp.error)}
      </InlineError>
    )
  }
  const error = removal.error ?? putBack.error ?? withResume.error
  return (
    <div className="space-y-3">
      {error && <Notice tone="danger">{errorMessage(t, error)}</Notice>}
      {nextUp.data.items.length === 0 ? (
        <EmptyState icon={ListPlusIcon} title={text.nextUpEmpty}>
          {text.nextUpEmptyHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={t.account.sections.nextUp}>
          {nextUp.data.items.map((item) => (
            <Row
              key={item.id}
              leading={<TitlePoster title={item} />}
              title={titleName(item)}
              meta={<span className="truncate">{text.kinds[item.kind] ?? item.kind}</span>}
              trailing={
                <Button
                  variant="ghost"
                  size="sm"
                  icon={XIcon}
                  aria-label={text.nextUpRemoveLabel(item.seriesName || item.name)}
                  loading={removal.isPending && removal.variables.id === item.id}
                  onClick={() => removal.mutate(item)}
                >
                  {text.remove}
                </Button>
              }
            />
          ))}
        </RowList>
      )}
      {nextUp.data.removed.length > 0 && (
        <>
          <p className="text-small font-medium text-ink-2">{text.nextUpRemovedList}</p>
          <RowList aria-label={text.nextUpRemovedList}>
            {nextUp.data.removed.map((entry) => (
              <Row
                key={entry.item.id}
                leading={<TitlePoster title={entry.item} />}
                title={entry.item.name}
                muted
                meta={
                  <span className="truncate">
                    {text.kinds[entry.item.kind] ?? entry.item.kind}
                    {' · '}
                    {text.nextUpRemovedOn} <RelativeTime iso={entry.removedAt} />
                  </span>
                }
                trailing={
                  <Button
                    variant="ghost"
                    size="sm"
                    icon={EyeIcon}
                    aria-label={text.nextUpShowLabel(entry.item.name)}
                    loading={putBack.isPending && putBack.variables.id === entry.item.id}
                    onClick={() => putBack.mutate(entry.item)}
                  >
                    {text.showAgain}
                  </Button>
                }
              />
            ))}
          </RowList>
        </>
      )}
      <div className="flex max-w-xl items-start justify-between gap-6 rounded-row border border-line bg-bg px-[18px] py-4 max-sm:gap-4 max-sm:p-3.5">
        <div className="min-w-0">
          <p id={labelId} className="text-control font-medium text-ink">
            {text.withResume}
          </p>
          <p id={helpId} className="mt-0.5 text-small text-ink-3">
            {text.withResumeHelp}
          </p>
        </div>
        <Switch
          checked={nextUp.data.withResume}
          disabled={withResume.isPending}
          labelledBy={labelId}
          describedById={helpId}
          onChange={(on) => withResume.mutate(on)}
          className="mt-0.5"
        />
      </div>
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
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownNextUp })
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

/** My account › Home rows: the rows shown to the user on the web player's home, each with a switch. */
export function HomeRowList() {
  const { t } = useI18n()
  const text = t.account.homeRows
  const places = t.settings.homeRow.places
  const kinds = t.settings.homeRow.kinds
  const toast = useToast()
  const rows = useQuery({
    queryKey: queryKeys.ownHomeRows,
    queryFn: ({ signal }) => fetchOwnHomeRows(signal),
  })
  const change = useMutation({
    mutationFn: (row: OwnHomeRow) => setOwnHomeRow(row.id, !row.off),
    onSuccess: (_data, row) => toast(row.off ? text.on(row.name) : text.off(row.name)),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.ownHomeRows }),
  })

  if (rows.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (rows.isError) {
    return (
      <InlineError onRetry={() => void rows.refetch()} retrying={rows.isFetching}>
        {errorMessage(t, rows.error)}
      </InlineError>
    )
  }
  return (
    <div className="space-y-3">
      {change.isError && <Notice tone="danger">{errorMessage(t, change.error)}</Notice>}
      {rows.data.rows.length === 0 ? (
        <EmptyState icon={RowsIcon} title={text.empty}>
          {text.emptyHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={t.account.sections.homeRows}>
          {rows.data.rows.map((row) => (
            <Row
              key={row.id}
              title={row.name}
              muted={row.off}
              meta={
                <span className="truncate">
                  {kinds[row.kind]} · {places[row.place]}
                </span>
              }
              trailing={
                <Switch
                  checked={!row.off}
                  label={text.shown(row.name)}
                  disabled={change.isPending && change.variables.id === row.id}
                  onChange={() => change.mutate(row)}
                />
              }
            />
          ))}
        </RowList>
      )}
    </div>
  )
}
