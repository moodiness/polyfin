import type { ReactNode } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  ArrowsClockwiseIcon,
  CalendarDotsIcon,
  DownloadSimpleIcon,
  HardDrivesIcon,
  PuzzlePieceIcon,
  TelevisionSimpleIcon,
  TrashIcon,
  WarningIcon,
} from '@phosphor-icons/react'
import {
  checkAddon,
  clearImageCache,
  goroutinesDownloadUrl,
  queryClient,
  queryKeys,
  type AddonHealth,
  type Health,
  type ShareHealth,
  type Sources,
} from '@/api'
import { settingsPath } from '@/app/navigation'
import {
  Badge,
  Button,
  ExternalButtonLink,
  EmptyState,
  InlineError,
  Notice,
  RelativeTime,
  Row,
  RowList,
  SkeletonRows,
  StatusPill,
  TextLink,
  useToast,
  type StatusTone,
} from '@/ui'
import { lineupPath } from '@/features/iptv/lineup'
import { hdrPathText } from '@/features/settings/hdrPath'
import { unreadableName } from '@/features/system/problems'
import { dateTime, errorMessage, formatBytes, formatSpan, relativeTime } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'
import BackupStatus from './BackupStatus'
import OwnerChip from './OwnerChip'
import { Facts, Meter } from './parts'

/**
 * The parts of Health a check tile or a row opens, by the `#anchor` that opens them: the same
 * anchors the server's problems and notifications link to.
 */
export const detailIds = [
  'addons',
  'iptv',
  'guides',
  'shares',
  'transcoder',
  'database',
  'cache',
  'images',
  'torrents',
  'disks',
  'backups',
  'thumbnails',
  'ratings',
  'artwork',
  'people',
  'process',
  'secrets',
  'proxies',
] as const

export type DetailId = (typeof detailIds)[number]

export const isDetailId = (id: string): id is DetailId =>
  (detailIds as readonly string[]).includes(id)

/** The title of each part, as its tile or row names it. */
export function detailTitle(t: Messages, id: DetailId): string {
  const text = t.system.health
  const titles: Record<DetailId, string> = {
    addons: text.addonsTitle,
    iptv: text.iptvTitle,
    guides: text.guidesTitle,
    shares: text.sharesTitle,
    transcoder: text.transcoderTitle,
    database: text.databaseTitle,
    cache: text.cacheTitle,
    images: text.imagesTitle,
    torrents: text.torrentsTitle,
    disks: text.disksTitle,
    backups: text.backupTitle,
    thumbnails: text.thumbnailsTitle,
    ratings: text.ratingsTitle,
    artwork: text.artworkTitle,
    people: text.peopleTitle,
    process: text.processTitle,
    secrets: text.secretsTitle,
    proxies: text.proxiesTitle,
  }
  return titles[id]
}

/** What the part says in full, with its own actions (Check, Clear the image cache…). */
export function DetailBody({
  id,
  health,
  sources,
}: {
  id: DetailId
  health: Health
  sources: Sources | undefined
}) {
  switch (id) {
    case 'addons':
      return <Addons health={health} sources={sources} />
    case 'iptv':
      return <Iptv sources={sources} />
    case 'guides':
      return <Guides sources={sources} />
    case 'shares':
      return <Shares health={health} />
    case 'transcoder':
      return <Transcoder health={health} />
    case 'database':
      return <Database health={health} />
    case 'cache':
      return <Cache health={health} />
    case 'images':
      return <ImageCache health={health} />
    case 'torrents':
      return <Torrents health={health} />
    case 'disks':
      return <Disks health={health} />
    case 'backups':
      return <Backups health={health} />
    case 'thumbnails':
      return <Thumbnails health={health} />
    case 'ratings':
      return <Ratings health={health} />
    case 'artwork':
      return <Artwork health={health} />
    case 'people':
      return <People health={health} />
    case 'process':
      return <Process health={health} />
    case 'secrets':
      return <Secrets health={health} />
    case 'proxies':
      return <ProxyNames health={health} />
  }
}

/** What a part shows, under its title. */
function Help({ children }: { children: ReactNode }) {
  return <p className="max-w-[66ch] text-small text-ink-2">{children}</p>
}

