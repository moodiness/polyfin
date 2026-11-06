import { BroadcastIcon, MagnifyingGlassIcon } from '@phosphor-icons/react'
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Link } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchAddons,
  fetchLineupChannels,
  previewSource,
  queryClient,
  queryKeys,
  scanTuner,
  updateIptvSource,
  type Addon,
  type IptvOptions,
  type IptvSource as Source,
  type IptvTuner,
  type Scope,
} from '@/api'
import { invalidateLineup, iptvCatalog, lineupPath } from '@/features/iptv/lineup'
import { Fact, Facts, Group } from '@/features/sources/parts'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  AlertTile,
  Button,
  ButtonLink,
  cx,
  EmptyState,
  FieldError,
  FigureTile,
  InlineError,
  Notice,
  ProgressBar,
  RelativeTime,
  Row,
  RowList,
  SaveBar,
  SettingRow,
  SkeletonRows,
  StatusPill,
  Tabs,
  TextLink,
  useToast,
} from '@/ui'
import CatalogGuides from './CatalogGuides'
import Categories from './Categories'
import Channels from './Channels'
import { optionsChanges, optionsValid, SourceOptions } from './ImportOptions'
import Mappings from './Mappings'
import { useNumber } from './shared'

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

/** Whether an address names one of the six parts (none names the Summary). */
export const isLineupSection = (section: string | undefined) =>
  section === undefined || (lineupSections as readonly string[]).includes(section)

/** How many channels the Summary lists before leading to Channels. */
const summaryChannels = 6

/**
 * One IPTV source's page under its head, part by part: its six words (Summary, Import options,
 * Categories, Channels, Guides, Guide mapping), then the part the address names. The Summary
 * holds its problem, its figures, its first channels, `forWhom` (Enabled, Used by, Order) and
 * its lists.
 */
