import { useEffect, useId, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchLiveSessions,
  messageLiveSession,
  queryClient,
  queryKeys,
  stopLiveSession,
  type LiveSession,
  type StreamInfo,
} from '@/api'
import { icons } from '@/components/icons'
import { Empty, Panel, Skeleton } from '@/components/panels'
import { Badge, buttonPrimary, buttonSecondary, ConfirmButton, Notice } from '@/components/ui'
import { errorMessage, formatBitrate, formatClock, relativeTime } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'

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

/** The playbacks under way, refreshed every 3 seconds, for administrators. */
export default function LiveSessions() {
  const { t } = useI18n()
  const sessions = useQuery({
    queryKey: queryKeys.liveSessions,
    queryFn: ({ signal }) => fetchLiveSessions(signal),
    refetchInterval: 3_000,
  })
  // Positions move on between refreshes, once a second, as players do.
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1_000)
    return () => window.clearInterval(timer)
  }, [])
  const elapsed = Math.max(0, (now - sessions.dataUpdatedAt) / 1000)

  return (
    <Panel
      id="now-playing"
      title={t.dashboard.live.title}
      description={t.dashboard.live.autoRefresh}
      actions={
        sessions.data !== undefined && sessions.data.length > 0 ? (
          <Badge tone="fin">{sessions.data.length}</Badge>
        ) : undefined
      }
    >
      {sessions.isPending ? (
        <Skeleton rows={2} label={t.common.loading} />
      ) : sessions.isError ? (
        <Notice kind="error">{errorMessage(t, sessions.error)}</Notice>
      ) : sessions.data.length === 0 ? (
        <Empty hint={t.dashboard.live.emptyHelp}>{t.dashboard.live.empty}</Empty>
      ) : (
        <ul className="grid grid-cols-[minmax(0,1fr)] gap-4 xl:grid-cols-2">
          {sessions.data.map((session) => (
            <li key={session.id} className="min-w-0">
              <SessionCard session={session} elapsed={elapsed} />
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}

function titleOf(session: LiveSession, t: Messages): string {
  const item = session.item
  if (item === null) return t.dashboard.live.unknownTitle
  return item.kind === 'episode' && item.seriesName ? item.seriesName : item.name
}

function SessionCard({ session, elapsed }: { session: LiveSession; elapsed: number }) {
  const { language, t } = useI18n()
  const live = t.dashboard.live
  const item = session.item
  const title = titleOf(session, t)
  const runtime = item?.runtime ?? 0
  const position = session.paused
    ? session.position
    : Math.min(session.position + elapsed, runtime || Number.POSITIVE_INFINITY)
  const isChannel = item?.kind === 'channel'
  const [notice, setNotice] = useState<string | null>(null)

  const stop = useMutation({
    mutationFn: () => stopLiveSession(session.id),
    onSuccess: () => {
      setNotice(live.stopped)
      void queryClient.invalidateQueries({ queryKey: queryKeys.liveSessions })
    },
  })

  return (
    <article
      aria-label={title}
      className="flex h-full flex-col gap-4 rounded-xl border border-line bg-surface-2/60 p-4"
    >
      <div className="flex gap-4">
        <Poster session={session} />
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 className="truncate font-semibold text-white" title={title}>
                {title}
              </h3>
              <p className="truncate text-sm text-muted">
                {item?.kind === 'episode'
                  ? `${live.episode(item.season, item.episode)} · ${item.name}`
                  : isChannel
                    ? live.liveChannel
                    : item && item.year > 0
                      ? String(item.year)
                      : ''}
              </p>
            </div>
            <DeliveryBadge session={session} />
          </div>
          <div className="mt-3 flex items-center gap-2.5 text-sm">
            <Avatar session={session} />
            <div className="min-w-0">
              <p className="truncate font-medium text-zinc-100">{session.user.name}</p>
              <p className="truncate text-xs text-muted">
                {[
                  session.device.name,
                  [session.device.app, session.device.appVersion].filter(Boolean).join(' '),
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </p>
            </div>
          </div>
        </div>
      </div>

      <div>
        {!isChannel && runtime > 0 ? (
          <div
            role="progressbar"
            aria-label={live.progress(title)}
            aria-valuemin={0}
            aria-valuemax={Math.round(runtime)}
            aria-valuenow={Math.round(position)}
            aria-valuetext={`${formatClock(position)} / ${formatClock(runtime)}`}
            className="h-1.5 overflow-hidden rounded-full bg-line"
          >
            <div
              className="bg-fin-gradient h-full origin-left rounded-full transition-transform duration-1000 ease-linear"
              style={{ transform: `scaleX(${Math.min(1, position / runtime)})` }}
            />
          </div>
        ) : null}
        <div className="mt-1.5 flex items-center justify-between gap-3 text-xs text-muted tabular-nums">
          <span>
            {formatClock(position)}
            {!isChannel && runtime > 0 && ` / ${formatClock(runtime)}`}
          </span>
          <span className="flex items-center gap-2">
            {session.paused && (
              <Badge tone="warning">
                <icons.pause className="size-3" />
                {live.paused}
              </Badge>
            )}
            <span>{live.startedAt(relativeTime(session.startedAt, language, t.time.justNow))}</span>
          </span>
        </div>
      </div>

      <HowItPlays session={session} />

      <div className="mt-auto space-y-3">
        {notice !== null && <Notice kind="success">{notice}</Notice>}
        {stop.isError && <Notice kind="error">{errorMessage(t, stop.error)}</Notice>}
        {session.controllable ? (
          <div className="flex flex-wrap items-start gap-2">
            <ConfirmButton
              label={live.stop}
              busyLabel={live.stopping}
              message={live.stopConfirm(session.user.name, title)}
              busy={stop.isPending}
              onConfirm={() => {
                setNotice(null)
                stop.mutate()
              }}
            />
            <MessageForm session={session} onSent={() => setNotice(live.messageSent)} />
          </div>
        ) : (
          <p className="text-xs text-muted">{live.notControllable}</p>
        )}
      </div>
    </article>
  )
}

function Poster({ session }: { session: LiveSession }) {
  const [failed, setFailed] = useState(false)
  const posterId = session.item?.posterId
  const wide = session.item?.kind === 'channel'
  const frame = `${wide ? 'h-14 w-20' : 'h-24 w-16'} shrink-0 overflow-hidden rounded-lg bg-surface`
  if (!posterId || failed) {
    return (
      <div aria-hidden="true" className={`${frame} flex items-center justify-center text-zinc-600`}>
        <icons.play className="size-6" />
      </div>
    )
  }
  return (
    <img
      src={`/Items/${encodeURIComponent(posterId)}/Images/Primary?maxHeight=192&quality=85`}
      alt=""
      loading="lazy"
      onError={() => setFailed(true)}
      className={`${frame} ${wide ? 'object-contain p-1.5' : 'object-cover'}`}
    />
  )
}

function Avatar({ session }: { session: LiveSession }) {
  const [failed, setFailed] = useState(false)
  const { user } = session
  if (user.imageTag === null || failed) {
    return (
      <span
        aria-hidden="true"
        className="flex size-8 shrink-0 items-center justify-center rounded-[0.6rem] bg-surface text-xs font-semibold text-fin-5"
      >
        {user.name.slice(0, 1).toUpperCase()}
      </span>
    )
  }
  return (
    <img
      src={`/UserImage?userId=${encodeURIComponent(user.id)}&tag=${encodeURIComponent(user.imageTag)}`}
      alt=""
      onError={() => setFailed(true)}
      className="size-8 shrink-0 rounded-[0.6rem] object-cover"
    />
  )
}

function DeliveryBadge({ session }: { session: LiveSession }) {
  const { t } = useI18n()
  const tone =
    session.delivery === 'conversion' ? 'warning' : session.delivery === 'directPlay' ? 'ok' : 'fin'
  return (
    <span title={t.dashboard.live.deliveryHelp[session.delivery]} className="shrink-0">
      <Badge tone={tone}>{t.dashboard.live.delivery[session.delivery]}</Badge>
    </span>
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
  const rows = [
    { label: live.source, value: streamText(session.source, language, live.unknown) },
    ...(session.delivery === 'directPlay'
      ? []
      : [{ label: live.sent, value: streamText(session.sent, language, live.unknown) }]),
    ...(encoder === null ? [] : [{ label: live.encoder, value: encoder }]),
    {
      label: live.qualityGroup,
      value: session.user.qualityGroup > 0 ? `${session.user.qualityGroup}p` : live.original,
    },
  ]

  return (
    <div className="rounded-lg border border-line bg-bg/50 px-3 py-2.5">
      <p className="sr-only">{live.howItPlays}</p>
      <dl className="grid gap-x-4 gap-y-1.5 text-xs sm:grid-cols-[auto_1fr]">
        {rows.map((row) => (
          <div key={row.label} className="contents">
            <dt className="text-muted">{row.label}</dt>
            <dd className="text-zinc-100 tabular-nums">{row.value}</dd>
          </div>
        ))}
        {session.reasons.length > 0 && (
          <div className="contents">
            <dt className="text-muted">{live.why}</dt>
            <dd>
              <ul className="flex flex-wrap gap-1">
                {session.reasons.map((reason) => (
                  <li key={reason}>
                    <Badge tone="muted">{t.dashboard.reasons[reason] ?? reason}</Badge>
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

function MessageForm({ session, onSent }: { session: LiveSession; onSent: () => void }) {
  const { t } = useI18n()
  const live = t.dashboard.live
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const id = useId()
  const send = useMutation({
    mutationFn: () =>
      messageLiveSession(session.id, { text: text.trim(), timeout: messageSeconds }),
    onSuccess: () => {
      setText('')
      setOpen(false)
      onSent()
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (text.trim() !== '') send.mutate()
  }

  if (!open) {
    return (
      <button
        type="button"
        className={buttonSecondary}
        aria-expanded={false}
        onClick={() => setOpen(true)}
      >
        <icons.message className="size-4" />
        {live.message}
      </button>
    )
  }
  return (
    <form onSubmit={submit} className="w-full space-y-2">
      <label htmlFor={id} className="block text-sm font-medium text-zinc-200">
        {live.messageLabel(session.device.name)}
      </label>
      <textarea
        id={id}
        value={text}
        autoFocus
        rows={2}
        maxLength={500}
        required
        onChange={(event) => setText(event.target.value)}
        aria-describedby={`${id}-hint`}
        className="block w-full rounded-lg border border-line bg-bg px-3 py-2 text-sm text-white placeholder:text-zinc-500"
      />
      <p id={`${id}-hint`} className="text-xs text-muted">
        {live.messageHint}
      </p>
      {send.isError && <Notice kind="error">{errorMessage(t, send.error)}</Notice>}
      <div className="flex gap-2">
        <button
          type="submit"
          className={buttonPrimary}
          disabled={send.isPending || text.trim() === ''}
        >
          {send.isPending ? live.sending : live.send}
        </button>
        <button type="button" className={buttonSecondary} onClick={() => setOpen(false)}>
          {t.common.cancel}
        </button>
      </div>
    </form>
  )
}
