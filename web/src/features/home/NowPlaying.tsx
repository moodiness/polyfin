import { useEffect, useId, useState, type FormEvent } from 'react'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ChatTextIcon,
  CpuIcon,
  DeviceMobileIcon,
  DownloadSimpleIcon,
  FilmStripIcon,
  InfoIcon,
  PackageIcon,
  PauseIcon,
  PlayIcon,
  StopIcon,
  TelevisionSimpleIcon,
  WavesIcon,
  type Icon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchBandwidth,
  fetchLiveSessions,
  messageLiveSession,
  queryClient,
  queryKeys,
  stopLiveSession,
  userImageUrl,
  type LiveSession,
  type StreamInfo,
} from '@/api'
import {
  errorMessage,
  formatBitrate,
  formatClock,
  formatEpisode,
  formatSpan,
  relativeTime,
} from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  ArtShelf,
  ArtTile,
  Avatar,
  Badge,
  Block,
  Button,
  ConfirmDialog,
  cx,
  EmptyState,
  Field,
  FigureTile,
  IconButton,
  InlineError,
  Panel,
  ProgressBar,
  Row,
  RowList,
  Skeleton,
  StatusPill,
  Textarea,
  useToast,
} from '@/ui'

/** How long a message shows on the device, in seconds. */
const messageSeconds = 10

const codecNames: Record<string, string> = {
  h264: 'H.264',
  hevc: 'HEVC',
  av1: 'AV1',
  vp9: 'VP9',
  mpeg2video: 'MPEG-2',
  aac: 'AAC',
  ac3: 'AC-3',
  eac3: 'E-AC-3',
  opus: 'Opus',
  flac: 'FLAC',
  truehd: 'TrueHD',
  dts: 'DTS',
  mp3: 'MP3',
}

const deliveryIcons: Record<string, Icon> = {
  directPlay: PlayIcon,
  remux: PackageIcon,
  conversion: CpuIcon,
  stream: WavesIcon,
}

/** The playbacks under way, refreshed every 3 seconds; shared with the page's summary. */
export function useLiveSessions() {
  return useQuery({
    queryKey: queryKeys.liveSessions,
    queryFn: ({ signal }) => fetchLiveSessions(signal),
    refetchInterval: 3_000,
  })
}

/** A rate in bytes per second as a bitrate, as the dashboard shows bitrates: "8.2 Mbps". */
function rateText(bytesPerSecond: number, language: string): string {
  return formatBitrate(bytesPerSecond * 8, language)
}

/** A rate split into its figure and its unit ("8.2", "Mbps"), for a big figure. */
function rateParts(bytesPerSecond: number, language: string): { value: string; unit: string } {
  const text = rateText(bytesPerSecond, language)
  const space = text.lastIndexOf(' ')
  return { value: text.slice(0, space), unit: text.slice(space + 1) }
}

/**
 * Home's Bandwidth panel: what every play moves now, sent to apps and received from sources, as
 * two big figures refreshed with Playing now, then the video conversions under way.
 */
