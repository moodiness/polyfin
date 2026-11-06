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
  EmptyState,
  Field,
  InlineError,
  RelativeTime,
  Row,
  RowList,
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
        <div className="flex flex-col gap-7">
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

  if (series !== null) {
    return (
      <section aria-label={text.episodesOf(series.name)} className="flex max-w-3xl flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="min-w-0 text-h3 break-words text-ink">{text.episodesOf(series.name)}</h3>
          <Button size="sm" variant="ghost" icon={ArrowLeftIcon} onClick={() => setSeries(null)}>
            {text.back}
          </Button>
        </div>
        {episodes.isPending ? (
          <SkeletonRows rows={3} />
        ) : episodes.isError ? (
          <InlineError onRetry={() => void episodes.refetch()} retrying={episodes.isFetching}>
            {errorMessage(t, episodes.error)}
          </InlineError>
        ) : episodes.data.length === 0 ? (
          <p className="text-small text-ink-3">{text.noEpisodes}</p>
        ) : (
          // Long seasons scroll inside, with room for the lit row's rise.
          <div className="-mx-2 max-h-[560px] overflow-y-auto px-2 py-1">
            <RowList compact aria-label={text.episodesOf(series.name)}>
              {episodes.data.map((episode) => (
                <Row
                  key={episode.id}
                  onClick={() => onPick(episode)}
                  leading={
                    <span className="figures w-[88px] shrink-0 text-micro whitespace-nowrap text-ink-3">
                      {formatEpisode(episode.season, episode.episode)}
                    </span>
                  }
                  title={episode.name}
                />
              ))}
            </RowList>
          </div>
        )}
      </section>
    )
  }

  return (
    <div className="flex max-w-3xl flex-col gap-5">
      <form onSubmit={submit} noValidate>
        <Field label={text.search} help={text.searchHelp}>
          <TextInput
            value={term}
            onValue={setTerm}
            icon={MagnifyingGlassIcon}
            autoComplete="off"
            spellCheck={false}
            end={
              <Button
                type="submit"
                size="sm"
                className="mr-2.5"
                loading={search.isFetching}
                disabled={term.trim() === ''}
              >
                {text.searchButton}
              </Button>
            }
          />
        </Field>
      </form>
      <div aria-live="polite" className="empty:hidden">
        {asked === null ? null : search.isPending ? (
          <SkeletonRows rows={2} label={text.searching} />
        ) : search.isError ? (
          <InlineError onRetry={() => void search.refetch()} retrying={search.isRefetching}>
            {errorMessage(t, search.error)}
          </InlineError>
        ) : search.data.length === 0 ? (
          <p className="text-small text-ink-3">{text.noTitles}</p>
        ) : (
          <RowList aria-label={text.titlesFound(asked)}>
            {search.data.map((found) => (
              <Row
                key={found.id}
                onClick={() => (found.kind === 'series' ? setSeries(found) : onPick(found))}
                leading={
                  // The poster, a little larger than its 32 px.
                  <span className="flex shrink-0 [&>*]:w-10">
                    <TitlePoster title={found} />
                  </span>
                }
                title={found.name}
                meta={[kinds[found.kind] ?? found.kind, found.year > 0 ? found.year : '']
                  .filter(Boolean)
                  .join(' · ')}
              />
            ))}
          </RowList>
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
      <p className="mb-5 max-w-[72ch] text-small text-ink-2">{text.everywhereHelp}</p>
      {blocks.isPending ? (
        <SkeletonRows rows={2} />
      ) : blocks.isError ? (
        <InlineError onRetry={() => void blocks.refetch()} retrying={blocks.isFetching}>
          {errorMessage(t, blocks.error)}
        </InlineError>
      ) : blocks.data.length === 0 ? (
        <EmptyState title={text.noBlocks} />
      ) : (
        <RowList compact aria-label={text.everywhere}>
          {blocks.data.map((block) => (
            <Row
              key={block.id}
              title={<span className="break-all">{block.name}</span>}
              meta={
                <span className="flex min-w-0 flex-col break-words">
                  <span>
                    {[block.filename, block.size > 0 ? formatBytes(block.size, language) : '']
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                  <span>
                    {text.blockedBy(block.by, block.reason)} · <RelativeTime iso={block.at} />
                  </span>
                </span>
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
          ))}
        </RowList>
      )}
    </Block>
  )
}
