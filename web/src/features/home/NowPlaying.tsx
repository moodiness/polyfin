import { useEffect, useId, useState, type FormEvent, type ReactNode } from 'react'
import {
  ChatCircleTextIcon,
  CpuIcon,
  PackageIcon,
  PauseIcon,
  PlayIcon,
  StopCircleIcon,
  TelevisionSimpleIcon,
  WavesIcon,
  type Icon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
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
  Avatar,
  Badge,
  Button,
  ConfirmDialog,
  cx,
  EmptyState,
  Field,
  IconButton,
  InlineError,
  ProgressBar,
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

/** The rows of what plays now, each over its blurred artwork, for administrators. */
export function NowPlaying({ aside }: { aside?: ReactNode }) {
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
  const titleId = useId()

  return (
    <section id="now-playing" aria-labelledby={titleId} className="scroll-mt-24">
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-2">
        <h2 id={titleId} className="flex items-center gap-2.5 text-h3 text-ink">
          {live.title}
          {sessions.data !== undefined && (
            <span className="inline-grid h-5 min-w-5 place-items-center rounded-md border border-line-2 bg-s2 px-1.5 font-mono text-micro text-ink-2">
              {sessions.data.length}
            </span>
          )}
        </h2>
        {aside && <div className="flex items-center gap-2 text-small text-ink-3">{aside}</div>}
      </div>
      {sessions.isPending ? (
        <div role="status" aria-label={t.common.loading} className="flex flex-col gap-3">
          {[0, 1].map((row) => (
            <div
              key={row}
              className="flex items-center gap-7 rounded-panel border border-line-2 bg-s1 p-[18px]"
            >
              <Skeleton className="h-[114px] w-[76px] rounded-field max-sm:h-[84px] max-sm:w-14" />
              <span className="flex flex-1 flex-col gap-3">
                <Skeleton className="h-3 w-40" />
                <Skeleton className="h-5 w-56" />
                <Skeleton className="h-1 w-full max-w-[520px]" />
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
        <ul className="flex flex-col gap-3">
          {sessions.data.map((session) => (
            <li key={session.id}>
              <SessionRow session={session} elapsed={elapsed} />
            </li>
          ))}
        </ul>
      )}
      <p className="sr-only">{live.autoRefresh}</p>
    </section>
  )
}

function titleOf(session: LiveSession, t: Messages): string {
  const item = session.item
  if (item === null) return t.dashboard.live.unknownTitle
  return item.kind === 'episode' && item.seriesName ? item.seriesName : item.name
}

function posterUrl(posterId: string, height: number): string {
  return `/Items/${encodeURIComponent(posterId)}/Images/Primary?maxHeight=${height}&quality=85`
}

function SessionRow({ session, elapsed }: { session: LiveSession; elapsed: number }) {
  const { language, t } = useI18n()
  const live = t.dashboard.live
  const toast = useToast()
  const item = session.item
  const title = titleOf(session, t)
  const runtime = item?.runtime ?? 0
  const position = session.paused
    ? session.position
    : Math.min(session.position + elapsed, runtime || Number.POSITIVE_INFINITY)
  const isChannel = item?.kind === 'channel'
  const [artFailed, setArtFailed] = useState(false)
  const art = item?.posterId && !artFailed ? item.posterId : null
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

  return (
    <article
      aria-label={title}
      className={cx(
        // Clip, not hidden: a hidden overflow scrolls when the message field takes focus.
        'relative isolate overflow-clip rounded-panel border border-line-2 bg-s1',
        art === null && 'bg-linear-to-r from-accent/12 to-s1',
      )}
    >
      {art !== null && (
        <>
          {/* A small image, blurred by the GPU: the title's light behind the row. */}
          <img
            src={posterUrl(art, 64)}
            alt=""
            aria-hidden="true"
            className="pointer-events-none absolute -inset-15 -z-20 h-[calc(100%+120px)] w-[calc(100%+120px)] max-w-none scale-105 object-cover opacity-60 blur-[46px] saturate-125"
          />
          <div
            aria-hidden="true"
            className="absolute inset-0 -z-10 bg-linear-to-r from-bg/40 via-bg/75 to-bg/90"
          />
        </>
      )}
      <div className="grid grid-cols-[76px_minmax(0,1fr)] items-center gap-x-7 gap-y-5 py-[18px] pr-5 pl-[18px] lg:grid-cols-[76px_minmax(0,1fr)_268px_auto] max-sm:grid-cols-[56px_minmax(0,1fr)] max-sm:gap-x-4 max-sm:p-4">
        {art !== null ? (
          <img
            src={posterUrl(art, 228)}
            alt=""
            loading="lazy"
            onError={() => setArtFailed(true)}
            className={cx(
              'w-full rounded-field shadow-[0_10px_30px_-8px] shadow-black/60 ring-1 ring-ink/8',
              isChannel ? 'aspect-square bg-s2 object-contain p-2' : 'aspect-[2/3] object-cover',
            )}
          />
        ) : (
          <span
            aria-hidden="true"
            className="grid aspect-[2/3] w-full place-items-center rounded-field border border-line-2 bg-s2 text-ink-3"
          >
            {isChannel ? <TelevisionSimpleIcon size={20} /> : <PlayIcon size={20} />}
          </span>
        )}

        <div className="min-w-0">
          <p className="flex min-w-0 items-center gap-2 text-[13px] text-ink-2">
            <Avatar name={session.user.name} image={userImageUrl(session.user)} size="sm" />
            <b className="truncate font-medium text-ink">{session.user.name}</b>
            <span aria-hidden="true" className="size-[3px] shrink-0 rounded-full bg-ink-3" />
            <span className="truncate">{live.appOn(app, session.device.name)}</span>
          </p>
          <h3 className="mt-2 flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-0.5 text-[21px] leading-tight font-semibold tracking-[-0.02em] text-ink max-sm:text-[18px]">
            <span className="min-w-0 truncate" title={title}>
              {title}
            </span>
            {subtitle && (
              <small className="text-[13px] font-normal tracking-normal text-ink-2">
                {subtitle}
              </small>
            )}
          </h3>
          <div className="mt-4 max-w-[520px]">
            {!isChannel && runtime > 0 && (
              <ProgressBar
                variant="brand"
                live={!session.paused}
                value={position / runtime}
                label={live.progress(title)}
                valueText={`${formatClock(position)} / ${formatClock(runtime)}`}
              />
            )}
            <div className="mt-2 flex items-center justify-between gap-3 text-[12.5px] text-ink-2">
              <span className="figures">{formatClock(position)}</span>
              {session.paused ? (
                <StatusPill tone="warn" icon={PauseIcon}>
                  {live.paused}
                </StatusPill>
              ) : isChannel ? (
                <StatusPill tone="live">{live.liveChannel}</StatusPill>
              ) : runtime > 0 ? (
                <span className="text-ink-3">
                  {live.remaining(formatSpan(runtime - position, language))}
                </span>
              ) : null}
              <span className="figures">
                {!isChannel && runtime > 0 ? formatClock(runtime) : ''}
              </span>
            </div>
            <p className="mt-1 text-micro text-ink-3">
              {live.startedAt(relativeTime(session.startedAt, language, t.time.justNow))}
            </p>
          </div>
        </div>

        <div className="col-span-full flex flex-col justify-center gap-1 self-stretch border-line-2 lg:col-span-1 lg:border-l lg:pl-7 max-lg:border-t max-lg:pt-4">
          <p className="text-[12.5px] text-ink-3">{live.howItPlays}</p>
          <p className="flex items-center gap-2 text-[14px] font-medium text-ink">
            <DeliveryIcon size={16} aria-hidden="true" className="text-link" />
            {live.delivery[session.delivery] ?? session.delivery}
          </p>
          <p className="text-[13px] leading-snug text-ink-2">
            {session.delivery === 'directPlay' || session.sent === null
              ? live.deliveryHelp[session.delivery]
              : `${streamText(session.source, language, live.unknown)} → ${streamText(session.sent, language, live.unknown)}`}
          </p>
          <div>
            <button
              type="button"
              aria-expanded={details}
              aria-controls={detailsId}
              onClick={() => setDetails(!details)}
              className="mt-1 rounded-md text-small font-medium text-link transition-colors duration-160 hover:text-link-hover"
            >
              {live.details}
            </button>
          </div>
        </div>

        <div className="col-span-full flex items-center gap-0.5 lg:col-span-1 max-lg:-mt-2 max-lg:justify-end">
          {session.controllable ? (
            <>
              <IconButton
                icon={ChatCircleTextIcon}
                label={live.messageTo(session.user.name)}
                aria-expanded={messaging}
                aria-controls={formId}
                onClick={() => setMessaging(!messaging)}
              />
              <IconButton
                icon={StopCircleIcon}
                label={live.stopFor(session.user.name)}
                danger
                onClick={() => {
                  stop.reset()
                  setConfirming(true)
                }}
              />
            </>
          ) : (
            <p className="text-small text-ink-3 lg:max-w-32">{live.notControllable}</p>
          )}
        </div>

        {details && (
          <div id={detailsId} className="col-span-full">
            <HowItPlays session={session} />
          </div>
        )}
        {messaging && (
          <div id={formId} className="col-span-full">
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
      </div>
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
    </article>
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

/** Why the sent video is smaller than the source, null when it is not or when that is not known. */
function sizeLimitText(
  session: LiveSession,
  language: string,
  limits: Messages['dashboard']['live']['sizeLimits'],
): string | null {
  const { video, source, sent } = session
  if (video === null || source === null || sent === null) return null
  if (video.limitedBy === '' || video.limitedBy === 'source') return null
  if (sent.width >= source.width && sent.height >= source.height) return null
  const height = video.maxHeight === 2160 ? '4K' : `${video.maxHeight}p`
  switch (video.limitedBy) {
    case 'bitrate': {
      const rate = formatBitrate(video.bitrateLimit, language)
      return video.bitrateLimitOf === 'user' ? limits.userBitrate(rate) : limits.appBitrate(rate)
    }
    default:
      return limits[video.limitedBy](height)
  }
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
  const rows = [
    { label: live.source, value: streamText(session.source, language, live.unknown) },
    ...(session.delivery === 'directPlay'
      ? []
      : [{ label: live.sent, value: streamText(session.sent, language, live.unknown) }]),
    ...(sizeLimit === null ? [] : [{ label: live.sizeLimit, value: sizeLimit }]),
    ...(encoder === null ? [] : [{ label: live.encoder, value: encoder }]),
    {
      label: live.qualityGroup,
      value: session.user.qualityGroup > 0 ? `${session.user.qualityGroup}p` : live.original,
    },
  ]

  return (
    <div className="rounded-row border border-line-2 bg-bg/60 px-4 py-3">
      <p className="mb-2 text-small text-ink-2">{live.deliveryHelp[session.delivery]}</p>
      <dl className="grid gap-x-6 gap-y-1.5 text-small sm:grid-cols-[auto_1fr]">
        {rows.map((row) => (
          <div key={row.label} className="contents">
            <dt className="text-ink-3">{row.label}</dt>
            <dd className="figures text-ink max-sm:mb-1">{row.value}</dd>
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
      className="flex flex-col gap-3 rounded-row border border-line-2 bg-bg/60 p-4"
    >
      <Field
        label={live.messageLabel(session.device.name)}
        help={live.messageHint}
        error={send.isError ? errorMessage(t, send.error) : undefined}
      >
        <Textarea value={text} autoFocus rows={2} maxLength={500} required onValue={setText} />
      </Field>
      <div className="flex gap-2">
        <Button
          type="submit"
          variant="primary"
          icon={ChatCircleTextIcon}
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