/** A heading inside a part: "Paused sources", "Servers waited for". */
function SubTitle({ children }: { children: ReactNode }) {
  return <h3 className="text-control font-semibold text-ink">{children}</h3>
}

/** A part the server tells nothing of (its feature is off): says so. */
function Unknown() {
  const { t } = useI18n()
  return <Help>{t.system.health.checks.unknown}</Help>
}

export function addonTone(
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
  // Eclipse addons' answers are recorded like the others': their row carries the kind's badge.
  const eclipse = new Set(
    (sources?.addons ?? []).filter((addon) => addon.music !== null).map((addon) => addon.id),
  )

  return (
    <div className="space-y-5">
      <Help>{text.addonsHelp}</Help>
      {check.isError && <InlineError>{errorMessage(t, check.error)}</InlineError>}
      {health.addons.length === 0 ? (
        <EmptyState icon={PuzzlePieceIcon} title={text.noAddons}>
          {text.noAddonsHint}
        </EmptyState>
      ) : (
        <RowList aria-label={text.addonsTitle}>
          {health.addons.map((addon) => {
            const [tone, status] = addonTone(addon)
            const isEclipse = eclipse.has(addon.id)
            const checking = check.isPending && check.variables === addon.id
            return (
              <Row
                key={addon.id}
                muted={!addon.enabled}
                title={addon.name}
                titleAside={
                  <span className="flex shrink-0 flex-wrap items-center gap-1.5">
                    {isEclipse && <Badge tone="accent">{t.sources.tag.eclipse}</Badge>}
                    <OwnerChip owner={addon.owner} />
                  </span>
                }
                meta={
                  <span className="flex flex-wrap items-center gap-x-2">
                    <StatusPill tone={tone}>{text.addonStatus[status]}</StatusPill>
                    {addon.failure !== '' && addon.enabled && (
                      <span>{text.failures[addon.failure] ?? addon.failure}</span>
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
                <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-small figures">
                  <SmallFact label={text.lastSuccess}>
                    {addon.lastSuccessAt ? <RelativeTime iso={addon.lastSuccessAt} /> : '–'}
                  </SmallFact>
                  <SmallFact label={text.lastFailure}>
                    {addon.lastFailureAt ? <RelativeTime iso={addon.lastFailureAt} /> : '–'}
                  </SmallFact>
                  <SmallFact label={text.responseTime}>
                    {addon.requests > 0 && addon.lastSuccessAt ? `${addon.responseTime} ms` : '–'}
                  </SmallFact>
                  <p className="self-end text-ink-3">
                    {text.requests(addon.requests, addon.failures)}
                  </p>
                </dl>
              </Row>
            )
          })}
        </RowList>
      )}
    </div>
  )
}

function SmallFact({ label, children }: { label: string; children: ReactNode }) {
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
  if (all === undefined) return <SkeletonRows rows={1} />
  if (sources.length === 0) {
    return (
      <EmptyState icon={TelevisionSimpleIcon} title={text.noIptv}>
        {text.noIptvHint}
      </EmptyState>
    )
  }
  return (
    <RowList aria-label={text.iptvTitle}>
      {sources.map((addon) => {
        const source = addon.source!
        const vod = source.options.movies || source.options.series
        return (
          <Row
            key={addon.id}
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
            <div className="space-y-1 text-small text-ink-2">
              {(source.options.liveTv || vod) && (
                <p className="figures">
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
                  <WarningIcon size={18} aria-hidden="true" className="mt-0.5 shrink-0" />
                  {t.iptv.errors[source.error] ?? source.error}
                </p>
              )}
              {addon.owner === null && (
                <p className="pt-1">
                  <TextLink to={lineupPath('shared', addon.id)} className="-ml-3">
                    {t.lineup.open}
                  </TextLink>
                </p>
              )}
            </div>
          </Row>
        )
      })}
    </RowList>
  )
}

function Guides({ sources }: { sources: Sources | undefined }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const catalogs = (sources?.guides ?? []).filter((library) => (library.guides ?? []).length > 0)
  if (sources === undefined) return <SkeletonRows rows={1} />
  if (catalogs.length === 0) {
    return (
      <EmptyState icon={CalendarDotsIcon} title={text.noGuides}>
        {text.noGuidesHint}
      </EmptyState>
    )
  }
  return (
    <RowList aria-label={text.guidesTitle}>
      {catalogs.map((library) => {
        const guides = library.guides ?? []
        const failing = guides.filter((guide) => guide.error !== '')
        const fetched = latest(guides.map((guide) => guide.fetchedAt))
        return (
          <Row
            key={`${library.addonId}-${library.catalogType}-${library.catalogId}`}
            title={library.name ?? library.catalogName}
            titleAside={<OwnerChip owner={library.owner} />}
            meta={
              <span>
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
            <div className="space-y-1 text-small text-ink-2">
              <p className="figures">
                {t.libraries.guideFetched}
                {t.common.colon} {fetched ? <RelativeTime iso={fetched} /> : t.libraries.guideNever}
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
                  <WarningIcon size={18} aria-hidden="true" className="mt-0.5 shrink-0" />
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
  )
}

/** The latest of some times, or undefined when none is set. */
export function latest(times: readonly (string | null | undefined)[]): string | undefined {
  return times
    .filter((at): at is string => typeof at === 'string' && at !== '')
    .sort()
    .at(-1)
}

export function shareState(
  health: Health,
  share: ShareHealth,
  text: Messages['system']['health'],
): [StatusTone, string, string] {
  const status = text.shareStatus
  if (!health.watchFolders) return ['muted', status.unwatched, text.sharesOff]
  if (share.pollMinutes === 0) return ['muted', status.scans, text.shareScansOnly]
  if (share.unwatched !== '') {
    return [
      'warn',
      status.unwatched,
      text.shareUnwatched[share.unwatched] ?? text.shareUnwatched.failed,
    ]
  }
  if (share.watch === 'notified') {
    return [
      'ok',
      status.notified,
      share.polled === 'partial' ? text.sharePartial : text.shareNotified,
    ]
  }
  if (share.pollMinutes !== null && share.polled === share.share) {
    return ['muted', status.polled, text.sharePolledEvery(share.share, share.pollMinutes)]
  }
  const why = text.sharePolled[share.polled || 'starting'] ?? text.sharePolled.failed
  const tone = share.polled === 'silent' || share.polled === 'failed' ? 'warn' : 'muted'
  return [tone, status.polled, why]
}

function Shares({ health }: { health: Health }) {
  const { t } = useI18n()
  const text = t.system.health
  if (health.shares.length === 0) return <Unknown />
  return (
    <RowList aria-label={text.sharesTitle}>
      {health.shares.map((share) => {
        const [tone, status, why] = shareState(health, share, text)
        return (
          <Row
            key={share.id}
            to={`/sources/shared/${share.id}`}
            leading={HardDrivesIcon}
            title={share.name}
            titleAside={<Badge>{text.shareKinds[share.share] ?? share.share}</Badge>}
            meta={<span>{why}</span>}
            trailing={<StatusPill tone={tone}>{status}</StatusPill>}
          />
        )
      })}
    </RowList>
  )
}

function Transcoder({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const transcoder = health.transcoder
  if (transcoder === null) return <Unknown />
  const hardware = transcoder.hardware
  const full = transcoder.limit > 0 && transcoder.conversions >= transcoder.limit
  const hdrMessages = t.settings.conversion.hdrPath
  const hdr = hardware?.hdr ? hdrPathText(hardware.hdr, hdrMessages, language) : null
  return (
    <div className="space-y-6">
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
                  <span className="mt-1 block text-small text-ink-2">{hardware.device}</span>
                )}
              </span>
            ) : (
              <span className="text-ink-2">{text.noGpu}</span>
            ),
          },
          ...(hardware
            ? [
                { label: t.dashboard.live.encoder, value: hardware.encoders.join(', ') },
                { label: text.toneMapping, value: hardware.toneMapping ? '✓' : '–' },
              ]
            : []),
          ...(hdr
            ? [
                {
                  label: hdrMessages.term,
                  value: (
                    <span>
                      {hdr.where}
                      <span className="block text-small text-ink-2">{hdr.why}</span>
                    </span>
                  ),
                },
              ]
            : []),
          { label: text.cpuEncoders, value: transcoder.encoders.join(', ') || '–' },
          {
            label: text.ffmpeg,
            value: hardware?.olderDrivers
              ? text.ffmpegOlderDrivers
              : transcoder.olderDriverFFmpeg?.state === 'downloading'
                ? text.ffmpegOlderDownloading
                : text.ffmpegShipped,
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
  )
}

function Database({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  return (
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
          value: health.database.size !== null ? formatBytes(health.database.size, language) : '–',
        },
      ]}
    />
  )
}

/**
 * The source cache against its limit, the files it reads and keeps, how long it keeps them, and its
 * hit ratio: the share of the bytes read since the start that were on disk when asked for.
 */
function Cache({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const cache = health.cache
  if (cache === null) return <Unknown />
  const read = cache.hits + cache.misses
  return (
    <div className="space-y-6">
      <Help>{text.cacheHelp}</Help>
      <Meter
        label={text.cacheTitle}
        figure={text.cacheUse(
          formatBytes(cache.used, language),
          formatBytes(cache.limit, language),
        )}
        value={cache.used}
        max={cache.limit}
      />
      <Facts
        items={[
          { label: text.sourcesOpen, value: cache.sources },
          { label: text.sourcesKept, value: cache.kept },
          ...(read > 0 ? [{ label: text.hitRatio, value: hitRatio(text, cache, language) }] : []),
          ...(health.torrents !== null
            ? [{ label: text.torrentPieces, value: formatBytes(cache.torrents, language) }]
            : []),
        ]}
      />
      <Help>{text.keptFor(cache.days)}</Help>
    </div>
  )
}

/** The share of the bytes read since the start that were on disk, and how much was read. */
export function hitRatio(
  text: Messages['system']['health'],
  cache: NonNullable<Health['cache']>,
  language: string,
): string {
  const read = cache.hits + cache.misses
  return text.hitRatioValue(
    (read > 0 ? cache.hits / read : 0).toLocaleString(language, {
      style: 'percent',
      maximumFractionDigits: 1,
    }),
    formatBytes(read, language),
  )
}

/**
 * The image cache, against its limit, and in memory; clearing it tells what it freed and shows the
 * cache empty at once.
 */
function ImageCache({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const toast = useToast()
  const clear = useMutation({
    mutationFn: clearImageCache,
    onSuccess: (freed) => {
      toast(
        text.imagesCleared(freed.files.toLocaleString(language), formatBytes(freed.used, language)),
        { tone: 'ok' },
      )
      queryClient.setQueryData<Health>(queryKeys.health, (current) =>
        current === undefined
          ? current
          : {
              ...current,
              images: { files: 0, used: 0, limit: freed.limit, memoryImages: 0, memoryBytes: 0 },
            },
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.health })
    },
  })
  const images = health.images
  if (images === null) return <Unknown />
  return (
    <div className="space-y-6">
      <Help>{text.imagesHelp}</Help>
      <Meter
        label={text.imagesTitle}
        figure={text.cacheUse(
          formatBytes(images.used, language),
          formatBytes(images.limit, language),
        )}
        value={images.used}
        max={images.limit}
      />
      <Facts
        items={[
          { label: text.imagesFiles, value: images.files.toLocaleString(language) },
          {
            label: text.imagesMemory,
            value: `${images.memoryImages.toLocaleString(language)} · ${formatBytes(images.memoryBytes, language)}`,
          },
        ]}
      />
      {clear.isError && <InlineError>{errorMessage(t, clear.error)}</InlineError>}
      <Button
        icon={TrashIcon}
        loading={clear.isPending}
        disabled={images.files === 0 && images.memoryImages === 0}
        onClick={() => clear.mutate()}
      >
        {text.clearImages}
      </Button>
    </div>
  )
}

/**
 * How torrent streams play: their route, a WireGuard tunnel's addresses and last handshake, the
 * torrents playing, their peers and those reached over uTP, what was shared, and the outages of
 * the route the plays went on through lately.
 */
function Torrents({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const torrents = health.torrents
  if (torrents === null) return <Unknown />
  const tunnel: (readonly [string, string])[] =
    torrents.route === 'wireguard'
      ? [
          [text.torrentAddresses, (torrents.addresses ?? []).join(', ')],
          [
            text.torrentHandshake,
            torrents.lastHandshake === undefined
              ? text.torrentNoHandshake
              : relativeTime(torrents.lastHandshake, language, t.time.justNow),
          ],
        ]
      : []
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  const outages = (torrents.outages ?? []).map(
    (outage) =>
      [
        text.torrentOutageAt(time.format(new Date(outage.at))),
        text.torrentOutage[torrents.route](formatSpan(outage.seconds, language)),
      ] as const,
  )
  const rows = [
    [text.torrentRoute[torrents.route], torrents.name],
    ...tunnel,
    [
      text.torrentsPlaying,
      torrents.atOnce > 0
        ? text.torrentsPlayingOf(torrents.playing, torrents.atOnce)
        : String(torrents.playing),
    ],
    [text.torrentPeers, String(torrents.peers)],
    [text.torrentUTPPeers, String(torrents.utpPeers)],
    [text.torrentsShared, formatBytes(torrents.uploaded, language)],
    ...outages,
  ] as const
  return (
    <div className="space-y-5">
      <Help>{text.torrentsHelp}</Help>
      <ul className="space-y-1.5">
        {rows.map(([label, value], index) => (
          <li key={`${index}-${label}`} className="text-small text-ink-2 figures">
            {label}
            {value !== '' && (
              <>
                {t.common.colon} <span className="text-ink">{value}</span>
              </>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}

function Disks({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  return (
    <ul className="space-y-6">
      {health.disks.map((disk) => {
        const low = disk.low
        return (
          <li key={disk.folder} className="space-y-1.5">
            <Meter
              label={
                <span className="inline-flex items-center gap-1.5">
                  {low && <WarningIcon size={18} aria-hidden="true" className="text-warn" />}
                  {text.folderTitle[disk.folder]}
                </span>
              }
              figure={
                <span className={low ? 'text-warn' : undefined}>
                  {disk.free >= 0 ? text.free(formatBytes(disk.free, language)) : text.notMeasured}
                </span>
              }
              value={disk.used}
              max={disk.free + disk.used}
              variant={low ? 'warn' : 'accent'}
              hideBar={disk.free < 0}
            />
            <p className="font-mono text-micro break-all text-ink-3">{disk.path}</p>
          </li>
        )
      })}
    </ul>
  )
}

function Secrets({ health }: { health: Health }) {
  const { t } = useI18n()
  const text = t.system.health
  const secrets = health.secrets
  if (secrets === null) return <Unknown />
  return (
    <div className="space-y-6">
      <Help>{text.secretsHelp}</Help>
      <Facts
        items={[
          {
            label: text.encryption,
            value: secrets.encrypted ? (
              <StatusPill tone="ok">{text.encrypted}</StatusPill>
            ) : (
              <StatusPill
                tone={secrets.plaintext > 0 ? 'warn' : 'muted'}
                className="whitespace-normal"
              >
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
    </div>
  )
}

/** What each host name of Known proxies resolves to, while some are listed. */
function ProxyNames({ health }: { health: Health }) {
  const { t } = useI18n()
  const text = t.system.health
  if (health.proxyNames.length === 0) return <Unknown />
  return (
    <div className="space-y-6">
      <Help>{text.proxiesHelp}</Help>
      <Facts
        items={health.proxyNames.map((proxy) => ({
          label: proxy.name,
          value:
            proxy.addresses.length === 0 ? (
              <StatusPill tone="warn">{text.resolvesToNothing}</StatusPill>
            ) : (
              proxy.addresses.join(', ')
            ),
        }))}
      />
    </div>
  )
}

function Backups({ health }: { health: Health }) {
  const { t } = useI18n()
  const text = t.system.health
  if (health.backup === null) {
    return (
      <div className="space-y-3">
        <Help>{text.backupOff}</Help>
        <TextLink to={settingsPath('backups')} className="-ml-3">
          {text.checks.openSettings}
        </TextLink>
      </div>
    )
  }
  return (
    <div className="space-y-4">
      <BackupStatus backup={health.backup} />
      <TextLink to="/system/schedule#backups" className="-ml-3">
        {text.problemAction.schedule}
      </TextLink>
    </div>
  )
}

function Thumbnails({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const thumbnails = health.thumbnails
  if (thumbnails === null) return <Unknown />
  if (!thumbnails.enabled) return <Help>{text.thumbnailsOff}</Help>
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  return (
    <div className="space-y-6">
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
      <div className="space-y-1.5">
        <SubTitle>{text.pausedHosts}</SubTitle>
        {thumbnails.pausedHosts.length === 0 ? (
          <p className="text-body text-ink-2">{text.noPausedHosts}</p>
        ) : (
          <ul className="space-y-1.5">
            {thumbnails.pausedHosts.map((paused) => (
              <li key={paused.host} className="flex flex-wrap justify-between gap-2">
                <span className="inline-flex items-center gap-1.5 text-small break-all text-warn">
                  <WarningIcon size={18} aria-hidden="true" />
                  {paused.host}
                </span>
                <span className="text-small text-ink-2 figures">
                  {text.until(time.format(new Date(paused.until)))}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

/** The lookups of titles' ratings on MDBList, while the server has a key for them. */
function Ratings({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const ratings = health.ratings
  if (ratings === null) return <Unknown />
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  const number = new Intl.NumberFormat(language)
  return (
    <div className="space-y-6">
      <Help>{text.ratingsHelp}</Help>
      <Facts
        items={[
          {
            label: text.requestsLeft,
            value:
              ratings.remaining !== null && ratings.limit !== null
                ? text.requestsLeftOf(
                    number.format(ratings.remaining),
                    number.format(ratings.limit),
                  )
                : text.notToldYet,
          },
          ...(ratings.resetsAt !== null
            ? [{ label: text.resets, value: time.format(new Date(ratings.resetsAt)) }]
            : []),
          {
            label: text.ratingsHeld,
            value:
              ratings.heldUntil !== null ? (
                <StatusPill
                  tone={ratings.held === 'refused' ? 'danger' : 'warn'}
                  className="whitespace-normal"
                >
                  {`${text.heldReasons[ratings.held] ?? ''} ${text.until(time.format(new Date(ratings.heldUntil)))}`.trim()}
                </StatusPill>
              ) : (
                text.notHeld
              ),
          },
          { label: text.titlesWaiting, value: number.format(ratings.waiting) },
          { label: text.titlesKept, value: number.format(ratings.kept) },
        ]}
      />
    </div>
  )
}

/** The kinds of artwork download failures, in the order the server tells them. */
const artworkKinds = ['not_found', 'rate_limited', 'timeout', 'server_error', 'other'] as const

/**
 * How artwork downloads fared in the last hour: those that failed, by kind and by server, and the
 * servers waited for. Only host names are shown, never an address.
 */
function Artwork({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const artwork = health.artwork
  if (artwork === null) return <Unknown />
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })
  const number = new Intl.NumberFormat(language)
  const held = [
    ...artwork.waiting.map((host) => ({
      ...host,
      kind: 'waiting',
      label: text.artworkWaitingUntil,
    })),
    ...artwork.spared.map((host) => ({ ...host, kind: 'spared', label: text.artworkSparedUntil })),
  ]
  return (
    <div className="space-y-6">
      <Help>{text.artworkHelp}</Help>
      {artwork.failed === 0 ? (
        <p className="text-body text-ink">{text.artworkNoFailures}</p>
      ) : (
        <>
          <Facts
            items={[
              { label: text.artworkFailed, value: number.format(artwork.failed) },
              ...artworkKinds
                .filter((kind) => (artwork.kinds[kind] ?? 0) > 0)
                .map((kind) => ({
                  label: text.artworkKinds[kind] ?? kind,
                  value: number.format(artwork.kinds[kind] ?? 0),
                })),
            ]}
          />
          <div className="space-y-1.5">
            <SubTitle>{text.artworkHosts}</SubTitle>
            <ul className="space-y-1.5">
              {artwork.hosts.map((host) => (
                <li key={host.host} className="flex flex-wrap justify-between gap-2">
                  <span className="text-small break-all text-ink">{host.host}</span>
                  <span className="text-small text-ink-2 figures">
                    {number.format(host.failed)}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        </>
      )}
      <div className="space-y-1.5">
        <SubTitle>{text.artworkWaiting}</SubTitle>
        {held.length === 0 ? (
          <p className="text-body text-ink-2">{text.artworkNoWaiting}</p>
        ) : (
          <ul className="space-y-1.5">
            {held.map((host) => (
              <li
                key={`${host.kind} ${host.host}`}
                className="flex flex-wrap justify-between gap-2"
              >
                <span className="inline-flex items-center gap-1.5 text-small break-all text-warn">
                  <WarningIcon size={18} aria-hidden="true" />
                  {host.host}
                </span>
                <span className="text-small text-ink-2 figures">
                  {host.label(time.format(new Date(host.until)))}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

/** How people's lookups on TMDB stand: their photos and biographies, while a TMDB key is saved. */
function People({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const people = health.people
  if (people === null) return <Unknown />
  const time = new Intl.DateTimeFormat(language, { dateStyle: 'medium', timeStyle: 'short' })
  const number = new Intl.NumberFormat(language)
  return (
    <div className="space-y-6">
      <Help>{text.peopleHelp}</Help>
      {!people.keySaved ? (
        <div className="space-y-2 text-small text-ink-2">
          <p>{text.peopleNoKey}</p>
          <ul className="list-disc space-y-1 pl-5">
            {text.peopleNoKeyBrings.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
          <p className="pt-2">
            <TextLink to={settingsPath('content', 'tmdb-key')} className="-ml-3">
              {text.peopleNoKeyLink}
            </TextLink>
          </p>
        </div>
      ) : (
        <Facts
          items={[
            {
              label: text.tmdbKey,
              value: <StatusPill tone="ok">{text.tmdbKeySaved}</StatusPill>,
            },
            { label: text.peopleFound, value: number.format(people.found) },
            { label: text.peopleNotFound, value: number.format(people.notFound) },
            {
              label: text.peopleFailing,
              value:
                people.failing > 0 ? (
                  <StatusPill tone="warn">{number.format(people.failing)}</StatusPill>
                ) : (
                  number.format(0)
                ),
            },
            { label: text.peopleWaiting, value: number.format(people.waiting) },
            {
              label: text.tmdbLimited,
              value:
                people.limitedAt !== null
                  ? text.tmdbLimitedAt(
                      time.format(new Date(people.limitedAt)),
                      people.limitWait > 0 ? formatSpan(people.limitWait, language) : '',
                    )
                  : text.tmdbNeverLimited,
            },
          ]}
        />
      )}
    </div>
  )
}

/** How long the server has run, as of the health's check. */
export function uptime(health: Health, language: string): string {
  return formatSpan(
    (Date.parse(health.checkedAt) - Date.parse(health.process.startedAt)) / 1000,
    language,
  )
}

function Process({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const process = health.process
  return (
    <div className="space-y-6">
      <Facts
        items={[
          { label: text.version, value: process.version },
          { label: text.uptime, value: uptime(health, language) },
          { label: text.memory, value: formatBytes(process.memory, language) },
          { label: text.heap, value: formatBytes(process.heap, language) },
          { label: text.goroutines, value: process.goroutines },
          { label: 'Go', value: process.goVersion.replace(/^go/, '') },
          {
            label: text.lastCrash,
            value:
              process.lastCrash === null ? (
                text.noCrash
              ) : (
                <time dateTime={process.lastCrash.at}>
                  {dateTime(process.lastCrash.at, language)}
                </time>
              ),
          },
        ]}
      />
      {process.lastCrash !== null && (
        <div className="space-y-2">
          <Help>{text.lastCrashHelp}</Help>
          <pre className="max-h-64 overflow-auto rounded-row bg-s1 p-4 font-mono text-micro whitespace-pre-wrap text-ink-2">
            {process.lastCrash.trace}
          </pre>
        </div>
      )}
      <div className="space-y-3">
        <Help>{text.goroutinesHelp}</Help>
        <ExternalButtonLink href={goroutinesDownloadUrl} download icon={DownloadSimpleIcon}>
          {text.goroutinesDownload}
        </ExternalButtonLink>
      </div>
    </div>
  )
}

/** The history state of a part opened from the page, which closing it goes back from. */
export const detailOpened = { healthDetail: true }
