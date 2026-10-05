import { useEffect } from 'react'
import { Link, useLocation } from 'react-router'
import { useMutation } from '@tanstack/react-query'
import {
  ArrowsClockwiseIcon,
  CalendarDotsIcon,
  PuzzlePieceIcon,
  TelevisionSimpleIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react'
import {
  checkAddon,
  queryClient,
  queryKeys,
  type AddonHealth,
  type Health,
  type Sources,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import {
  Block,
  Button,
  EmptyState,
  IconTile,
  InlineError,
  Notice,
  Panel,
  Row,
  RowList,
  Skeleton,
  SkeletonRows,
  SkeletonText,
  StatusPill,
  type StatusTone,
  RelativeTime,
} from '@/ui'
import { MusicBadge } from '@/features/sources/AddonSettings'
import { lineupPath } from '@/features/iptv/lineup'
import { findProblems, lowOnSpace, unreadableName, useHealthData } from '@/features/system/problems'
import { errorMessage, formatBytes, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'
import BackupStatus from './BackupStatus'
import OwnerChip from './OwnerChip'
import { Anchor, Facts, Meter } from './parts'

/** `/system/health`: problems first, then the state of each part of the server. */
export default function HealthRoute() {
  const { t } = useI18n()
  const text = t.system.health
  const { health, sources, tasks } = useHealthData()
  const location = useLocation()
  const loaded = health.data !== undefined

  // A link to a section of this page scrolls to it once the page has drawn it.
  useEffect(() => {
    if (loaded && location.hash) {
      document.getElementById(location.hash.slice(1))?.scrollIntoView()
    }
  }, [loaded, location.hash])

  return (
    <PageLayout title={text.title} lede={text.description}>
      {health.isPending ? (
        <HealthSkeleton />
      ) : health.isError ? (
        <InlineError onRetry={() => void health.refetch()} retrying={health.isFetching}>
          {errorMessage(t, health.error)}
        </InlineError>
      ) : (
        <>
          <Problems health={health.data} sources={sources.data} tasks={tasks.data} />
          <Addons health={health.data} sources={sources.data} />
          <div className="grid gap-x-8 gap-y-block xl:grid-cols-2">
            <Iptv sources={sources.data} />
            <Guides sources={sources.data} />
          </div>
          <Block title={text.serverTitle}>
            <div className="grid items-start gap-5 lg:grid-cols-2">
              <Transcoder health={health.data} />
              <Database health={health.data} />
              <Cache health={health.data} />
              <Disks health={health.data} />
              <Backups health={health.data} />
              <Thumbnails health={health.data} />
              <Process health={health.data} />
              <Secrets health={health.data} />
            </div>
          </Block>
        </>
      )}
    </PageLayout>
  )
}

function HealthSkeleton() {
  return (
    <>
      <div>
        <Skeleton className="mb-4 h-4 w-40" />
        <SkeletonRows rows={2} />
      </div>
      <div>
        <Skeleton className="mb-4 h-4 w-28" />
        <SkeletonRows rows={3} />
      </div>
      <div className="grid gap-5 lg:grid-cols-2">
        {[0, 1, 2, 3].map((index) => (
          <div key={index} className="rounded-panel border border-line-2 bg-s1 p-6">
            <Skeleton className="mb-5 h-4 w-32" />
            <SkeletonText lines={3} />
          </div>
        ))}
      </div>
    </>
  )
}

function Problems({
  health,
  sources,
  tasks,
}: {
  health: Health
  sources: Parameters<typeof findProblems>[3]
  tasks: Parameters<typeof findProblems>[4]
}) {
  const { language, t } = useI18n()
  const text = t.system.health
  const problems = findProblems(t, language, health, sources, tasks)
  const checked = text.checkedAt(
    new Intl.DateTimeFormat(language, { timeStyle: 'medium' }).format(new Date(health.checkedAt)),
  )
  return (
    <Block
      title={text.problemsTitle}
      count={problems.length > 0 ? problems.length : undefined}
      aside={<span className="tabular-nums">{checked}</span>}
    >
      {problems.length === 0 ? (
        <Notice tone="ok">{text.allGood}</Notice>
      ) : (
        <RowList aria-label={text.problemsTitle}>
          {problems.map((problem) => (
            <Row
              key={problem.text}
              to={problem.to}
              leading={
                <IconTile
                  icon={problem.tone === 'error' ? WarningCircleIcon : WarningIcon}
                  tone="warn"
                />
              }
              title={<span className="whitespace-normal">{problem.text}</span>}
              trailing={
                problem.tone === 'error' ? (
                  <StatusPill tone="danger">{text.error}</StatusPill>
                ) : (
                  <StatusPill tone="warn">{text.warning}</StatusPill>
                )
              }
              className="[&_.truncate]:whitespace-normal"
            />
          ))}
        </RowList>
      )}
    </Block>
  )
}

function addonTone(
  addon: AddonHealth,
): [StatusTone, keyof Messages['system']['health']['addonStatus']] {
  if (!addon.enabled) return ['muted', 'off']
  if (addon.requests === 0) return ['muted', 'unknown']
  return addon.failure === '' ? ['ok', 'ok'] : ['warn', 'failing']
}

function Addons({ health, sources }: { health: Health; sources: Sources | undefined }) {
  const { t } = useI18n()
  const text = t.system.health
  const check = useMutation({
    mutationFn: checkAddon,
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.health }),
  })
  // Music addons' answers are recorded like the others': their row carries their badge.
  const music = new Map(
    (sources?.addons ?? [])
      .filter((addon) => addon.music !== null)
      .map((addon) => [addon.id, addon.music]),
  )

  return (
    <Anchor id="addons">
      <Block title={text.addonsTitle} count={health.addons.length}>
        <p className="-mt-2 mb-4 max-w-[70ch] text-small text-ink-3">{text.addonsHelp}</p>
        {check.isError && (
          <InlineError className="mb-3">{errorMessage(t, check.error)}</InlineError>
        )}
        {health.addons.length === 0 ? (
          <EmptyState icon={PuzzlePieceIcon} title={text.noAddons}>
            {text.noAddonsHint}
          </EmptyState>
        ) : (
          <RowList aria-label={text.addonsTitle}>
            {health.addons.map((addon) => {
              const [tone, status] = addonTone(addon)
              const musicOf = music.get(addon.id)
              const checking = check.isPending && check.variables === addon.id
              return (
                <Row
                  key={addon.id}
                  leading={PuzzlePieceIcon}
                  muted={!addon.enabled}
                  title={addon.name}
                  titleAside={
                    <span className="flex shrink-0 items-center gap-1.5">
                      {musicOf && <MusicBadge music={musicOf} />}
                      <OwnerChip owner={addon.owner} />
                    </span>
                  }
                  meta={
                    <span className="truncate">
                      <StatusPill tone={tone} className="align-middle">
                        {text.addonStatus[status]}
                      </StatusPill>
                      {addon.failure !== '' && addon.enabled && (
                        <span> · {text.failures[addon.failure] ?? addon.failure}</span>
                      )}
                    </span>
                  }
                  trailing={
                    <Button
                      size="sm"
                      icon={ArrowsClockwiseIcon}
                      aria-label={text.checkLabel(addon.name)}
                      disabled={!addon.enabled}
                      loading={checking}
                      onClick={() => check.mutate(addon.id)}
                    >
                      {checking ? text.checking : text.check}
                    </Button>
                  }
                >
                  <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-small tabular-nums sm:grid-cols-4">
                    <SmallFact label={text.lastSuccess}>
                      {addon.lastSuccessAt ? <RelativeTime iso={addon.lastSuccessAt} /> : '–'}
                    </SmallFact>
                    <SmallFact label={text.lastFailure}>
                      {addon.lastFailureAt ? <RelativeTime iso={addon.lastFailureAt} /> : '–'}
                    </SmallFact>
                    <SmallFact label={text.responseTime}>
                      {addon.requests > 0 && addon.lastSuccessAt ? `${addon.responseTime} ms` : '–'}
                    </SmallFact>
                    <p className="col-span-full text-ink-3 sm:col-span-1 sm:self-end">
                      {text.requests(addon.requests, addon.failures)}
                    </p>
                  </dl>
                </Row>
              )
            })}
          </RowList>
        )}
      </Block>
    </Anchor>
  )
}

function SmallFact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-ink-3">{label}</dt>
      <dd className="text-ink">{children}</dd>
    </div>
  )
}