export function Bandwidth({
  conversions,
}: {
  /** The conversions under way and their limit, once Health tells them. */
  conversions?: { count: number; limit: number }
}) {
  const { language, t } = useI18n()
  const text = t.dashboard.overview
  const total = useQuery({
    queryKey: queryKeys.bandwidth,
    queryFn: ({ signal }) => fetchBandwidth(signal),
    refetchInterval: 3_000,
  })
  const sent = total.data && rateParts(total.data.sent, language)
  const received = total.data && rateParts(total.data.received, language)
  return (
    <Block title={text.bandwidth}>
      <Panel as="div">
        {total.isError && total.data === undefined ? (
          <InlineError onRetry={() => void total.refetch()} retrying={total.isFetching}>
            {errorMessage(t, total.error)}
          </InlineError>
        ) : sent && received ? (
          <div className="grid grid-cols-2 gap-6 max-md:gap-4">
            <FigureTile
              value={sent.value}
              unit={sent.unit}
              label={
                <span className="flex items-center gap-1.5">
                  <ArrowUpIcon size="1.1875rem" aria-hidden="true" className="shrink-0" />
                  {text.sentToApps}
                </span>
              }
            />
            <FigureTile
              value={received.value}
              unit={received.unit}
              label={
                <span className="flex items-center gap-1.5">
                  <ArrowDownIcon size="1.1875rem" aria-hidden="true" className="shrink-0" />
                  {text.receivedFromSources}
                </span>
              }
            />
          </div>
        ) : (
          <div role="status" aria-label={t.common.loading} className="grid grid-cols-2 gap-6">
            {[0, 1].map((figure) => (
              <span key={figure} className="flex flex-col gap-3">
                <Skeleton className="h-11 w-24" />
                <Skeleton className="h-4 w-32" />
              </span>
            ))}
          </div>
        )}
        {conversions && (
          <div className="mt-6 pt-2 shadow-[inset_0_1px_0] shadow-line-2">
            <RowList aria-label={text.conversions}>
              <Row
                to="/system/health#transcoder"
                title={<span className="text-body font-medium">{text.conversions}</span>}
                trailing={
                  <span className="figures font-semibold text-ink">
                    {text.conversionsOf(conversions.count, conversions.limit)}
                  </span>
                }
                className="min-h-16 max-md:min-h-14"
              />
            </RowList>
          </div>
        )}
      </Panel>
    </Block>
  )
}

/**
 * Playing now: each playback as a large art tile on its blurred poster, for administrators. The
 * lit tile opens wide and tells `onLight` whose poster tints the top of the page.
 */
export function PlayingNow({ onLight }: { onLight: (sessionId: string) => void }) {
  const { t } = useI18n()
  const live = t.dashboard.live
  const sessions = useLiveSessions()
  // Positions move on between refreshes, once a second, as players do.
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1_000)
    return () => window.clearInterval(timer)
  }, [])
  const elapsed = Math.max(0, (now - sessions.dataUpdatedAt) / 1000)

  return (
    <Block title={live.title} count={sessions.data?.length} className="scroll-mt-24">
      <div id="now-playing" className="scroll-mt-24">
        {sessions.isPending ? (
          <div role="status" aria-label={t.common.loading} className="flex gap-6 max-md:flex-col">
            {[7, 5].map((grow) => (
              <div
                key={grow}
                style={{ flexGrow: grow }}
                className="flex basis-0 gap-[1.875rem] rounded-tile bg-s1 px-8 pt-[1.625rem] pb-6 max-md:rounded-[1.75rem] max-md:p-[1.125rem]"
              >
                <Skeleton className="aspect-[400/560] w-[clamp(6.75rem,15.5vh,8.75rem)] rounded-[1.125rem] max-md:w-24" />
                <span className="flex flex-1 flex-col gap-3 pt-2">
                  <Skeleton className="h-4 w-40" />
                  <Skeleton className="h-9 w-56 max-w-full" />
                  <Skeleton className="h-4 w-24" />
                </span>
              </div>
            ))}
          </div>
        ) : sessions.isError ? (
          <InlineError onRetry={() => void sessions.refetch()} retrying={sessions.isFetching}>
            {errorMessage(t, sessions.error)}
          </InlineError>
        ) : sessions.data.length === 0 ? (
          <EmptyState icon={TelevisionSimpleIcon} title={live.empty}>
            {live.emptyHelp}
          </EmptyState>
        ) : (
          <ArtShelf label={live.title}>
            {sessions.data.map((session) => (
              <SessionTile
                key={session.id}
                session={session}
                elapsed={elapsed}
                onLight={() => onLight(session.id)}
              />
            ))}
          </ArtShelf>
        )}
      </div>
      <p className="sr-only">{live.autoRefresh}</p>
    </Block>
  )
}

export function titleOf(session: LiveSession, t: Messages): string {
  const item = session.item
  if (item === null) return t.dashboard.live.unknownTitle
  return item.kind === 'episode' && item.seriesName ? item.seriesName : item.name
}

/** A title's poster, `height` pixels high at most. */
export function posterUrl(posterId: string, height: number): string {
  return `/Items/${encodeURIComponent(posterId)}/Images/Primary?maxHeight=${height}&quality=85`
}

