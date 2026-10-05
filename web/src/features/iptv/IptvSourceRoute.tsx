import { ArrowsClockwiseIcon, BroadcastIcon } from '@phosphor-icons/react'
import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchAddons,
  previewSource,
  queryClient,
  queryKeys,
  refreshAddon,
  updateIptvSource,
  type Addon,
  type IptvOptions,
  type IptvSource,
  type Scope,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { invalidateLineup, iptvCatalog, lineupPath, routeScope } from '@/features/iptv/lineup'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  ButtonLink,
  EmptyState,
  InlineError,
  Notice,
  Panel,
  SaveBar,
  Skeleton,
  SkeletonRows,
  Tabs,
  TextLink,
  useToast,
  RelativeTime,
} from '@/ui'
import CatalogGuides from './CatalogGuides'
import Categories from './Categories'
import Channels from './Channels'
import { optionsValid, SourceOptions } from './ImportOptions'
import Mappings from './Mappings'
import { Figures, useNumber } from './shared'

export const lineupSections = [
  'summary',
  'options',
  'categories',
  'channels',
  'guides',
  'mapping',
] as const
export type LineupSection = (typeof lineupSections)[number]

/** The sections about live channels, hidden while the source does not import them. */
const liveSections: ReadonlySet<LineupSection> = new Set([
  'categories',
  'channels',
  'guides',
  'mapping',
])

/**
 * `/sources/:scope/:id/:section?` and `/me/sources/:id/:section?` (with `scope="me"`): one IPTV
 * source, section by section: its summary, what it imports, and its Live TV line-up (categories,
 * channels, guides, mapping). The sources area's dispatcher renders it for IPTV sources.
 */
export default function IptvSourceRoute({ scope: fixedScope }: { scope?: Scope }) {
  const { t } = useI18n()
  const toast = useToast()
  const params = useParams()
  // `/me/sources/:id` names no scope: its route gives it.
  const scope = fixedScope ?? routeScope(params.scope)
  const id = params.id ?? ''
  const section = (lineupSections as readonly string[]).includes(params.section ?? 'summary')
    ? ((params.section ?? 'summary') as LineupSection)
    : null
  const addons = useQuery({
    queryKey: queryKeys.addons(scope ?? 'me'),
    queryFn: ({ signal }) => fetchAddons(scope ?? 'me', signal),
    enabled: scope !== null,
  })
  const addon = addons.data?.find((item) => item.id === id && item.source !== null)
  const source = addon?.source ?? undefined
  // Until the source is read, every section is offered: the URL may name any of them.
  const liveTv = source?.options.liveTv ?? true
  const back = {
    to: scope === 'shared' ? '/sources' : '/me/sources',
    label: scope === 'shared' ? t.lineup.backShared : t.lineup.backMine,
  }
  const refresh = useMutation({
    mutationFn: () => refreshAddon(scope ?? 'me', id),
    onSuccess: (updated) => {
      // The server answers with the source; a failed download is told by its error code.
      const failed = updated.source?.error ?? ''
      if (failed === '') toast(t.lineup.summary.refreshed)
      else toast(t.iptv.errors[failed] ?? t.errors.generic, { tone: 'danger' })
    },
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
    onSettled: () => invalidateLineup(scope ?? 'me', id),
  })

  if (scope === null || section === null || (addons.isSuccess && addon === undefined)) {
    return (
      <PageLayout title={t.lineup.notFoundTitle} back={back}>
        <EmptyState
          icon={BroadcastIcon}
          title={t.lineup.notFoundTitle}
          action={<ButtonLink to={back.to}>{back.label}</ButtonLink>}
        >
          {t.lineup.notFound}
        </EmptyState>
      </PageLayout>
    )
  }

  return (
    <PageLayout
      back={back}
      title={addon ? addon.name : t.lineup.titleLoading}
      titleAside={
        addon && <Badge>{addon.kind === 'xtream' ? t.iptv.kindXtream : t.iptv.kindM3u}</Badge>
      }
      lede={t.lineup.description}
      actions={
        source && (
          <Button
            icon={ArrowsClockwiseIcon}
            loading={refresh.isPending}
            title={t.lineup.summary.refreshHelp}
            onClick={() => refresh.mutate()}
          >
            {refresh.isPending ? t.lineup.summary.refreshing : t.lineup.summary.refresh}
          </Button>
        )
      }
    >
      <Tabs
        variant="underline"
        label={t.lineup.sectionsLabel}
        items={lineupSections
          .filter((name) => liveTv || !liveSections.has(name))
          .map((name) => ({
            id: name,
            label: t.lineup.sections[name],
            to: lineupPath(scope, id, name === 'summary' ? undefined : name),
            end: true,
          }))}
      />
      <div className="!mt-8">
        {addons.isPending ? (
          <SectionSkeleton />
        ) : addons.isError ? (
          <InlineError onRetry={() => void addons.refetch()} retrying={addons.isFetching}>
            {errorMessage(t, addons.error)}
          </InlineError>
        ) : addon === undefined || source === undefined ? null : section === 'summary' ? (
          <Summary scope={scope} addon={addon} source={source} />
        ) : section === 'options' ? (
          <Options key={id} scope={scope} id={id} source={source} />
        ) : !liveTv ? (
          <LiveOff scope={scope} id={id} />
        ) : section === 'categories' ? (
          <Categories scope={scope} id={id} />
        ) : section === 'channels' ? (
          <Channels scope={scope} id={id} />
        ) : section === 'guides' ? (
          <CatalogGuides
            scope={scope}
            target={iptvCatalog(id)}
            mappingPath={lineupPath(scope, id, 'mapping')}
            onChanged={() => invalidateLineup(scope, id)}
          />
        ) : (
          <Mappings
            scope={scope}
            target={iptvCatalog(id)}
            onChanged={() => {
              // The line-up's counts follow; the mapping list keeps its rows until it is read again.
              void queryClient.invalidateQueries({ queryKey: queryKeys.lineup(scope, id) })
              void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
            }}
          />
        )}
      </div>
    </PageLayout>
  )
}