function Iptv({ sources: all }: { sources: Sources | undefined }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const sources = (all?.addons ?? []).filter((addon) => addon.source !== null)
  return (
    <Anchor id="iptv">
      <Block title={text.iptvTitle} count={all ? sources.length : undefined}>
        {all === undefined ? (
          <SkeletonRows rows={1} />
        ) : sources.length === 0 ? (
          <EmptyState icon={TelevisionSimpleIcon} title={text.noIptv}>
            {text.noIptvHint}
          </EmptyState>
        ) : (
          <RowList aria-label={text.iptvTitle}>
            {sources.map((addon) => {
              const source = addon.source!
              const vod = source.options.movies || source.options.series
              return (
                <Row
                  key={addon.id}
                  leading={TelevisionSimpleIcon}
                  muted={!addon.enabled}
                  title={addon.name}
                  titleAside={<OwnerChip owner={addon.owner} />}
                  trailing={
                    !addon.enabled ? (
                      <StatusPill tone="muted">{text.addonStatus.off}</StatusPill>
                    ) : source.error !== '' ? (
                      <StatusPill tone="warn">{text.addonStatus.failing}</StatusPill>
                    ) : source.fetchedAt ? (
                      <StatusPill tone="ok">{text.ok}</StatusPill>
                    ) : (
                      <StatusPill tone="muted">{t.system.schedule.notYet}</StatusPill>
                    )
                  }
                >
                  <div className="space-y-1 text-small text-ink-3">
                    {(source.options.liveTv || vod) && (
                      <p className="tabular-nums">
                        {source.options.liveTv && (
                          <>
                            {t.lineup.summary.shownOf(
                              source.lineup.shownChannels.toLocaleString(language),
                              source.lineup.channels.toLocaleString(language),
                            )}
                            {' · '}
                            {t.lineup.summary.mappedOf(
                              source.lineup.mapped.toLocaleString(language),
                              source.lineup.channels.toLocaleString(language),
                            )}
                          </>
                        )}
                        {source.options.liveTv && vod && ' · '}
                        {vod &&
                          t.lineup.summary.vodShort(
                            source.vod.shownMovies.toLocaleString(language),
                            source.vod.shownSeries.toLocaleString(language),
                          )}
                      </p>
                    )}
                    <p>
                      {t.iptv.lastFetch}
                      {t.common.colon}{' '}
                      {source.fetchedAt ? <RelativeTime iso={source.fetchedAt} /> : t.iptv.never}
                      {source.nextAt && (
                        <>
                          {' · '}
                          {t.iptv.nextFetch}
                          {t.common.colon} <RelativeTime iso={source.nextAt} />
                        </>
                      )}
                    </p>
                    {source.error !== '' && (
                      <p className="flex items-start gap-1.5 text-warn">
                        <WarningIcon size={14} aria-hidden="true" className="mt-0.5 shrink-0" />
                        {t.iptv.errors[source.error] ?? source.error}
                      </p>
                    )}
                    {addon.owner === null && (
                      <p>
                        <Link
                          to={lineupPath('shared', addon.id)}
                          className="rounded-md font-medium text-link hover:text-link-hover"
                        >
                          {t.lineup.open}
                        </Link>
                      </p>
                    )}
                  </div>
                </Row>
              )
            })}
          </RowList>
        )}
      </Block>
    </Anchor>
  )
}