/**
 * One playback as an art tile. `mine` draws a member's own: "You" and the app on the device on
 * top, its facts when wide, and no Details, message or stop.
 */
function SessionTile({
  session,
  elapsed,
  onLight,
  mine = false,
}: {
  session: LiveSession
  elapsed: number
  onLight: () => void
  mine?: boolean
}) {
  const { language, t } = useI18n()
  const live = t.dashboard.live
  const toast = useToast()
  const item = session.item
  const title = titleOf(session, t)
  const runtime = item?.runtime ?? 0
  // A download's position is how far its file is made, told by the server alone.
  const position =
    session.paused || session.download
      ? session.position
      : Math.min(session.position + elapsed, runtime || Number.POSITIVE_INFINITY)
  const isChannel = item?.kind === 'channel'
  const art = item?.posterId || null
  const [confirming, setConfirming] = useState(false)
  const [messaging, setMessaging] = useState(false)
  const [details, setDetails] = useState(false)
  const detailsId = useId()
  const formId = useId()

  const stop = useMutation({
    mutationFn: () => stopLiveSession(session.id),
    onSuccess: () => {
      setConfirming(false)
      toast(live.stopped, { tone: 'ok' })
      void queryClient.invalidateQueries({ queryKey: queryKeys.liveSessions })
    },
  })

  const subtitle =
    item?.kind === 'episode'
      ? `${formatEpisode(item.season, item.episode)}${item.seriesName ? ` · ${item.name}` : ''}`
      : item?.kind === 'movie'
        ? live.movie(item.year)
        : item && !isChannel && item.year > 0
          ? String(item.year)
          : ''
  const app = [session.device.app, session.device.appVersion].filter(Boolean).join(' ')
  const DeliveryIcon = deliveryIcons[session.delivery] ?? PlayIcon
  /** Where the version is read from: "Torrent · Its addon · 12 peers". */
  const origin =
    session.origin &&
    [
      live.origins[session.origin.kind] ?? session.origin.kind,
      session.origin.name,
      session.origin.kind === 'torrent' ? live.peers(session.origin.peers) : '',
    ]
      .filter(Boolean)
      .join(' · ')
  const where = session.shared
    ? live.sharedLink
    : session.download
      ? live.downloadOn(app, session.device.name)
      : session.device.name
  const timed = !isChannel && runtime > 0

  return (
    <ArtTile
      title={title}
      // A channel's logo is shown whole on a dark fill; a missing or broken poster gives the
      // frame the icon of its kind.
      poster={art !== null ? posterUrl(art, 600) : undefined}
      fit={isChannel ? 'contain' : 'cover'}
      fallback={isChannel ? <TelevisionSimpleIcon size="1em" /> : <FilmStripIcon size="1em" />}
      backdrop={art !== null ? posterUrl(art, 64) : undefined}
      onLight={onLight}
      top={
        <>
          <Avatar name={session.user.name} image={userImageUrl(session.user)} size="sm" />
          <span className="min-w-0 truncate">
            <b className="font-semibold text-ink">
              {mine ? t.dashboard.member.you : session.user.name}
            </b>
            {mine
              ? ` · ${session.download ? live.downloadOn(app, session.device.name) : live.appOn(app, session.device.name)}`
              : where && ` · ${where}`}
          </span>
        </>
      }
      meta={subtitle || undefined}
      tools={
        mine ? undefined : (
          <>
            <IconButton
              icon={InfoIcon}
              label={live.details}
              aria-expanded={details}
              aria-controls={detailsId}
              onClick={() => setDetails(!details)}
              className="bg-s2"
            />
            {session.controllable && (
              <>
                <IconButton
                  icon={ChatTextIcon}
                  label={live.messageTo(session.user.name)}
                  aria-expanded={messaging}
                  aria-controls={formId}
                  onClick={() => setMessaging(!messaging)}
                  className="bg-s2"
                />
                <IconButton
                  icon={StopIcon}
                  label={live.stopFor(session.user.name)}
                  danger
                  onClick={() => {
                    stop.reset()
                    setConfirming(true)
                  }}
                  className="bg-s2"
                />
              </>
            )}
          </>
        )
      }
      detail={
        mine ? (
          <dl className="figures mt-1 grid max-w-[27.5rem] grid-cols-[auto_minmax(0,1fr)] gap-x-6 gap-y-1.5 rounded-[1.375rem] bg-s1 px-5 py-4 max-md:grid-cols-[minmax(0,1fr)] max-md:gap-y-0.5 max-md:px-4 max-md:[&_dd]:mb-1.5 max-md:[&_dd]:text-left">
            {origin && (
              <>
                <dt className="text-ink-3">{live.source}</dt>
                <dd className="min-w-0 text-right break-words text-ink">{origin}</dd>
              </>
            )}
            <dt className="text-ink-3">{live.qualityGroup}</dt>
            <dd className="text-right text-ink">
              {session.user.qualityGroup > 0 ? `${session.user.qualityGroup}p` : live.original}
            </dd>
          </dl>
        ) : (
          <div className="flex flex-col gap-1">
            {origin && (
              <p className="flex min-w-0 items-center gap-2" title={origin}>
                <span className="min-w-0 truncate">{origin}</span>
              </p>
            )}
            {!session.controllable && (
              <p className="text-ink-3">
                {session.shared
                  ? live.sharedNotControllable
                  : session.download
                    ? live.downloadNotControllable
                    : live.notControllable}
              </p>
            )}
          </div>
        )
      }
      footer={
        <div className="flex flex-col">
          {timed && (
            <ProgressBar
              variant="playback"
              live={!session.paused}
              value={position / runtime}
              label={session.download ? live.downloadProgress(title) : live.progress(title)}
              valueText={`${formatClock(position)} / ${formatClock(runtime)}`}
            />
          )}
          <div
            className={cx(
              'flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-[1.0625rem] text-ink-2 max-md:text-micro',
              timed && 'mt-3',
            )}
          >
            <span className="figures">
              <b className="font-semibold text-ink">{formatClock(position)}</b>
              {timed && ` ${live.ofRuntime(formatClock(runtime))}`}
            </span>
            {session.download ? (
              <StatusPill tone="muted" icon={DownloadSimpleIcon}>
                {live.downloading}
              </StatusPill>
            ) : session.paused ? (
              <StatusPill tone="warn" icon={PauseIcon}>
                {live.paused}
              </StatusPill>
            ) : isChannel ? (
              <StatusPill tone="live">{live.liveChannel}</StatusPill>
            ) : runtime > 0 ? (
              <span>{live.remaining(formatSpan(runtime - position, language))}</span>
            ) : null}
          </div>
          <p className="figures mt-2 flex flex-wrap items-center gap-x-5 gap-y-1 text-[1.03125rem] text-ink-2 max-md:text-micro">
            {session.bandwidth.redirected ? (
              <span>{live.bandwidth.redirected}</span>
            ) : (
              <span className="flex items-center gap-1.5">
                <ArrowUpIcon size="1.0625rem" aria-hidden="true" className="shrink-0" />
                {live.bandwidth.sent(rateText(session.bandwidth.sent, language))}
              </span>
            )}
            {(session.origin !== null || session.bandwidth.received > 0) && (
              <span className="flex items-center gap-1.5">
                <ArrowDownIcon size="1.0625rem" aria-hidden="true" className="shrink-0" />
                {live.bandwidth.received(rateText(session.bandwidth.received, language))}
              </span>
            )}
            <span className="text-ink-3">
              {live.startedAt(relativeTime(session.startedAt, language, t.time.justNow))}
            </span>
          </p>
          {details && (
            <div id={detailsId} className="mt-5">
              <HowItPlays session={session} />
            </div>
          )}
          {messaging && (
            <div id={formId} className="mt-5">
              <MessageForm
                session={session}
                onClose={() => setMessaging(false)}
                onSent={() => {
                  setMessaging(false)
                  toast(live.messageSent, { tone: 'ok' })
                }}
              />
            </div>
          )}
          <ConfirmDialog
            open={confirming}
            onClose={() => setConfirming(false)}
            onConfirm={() => stop.mutate()}
            title={live.stopTitle}
            confirmLabel={live.stop}
            cancelLabel={t.common.cancel}
            tone="danger"
            busy={stop.isPending}
            error={stop.isError ? errorMessage(t, stop.error) : undefined}
          >
            {live.stopConfirm(session.user.name, title)}
          </ConfirmDialog>
        </div>
      }
    >
      <p className="flex flex-col gap-0.5">
        <strong className="flex items-center gap-2 text-[1.1875rem] font-semibold text-ink max-md:text-[1.03125rem]">
          <DeliveryIcon size="1.1875rem" aria-hidden="true" className="shrink-0" />
          {live.delivery[session.delivery] ?? session.delivery}
        </strong>
        <span className="max-w-[46ch]">
          {session.delivery === 'directPlay' || session.sent === null
            ? live.deliveryHelp[session.delivery]
            : `${streamText(session.source, language, live.unknown)} → ${streamText(session.sent, language, live.unknown)}`}
        </span>
      </p>
      {app && !mine && !session.shared && !session.download && (
        <p className="mt-2 flex min-w-0 items-center gap-2">
          <DeviceMobileIcon size="1.125rem" aria-hidden="true" className="shrink-0" />
          <span className="min-w-0 truncate">{app}</span>
        </p>
      )}
    </ArtTile>
  )
}

