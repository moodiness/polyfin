import { useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ArchiveIcon,
  ArrowCircleUpIcon,
  ArrowSquareOutIcon,
  CalendarDotsIcon,
  CaretRightIcon,
  CpuIcon,
  DatabaseIcon,
  DownloadSimpleIcon,
  HardDrivesIcon,
  KeyIcon,
  PuzzlePieceIcon,
  TelevisionSimpleIcon,
} from '@phosphor-icons/react'
import {
  ApiError,
  downloadLanguage,
  fetchSources,
  queryClient,
  queryKeys,
  type Health,
  type HealthProblem,
  type Sources,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { settingsPath } from '@/app/navigation'
import {
  AlertTile,
  Block,
  Button,
  ButtonLink,
  ExternalButtonLink,
  InlineError,
  Modal,
  Notice,
  ProgressBar,
  RelativeTime,
  Row,
  RowList,
  Skeleton,
  SkeletonRows,
  SkeletonText,
  StatusPill,
  type StatusTone,
} from '@/ui'
import { hdrPathText } from '@/features/settings/hdrPath'
import { destinationStatus } from './BackupStatus'
import {
  ocrLanguageName,
  problemText,
  useHealth,
  useHealthProblems,
} from '@/features/system/problems'
import { errorMessage, formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'
import { CheckTile } from './CheckTile'
import {
  addonTone,
  DetailBody,
  detailOpened,
  detailTitle,
  hitRatio,
  isDetailId,
  latest,
  shareState,
  uptime,
  type DetailId,
} from './healthDetails'

/**
 * `/system/health`: what to look at first, as alert tiles; then the checks, as tiles; then the
 * other parts, as rows. A tile or a row opens its part over the page, as `#anchor`, the address
 * the server's problems and notifications link to.
 */
export default function HealthRoute() {
  const { language, t } = useI18n()
  const text = t.system.health
  const health = useHealth()
  // The server's addons, IPTV sources and guides, then each user's own.
  const sources = useQuery({
    queryKey: queryKeys.sources,
    queryFn: ({ signal }) => fetchSources(signal),
    refetchInterval: 30_000,
  })
  const location = useLocation()
  const navigate = useNavigate()
  const anchor = location.hash.slice(1)
  const open = isDetailId(anchor) ? anchor : null
  // The part last opened stays drawn while its dialog closes.
  const [shown, setShown] = useState<DetailId | null>(open)
  if (open !== null && open !== shown) setShown(open)

  function close() {
    // Opened from the page: going back closes it. Reached by a link: the address drops its anchor.
    if ((location.state as typeof detailOpened | null)?.healthDetail) void navigate(-1)
    else void navigate({ hash: '' }, { replace: true, preventScrollReset: true })
  }

  const checked =
    health.data &&
    text.checkedAt(
      new Intl.DateTimeFormat(language, { timeStyle: 'medium' }).format(
        new Date(health.data.checkedAt),
      ),
    )

  return (
    <PageLayout
      title={text.title}
      lede={
        <>
          {text.description}
          {checked && (
            <span className="mt-2 flex items-center gap-2 text-small text-ink-3 figures">
              <span
                aria-hidden="true"
                className="size-1.5 shrink-0 rounded-full bg-ok ring-3 ring-ok/14"
              />
              {checked}
            </span>
          )}
        </>
      }
    >
      {health.isPending ? (
        <HealthSkeleton />
      ) : health.isError ? (
        <InlineError onRetry={() => void health.refetch()} retrying={health.isFetching}>
          {errorMessage(t, health.error)}
        </InlineError>
      ) : (
        <>
          <Problems health={health.data} />
          <Checks health={health.data} sources={sources.data} />
          <More health={health.data} />
        </>
      )}
      {health.data && shown !== null && (
        <Modal open={open !== null} onClose={close} title={detailTitle(t, shown)} width={680}>
          <div className="px-5 pb-5">
            <DetailBody id={shown} health={health.data} sources={sources.data} />
          </div>
        </Modal>
      )}
    </PageLayout>
  )
}

function HealthSkeleton() {
  return (
    <>
      <div>
        <Skeleton className="mb-3 h-6 w-40" />
        <SkeletonRows rows={1} />
      </div>
      <div>
        <Skeleton className="mb-3 h-6 w-32" />
        <div className="grid gap-2.5 md:grid-cols-2 xl:grid-cols-3">
          {[0, 1, 2, 3, 4, 5].map((index) => (
            <div key={index} className="min-h-[9.25rem] rounded-panel bg-s1 px-4 py-3.5">
              <Skeleton className="mb-5 size-7.5 rounded-full" />
              <SkeletonText lines={3} />
            </div>
          ))}
        </div>
      </div>
    </>
  )
}

/** Where a problem's link leads, in words for its button; null for none. */
function problemLink(t: Messages, to: string): string | null {
  const words = t.system.health.problemAction
  const [path, anchor = ''] = to.split('#')
  if (to === '' || (path === '/system/health' && anchor === '')) return null
  if (path === '/system/health') return words.details
  if (path.startsWith('/settings')) return words.settings
  if (path === '/sources') return words.sources
  if (path.startsWith('/sources/')) return words.source
  if (path.startsWith('/users/')) return words.user
  if (path === '/system/schedule') return words.schedule
  return words.open
}

/** The first sentence of a problem, as its title, and the rest. */
function splitSentence(language: string, said: string): [string, string] {
  const first = new Intl.Segmenter(language, { granularity: 'sentence' })
    .segment(said)
    [Symbol.iterator]()
    .next().value?.segment
  if (first === undefined) return [said, '']
  return [first.trim(), said.slice(first.length).trim()]
}

function Problems({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const query = useHealthProblems()
  const problems = query.data ?? []
  // A language's data downloaded, its warning goes, and the tracks waiting for it are read.
  const download = useMutation({
    mutationFn: downloadLanguage,
    onSettled: () =>
      void queryClient.invalidateQueries({ queryKey: queryKeys.healthProblems(language) }),
  })
  const items = problems.length + (health.update ? 1 : 0)
  return (
    <Block title={text.problemsTitle} count={items > 0 ? items : undefined}>
      {query.isPending ? (
        <SkeletonRows rows={1} />
      ) : query.isError ? (
        <InlineError onRetry={() => void query.refetch()} retrying={query.isFetching}>
          {errorMessage(t, query.error)}
        </InlineError>
      ) : items === 0 ? (
        <Notice tone="ok">{text.allGood}</Notice>
      ) : (
        <>
          {download.isError && (
            <InlineError className="mb-3">{downloadError(t, download.error)}</InlineError>
          )}
          <ul
            aria-label={text.problemsTitle}
            className="grid gap-2.5 lg:grid-cols-2 lg:[&>li:last-child:nth-child(odd)]:col-span-2"
          >
            {problems.map((problem) => {
              const [title, rest] = splitSentence(language, problemText(t, language, problem))
              const missing = downloadable(problem, language)
              const downloading = download.isPending && download.variables === missing?.code
              const link = problemLink(t, problem.to)
              const [path, anchor] = problem.to.split('#')
              return (
                <li key={problem.key} className="grid">
                  <AlertTile
                    tone={problem.tone === 'error' ? 'danger' : 'warn'}
                    title={title}
                    actions={
                      missing !== null ? (
                        <Button
                          icon={DownloadSimpleIcon}
                          aria-label={text.downloadLanguageLabel(missing.name)}
                          loading={downloading}
                          disabled={download.isPending && !downloading}
                          onClick={() => download.mutate(missing.code)}
                        >
                          {downloading ? text.downloadingLanguage : text.downloadLanguage}
                        </Button>
                      ) : link !== null ? (
                        path === '/system/health' ? (
                          <ButtonLink
                            to={{ hash: anchor }}
                            state={detailOpened}
                            preventScrollReset
                            iconEnd={CaretRightIcon}
                          >
                            {link}
                          </ButtonLink>
                        ) : (
                          <ButtonLink to={problem.to} iconEnd={CaretRightIcon}>
                            {link}
                          </ButtonLink>
                        )
                      ) : undefined
                    }
                  >
                    {rest === '' ? undefined : rest}
                  </AlertTile>
                </li>
              )
            })}
            {health.update && (
              <li className="grid">
                <AlertTile
                  tone="info"
                  icon={ArrowCircleUpIcon}
                  kind={text.updateKind}
                  title={text.update(health.update.version)}
                  actions={
                    <ExternalButtonLink
                      href={health.update.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      iconEnd={ArrowSquareOutIcon}
                    >
                      {text.releaseNotes}
                    </ExternalButtonLink>
                  }
                >
                  {text.updateRunning(health.process.version)}
                </AlertTile>
              </li>
            )}
          </ul>
        </>
      )}
    </Block>
  )
}

/**
 * The Tesseract code and name of a missing language Polyfin can download the data of, named in
 * `language`; null for any other problem.
 */
function downloadable(
  problem: HealthProblem,
  language: string,
): { code: string; name: string } | null {
  if (problem.code !== 'subtitle_ocr_language' || !problem.download) return null
  return {
    code: problem.file.replace(/\.traineddata$/, ''),
    name: ocrLanguageName(language, problem),
  }
}

/** Why a download failed, with the cause the server names: the folder, or the download's error. */
function downloadError(t: Messages, error: unknown): string {
  const message = errorMessage(t, error)
  return error instanceof ApiError && error.detail ? `${message} ${error.detail}` : message
}

type CheckState = { tone: Exclude<StatusTone, 'live'>; word: string }

type Check = {
  id: string
  to: Parameters<typeof CheckTile>[0]['to']
  state: CheckState
  icon: Parameters<typeof CheckTile>[0]['icon']
  kind: string
  title: ReactNode
  body?: ReactNode
  foot: ReactNode
}

/** A label and its value, as one line: "Last answer: 3 minutes ago". */
function Said({ label, children }: { label: string; children: ReactNode }) {
  const { t } = useI18n()
  return (
    <>
      {label}
      {t.common.colon} {children}
    </>
  )
}

/** The checks of the server's parts, as tiles. */
function Checks({ health, sources }: { health: Health; sources: Sources | undefined }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const words = text.checks
  const number = (value: number) => value.toLocaleString(language)
  const checks: Check[] = []

  const addons = health.addons
  const addonTones = addons.map((addon) => addonTone(addon)[0])
  const one = addons.length === 1 ? addons[0] : undefined
  const answered = latest(addons.map((addon) => addon.lastSuccessAt))
  checks.push({
    id: 'addons',
    to: { hash: 'addons' },
    icon: PuzzlePieceIcon,
    kind: text.addonsTitle,
    state:
      addons.length === 0
        ? { tone: 'muted', word: words.none }
        : addonTones.includes('warn')
          ? { tone: 'warn', word: text.addonStatus.failing }
          : addonTones.includes('ok')
            ? { tone: 'ok', word: text.addonStatus.ok }
            : addons.every((addon) => !addon.enabled)
              ? { tone: 'muted', word: text.addonStatus.off }
              : { tone: 'muted', word: text.addonStatus.unknown },
    title: one ? one.name : addons.length === 0 ? words.none : words.addons(addons.length),
    body:
      addons.length === 0 ? (
        text.noAddonsHint
      ) : (
        <>
          {text.requests(
            addons.reduce((sum, addon) => sum + addon.requests, 0),
            addons.reduce((sum, addon) => sum + addon.failures, 0),
          )}
          {one && one.requests > 0 && one.lastSuccessAt && (
            <>
              {' · '}
              <Said label={text.responseTime}>{`${one.responseTime} ms`}</Said>
            </>
          )}
        </>
      ),
    foot: <Said label={text.lastSuccess}>{answered ? <RelativeTime iso={answered} /> : '–'}</Said>,
  })

  if (sources !== undefined) {
    const iptv = sources.addons.filter((addon) => addon.source !== null)
    const on = iptv.filter((addon) => addon.enabled)
    const live = on.filter((addon) => addon.source!.options.liveTv)
    const lineups = live.map((addon) => addon.source!.lineup)
    const channels = lineups.reduce((sum, lineup) => sum + lineup.channels, 0)
    const next = on
      .map((addon) => addon.source!.nextAt)
      .filter((at): at is string => typeof at === 'string')
      .sort()[0]
    checks.push({
      id: 'iptv',
      to: { hash: 'iptv' },
      icon: TelevisionSimpleIcon,
      kind: text.iptvTitle,
      state:
        iptv.length === 0
          ? { tone: 'muted', word: words.none }
          : on.length === 0
            ? { tone: 'muted', word: text.addonStatus.off }
            : on.some((addon) => addon.source!.error !== '')
              ? { tone: 'warn', word: text.addonStatus.failing }
              : on.some((addon) => addon.source!.fetchedAt)
                ? { tone: 'ok', word: text.ok }
                : { tone: 'muted', word: t.system.schedule.notYet },
      title:
        iptv.length === 1 ? iptv[0].name : iptv.length === 0 ? words.none : words.iptv(iptv.length),
      body:
        iptv.length === 0 ? (
          text.noIptvHint
        ) : live.length > 0 ? (
          <>
            {t.lineup.summary.shownOf(
              number(lineups.reduce((sum, lineup) => sum + lineup.shownChannels, 0)),
              number(channels),
            )}
            {' · '}
            {t.lineup.summary.mappedOf(
              number(lineups.reduce((sum, lineup) => sum + lineup.mapped, 0)),
              number(channels),
            )}
          </>
        ) : undefined,
      foot: next ? (
        <Said label={t.iptv.nextFetch}>
          <RelativeTime iso={next} />
        </Said>
      ) : (
        words.details
      ),
    })

    const catalogs = sources.guides.filter((library) => (library.guides ?? []).length > 0)
    const guides = catalogs.flatMap((library) => library.guides ?? [])
    const mapped = catalogs.flatMap((library) =>
      library.guide !== null && library.guide.channels > 0 ? [library.guide] : [],
    )
    const fetched = latest(guides.map((guide) => guide.fetchedAt))
    checks.push({
      id: 'guides',
      to: { hash: 'guides' },
      icon: CalendarDotsIcon,
      kind: text.guidesTitle,
      state:
        catalogs.length === 0
          ? { tone: 'muted', word: words.none }
          : guides.some((guide) => guide.error !== '')
            ? { tone: 'warn', word: text.addonStatus.failing }
            : fetched
              ? { tone: 'ok', word: text.ok }
              : { tone: 'muted', word: t.system.schedule.notYet },
      title:
        catalogs.length === 1
          ? (catalogs[0].name ?? catalogs[0].catalogName)
          : catalogs.length === 0
            ? words.none
            : words.guides(catalogs.length),
      body:
        catalogs.length === 0
          ? text.noGuidesHint
          : mapped.length > 0
            ? t.lineup.library.mapped(
                number(mapped.reduce((sum, guide) => sum + guide.matched, 0)),
                number(mapped.reduce((sum, guide) => sum + guide.channels, 0)),
              )
            : t.lineup.library.guides(guides.length),
      foot: (
        <Said label={t.libraries.guideFetched}>
          {fetched ? <RelativeTime iso={fetched} /> : t.libraries.guideNever}
        </Said>
      ),
    })
  }

  checks.push({
    id: 'database',
    to: { hash: 'database' },
    icon: DatabaseIcon,
    kind: text.databaseTitle,
    state: health.database.reachable
      ? { tone: 'ok', word: text.reachable }
      : { tone: 'danger', word: text.unreachable },
    title: health.database.size !== null ? formatBytes(health.database.size, language) : '–',
    body: words.databaseSize,
    foot: words.details,
  })

  const transcoder = health.transcoder
  if (transcoder !== null) {
    const hardware = transcoder.hardware
    const hdr = hardware?.hdr
      ? hdrPathText(hardware.hdr, t.settings.conversion.hdrPath, language)
      : null
    checks.push({
      id: 'transcoder',
      to: { hash: 'transcoder' },
      icon: CpuIcon,
      kind: text.transcoderTitle,
      state: !transcoder.enabled
        ? { tone: 'muted', word: words.off }
        : transcoder.limit > 0 && transcoder.conversions >= transcoder.limit
          ? { tone: 'warn', word: words.full }
          : { tone: 'ok', word: words.ready },
      title: hardware ? t.dashboard.live.gpu(hardware.method) : words.processor,
      body: hardware ? (
        <>
          {hardware.encoders.join(', ')}
          {hdr && <span className="block">{hdr.why}</span>}
        </>
      ) : (
        text.noGpu
      ),
      foot: (
        <Said label={text.conversions}>
          {`${transcoder.conversions} / ${transcoder.limit > 0 ? transcoder.limit : text.noLimit}`}
        </Said>
      ),
    })
  }

  const backup = health.backup
  if (backup === null) {
    checks.push({
      id: 'backups',
      to: settingsPath('backups'),
      icon: ArchiveIcon,
      kind: text.backupTitle,
      state: { tone: 'muted', word: words.off },
      title: words.off,
      body: text.backupOff,
      foot: words.openSettings,
    })
  } else {
    const problem = backup.destinations
      .map((destination) => destinationStatus(t, destination))
      .find((status) => status.tone === 'danger' || status.tone === 'warn')
    checks.push({
      id: 'backups',
      to: { hash: 'backups' },
      icon: ArchiveIcon,
      kind: text.backupTitle,
      state:
        backup.error !== ''
          ? { tone: 'danger', word: text.backup.failed }
          : problem
            ? { tone: problem.tone === 'danger' ? 'danger' : 'warn', word: problem.label }
            : backup.ranAt === null
              ? { tone: 'muted', word: text.backup.none }
              : { tone: 'ok', word: text.backup.succeeded },
      title: backup.madeAt ? <RelativeTime iso={backup.madeAt} /> : text.backup.none,
      body: backup.madeAt ? text.backup.made : undefined,
      foot: backup.next ? (
        <Said label={text.backup.next}>
          <RelativeTime iso={backup.next} />
        </Said>
      ) : (
        words.details
      ),
    })
  }

  const cache = health.cache
  if (cache !== null) {
    checks.push({
      id: 'cache',
      to: { hash: 'cache' },
      icon: HardDrivesIcon,
      kind: text.cacheTitle,
      state: { tone: 'ok', word: words.fine },
      title: text.cacheUse(formatBytes(cache.used, language), formatBytes(cache.limit, language)),
      body: (
        <>
          {text.keptFor(cache.days)}
          <ProgressBar
            value={cache.limit > 0 ? cache.used / cache.limit : 0}
            label={text.cacheTitle}
            variant="neutral"
            size="md"
            className="mt-3"
          />
        </>
      ),
      foot:
        cache.hits + cache.misses > 0 ? (
          <Said label={text.hitRatio}>{hitRatio(text, cache, language)}</Said>
        ) : (
          words.details
        ),
    })
  }

  const secrets = health.secrets
  if (secrets !== null) {
    checks.push({
      id: 'secrets',
      to: { hash: 'secrets' },
      icon: KeyIcon,
      kind: text.secretsTitle,
      state:
        secrets.unreadable.length > 0
          ? { tone: 'danger', word: text.unreadable }
          : secrets.encrypted
            ? { tone: 'ok', word: words.encrypted }
            : { tone: secrets.plaintext > 0 ? 'warn' : 'muted', word: words.notEncrypted },
      title: secrets.encrypted ? words.keySet : words.keyNotSet,
      body: <Said label={text.storedPlain}>{number(secrets.plaintext)}</Said>,
      foot: (
        <Said label={text.unreadable}>
          {secrets.unreadable.length === 0
            ? text.noneUnreadable
            : number(secrets.unreadable.length)}
        </Said>
      ),
    })
  }

  // Three columns: one wide tile first, or a wide one first and last, so that no row ends with a
  // hole (see CheckTile for two columns).
  const holes = (3 - (checks.length % 3)) % 3
  const fine = checks.filter((check) => check.state.tone === 'ok').length
  const failing = checks.filter(
    (check) => check.state.tone === 'warn' || check.state.tone === 'danger',
  ).length
  const quiet = checks.length - fine - failing
  return (
    <Block
      title={text.checksTitle}
      aside={
        <span className="mr-3 figures">
          {words.summary(
            checks.length,
            [
              fine > 0 ? words.fineCount(fine) : '',
              failing > 0 ? words.failingCount(failing) : '',
              quiet > 0 ? words.quietCount(quiet) : '',
            ].filter((part) => part !== ''),
          )}
        </span>
      }
    >
      <div className="grid gap-2.5 md:grid-cols-2 xl:grid-cols-3">
        {checks.map((check, index) => (
          <CheckTile
            key={check.id}
            to={check.to}
            wide={(holes >= 1 && index === 0) || (holes === 2 && index === checks.length - 1)}
            icon={check.icon}
            kind={check.kind}
            title={check.title}
            state={<StatusPill tone={check.state.tone}>{check.state.word}</StatusPill>}
            foot={check.foot}
          >
            {check.body}
          </CheckTile>
        ))}
      </div>
    </Block>
  )
}

/** The other parts of the server, as rows that open them. */
function More({ health }: { health: Health }) {
  const { language, t } = useI18n()
  const text = t.system.health
  const words = text.checks
  const navigate = useNavigate()
  const number = new Intl.NumberFormat(language)
  const rows: { id: DetailId; meta?: ReactNode; value: ReactNode }[] = []

  const images = health.images
  if (images !== null) {
    rows.push({
      id: 'images',
      value: text.cacheUse(formatBytes(images.used, language), formatBytes(images.limit, language)),
    })
  }
  // The disk shortest of room first.
  const disk = [...health.disks].sort(
    (a, b) => Number(b.low) - Number(a.low) || (a.free < 0 ? 1 : b.free < 0 ? -1 : a.free - b.free),
  )[0]
  if (disk !== undefined) {
    const free = disk.free >= 0 ? text.free(formatBytes(disk.free, language)) : text.notMeasured
    rows.push({
      id: 'disks',
      meta:
        health.disks.length > 1
          ? words.moreFolders(text.folderTitle[disk.folder], health.disks.length - 1)
          : text.folderTitle[disk.folder],
      value: disk.low ? <StatusPill tone="warn">{free}</StatusPill> : free,
    })
  }
  const thumbnails = health.thumbnails
  if (thumbnails !== null) {
    rows.push({
      id: 'thumbnails',
      value: !thumbnails.enabled ? (
        words.off
      ) : thumbnails.pausedHosts.length > 0 ? (
        <StatusPill tone="warn">{text.pausedHosts}</StatusPill>
      ) : thumbnails.working ? (
        text.working
      ) : (
        text.idle
      ),
    })
  }
  if (health.shares.length > 0) {
    const warn = health.shares.some((share) => shareState(health, share, text)[0] === 'warn')
    rows.push({
      id: 'shares',
      value: warn ? (
        <StatusPill tone="warn">{text.shareStatus.unwatched}</StatusPill>
      ) : (
        number.format(health.shares.length)
      ),
    })
  }
  const torrents = health.torrents
  if (torrents !== null) {
    rows.push({
      id: 'torrents',
      meta: text.torrentsPlaying,
      value:
        torrents.atOnce > 0
          ? text.torrentsPlayingOf(torrents.playing, torrents.atOnce)
          : number.format(torrents.playing),
    })
  }
  const ratings = health.ratings
  if (ratings !== null) {
    rows.push({
      id: 'ratings',
      meta: text.requestsLeft,
      value:
        ratings.heldUntil !== null ? (
          <StatusPill tone={ratings.held === 'refused' ? 'danger' : 'warn'}>
            {text.heldReasons[ratings.held] || words.held}
          </StatusPill>
        ) : ratings.remaining !== null && ratings.limit !== null ? (
          text.requestsLeftOf(number.format(ratings.remaining), number.format(ratings.limit))
        ) : (
          '–'
        ),
    })
  }
  const artwork = health.artwork
  if (artwork !== null) {
    rows.push({
      id: 'artwork',
      meta: text.artworkFailed,
      value:
        artwork.failed > 0 ? (
          <StatusPill tone="warn">{number.format(artwork.failed)}</StatusPill>
        ) : (
          number.format(0)
        ),
    })
  }
  const people = health.people
  if (people !== null) {
    rows.push({
      id: 'people',
      meta: people.keySaved ? text.peopleFound : undefined,
      value: people.keySaved ? number.format(people.found) : words.noKey,
    })
  }
  if (health.proxyNames.length > 0) {
    rows.push({
      id: 'proxies',
      value: health.proxyNames.some((proxy) => proxy.addresses.length === 0) ? (
        <StatusPill tone="warn">{text.resolvesToNothing}</StatusPill>
      ) : (
        number.format(health.proxyNames.length)
      ),
    })
  }
  rows.push({
    id: 'process',
    meta:
      health.process.lastCrash !== null ? (
        <Said label={text.lastCrash}>
          <RelativeTime iso={health.process.lastCrash.at} />
        </Said>
      ) : undefined,
    value: words.versionUptime(health.process.version, uptime(health, language)),
  })

  return (
    <Block title={text.moreTitle}>
      <RowList aria-label={text.moreTitle} className="lg:grid-cols-2">
        {rows.map((row) => (
          <Row
            key={row.id}
            title={detailTitle(t, row.id)}
            meta={row.meta}
            trailing={<span className="figures">{row.value}</span>}
            onClick={() =>
              void navigate({ hash: row.id }, { state: detailOpened, preventScrollReset: true })
            }
          />
        ))}
      </RowList>
    </Block>
  )
}