function Guides({ sources }: { sources: Sources | undefined }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const catalogs = (sources?.guides ?? []).filter((library) => (library.guides ?? []).length > 0)
  return (
    <Anchor id="guides">
      <Block title={text.guidesTitle} count={sources ? catalogs.length : undefined}>
        {sources === undefined ? (
          <SkeletonRows rows={1} />
        ) : catalogs.length === 0 ? (
          <EmptyState icon={CalendarDotsIcon} title={text.noGuides}>
            {text.noGuidesHint}
          </EmptyState>
        ) : (
          <RowList aria-label={text.guidesTitle}>
            {catalogs.map((library) => {
              const guides = library.guides ?? []
              const failing = guides.filter((guide) => guide.error !== '')
              const fetched = guides
                .map((guide) => guide.fetchedAt)
                .filter((at) => at !== null)
                .sort()
                .at(-1)
              return (
                <Row
                  key={`${library.addonId}-${library.catalogType}-${library.catalogId}`}
                  leading={CalendarDotsIcon}
                  title={library.name ?? library.catalogName}
                  titleAside={<OwnerChip owner={library.owner} />}
                  meta={
                    <span className="truncate">
                      {library.addonName} · {t.lineup.library.guides(guides.length)}
                    </span>
                  }
                  trailing={
                    failing.length > 0 ? (
                      <StatusPill tone="warn">{text.addonStatus.failing}</StatusPill>
                    ) : fetched ? (
                      <StatusPill tone="ok">{text.ok}</StatusPill>
                    ) : (
                      <StatusPill tone="muted">{t.system.schedule.notYet}</StatusPill>
                    )
                  }
                >
                  <div className="space-y-1 text-small text-ink-3">
                    <p className="tabular-nums">
                      {t.libraries.guideFetched}
                      {t.common.colon}{' '}
                      {fetched ? <RelativeTime iso={fetched} /> : t.libraries.guideNever}
                      {library.guide !== null && library.guide.channels > 0 && (
                        <>
                          {' · '}
                          {t.lineup.library.mapped(
                            library.guide.matched.toLocaleString(language),
                            library.guide.channels.toLocaleString(language),
                          )}
                        </>
                      )}
                    </p>
                    {failing.map((guide) => (
                      <p key={guide.id || guide.url} className="flex items-start gap-1.5 text-warn">
                        <WarningIcon size={14} aria-hidden="true" className="mt-0.5 shrink-0" />
                        <span>
                          {t.lineup.picker.guideN(guide.position)}
                          {t.common.colon} {t.lineup.guideErrors[guide.error] ?? guide.error}
                        </span>
                      </p>
                    ))}
                  </div>
                </Row>
              )
            })}
          </RowList>
        )}
      </Block>
    </Anchor>
  )
}