/**
 * A member's own playbacks, as wide art tiles under "Now playing", refreshed every 3 seconds as
 * an administrator's are. The server answers a member their own only. Nothing shows while they
 * play nothing, nor while the list loads or fails: it is not what their Home is for.
 */
export function MyPlayback({ onLight }: { onLight: (sessionId: string) => void }) {
  const { t } = useI18n()
  const live = t.dashboard.live
  const sessions = useLiveSessions()
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1_000)
    return () => window.clearInterval(timer)
  }, [])
  const elapsed = Math.max(0, (now - sessions.dataUpdatedAt) / 1000)
  if (sessions.data === undefined || sessions.data.length === 0) return null
  return (
    <Block title={live.title}>
      <ArtShelf label={live.title}>
        {sessions.data.map((session) => (
          <SessionTile
            key={session.id}
            session={session}
            elapsed={elapsed}
            onLight={() => onLight(session.id)}
            mine
          />
        ))}
      </ArtShelf>
      <p className="sr-only">{live.autoRefresh}</p>
    </Block>
  )
}

function streamText(stream: StreamInfo | null, language: string, unknown: string): string {
  if (stream === null) return unknown
  const parts = [
    stream.height > 0 ? `${stream.height}p` : '',
    [stream.videoCodec, stream.audioCodec]
      .filter(Boolean)
      .map((codec) => codecNames[codec] ?? codec.toUpperCase())
      .join(' · '),
    stream.bitrate > 0 ? formatBitrate(stream.bitrate, language) : '',
  ].filter(Boolean)
  return parts.length > 0 ? parts.join(' · ') : unknown
}