/** The page loading: a row of figures, then rows. */
function SectionSkeleton() {
  const { t } = useI18n()
  return (
    <div role="status" aria-label={t.common.loading} className="space-y-8">
      <div className="grid grid-cols-2 gap-6 border-y border-line py-5 md:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => (
          <span key={index} className="space-y-2.5">
            <Skeleton className="h-3 w-24" />
            <Skeleton className="h-6 w-16" />
          </span>
        ))}
      </div>
      <SkeletonRows rows={3} />
    </div>
  )
}

/** Where the Live TV sections would be, while the source does not import live channels. */
function LiveOff({ scope, id }: { scope: Scope; id: string }) {
  const { t } = useI18n()
  return (
    <EmptyState
      icon={BroadcastIcon}
      title={t.lineup.content.liveOff}
      action={
        <ButtonLink to={lineupPath(scope, id, 'options')}>
          {t.lineup.content.liveOffAction}
        </ButtonLink>
      }
    />
  )
}

function Summary({ scope, addon, source }: { scope: Scope; addon: Addon; source: IptvSource }) {
  const { t } = useI18n()
  const number = useNumber()
  const text = t.lineup.summary
  const lineup = source.lineup
  const vod = source.vod
  const { liveTv, movies, series } = source.options
  const vodPath = scope === 'shared' ? '/libraries' : '/me/sources'
  return (
    <div className="space-y-block max-md:space-y-11">
      {source.error !== '' && (
        <Notice tone="danger">{t.iptv.errors[source.error] ?? t.errors.generic}</Notice>
      )}
      {liveTv ? (
        <Block title={t.lineup.content.liveTitle}>
          <Figures
            label={t.lineup.content.liveTitle}
            items={[
              {
                label: text.categories,
                value: number(lineup.enabledCategories),
                detail: text.ofTotal(number(lineup.categories)),
                to: lineupPath(scope, addon.id, 'categories'),
              },
              {
                label: text.shown,
                value: number(lineup.shownChannels),
                detail: text.enabledOf(number(lineup.enabledChannels), number(lineup.channels)),
                to: lineupPath(scope, addon.id, 'channels'),
              },
              {
                label: text.mapped,
                value: number(lineup.mapped),
                detail: text.unmapped(number(lineup.unmapped)),
                to: lineupPath(scope, addon.id, 'mapping'),
              },
              {
                label: text.entries,
                value: number(source.channels),
                detail: <span className="figures">{source.address}</span>,
              },
            ]}
          />
          {lineup.unmapped > 0 && (
            <Notice
              tone="warn"
              className="mt-5"
              action={
                <TextLink to={lineupPath(scope, addon.id, 'mapping')}>
                  {t.lineup.openMapping}
                </TextLink>
              }
            >
              {t.lineup.unmappedNotice(number(lineup.unmapped))}
            </Notice>
          )}
        </Block>
      ) : (
        <LiveOff scope={scope} id={addon.id} />
      )}
      {(movies || series) && (
        <Block title={text.vodTitle}>
          <Figures
            label={text.vodTitle}
            items={[
              ...(movies
                ? [
                    {
                      label: text.movies,
                      value: number(vod.shownMovies),
                      detail: text.titlesOf(number(vod.movies), number(vod.movieCategories)),
                      to: vodPath,
                    },
                  ]
                : []),
              ...(series
                ? [
                    {
                      label: text.series,
                      value: number(vod.shownSeries),
                      detail: text.titlesOf(number(vod.series), number(vod.seriesCategories)),
                      to: vodPath,
                    },
                    {
                      label: text.episodes,
                      value: number(vod.episodes),
                      detail: text.episodesHelp,
                    },
                  ]
                : []),
            ]}
          />
        </Block>
      )}
      <Panel title={text.listTitle} description={text.refreshHelp}>
        <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-3">
          {[
            { label: t.iptv.lastFetch, iso: source.fetchedAt, none: t.iptv.never },
            { label: text.lastAttempt, iso: source.checkedAt, none: t.iptv.never },
            { label: t.iptv.nextFetch, iso: source.nextAt, none: '–' },
          ].map((item) => (
            <div key={item.label}>
              <dt className="text-[12.5px] text-ink-3">{item.label}</dt>
              <dd className="mt-1 text-control text-ink">
                {item.iso ? <RelativeTime iso={item.iso} /> : item.none}
              </dd>
            </div>
          ))}
        </dl>
      </Panel>
    </div>
  )
}

