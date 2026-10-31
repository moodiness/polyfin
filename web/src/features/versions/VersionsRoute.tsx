import { ArrowLeftIcon, MagnifyingGlassIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import {
  fetchVersionBlocks,
  fetchVersionEpisodes,
  queryClient,
  queryKeys,
  searchVersionTitles,
  unblockVersion,
  type AccountTitle,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { TitlePoster } from '@/features/account/WatchingLists'
import { errorMessage, formatBytes, formatEpisode } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  Button,
  cx,
  EmptyState,
  Field,
  InlineError,
  RelativeTime,
  Row,
  RowList,
  Skeleton,
  SkeletonRows,
  TextInput,
  useToast,
} from '@/ui'
import { userQuery, UserPicker, useVersionsUser } from './parts'

/**
 * `/versions`: finds a movie or an episode, as a user sees it, to explain its versions, and lists
 * the files blocked for every title.
 */
export default function VersionsRoute() {
  const { t } = useI18n()
  const [user, setUser] = useVersionsUser()
  const navigate = useNavigate()
  return (
    <PageLayout title={t.versions.title} lede={t.versions.description}>
      <Block title={t.versions.find}>
        <div className="flex flex-col gap-5">
          <UserPicker user={user} onUser={setUser} />
          <TitleFinder
            key={user}
            user={user}
            onPick={(title) => void navigate(`/versions/${title.id}${userQuery(user)}`)}
          />
        </div>
      </Block>
      <BlockedEverywhere />
    </PageLayout>
  )
}

/** Searches movies and series; a series lists its episodes, one of which is picked. */
function TitleFinder({ user, onPick }: { user: string; onPick: (title: AccountTitle) => void }) {
  const { t } = useI18n()
  const text = t.versions
  const kinds = t.account.watching.kinds
  const [term, setTerm] = useState('')
  const [asked, setAsked] = useState<string | null>(null)
  const [series, setSeries] = useState<AccountTitle | null>(null)
  const search = useQuery({
    queryKey: queryKeys.versionTitles(user, asked ?? ''),
    queryFn: ({ signal }) => searchVersionTitles(user, asked ?? '', signal),
    enabled: asked !== null,
  })
  const episodes = useQuery({
    queryKey: queryKeys.versionEpisodes(user, series?.id ?? ''),
    queryFn: ({ signal }) => fetchVersionEpisodes(user, series?.id ?? '', signal),
    enabled: series !== null,
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = term.trim()
    if (next === '') return
    if (next === asked) void search.refetch()
    else setAsked(next)
  }

  const pickButton =
    'flex w-full cursor-pointer items-center gap-3 rounded-field p-1.5 text-left transition-colors duration-160 hover:bg-s3 focus-visible:bg-s3'

  if (series !== null) {
    return (
      <section aria-label={text.episodesOf(series.name)} className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-3">
          <p className="truncate text-control font-medium text-ink">
            {text.episodesOf(series.name)}
          </p>
          <Button size="sm" variant="ghost" icon={ArrowLeftIcon} onClick={() => setSeries(null)}>
            {text.back}
          </Button>
        </div>
        {episodes.isPending ? (
          <Skeleton className="h-24 rounded-field" />
        ) : episodes.isError ? (
          <InlineError onRetry={() => void episodes.refetch()} retrying={episodes.isFetching}>
            {errorMessage(t, episodes.error)}
          </InlineError>
        ) : episodes.data.length === 0 ? (
          <p className="text-small text-ink-3">{text.noEpisodes}</p>
        ) : (
          <ul className="flex max-h-96 flex-col gap-0.5 overflow-y-auto rounded-field border border-line bg-s1 p-1">
            {episodes.data.map((episode) => (
              <li key={episode.id}>
                <button
                  type="button"
                  onClick={() => onPick(episode)}
                  className={cx(pickButton, 'items-baseline px-2.5 py-2')}
                >
                  <span className="figures shrink-0 text-small text-ink-3">
                    {formatEpisode(episode.season, episode.episode)}
                  </span>
                  <span className="min-w-0 truncate text-control text-ink">{episode.name}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <form onSubmit={submit} noValidate className="flex max-w-xl items-start gap-2">
        <Field label={text.search} help={text.searchHelp} className="flex-1">
          <TextInput
            value={term}
            onValue={setTerm}
            icon={MagnifyingGlassIcon}
            autoComplete="off"
            spellCheck={false}
          />
        </Field>
        <Button
          type="submit"
          className="mt-[26px]"
          loading={search.isFetching}
          disabled={term.trim() === ''}
        >
          {text.searchButton}
        </Button>
      </form>
      <div aria-live="polite" className="empty:hidden">
        {asked === null ? null : search.isPending ? (
          <div className="flex flex-col gap-2" aria-label={text.searching}>
            <Skeleton className="h-12 rounded-field" />
            <Skeleton className="h-12 rounded-field" />
          </div>
        ) : search.isError ? (
          <InlineError onRetry={() => void search.refetch()} retrying={search.isRefetching}>
            {errorMessage(t, search.error)}
          </InlineError>
        ) : search.data.length === 0 ? (
          <p className="text-small text-ink-3">{text.noTitles}</p>
        ) : (
          <ul className="flex max-w-xl flex-col gap-1" aria-label={text.titlesFound(asked)}>
            {search.data.map((found) => {
              const details = [kinds[found.kind] ?? found.kind, found.year > 0 ? found.year : '']
                .filter(Boolean)
                .join(' · ')
              return (
                <li key={found.id}>
                  <button
                    type="button"
                    onClick={() => (found.kind === 'series' ? setSeries(found) : onPick(found))}
                    aria-label={text.pickTitle(found.name, details)}
                    className={pickButton}
                  >
                    <TitlePoster title={found} />
                    <span className="min-w-0">
                      <span className="block truncate text-control text-ink">{found.name}</span>
                      <span className="block text-small text-ink-3">{details}</span>
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </div>
  )
}

/** The files blocked for every title, each with Unblock. */
function BlockedEverywhere() {
  const { t, language } = useI18n()
  const text = t.versions
  const toast = useToast()
  const blocks = useQuery({
    queryKey: queryKeys.versionBlocks,
    queryFn: ({ signal }) => fetchVersionBlocks(signal),
  })
  const unblock = useMutation({
    mutationFn: unblockVersion,
    onSuccess: () => {
      toast(text.unblockDone, { tone: 'ok' })
      void queryClient.invalidateQueries({ queryKey: queryKeys.versionBlocks })
    },
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
  })
  return (
    <Block title={text.everywhere} count={blocks.data?.length}>
      <p className="mb-4 max-w-2xl text-small text-ink-3">{text.everywhereHelp}</p>
      {blocks.isPending ? (
        <SkeletonRows rows={2} />
      ) : blocks.isError ? (
        <InlineError onRetry={() => void blocks.refetch()} retrying={blocks.isFetching}>
          {errorMessage(t, blocks.error)}
        </InlineError>
      ) : blocks.data.length === 0 ? (
        <EmptyState title={text.noBlocks} />
      ) : (
        <RowList>
          {blocks.data.map((block) => (
            <li key={block.id}>
              <Row
                title={<span className="break-all">{block.name}</span>}
                meta={
                  <>
                    {[block.filename, block.size > 0 ? formatBytes(block.size, language) : '']
                      .filter(Boolean)
                      .join(' · ')}
                    <span className="block">
                      {text.blockedBy(block.by, block.reason)} · <RelativeTime iso={block.at} />
                    </span>
                  </>
                }
                trailing={
                  <Button
                    size="sm"
                    loading={unblock.isPending && unblock.variables === block.id}
                    onClick={() => unblock.mutate(block.id)}
                  >
                    {text.unblock}
                  </Button>
                }
              />
            </li>
          ))}
        </RowList>
      )}
    </Block>
  )
}