/** A height as the dashboard names it: 4K, or 1080p. */
function heightName(height: number): string {
  return height === 2160 ? '4K' : `${height}p`
}

/**
 * Why the sent video is smaller than the source, null when it is not or when that is not known:
 * what set the size of the first quality, and that the app stepped down from it.
 */
function sizeLimitText(
  session: LiveSession,
  language: string,
  limits: Messages['dashboard']['live']['sizeLimits'],
): string | null {
  const { video, source, sent } = session
  if (video === null || source === null || sent === null) return null
  const stepped =
    video.steppedDownFrom > 0 ? limits.steppedDown(heightName(video.steppedDownFrom)) : null
  let reason: string | null = null
  switch (video.limitedBy) {
    case '':
    case 'source':
      break
    case 'bitrate': {
      const rate = formatBitrate(video.bitrateLimit, language)
      reason = video.bitrateLimitOf === 'user' ? limits.userBitrate(rate) : limits.appBitrate(rate)
      break
    }
    default:
      reason = limits[video.limitedBy](heightName(video.maxHeight))
  }
  if (stepped !== null) return reason === null ? stepped : `${reason} · ${stepped}`
  if (sent.width >= source.width && sent.height >= source.height) return null
  return reason
}

/** Everything about how the file reaches the app: source, what is sent, encoder, reasons. */
function HowItPlays({ session }: { session: LiveSession }) {
  const { language, t } = useI18n()
  const live = t.dashboard.live
  const video = session.video
  const encoder =
    video === null
      ? session.delivery === 'directPlay'
        ? null
        : live.copied
      : `${video.hardware ? live.gpu(video.hardware) : live.cpu} · ${video.encoder}${
          video.toneMap ? ` · ${live.toneMapped}` : ''
        }${video.burnSubtitles ? ` · ${live.burned}` : ''}`
  const sizeLimit = sizeLimitText(session, language, live.sizeLimits)
  /** The quality played now among those offered, when the app may switch between several. */
  const quality =
    video !== null && video.qualities.length > 1
      ? live.qualityAmong(heightName(video.height), video.qualities.map(heightName).join(' · '))
      : null
  const rows = [
    { label: live.source, value: streamText(session.source, language, live.unknown) },
    ...(session.switchedVersion === ''
      ? []
      : [{ label: live.switchedVersion, value: session.switchedVersion }]),
    ...(session.delivery === 'directPlay'
      ? []
      : [{ label: live.sent, value: streamText(session.sent, language, live.unknown) }]),
    ...(quality === null ? [] : [{ label: live.qualityPlayed, value: quality }]),
    ...(sizeLimit === null ? [] : [{ label: live.sizeLimit, value: sizeLimit }]),
    ...(encoder === null ? [] : [{ label: live.encoder, value: encoder }]),
    {
      label: live.qualityGroup,
      value: session.user.qualityGroup > 0 ? `${session.user.qualityGroup}p` : live.original,
    },
  ]

  return (
    <div className="rounded-[1.375rem] bg-bg/55 px-5 py-4 text-[1.03125rem] max-md:px-4 max-md:text-micro">
      {/* The tile tells this help already when it shows no streams in its place. */}
      {session.delivery !== 'directPlay' && session.sent !== null && (
        <p className="mb-3 text-ink-2">{live.deliveryHelp[session.delivery]}</p>
      )}
      <dl className="grid gap-x-6 gap-y-1.5 sm:grid-cols-[auto_minmax(0,1fr)]">
        {rows.map((row) => (
          <div key={row.label} className="contents">
            <dt className="text-ink-3">{row.label}</dt>
            <dd className="figures break-words text-ink max-sm:mb-1">{row.value}</dd>
          </div>
        ))}
        {session.reasons.length > 0 && (
          <div className="contents">
            <dt className="text-ink-3">{live.why}</dt>
            <dd>
              <ul className="flex flex-wrap gap-1">
                {session.reasons.map((reason) => (
                  <li key={reason}>
                    <Badge>{t.dashboard.reasons[reason] ?? reason}</Badge>
                  </li>
                ))}
              </ul>
            </dd>
          </div>
        )}
      </dl>
    </div>
  )
}

