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
import { lineupPath } from '@/components/lineup/common'
import { dateTime, errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  ButtonLink,
  cx,
  EmptyState,
  IconTile,
  InlineError,
  Notice,
  Panel,
  Row,
  RowList,
  Skeleton,
  SkeletonRows,
  StatusPill,
  TextLink,
} from '@/ui'
import {
  catalogState,
  failingGuides,
  guideSummary,
  guidesLink,
  isIptv,
  isTvCatalog,
} from './catalogs'
import { RelativeTime } from './RelativeTime'

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
          <div role="status" aria-label={t.livetv.iptvLoading}>
            <Skeleton className="h-[188px] w-full rounded-panel" />
          </div>
        ) : addons.isError ? (
          <InlineError onRetry={() => void addons.refetch()} retrying={addons.isFetching}>
            {errorMessage(t, addons.error)}
          </InlineError>
        ) : (
          <IptvSources addons={addons.data.filter(isIptv)} />
        )}
      </Block>

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
        const guidesButton = (
          <ButtonLink
            size="sm"
            to={guidesLink('shared', library, iptv)}
            aria-label={iptv ? t.livetv.lineupLabel(name) : t.livetv.manageLabel(name)}
          >
            {iptv ? t.livetv.lineup : t.livetv.manage}
          </ButtonLink>
        )
        return (
          <Row
            key={`${library.addonId}:${library.catalogId}`}
            leading={iptv ? BroadcastIcon : TelevisionSimpleIcon}
            title={name}
            muted={!library.enabled}
            meta={
              <span className="truncate">
                {library.addonName} · {iptv ? t.livetv.kinds.iptv : t.livetv.kinds.stremio}
              </span>
            }
            trailing={
              <div className="flex items-center gap-4">
                <StatusPill tone={state.tone}>{state.label}</StatusPill>
                {open && <span className="max-sm:hidden">{guidesButton}</span>}
              </div>
            }
          >
            <div className="-mt-2 space-y-1.5 pr-[18px] pl-3 text-small text-ink-3 max-sm:px-4">
              {library.enabled ? (
                <p>{guideSummary(t, language, library)}</p>
              ) : (
                <p>
                  {t.livetv.notShownHelp}{' '}
                  <TextLink to="/libraries" arrow={false} className="text-small">
                    {t.livetv.openLibraries}
                  </TextLink>
                </p>
              )}
              {failing.map((guide) => (
                <p key={guide.id} className="flex items-start gap-1.5 text-danger">
                  <WarningCircleIcon size={14} aria-hidden="true" className="mt-0.5 shrink-0" />
                  {t.livetv.guideLine(
                    guide.position,
                    t.libraries.guideErrors[guide.error] ?? t.errors.generic,
                  )}
                </p>
              ))}
              {open && <div className="pt-1.5 sm:hidden">{guidesButton}</div>}
            </div>
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
    <div className="space-y-4">
      {addons.map((addon) =>
        addon.source === null ? null : (
          <IptvSourcePanel key={addon.id} addon={addon} source={addon.source} />
        ),
      )}
    </div>
  )
}

function IptvSourcePanel({ addon, source }: { addon: Addon; source: IptvSource }) {
  const { language, t } = useI18n()
  const number = (value: number) => value.toLocaleString(language)
  const lineup = source.lineup
  const figures = [
    { label: t.livetv.figures.entries, value: source.channels },
    { label: t.livetv.figures.enabled, value: lineup.enabledChannels },
    { label: t.livetv.figures.shown, value: lineup.shownChannels },
    { label: t.livetv.figures.mapped, value: lineup.mapped },
    { label: t.livetv.figures.unmapped, value: lineup.unmapped, warn: lineup.unmapped > 0 },
  ]
  return (
    <Panel
      title={addon.name}
      titleAs="h3"
      media={<IconTile icon={BroadcastIcon} />}
      description={
        <>
          {addon.kind === 'xtream' ? t.livetv.kindXtream : t.livetv.kindM3u} ·{' '}
          {t.livetv.lastDownload}
          {t.common.colon}{' '}
          {source.fetchedAt === null ? t.livetv.never : <RelativeTime iso={source.fetchedAt} />}
          {source.nextAt !== null && (
            <>
              {', '}
              {t.livetv.nextDownload} <RelativeTime iso={source.nextAt} />
            </>
          )}
        </>
      }
      actions={
        <>
          {!addon.enabled && <StatusPill tone="muted">{t.livetv.turnedOff}</StatusPill>}
          {source.options.liveTv && (
            <ButtonLink
              size="sm"
              to={lineupPath('shared', addon.id, 'channels')}
              aria-label={t.livetv.channelsLabel(addon.name)}
              className="max-sm:hidden"
            >
              {t.livetv.channels}
            </ButtonLink>
          )}
        </>
      }
    >
      <div className="space-y-5">
        {source.error !== '' && (
          <Notice tone="warn">{t.livetv.iptvErrors[source.error] ?? t.errors.generic}</Notice>
        )}
        {source.options.liveTv ? (
          <>
            <dl
              aria-label={t.livetv.figuresLabel(addon.name)}
              className="grid grid-cols-5 gap-y-5 border-t border-line pt-5 max-md:grid-cols-3 max-sm:grid-cols-2"
            >
              {figures.map((figure) => (
                <div
                  key={figure.label}
                  className="min-w-0 md:border-l md:border-line md:px-5 md:first:border-l-0 md:first:pl-0"
                >
                  <dt className="truncate text-small text-ink-3">{figure.label}</dt>
                  <dd
                    className={cx(
                      'figures mt-1.5 text-h2 font-medium',
                      figure.warn ? 'text-warn' : 'text-ink',
                    )}
                  >
                    {number(figure.value)}
                  </dd>
                </div>
              ))}
            </dl>
            <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
              <TextLink to={lineupPath('shared', addon.id, 'channels')} className="sm:hidden">
                {t.livetv.channels}
              </TextLink>
              {lineup.unmapped > 0 && (
                <TextLink to={lineupPath('shared', addon.id, 'mapping')}>
                  {t.livetv.openMapping}
                </TextLink>
              )}
            </div>
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
      </div>
    </Panel>
  )
}

/** The recordings scheduled and under way; read once, as the Schedule page polls them. */
function Recordings() {
  const { t } = useI18n()
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
    </Block>
  )
}

function TimerRow({ timer }: { timer: Timer }) {
  const { language, t } = useI18n()
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  return (
    <Row
      leading={RecordIcon}
      title={timer.name}
      titleAside={timer.series ? <Badge>{t.livetv.series}</Badge> : undefined}
      meta={
        <span className="truncate">
          {timer.channel || t.livetv.unknownChannel} · {t.livetv.scheduledBy(timer.userName)}
        </span>
      }
      trailing={
        <>
          <time dateTime={timer.from} className="figures text-small text-ink">
            {dateTime(timer.from, language)} – {time.format(new Date(timer.until))}
          </time>
          {timer.status === 'InProgress' ? (
            <StatusPill tone="live">{t.livetv.recordingNow}</StatusPill>
          ) : timer.status === 'Error' ? (
            <StatusPill tone="danger">{t.livetv.failed}</StatusPill>
          ) : (
            <StatusPill tone="muted" icon={ClockIcon}>
              {t.livetv.scheduled}
            </StatusPill>
          )}
        </>
      }
    />
  )
}
