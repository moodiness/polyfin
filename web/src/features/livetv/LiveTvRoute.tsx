import {
  BroadcastIcon,
  ClockIcon,
  GearIcon,
  PlusIcon,
  RecordIcon,
  TelevisionSimpleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import {
  fetchAddons,
  fetchLibraries,
  fetchTimers,
  queryKeys,
  type Addon,
  type IptvSource,
  type Library,
  type Timer,
} from '@/api'
import { settingsPath } from '@/app/navigation'
import { PageLayout } from '@/app/PageLayout'
import { lineupPath } from '@/features/iptv/lineup'
import { dateTime, errorMessage, formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  ButtonLink,
  EmptyState,
  FigureTile,
  IconTile,
  InlineError,
  Notice,
  RelativeTime,
  Row,
  RowList,
  Skeleton,
  SkeletonRows,
  StatusPill,
  TextLink,
  type StatusTone,
} from '@/ui'
import {
  catalogState,
  failingGuides,
  guideSummary,
  guidesLink,
  isIptv,
  isTvCatalog,
} from './catalogs'
import { TunersShort } from './TunersShort'
import { OwnChannels, PhoneState, WideState } from './OwnChannels'

/**
 * `/live-tv`: the server's TV catalogs with the state of their guides, the channels of its IPTV
 * sources, and the recordings ahead.
 */
export default function LiveTvRoute() {
  const { t } = useI18n()
  const libraries = useQuery({
    queryKey: queryKeys.libraries('shared'),
    queryFn: ({ signal }) => fetchLibraries('shared', signal),
  })
  const addons = useQuery({
    queryKey: queryKeys.addons('shared'),
    queryFn: ({ signal }) => fetchAddons('shared', signal),
  })

  return (
    <PageLayout
      title={t.livetv.title}
      lede={t.livetv.description}
      actions={
        <ButtonLink to={settingsPath('live-tv')} icon={GearIcon}>
          {t.livetv.settings}
        </ButtonLink>
      }
    >
      <Block title={t.livetv.catalogsTitle} count={libraries.data?.filter(isTvCatalog).length}>
        <p className="mb-5 max-w-[72ch] text-small text-ink-2">{t.livetv.catalogsHelp}</p>
        {libraries.isPending ? (
          <SkeletonRows rows={2} label={t.livetv.catalogsLoading} />
        ) : libraries.isError ? (
          <InlineError onRetry={() => void libraries.refetch()} retrying={libraries.isFetching}>
            {errorMessage(t, libraries.error)}
          </InlineError>
        ) : (
          <TvCatalogs libraries={libraries.data} addons={addons.data ?? []} />
        )}
      </Block>

      <Block title={t.livetv.iptvTitle} count={addons.data?.filter(isIptv).length}>
        <p className="mb-5 max-w-[72ch] text-small text-ink-2">{t.livetv.iptvHelp}</p>
        {addons.isPending ? (
          <div role="status" aria-label={t.livetv.iptvLoading} className="space-y-2.5">
            <Skeleton className="h-20 w-full rounded-row" />
            <Skeleton className="h-[120px] w-full rounded-row" />
          </div>
        ) : addons.isError ? (
          <InlineError onRetry={() => void addons.refetch()} retrying={addons.isFetching}>
            {errorMessage(t, addons.error)}
          </InlineError>
        ) : (
          <IptvSources addons={addons.data.filter(isIptv)} />
        )}
      </Block>

      <OwnChannels />

      <Recordings />
    </PageLayout>
  )
}

function TvCatalogs({ libraries, addons }: { libraries: Library[]; addons: Addon[] }) {
  const { language, t } = useI18n()
  // The ones in Live TV first, in the order of the libraries, then the others.
  const catalogs = libraries
    .filter(isTvCatalog)
    .sort((a, b) => Number(b.enabled) - Number(a.enabled))
  if (catalogs.length === 0) {
    return (
      <EmptyState
        icon={TelevisionSimpleIcon}
        title={t.livetv.catalogsEmptyTitle}
        action={
          <ButtonLink to="/sources" variant="primary" icon={PlusIcon}>
            {t.livetv.addSource}
          </ButtonLink>
        }
      >
        {t.livetv.catalogsEmpty}
      </EmptyState>
    )
  }
  const byId = new Map(addons.map((addon) => [addon.id, addon]))
  return (
    <RowList aria-label={t.livetv.catalogsLabel}>
      {catalogs.map((library) => {
        const addon = byId.get(library.addonId)
        const iptv = addon !== undefined && isIptv(addon)
        const name = library.name ?? library.catalogName
        const state = catalogState(t, library, addon?.enabled === false)
        const failing = failingGuides(library)
        const open = library.enabled && library.browsable
        const kind =
          addon?.kind === 'hdhomerun'
            ? t.livetv.kindTuner
            : iptv
              ? t.livetv.kinds.iptv
              : t.livetv.kinds.stremio
        return (
          <Row
            key={`${library.addonId}:${library.catalogId}`}
            leading={
              <IconTile
                icon={iptv ? BroadcastIcon : TelevisionSimpleIcon}
                tone={state.tone === 'danger' || state.tone === 'warn' ? state.tone : undefined}
              />
            }
            to={open ? guidesLink('shared', library, iptv) : undefined}
            title={name}
            muted={!library.enabled}
            meta={
              <>
                <span>
                  {[
                    library.addonName,
                    kind,
                    library.enabled ? guideSummary(t, language, library) : '',
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </span>
                <PhoneState>
                  <StatusPill tone={state.tone}>{state.label}</StatusPill>
                </PhoneState>
              </>
            }
            trailing={
              <WideState>
                <StatusPill tone={state.tone}>{state.label}</StatusPill>
              </WideState>
            }
          >
            {!library.enabled || failing.length > 0 ? (
              <div className="-mt-2 space-y-1.5 text-small text-ink-2">
                {!library.enabled && (
                  <p>
                    {t.livetv.notShownHelp}{' '}
                    <TextLink to="/libraries" arrow={false} className="text-small">
                      {t.livetv.openLibraries}
                    </TextLink>
                  </p>
                )}
                {failing.map((guide) => (
                  <p key={guide.id} className="flex items-start gap-2 text-danger">
                    <WarningCircleIcon size={20} aria-hidden="true" className="mt-px shrink-0" />
                    {t.livetv.guideLine(
                      guide.position,
                      t.lineup.guideErrors[guide.error] ?? t.errors.generic,
                    )}
                  </p>
                ))}
              </div>
            ) : undefined}
          </Row>
        )
      })}
    </RowList>
  )
}

function IptvSources({ addons }: { addons: Addon[] }) {
  const { t } = useI18n()
  if (addons.length === 0) {
    return (
      <EmptyState
        icon={BroadcastIcon}
        title={t.livetv.iptvEmptyTitle}
        action={
          <ButtonLink to="/sources" variant="primary" icon={PlusIcon}>
            {t.livetv.addSource}
          </ButtonLink>
        }
      >
        {t.livetv.iptvEmpty}
      </EmptyState>
    )
  }
  return (
    <div className="space-y-10">
      {addons.map((addon) =>
        addon.source === null ? null : (
          <IptvSourceBlock key={addon.id} addon={addon} source={addon.source} />
        ),
      )}
    </div>
  )
}

/** An IPTV source as a row opening its channels, then the figures of its line-up. */
function IptvSourceBlock({ addon, source }: { addon: Addon; source: IptvSource }) {
  const { language, t } = useI18n()
  const number = (value: number) => value.toLocaleString(language)
  const lineup = source.lineup
  const state: { tone: StatusTone; label: string } = !addon.enabled
    ? { tone: 'muted', label: t.livetv.turnedOff }
    : source.error !== ''
      ? { tone: 'danger', label: t.livetv.downloadFailed }
      : { tone: 'ok', label: t.livetv.active }
  const kind =
    addon.kind === 'hdhomerun'
      ? t.livetv.kindTuner
      : addon.kind === 'xtream'
        ? t.livetv.kindXtream
        : t.livetv.kindM3u
  return (
    <section aria-label={addon.name} className="space-y-2.5">
      <RowList>
        <Row
          leading={
            <IconTile icon={BroadcastIcon} tone={state.tone === 'danger' ? 'danger' : undefined} />
          }
          to={lineupPath('shared', addon.id, source.options.liveTv ? 'channels' : 'options')}
          title={addon.name}
          muted={!addon.enabled}
          meta={
            <>
              <span>
                {kind} · {t.livetv.lastDownload}
                {t.common.colon}{' '}
                {source.fetchedAt === null ? (
                  t.livetv.never
                ) : (
                  <RelativeTime iso={source.fetchedAt} />
                )}
                {source.nextAt !== null && (
                  <>
                    {', '}
                    {t.livetv.nextDownload} <RelativeTime iso={source.nextAt} />
                  </>
                )}
              </span>
              <PhoneState>
                <StatusPill tone={state.tone}>{state.label}</StatusPill>
              </PhoneState>
            </>
          }
          trailing={
            <WideState>
              <StatusPill tone={state.tone}>{state.label}</StatusPill>
            </WideState>
          }
        />
      </RowList>
      {source.error !== '' && (
        <Notice tone="warn">{t.iptv.errors[source.error] ?? t.errors.generic}</Notice>
      )}
      {source.options.liveTv ? (
        <>
          <div
            role="group"
            aria-label={t.livetv.figuresLabel(addon.name)}
            className="grid grid-cols-4 gap-2.5 max-md:grid-cols-2"
          >
            <FigureTile
              value={number(lineup.enabledChannels)}
              total={number(source.channels)}
              label={t.livetv.figures.enabled}
            />
            <FigureTile
              value={number(lineup.shownChannels)}
              total={number(lineup.enabledChannels)}
              label={t.livetv.figures.shown}
            />
            <FigureTile value={number(lineup.mapped)} label={t.livetv.figures.mapped} />
            <FigureTile
              value={number(lineup.unmapped)}
              label={t.livetv.figures.unmapped}
              tone={lineup.unmapped > 0 ? 'warn' : undefined}
            />
          </div>
          {lineup.unmapped > 0 && (
            <div className="px-1.5 pt-1.5">
              <TextLink to={lineupPath('shared', addon.id, 'mapping')}>
                {t.livetv.openMapping}
              </TextLink>
            </div>
          )}
        </>
      ) : (
        <Notice
          action={
            <TextLink to={lineupPath('shared', addon.id, 'options')} arrow={false}>
              {t.livetv.importOptions}
            </TextLink>
          }
        >
          {t.livetv.noLiveTv}
        </Notice>
      )}
    </section>
  )
}

/**
 * The recordings scheduled and under way, and the space their conversion to HEVC saved; read
 * once, as the Schedule page polls them.
 */
function Recordings() {
  const { language, t } = useI18n()
  const timers = useQuery({
    queryKey: queryKeys.timers,
    queryFn: ({ signal }) => fetchTimers(signal),
  })
  return (
    <Block
      title={t.livetv.recordingsTitle}
      count={timers.data?.available ? timers.data.timers.length : undefined}
      aside={
        <TextLink to={settingsPath('recordings')} arrow>
          {t.livetv.recordings}
        </TextLink>
      }
    >
      {timers.isPending ? (
        <SkeletonRows rows={2} label={t.livetv.recordingsLoading} />
      ) : timers.isError ? (
        <InlineError onRetry={() => void timers.refetch()} retrying={timers.isFetching}>
          {errorMessage(t, timers.error)}
        </InlineError>
      ) : !timers.data.available ? (
        <EmptyState icon={RecordIcon} title={t.livetv.recordingsOffTitle}>
          {t.livetv.recordingsOff}
        </EmptyState>
      ) : timers.data.timers.length === 0 ? (
        <EmptyState icon={RecordIcon} title={t.livetv.noTimersTitle}>
          {t.livetv.noTimers}
        </EmptyState>
      ) : (
        <RowList aria-label={t.livetv.recordingsTitle}>
          {timers.data.timers.map((timer) => (
            <TimerRow key={timer.id} timer={timer} />
          ))}
        </RowList>
      )}
      {timers.data?.converted && timers.data.converted.recordings > 0 && (
        <p className="mt-4 text-small text-ink-2">
          {t.livetv.spaceSaved(
            timers.data.converted.recordings,
            formatBytes(timers.data.converted.saved, language),
          )}
        </p>
      )}
    </Block>
  )
}

function TimerRow({ timer }: { timer: Timer }) {
  const { language, t } = useI18n()
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  const state =
    timer.status === 'InProgress' ? (
      <StatusPill tone="live">{t.livetv.recordingNow}</StatusPill>
    ) : timer.status === 'Error' ? (
      <StatusPill tone="danger">{t.livetv.failed}</StatusPill>
    ) : (
      <StatusPill tone="muted" icon={ClockIcon}>
        {t.livetv.scheduled}
      </StatusPill>
    )
  return (
    <Row
      leading={RecordIcon}
      title={timer.name}
      titleAside={
        timer.series || timer.tunersShort ? (
          <span className="flex shrink-0 gap-1">
            {timer.series && <Badge>{t.livetv.series}</Badge>}
            {timer.tunersShort && <TunersShort text={t.livetv} />}
          </span>
        ) : undefined
      }
      meta={
        <>
          <span className="min-w-0 break-words">
            {timer.channel || t.livetv.unknownChannel} · {t.livetv.scheduledBy(timer.userName)}
          </span>
          <PhoneState>{state}</PhoneState>
        </>
      }
      trailing={
        <>
          <time dateTime={timer.from} className="figures text-ink">
            {dateTime(timer.from, language)} – {time.format(new Date(timer.until))}
          </time>
          <WideState>{state}</WideState>
        </>
      }
    />
  )
}