function MessageForm({
  session,
  onSent,
  onClose,
}: {
  session: LiveSession
  onSent: () => void
  onClose: () => void
}) {
  const { t } = useI18n()
  const live = t.dashboard.live
  const [text, setText] = useState('')
  const send = useMutation({
    mutationFn: () =>
      messageLiveSession(session.id, { text: text.trim(), timeout: messageSeconds }),
    onSuccess: () => {
      setText('')
      onSent()
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (text.trim() !== '') send.mutate()
  }

  return (
    <form
      onSubmit={submit}
      className="flex flex-col gap-4 rounded-[1.375rem] bg-bg/55 p-5 max-md:p-4"
    >
      <Field
        label={live.messageLabel(session.device.name)}
        help={live.messageHint}
        error={send.isError ? errorMessage(t, send.error) : undefined}
      >
        <Textarea value={text} autoFocus rows={2} maxLength={500} required onValue={setText} />
      </Field>
      <div className="flex flex-wrap gap-3">
        <Button
          type="submit"
          variant="primary"
          icon={ChatTextIcon}
          loading={send.isPending}
          disabled={text.trim() === ''}
        >
          {live.send}
        </Button>
        <Button variant="ghost" onClick={onClose}>
          {t.common.cancel}
        </Button>
      </div>
    </form>
  )
}
