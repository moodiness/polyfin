import { useEffect, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router'
import { useMutation } from '@tanstack/react-query'
import {
  checkAddon,
  queryClient,
  queryKeys,
  type Addon,
  type AddonHealth,
  type Health,
  type Library,
} from '@/api'
import { icons } from '@/components/icons'
import { Empty, Facts, Meter, Panel, Skeleton, StatusText, type Tone } from '@/components/panels'
import { MusicBadge } from '@/components/AddonSettings'
import { findProblems, lowOnSpace, useHealthData } from '@/components/problems'
import { buttonSecondary, Notice, PageHeader, RelativeTime } from '@/components/ui'
import { errorMessage, formatBytes, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n/en'

export default function HealthPage() {
  const { language, t } = useI18n()
  const text = t.dashboard.health
  const { health, addons, libraries, tasks } = useHealthData()
  const problems = findProblems(t, language, health.data, addons.data, libraries.data, tasks.data)
  const location = useLocation()
  const loaded = health.data !== undefined

  // A link to a section of this page scrolls to it once the page has drawn it.
  useEffect(() => {
    if (loaded && location.hash) {
      document.getElementById(location.hash.slice(1))?.scrollIntoView()
    }
  }, [loaded, location.hash])

  return (
    <>
      <PageHeader title={text.title} description={text.description} />
      {health.isPending ? (
        <Skeleton rows={6} label={t.common.loading} />
      ) : health.isError ? (
        <Notice kind="error">{errorMessage(t, health.error)}</Notice>
      ) : (
        <div className="space-y-6">
          <section
            aria-labelledby="problems-title"
            className={`rounded-2xl border p-5 ${
              problems.length === 0
                ? 'border-emerald-400/30 bg-emerald-400/5'
                : problems.some((problem) => problem.tone === 'error')
                  ? 'border-rose-400/40 bg-rose-400/5'
                  : 'border-amber-400/40 bg-amber-400/5'
            }`}
          >
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <h2 id="problems-title" className="text-base font-semibold text-white">
                {text.problemsTitle}
              </h2>
              <p className="text-xs text-muted">
                {text.checkedAt(
                  new Intl.DateTimeFormat(language, { timeStyle: 'medium' }).format(
                    new Date(health.data.checkedAt),
                  ),
                )}
              </p>
            </div>
            {problems.length === 0 ? (
              <p className="mt-2 text-sm">
                <StatusText tone="ok">{text.allGood}</StatusText>
              </p>
            ) : (
              <ul className="mt-3 space-y-2">
                {problems.map((problem) => (
                  <li key={problem.text} className="flex items-start gap-2.5 text-sm">
                    <icons.alert
                      className={`mt-0.5 size-4 shrink-0 ${problem.tone === 'error' ? 'text-rose-300' : 'text-amber-300'}`}
                    />
                    <span className="sr-only">
                      {problem.tone === 'error' ? text.error : text.warning}:{' '}
                    </span>
                    <Link
                      to={problem.to}
                      className="text-zinc-100 underline decoration-line underline-offset-4 hover:decoration-fin-5"
                    >
                      {problem.text}
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <Addons health={health.data} addons={addons.data} />

          <div className="grid gap-6 xl:grid-cols-2">
            <Iptv addons={addons.data} />
            <Guides libraries={libraries.data} />
          </div>

          <div className="grid gap-6 lg:grid-cols-2 2xl:grid-cols-3">
            <Transcoder health={health.data} />
            <Storage health={health.data} />
            <Thumbnails health={health.data} />
            <Process health={health.data} />
          </div>
        </div>
      )}
    </>
  )
}

function addonTone(
  addon: AddonHealth,
): [Tone, keyof Messages['dashboard']['health']['addonStatus']] {
  if (!addon.enabled) return ['muted', 'off']
  if (addon.requests === 0) return ['muted', 'unknown']
  return addon.failure === '' ? ['ok', 'ok'] : ['warning', 'failing']
}

function Addons({ health, addons }: { health: Health; addons: Addon[] | undefined }) {
  const { t } = useI18n()
  const text = t.dashboard.health
  const check = useMutation({
    mutationFn: checkAddon,
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.health }),
  })
  // Music addons' answers are recorded like the others': their row carries their badge.
  const music = new Map(
    (addons ?? []).filter((addon) => addon.music !== null).map((addon) => [addon.id, addon.music]),
  )

  return (
    <Panel id="addons" title={text.addonsTitle} description={text.addonsHelp}>
      {check.isError && (
        <div className="mb-3">
          <Notice kind="error">{errorMessage(t, check.error)}</Notice>
        </div>
      )}
      {health.addons.length === 0 ? (
        <Empty>{text.noAddons}</Empty>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {health.addons.map((addon) => {
            const [tone, status] = addonTone(addon)
            const musicOf = music.get(addon.id)
            return (
              <li
                key={addon.id}
                className="grid gap-3 px-4 py-3 text-sm md:grid-cols-[minmax(0,1.3fr)_minmax(0,2fr)_auto] md:items-center"
              >
                <div className="min-w-0">
                  <p className="flex flex-wrap items-center gap-2 font-medium text-white">
                    <span className="truncate">{addon.name}</span>
                    {musicOf && <MusicBadge music={musicOf} />}
                  </p>
                  <p className="text-xs">
                    <StatusText tone={tone}>
                      {text.addonStatus[status]}
                      {addon.failure !== '' && addon.enabled && (
                        <span className="font-normal">
                          {' '}
                          · {text.failures[addon.failure] ?? addon.failure}
                        </span>
                      )}
                    </StatusText>
                  </p>
                </div>
                <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs sm:grid-cols-3">
                  <Fact label={text.lastSuccess}>
                    {addon.lastSuccessAt ? <RelativeTime iso={addon.lastSuccessAt} /> : '–'}
                  </Fact>
                  <Fact label={text.lastFailure}>
                    {addon.lastFailureAt ? <RelativeTime iso={addon.lastFailureAt} /> : '–'}
                  </Fact>
                  <Fact label={text.responseTime}>
                    {addon.requests > 0 && addon.lastSuccessAt ? `${addon.responseTime} ms` : '–'}
                  </Fact>
                  <p className="col-span-full text-muted">
                    {text.requests(addon.requests, addon.failures)}
                  </p>
                </dl>
                <div className="md:text-right">
                  <button
                    type="button"
                    className={buttonSecondary}
                    aria-label={text.checkLabel(addon.name)}
                    disabled={!addon.enabled || (check.isPending && check.variables === addon.id)}
                    onClick={() => check.mutate(addon.id)}
                  >
                    {check.isPending && check.variables === addon.id ? text.checking : text.check}
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </Panel>
  )
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted">{label}</dt>
      <dd className="text-zinc-100 tabular-nums">{children}</dd>
    </div>
  )
}

function Iptv({ addons }: { addons: Addon[] | undefined }) {
  const { t } = useI18n()
  const text = t.dashboard.health
  const sources = (addons ?? []).filter((addon) => addon.source !== null)
  return (
    <Panel id="iptv" title={text.iptvTitle}>
      {sources.length === 0 ? (
        <Empty>{t.dashboard.schedule.noRefreshes}</Empty>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {sources.map((addon) => {
            const source = addon.source!
            return (
              <li key={addon.id} className="space-y-1 px-4 py-3 text-sm">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="font-medium text-white">{addon.name}</p>
                  {!addon.enabled ? (
                    <StatusText tone="muted">{text.addonStatus.off}</StatusText>
                  ) : source.error !== '' ? (
                    <StatusText tone="warning">{text.addonStatus.failing}</StatusText>
                  ) : source.fetchedAt ? (
                    <StatusText tone="ok">{text.ok}</StatusText>
                  ) : (
                    <StatusText tone="muted">{t.dashboard.schedule.notYet}</StatusText>
                  )}
                </div>
                <p className="text-xs text-muted">
                  {t.iptv.channelCount(source.channels)} · {t.iptv.lastFetch}:{' '}
                  {source.fetchedAt ? <RelativeTime iso={source.fetchedAt} /> : t.iptv.never}
                  {source.nextAt && (
                    <>
                      {' '}
                      · {t.iptv.nextFetch}: <RelativeTime iso={source.nextAt} />
                    </>
                  )}
                </p>
                {source.error !== '' && (
                  <p className="text-xs text-amber-200">
                    {t.iptv.errors[source.error] ?? source.error}
                  </p>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </Panel>
  )
}

function Guides({ libraries }: { libraries: Library[] | undefined }) {
  const { t } = useI18n()
  const text = t.dashboard.health
  const guides = (libraries ?? []).filter(
    (library) => library.guide !== null && library.guide.url !== '',
  )
  return (
    <Panel id="guides" title={text.guidesTitle}>
      {guides.length === 0 ? (
        <Empty>{t.libraries.guideNone}</Empty>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {guides.map((library) => {
            const guide = library.guide!
            return (
              <li
                key={`${library.addonId}-${library.catalogType}-${library.catalogId}`}
                className="space-y-1 px-4 py-3 text-sm"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="font-medium text-white">{library.name ?? library.catalogName}</p>
                  {guide.error !== '' ? (
                    <StatusText tone="warning">{text.addonStatus.failing}</StatusText>
                  ) : guide.fetchedAt ? (
                    <StatusText tone="ok">{text.ok}</StatusText>
                  ) : (
                    <StatusText tone="muted">{t.dashboard.schedule.notYet}</StatusText>
                  )}
                </div>
                <p className="text-xs text-muted">
                  {library.addonName} · {t.libraries.guideFetched}:{' '}
                  {guide.fetchedAt ? (
                    <RelativeTime iso={guide.fetchedAt} />
                  ) : (
                    t.libraries.guideNever
                  )}
                  {guide.channels > 0 && (
                    <> · {text.channelsMatched(guide.matched, guide.channels)}</>
                  )}
                </p>
                {guide.error !== '' && (
                  <p className="text-xs text-amber-200">
                    {t.libraries.guideErrors[guide.error] ?? guide.error}
                  </p>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </Panel>
  )
}

function Transcoder({ health }: { health: Health }) {
  const { t } = useI18n()
  const text = t.dashboard.health
  const transcoder = health.transcoder
  if (transcoder === null) return null
  const hardware = transcoder.hardware
  return (
    <Panel id="transcoder" title={text.transcoderTitle}>
      <div className="space-y-4">
        {!transcoder.enabled && <Notice kind="error">{text.transcodingOff}</Notice>}
        <div>
          <div className="mb-1.5 flex items-baseline justify-between text-sm">
            <span className="text-muted">{text.conversions}</span>
            <span className="text-zinc-100 tabular-nums">
              {transcoder.conversions} / {transcoder.limit > 0 ? transcoder.limit : text.noLimit}
            </span>
          </div>
          {transcoder.limit > 0 && (
            <Meter
              value={transcoder.conversions}
              max={transcoder.limit}
              label={text.conversions}
              tone={transcoder.conversions >= transcoder.limit ? 'warning' : 'active'}
            />
          )}
        </div>
        <Facts
          items={[
            {
              label: text.gpu,
              value: hardware ? (
                <StatusText tone="ok">
                  {t.dashboard.live.gpu(hardware.method)}
                  {hardware.device && (
                    <span className="font-normal text-muted"> {hardware.device}</span>
                  )}
                </StatusText>
              ) : (
                <span className="text-muted">{text.noGpu}</span>
              ),
            },
            ...(hardware
              ? [
                  { label: t.dashboard.live.encoder, value: hardware.encoders.join(', ') },
                  { label: text.toneMapping, value: hardware.toneMapping ? '✓' : '–' },
                ]
              : []),
            { label: text.cpuEncoders, value: transcoder.encoders.join(', ') || '–' },
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
  )
}

function Storage({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.dashboard.health
  return (
    <>
      <Panel id="database" title={text.databaseTitle}>
        <Facts
          items={[
            {
              label: text.databaseTitle,
              value: health.database.reachable ? (
                <StatusText tone="ok">{text.reachable}</StatusText>
              ) : (
                <StatusText tone="error">{text.unreachable}</StatusText>
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
      {health.cache !== null && (
        <Panel id="cache" title={text.cacheTitle} description={text.cacheHelp}>
          <div className="space-y-3">
            <div className="flex items-baseline justify-between text-sm">
              <span className="text-zinc-100 tabular-nums">
                {text.cacheUse(
                  formatBytes(health.cache.used, language),
                  formatBytes(health.cache.limit, language),
                )}
              </span>
              <span className="text-xs text-muted tabular-nums">
                {text.sourcesOpen}: {health.cache.sources}
              </span>
            </div>
            <Meter value={health.cache.used} max={health.cache.limit} label={text.cacheTitle} />
          </div>
        </Panel>
      )}
      <Panel id="disks" title={text.disksTitle}>
        <ul className="space-y-4">
          {health.disks.map((disk) => {
            const total = disk.free + disk.used
            const low = lowOnSpace(disk)
            return (
              <li key={disk.folder} className="space-y-1.5 text-sm">
                <div className="flex flex-wrap items-baseline justify-between gap-2">
                  <span className="text-zinc-100">{text.folderTitle[disk.folder]}</span>
                  <span className={`text-xs tabular-nums ${low ? 'text-amber-200' : 'text-muted'}`}>
                    {disk.free >= 0
                      ? text.free(formatBytes(disk.free, language))
                      : text.notMeasured}
                  </span>
                </div>
                <p className="truncate font-mono text-xs text-muted" title={disk.path}>
                  {disk.path}
                </p>
                {disk.free >= 0 && (
                  <Meter
                    value={disk.used}
                    max={total}
                    label={text.folderTitle[disk.folder]}
                    tone={low ? 'warning' : 'active'}
                  />
                )}
              </li>
            )
          })}
        </ul>
      </Panel>
    </>
  )
}

function Thumbnails({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.dashboard.health
  const thumbnails = health.thumbnails
  if (thumbnails === null) return null
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  return (
    <Panel id="thumbnails" title={text.thumbnailsTitle}>
      {!thumbnails.enabled ? (
        <p className="text-sm text-muted">{text.thumbnailsOff}</p>
      ) : (
        <div className="space-y-4">
          <div>
            <div className="mb-1.5 flex items-baseline justify-between text-sm">
              <StatusText tone={thumbnails.working ? 'active' : 'muted'}>
                {thumbnails.working ? text.working : text.idle}
              </StatusText>
              <span className="text-xs text-muted tabular-nums">
                {text.waiting(thumbnails.waiting, thumbnails.queueLength)}
              </span>
            </div>
            <Meter
              value={thumbnails.waiting}
              max={thumbnails.queueLength}
              label={text.thumbnailsTitle}
            />
          </div>
          <div>
            <h3 className="text-xs text-muted">{text.pausedHosts}</h3>
            {thumbnails.pausedHosts.length === 0 ? (
              <p className="mt-1 text-sm text-zinc-100">{text.noPausedHosts}</p>
            ) : (
              <ul className="mt-1 space-y-1 text-sm">
                {thumbnails.pausedHosts.map((paused) => (
                  <li key={paused.host} className="flex flex-wrap justify-between gap-2">
                    <span className="font-mono text-amber-200">{paused.host}</span>
                    <span className="text-xs text-muted">
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
  )
}

function Process({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.dashboard.health
  const process = health.process
  return (
    <Panel id="process" title={text.processTitle}>
      <Facts
        items={[
          { label: text.version, value: process.version },
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
          { label: 'Go', value: process.goVersion.replace(/^go/, '') },
        ]}
      />
    </Panel>
  )
}