export function IptvSource({
  scope,
  addon: given,
  section: named,
  forWhom,
  retry,
}: {
  scope: Scope
  addon: Addon
  section?: string
  forWhom: ReactNode
  /** The button that downloads the lists again, under a failed download's alert. */
  retry?: ReactNode
}) {
  const { t } = useI18n()
  const id = given.id
  // A tuner's scan under way is followed until it ends.
  const addons = useQuery({
    queryKey: queryKeys.addons(scope),
    queryFn: ({ signal }) => fetchAddons(scope, signal),
    refetchInterval: (query) =>
      query.state.data?.some((item) => item.id === id && item.source?.tuner?.scan) ? 2000 : false,
  })
  const addon = addons.data?.find((item) => item.id === id && item.source !== null) ?? given
  const source = addon.source
  if (source === null) return null
  const section = (named ?? 'summary') as LineupSection
  const liveTv = source.options.liveTv

  return (
    <>
      <p className="mt-[18px] max-w-[64ch] text-[18.5px] text-ink-2 max-md:text-[16.5px]">
        {t.lineup.description}
      </p>
      <Tabs
        variant="underline"
        label={t.lineup.sectionsLabel}
        className="mt-[18px]"
        items={lineupSections
          .filter((name) => liveTv || !liveSections.has(name))
          .map((name) => ({
            id: name,
            label: t.lineup.sections[name],
            to: lineupPath(scope, id, name === 'summary' ? undefined : name),
            end: true,
          }))}
      />
      <div className="mt-7">
        {section === 'summary' ? (
          <Summary scope={scope} addon={addon} source={source} forWhom={forWhom} retry={retry} />
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
    </>
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

/** A figure that opens the part it counts, lit on focus. */
function FigureLink({ to, children }: { to?: string; children: ReactNode }) {
  if (to === undefined) return children
  return (
    <Link
      to={to}
      className="focus-lit block rounded-row [--lift:1.02] hover:[&>div]:bg-s2 focus-visible:[&>div]:bg-transparent [&>div]:h-full"
    >
      {children}
    </Link>
  )
}

function Summary({
  scope,
  addon,
  source,
  forWhom,
  retry,
}: {
  scope: Scope
  addon: Addon
  source: Source
  forWhom: ReactNode
  retry?: ReactNode
}) {
  const { t } = useI18n()
  const number = useNumber()
  const text = t.lineup.summary
  const lineup = source.lineup
  const vod = source.vod
  const { liveTv, movies, series } = source.options
  const vodPath = scope === 'shared' ? '/libraries' : '/me/sources'
  const figures = 'grid grid-cols-4 gap-2.5 max-lg:grid-cols-2'
  return (
    <div>
      {source.error !== '' && (
        <AlertTile
          tone="danger"
          title={t.iptv.errors[source.error] ?? t.errors.generic}
          className="mb-13 max-md:mb-10"
          actions={retry}
        >
          {source.checkedAt !== null && (
            <>
              {text.lastAttempt}: <RelativeTime iso={source.checkedAt} />
            </>
          )}
        </AlertTile>
      )}
      {source.tuner !== null && (
        <TunerPart scope={scope} id={addon.id} source={source} tuner={source.tuner} />
      )}
      {liveTv ? (
        <Group title={t.lineup.content.liveTitle} className="first:mt-0">
          <div className={figures}>
            <FigureLink to={lineupPath(scope, addon.id, 'categories')}>
              <FigureTile
                value={number(lineup.enabledCategories)}
                total={number(lineup.categories)}
                label={text.categories}
              />
            </FigureLink>
            <FigureLink to={lineupPath(scope, addon.id, 'channels')}>
              <FigureTile
                value={number(lineup.shownChannels)}
                total={number(lineup.channels)}
                label={text.shown}
              />
            </FigureLink>
            <FigureLink to={lineupPath(scope, addon.id, 'mapping')}>
              <FigureTile
                value={number(lineup.mapped)}
                label={text.mapped}
                tone={lineup.unmapped > 0 ? 'warn' : undefined}
              />
            </FigureLink>
            <FigureTile value={number(source.channels)} label={text.entries} />
          </div>
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
          {lineup.archived > 0 && (
            <Notice
              className="mt-5"
              action={
                <TextLink to={`${lineupPath(scope, addon.id, 'channels')}?archive=yes`}>
                  {t.lineup.openArchived}
                </TextLink>
              }
            >
              {t.lineup.archivedNotice(lineup.archived, number(lineup.archived))}
            </Notice>
          )}
        </Group>
      ) : (
        <LiveOff scope={scope} id={addon.id} />
      )}
      {liveTv && <SummaryChannels scope={scope} id={addon.id} total={lineup.channels} />}
      {(movies || series) && (
        <Group title={text.vodTitle}>
          <div className={figures}>
            {movies && (
              <FigureLink to={vodPath}>
                <FigureTile
                  value={number(vod.shownMovies)}
                  total={number(vod.movies)}
                  label={text.movies}
                />
              </FigureLink>
            )}
            {series && (
              <>
                <FigureLink to={vodPath}>
                  <FigureTile
                    value={number(vod.shownSeries)}
                    total={number(vod.series)}
                    label={text.series}
                  />
                </FigureLink>
                <FigureTile value={number(vod.episodes)} label={text.episodes} />
              </>
            )}
          </div>
        </Group>
      )}
      {forWhom}
      <Group title={text.listTitle}>
        <Facts>
          {[
            { label: t.iptv.lastFetch, iso: source.fetchedAt, none: t.iptv.never },
            { label: text.lastAttempt, iso: source.checkedAt, none: t.iptv.never },
            { label: t.iptv.nextFetch, iso: source.nextAt, none: '–' },
          ].map((item) => (
            <Fact key={item.label} label={item.label}>
              {item.iso ? <RelativeTime iso={item.iso} /> : item.none}
            </Fact>
          ))}
          <Fact label={t.iptv.address} wide>
            <span className="font-mono text-body break-all">{source.address}</span>
          </Fact>
        </Facts>
        <p className="mt-4 max-w-[66ch] px-2 text-small text-ink-3">{text.refreshHelp}</p>
      </Group>
    </div>
  )
}

/**
 * The source's first channels, in their order, as compact rows that open Channels: number, name,
 * category, and whether apps show it and it has a guide. The line-up holds no programmes, so what
 * is on now is not shown.
 */
function SummaryChannels({ scope, id, total }: { scope: Scope; id: string; total: number }) {
  const { t } = useI18n()
  const number = useNumber()
  const text = t.lineup.summary
  const channels = useQuery({
    queryKey: [...queryKeys.lineup(scope, id), 'channels', {}, 0, summaryChannels],
    queryFn: ({ signal }) => fetchLineupChannels(scope, id, {}, 0, summaryChannels, signal),
  })
  const to = lineupPath(scope, id, 'channels')
  return (
    <Group
      title={t.lineup.sections.channels}
      aside={total > 0 && <TextLink to={to}>{text.allChannels(number(total))}</TextLink>}
    >
      {channels.isPending ? (
        <SkeletonRows rows={3} />
      ) : channels.isError ? (
        <InlineError onRetry={() => void channels.refetch()} retrying={channels.isFetching}>
          {errorMessage(t, channels.error)}
        </InlineError>
      ) : channels.data.items.length === 0 ? (
        <p className="text-lead text-ink-2">{t.lineup.channelsEmpty}</p>
      ) : (
        <RowList compact aria-label={t.lineup.sections.channels}>
          {channels.data.items.map((channel, index) => {
            const state = !channel.shown ? (
              <StatusPill tone="muted">{text.hidden}</StatusPill>
            ) : channel.mapping === null ? (
              <StatusPill tone="warn">{text.noGuide}</StatusPill>
            ) : (
              <StatusPill tone="ok">{text.mapped}</StatusPill>
            )
            return (
              <Row
                key={channel.id}
                to={to}
                leading={
                  <span className="figures w-9 shrink-0 text-control font-semibold text-ink-3">
                    {channel.number ?? number(index + 1)}
                  </span>
                }
                title={channel.name}
                // On a phone the state goes under the name, which keeps the room it needs.
                meta={
                  <>
                    <span>{channel.category.name}</span>
                    <span className="basis-full md:hidden">{state}</span>
                  </>
                }
                muted={!channel.shown}
                trailing={<span className="max-md:hidden">{state}</span>}
              />
            )
          })}
        </RowList>
      )}
    </Group>
  )
}

/**
 * A tuner's own part of its summary: its last failure, its model, device and tuner count, the
 * choice of its favorite channels only, and its channel scan with its progress.
 */
function TunerPart({
  scope,
  id,
  source,
  tuner,
}: {
  scope: Scope
  id: string
  source: Source
  tuner: IptvTuner
}) {
  const { t } = useI18n()
  const toast = useToast()
  const number = useNumber()
  const text = t.lineup.tuner
  const favorites = useMutation({
    mutationFn: (favoritesOnly: boolean) =>
      updateIptvSource(scope, id, { options: { favoritesOnly } }),
    onSuccess: (updated) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (list) =>
        list?.map((item) => (item.id === updated.id ? updated : item)),
      )
      toast(text.favoritesSaved(updated.source?.options.favoritesOnly ?? false))
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  const scan = useMutation({
    mutationFn: () => scanTuner(id),
    onSuccess: (updated) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (list) =>
        list?.map((item) => (item.id === updated.id ? updated : item)),
      )
      toast(text.scanStarted)
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  // The page follows the scan while it runs; its end is told once, and the line-up read again.
  const scanning = tuner.scan !== null
  const wasScanning = useRef(scanning)
  useEffect(() => {
    if (wasScanning.current && !scanning) {
      toast(text.scanDone)
      invalidateLineup(scope, id)
    }
    wasScanning.current = scanning
  }, [scanning, scope, id, toast, text])

  return (
    <Group
      title={text.title}
      className="first:mt-0"
      aside={
        <Button
          icon={MagnifyingGlassIcon}
          loading={scan.isPending || scanning}
          title={text.scanHelp}
          onClick={() => {
            favorites.reset()
            scan.mutate()
          }}
        >
          {scanning ? text.scanning : text.scan}
        </Button>
      }
    >
      <p className="mb-5 max-w-[66ch] text-small text-ink-2">{text.help}</p>
      {tuner.failure !== '' && (
        <AlertTile
          tone="warn"
          title={tuner.failure === 'unreachable' ? text.unreachable : text.failed}
          className="mb-5"
        >
          {tuner.failedAt !== null && <RelativeTime iso={tuner.failedAt} />}
        </AlertTile>
      )}
      <Facts>
        <Fact label={text.model}>{tuner.model || '–'}</Fact>
        <Fact label={text.tuners}>
          {number(tuner.tuners)}
          <span className="block text-small text-ink-3">{text.tunersHelp}</span>
        </Fact>
        <Fact label={text.device}>
          <span className="break-all">{tuner.device}</span>
        </Fact>
      </Facts>
      {tuner.scan !== null && (
        <div className="mt-5 space-y-2 rounded-row bg-s1 px-7 py-5 max-md:px-5">
          <ProgressBar
            value={tuner.scan.progress / 100}
            label={text.scanProgress}
            valueText={text.scanFound(number(tuner.scan.progress), number(tuner.scan.found))}
            size="lg"
          />
          <p className="text-small text-ink-2" role="status">
            {text.scanFound(number(tuner.scan.progress), number(tuner.scan.found))}
          </p>
        </div>
      )}
      <p className="mt-4 max-w-[66ch] px-2 text-small text-ink-3">{text.scanHelp}</p>
      <div className={cx('mt-5 grid grid-cols-[minmax(0,1fr)] gap-2')}>
        <SettingRow
          kind="switch"
          label={t.sourceAdd.favoritesOnly}
          help={t.sourceAdd.favoritesOnlyHelp}
          checked={
            favorites.isPending
              ? (favorites.variables ?? source.options.favoritesOnly)
              : source.options.favoritesOnly
          }
          disabled={favorites.isPending}
          error={favorites.isError ? errorMessage(t, favorites.error) : undefined}
          onChange={(on) => {
            scan.reset()
            favorites.mutate(on)
          }}
        />
      </div>
      {scan.isError && (
        <div className="mt-4">
          <FieldError>{errorMessage(t, scan.error)}</FieldError>
        </div>
      )}
    </Group>
  )
}

function Options({ scope, id, source }: { scope: Scope; id: string; source: Source }) {
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
      <p className="mb-8 max-w-[66ch] text-small text-ink-2">{text.help}</p>
      <SourceOptions
        value={draft}
        saved={source.options}
        tuner={source.tuner !== null}
        onChange={(patch) => {
          save.reset()
          setDraft((current) => ({ ...current, ...patch }))
        }}
        queryKey={['lineup', scope, id, 'preview']}
        load={(by, signal) => previewSource(scope, id, by, signal)}
      />
      <SaveBar
        dirty={dirty}
        changes={optionsChanges(draft, source.options)}
        saving={save.isPending}
        onDiscard={() => {
          save.reset()
          setDraft(source.options)
        }}
        error={save.isError ? errorMessage(t, save.error) : undefined}
        saveLabel={text.save}
        savingLabel={text.saving}
      />
    </form>
  )
}