function Transcoder({ health }: { health: Health }) {
  const { t } = useI18n()
  const text = t.system.health
  const transcoder = health.transcoder
  if (transcoder === null) return null
  const hardware = transcoder.hardware
  const full = transcoder.limit > 0 && transcoder.conversions >= transcoder.limit
  return (
    <Anchor id="transcoder">
      <Panel title={text.transcoderTitle} titleAs="h3">
        <div className="space-y-5">
          {!transcoder.enabled && <Notice tone="danger">{text.transcodingOff}</Notice>}
          <Meter
            label={text.conversions}
            figure={`${transcoder.conversions} / ${transcoder.limit > 0 ? transcoder.limit : text.noLimit}`}
            value={transcoder.conversions}
            max={transcoder.limit}
            variant={full ? 'warn' : 'accent'}
            hideBar={transcoder.limit <= 0}
          />
          <Facts
            items={[
              {
                label: text.gpu,
                value: hardware ? (
                  <span>
                    <StatusPill tone="ok">{t.dashboard.live.gpu(hardware.method)}</StatusPill>
                    {hardware.device && (
                      <span className="ml-1.5 text-small text-ink-3">{hardware.device}</span>
                    )}
                  </span>
                ) : (
                  <span className="text-ink-3">{text.noGpu}</span>
                ),
              },
              ...(hardware
                ? [
                    { label: t.dashboard.live.encoder, value: hardware.encoders.join(', ') },
                    {
                      label: text.toneMapping,
                      value: hardware.toneMapping ? '✓' : '–',
                    },
                  ]
                : []),
              {
                label: text.cpuEncoders,
                value: (
                  <span className="font-mono text-small">
                    {transcoder.encoders.join(', ') || '–'}
                  </span>
                ),
              },
              { label: text.remuxes, value: transcoder.remuxes },
              {
                label: text.maxHeight,
                value:
                  transcoder.maxHeight > 0 ? `${transcoder.maxHeight}p` : t.dashboard.live.original,
              },
            ]}
          />
        </div>
      </Panel>
    </Anchor>
  )
}