function Options({ scope, id, source }: { scope: Scope; id: string; source: IptvSource }) {
  const { t } = useI18n()
  const toast = useToast()
  const text = t.lineup.options
  const [draft, setDraft] = useState<IptvOptions>(source.options)
  const dirty = JSON.stringify(draft) !== JSON.stringify(source.options)
  const save = useMutation({
    mutationFn: () => updateIptvSource(scope, id, { options: draft }),
    onSuccess: (updated) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (list) =>
        list?.map((item) => (item.id === updated.id ? updated : item)),
      )
      if (updated.source !== null) setDraft(updated.source.options)
      // The preview's ticks follow the options saved.
      void queryClient.invalidateQueries({ queryKey: ['lineup', scope, id, 'preview'] })
      toast(text.saved)
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (dirty && optionsValid(draft)) save.mutate()
  }
  return (
    <form onSubmit={submit} noValidate>
      <Panel title={text.title} description={text.help}>
        <SourceOptions
          value={draft}
          saved={source.options}
          onChange={(patch) => {
            save.reset()
            setDraft((current) => ({ ...current, ...patch }))
          }}
          queryKey={['lineup', scope, id, 'preview']}
          load={(by, signal) => previewSource(scope, id, by, signal)}
        />
      </Panel>
      {/* The bar shows while there is something to save: once saved, a toast says so. */}
      {(dirty || save.isPending || save.isError) && (
        <SaveBar
          dirty={dirty}
          saving={save.isPending}
          onDiscard={() => {
            save.reset()
            setDraft(source.options)
          }}
          error={save.isError ? errorMessage(t, save.error) : undefined}
          saveLabel={text.save}
          savingLabel={text.saving}
        />
      )}
    </form>
  )
}