function Database({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  return (
    <Anchor id="database">
      <Panel title={text.databaseTitle} titleAs="h3">
        <Facts
          items={[
            {
              label: text.databaseTitle,
              value: health.database.reachable ? (
                <StatusPill tone="ok">{text.reachable}</StatusPill>
              ) : (
                <StatusPill tone="danger">{text.unreachable}</StatusPill>
              ),
            },
            {
              label: text.size,
              value:
                health.database.size !== null ? formatBytes(health.database.size, language) : '–',
            },
          ]}
        />
      </Panel>
    </Anchor>
  )
}

function Cache({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const cache = health.cache
  if (cache === null) return null
  return (
    <Anchor id="cache">
      <Panel title={text.cacheTitle} titleAs="h3" description={text.cacheHelp}>
        <div className="space-y-4">
          <Meter
            label={text.cacheTitle}
            figure={text.cacheUse(
              formatBytes(cache.used, language),
              formatBytes(cache.limit, language),
            )}
            value={cache.used}
            max={cache.limit}
          />
          <p className="text-small text-ink-3 tabular-nums">
            {text.sourcesOpen}
            {t.common.colon} <span className="text-ink">{cache.sources}</span>
          </p>
        </div>
      </Panel>
    </Anchor>
  )
}

function Disks({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  return (
    <Anchor id="disks">
      <Panel title={text.disksTitle} titleAs="h3">
        <ul className="space-y-5">
          {health.disks.map((disk) => {
            const total = disk.free + disk.used
            const low = lowOnSpace(disk)
            const name = text.folderTitle[disk.folder]
            return (
              <li key={disk.folder} className="space-y-1.5">
                <Meter
                  label={
                    <span className="inline-flex items-center gap-1.5">
                      {low && <WarningIcon size={14} aria-hidden="true" className="text-warn" />}
                      {name}
                    </span>
                  }
                  figure={
                    <span className={low ? 'text-warn' : undefined}>
                      {disk.free >= 0
                        ? text.free(formatBytes(disk.free, language))
                        : text.notMeasured}
                    </span>
                  }
                  value={disk.used}
                  max={total}
                  variant={low ? 'warn' : 'accent'}
                  hideBar={disk.free < 0}
                />
                <p className="truncate font-mono text-micro text-ink-3" title={disk.path}>
                  {disk.path}
                </p>
              </li>
            )
          })}
        </ul>
      </Panel>
    </Anchor>
  )
}

function Secrets({ health }: { health: Health }) {
  const { t } = useI18n()
  const text = t.system.health
  const secrets = health.secrets
  if (secrets === null) return null
  return (
    <Anchor id="secrets">
      <Panel title={text.secretsTitle} titleAs="h3" description={text.secretsHelp}>
        <Facts
          items={[
            {
              label: text.encryption,
              value: secrets.encrypted ? (
                <StatusPill tone="ok">{text.encrypted}</StatusPill>
              ) : (
                <StatusPill tone={secrets.plaintext > 0 ? 'warn' : 'muted'}>
                  {text.notEncrypted}
                </StatusPill>
              ),
            },
            { label: text.storedPlain, value: secrets.plaintext },
            {
              label: text.unreadable,
              value:
                secrets.unreadable.length === 0 ? (
                  text.noneUnreadable
                ) : (
                  <StatusPill tone="danger" className="whitespace-normal">
                    {secrets.unreadable.map((secret) => unreadableName(t, secret)).join(', ')}
                  </StatusPill>
                ),
            },
          ]}
        />
      </Panel>
    </Anchor>
  )
}

function Backups({ health }: { health: Health }) {
  const { t } = useI18n()
  const text = t.system.health
  return (
    <Anchor id="backups">
      <Panel title={text.backupTitle} titleAs="h3">
        {health.backup === null ? (
          <p className="text-small text-ink-2">{text.backupOff}</p>
        ) : (
          <BackupStatus backup={health.backup} />
        )}
      </Panel>
    </Anchor>
  )
}

function Thumbnails({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const thumbnails = health.thumbnails
  if (thumbnails === null) return null
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  return (
    <Anchor id="thumbnails">
      <Panel title={text.thumbnailsTitle} titleAs="h3">
        {!thumbnails.enabled ? (
          <p className="text-small text-ink-2">{text.thumbnailsOff}</p>
        ) : (
          <div className="space-y-5">
            <Meter
              label={
                <StatusPill tone={thumbnails.working ? 'live' : 'muted'}>
                  {thumbnails.working ? text.working : text.idle}
                </StatusPill>
              }
              figure={text.waiting(thumbnails.waiting, thumbnails.queueLength)}
              value={thumbnails.waiting}
              max={thumbnails.queueLength}
            />
            <div>
              <h4 className="text-small text-ink-3">{text.pausedHosts}</h4>
              {thumbnails.pausedHosts.length === 0 ? (
                <p className="mt-1 text-body text-ink">{text.noPausedHosts}</p>
              ) : (
                <ul className="mt-1.5 space-y-1.5">
                  {thumbnails.pausedHosts.map((paused) => (
                    <li key={paused.host} className="flex flex-wrap justify-between gap-2">
                      <span className="inline-flex items-center gap-1.5 font-mono text-small text-warn">
                        <WarningIcon size={14} aria-hidden="true" />
                        {paused.host}
                      </span>
                      <span className="text-small text-ink-3 tabular-nums">
                        {text.until(time.format(new Date(paused.until)))}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        )}
      </Panel>
    </Anchor>
  )
}

function Process({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const process = health.process
  return (
    <Anchor id="process">
      <Panel title={text.processTitle} titleAs="h3">
        <Facts
          items={[
            { label: text.version, value: <span className="font-mono">{process.version}</span> },
            {
              label: text.uptime,
              value: formatSpan(
                (Date.parse(health.checkedAt) - Date.parse(process.startedAt)) / 1000,
                language,
              ),
            },
            { label: text.memory, value: formatBytes(process.memory, language) },
            { label: text.heap, value: formatBytes(process.heap, language) },
            { label: text.goroutines, value: process.goroutines },
            {
              label: 'Go',
              value: <span className="font-mono">{process.goVersion.replace(/^go/, '')}</span>,
            },
          ]}
        />
      </Panel>
    </Anchor>
  )
}
