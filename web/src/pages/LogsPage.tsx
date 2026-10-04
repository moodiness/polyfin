import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { ApiError, fetchLogLines, logDownloadUrl, queryClient, queryKeys } from '@/api'
import { icons } from '@/components/icons'
import { buttonSecondary, Notice, PageHeader } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

/** Lines kept in the page, and drawn at most: the newest of those that match. */
const keptLines = 5000
const drawnLines = 2000
const pollInterval = 2_000

const levelRank = { DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3 } as const
type Level = keyof typeof levelRank

/** The minimum level each filter shows. */
const filters = { all: 'DEBUG', info: 'INFO', warn: 'WARN', error: 'ERROR' } as const
type Filter = keyof typeof filters

type Line = { seq: number; time: string; level: Level; text: string }

const levelStyles: Record<Level, string> = {
  DEBUG: 'text-zinc-500',
  INFO: 'text-fin-5',
  WARN: 'text-amber-300',
  ERROR: 'text-rose-300',
}

/** Splits a line of Polyfin's text log into its time, its level and the rest. */
function parse(raw: string, seq: number): Line {
  let rest = raw
  let time = ''
  const timeMatch = /^time=(\S+)\s*/.exec(rest)
  if (timeMatch) {
    const date = new Date(timeMatch[1])
    time = Number.isNaN(date.getTime()) ? '' : date.toLocaleTimeString([], { hour12: false })
    rest = rest.slice(timeMatch[0].length)
  }
  let level: Level = 'INFO'
  const levelMatch = /^level=(DEBUG|INFO|WARN|ERROR)\S*\s*/.exec(rest)
  if (levelMatch) {
    level = levelMatch[1] as Level
    rest = rest.slice(levelMatch[0].length)
  }
  return { seq, time, level, text: rest }
}

export default function LogsPage() {
  const { language, t } = useI18n()
  const text = t.dashboard.logs
  const [lines, setLines] = useState<Line[]>([])
  const [following, setFollowing] = useState(true)
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const [error, setError] = useState<unknown>(null)
  const next = useRef(0)
  const view = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)
  const searchId = useId()
  const levelId = useId()

  // Reads the lines written since the last read, now and every 2 seconds while following.
  useEffect(() => {
    if (!following) return
    const controller = new AbortController()
    let timer = 0
    async function read() {
      try {
        const page = await fetchLogLines(next.current, keptLines, controller.signal)
        const first = page.next - page.lines.length
        // Fewer lines than read so far: the server started again, with a new log.
        const restarted = page.next < next.current
        next.current = page.next
        setError(null)
        if (page.lines.length > 0 || restarted) {
          const added = page.lines.map((line, i) => parse(line, first + i))
          setLines((current) => (restarted ? added : [...current, ...added].slice(-keptLines)))
        }
      } catch (caught) {
        if (controller.signal.aborted) return
        if (caught instanceof ApiError && caught.status === 401) {
          queryClient.setQueryData(queryKeys.session, null)
          return
        }
        setError(caught)
      }
      timer = window.setTimeout(read, pollInterval)
    }
    void read()
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [following])

  const minimum = levelRank[filters[filter]]
  const needle = search.trim().toLocaleLowerCase(language)
  const matching = lines.filter(
    (line) =>
      levelRank[line.level] >= minimum &&
      (needle === '' || line.text.toLocaleLowerCase(language).includes(needle)),
  )
  const drawn = matching.slice(-drawnLines)

  // New lines keep the view at its end, unless the reader scrolled up.
  useLayoutEffect(() => {
    const element = view.current
    if (element && following && atBottom.current) element.scrollTop = element.scrollHeight
  }, [drawn.length, following, lines])

  return (
    <>
      <PageHeader
        title={text.title}
        description={text.description}
        actions={
          <>
            <button
              type="button"
              className={buttonSecondary}
              aria-pressed={following}
              onClick={() => setFollowing((value) => !value)}
            >
              {following ? <icons.pause className="size-4" /> : <icons.play className="size-4" />}
              {following ? text.pause : text.follow}
            </button>
            <a href={logDownloadUrl} download="polyfin.log" className={buttonSecondary}>
              <icons.download className="size-4" />
              {text.download}
            </a>
          </>
        }
      />
      <div className="mb-3 flex flex-col gap-3 md:flex-row md:items-end">
        <div>
          <label htmlFor={levelId} className="block text-xs font-medium text-muted">
            {text.level}
          </label>
          <select
            id={levelId}
            value={filter}
            onChange={(event) => setFilter(event.target.value as Filter)}
            className="mt-1 block min-h-9 rounded-lg border border-line bg-ink px-3 py-1.5 text-sm text-white"
          >
            {(Object.keys(filters) as Filter[]).map((key) => (
              <option key={key} value={key}>
                {text.levels[key]}
              </option>
            ))}
          </select>
        </div>
        <div className="relative flex-1 md:max-w-sm">
          <label htmlFor={searchId} className="block text-xs font-medium text-muted">
            {text.search}
          </label>
          <icons.search className="pointer-events-none absolute bottom-2.5 left-3 size-4 text-muted" />
          <input
            id={searchId}
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="mt-1 block min-h-9 w-full rounded-lg border border-line bg-ink py-1.5 pr-3 pl-9 text-sm text-white"
          />
        </div>
        <button
          type="button"
          className={`${buttonSecondary} md:ml-auto`}
          onClick={() => setLines([])}
          disabled={lines.length === 0}
        >
          {text.clear}
        </button>
      </div>
      {error !== null && (
        <div className="mb-3">
          <Notice kind="error">{errorMessage(t, error)}</Notice>
        </div>
      )}
      <div
        ref={view}
        role="log"
        aria-label={text.region}
        aria-live="off"
        tabIndex={0}
        onScroll={(event) => {
          const element = event.currentTarget
          atBottom.current = element.scrollHeight - element.scrollTop - element.clientHeight < 24
        }}
        className="h-[65dvh] min-h-80 overflow-auto rounded-2xl border border-line bg-[#0b0b13] py-2 font-mono text-[0.78rem] leading-relaxed"
      >
        {drawn.length === 0 ? (
          <p className="px-4 py-6 text-center font-sans text-sm text-muted">
            {lines.length === 0 ? text.empty : text.noMatch}
          </p>
        ) : (
          <ol>
            {drawn.map((line) => (
              <li
                key={line.seq}
                className="grid grid-cols-[4.5rem_3.25rem_1fr] gap-x-3 px-4 py-px hover:bg-surface-2"
              >
                <span className="text-zinc-500 tabular-nums">{line.time}</span>
                <span className={`font-semibold ${levelStyles[line.level]}`}>{line.level}</span>
                <span className="break-words whitespace-pre-wrap text-zinc-200">{line.text}</span>
              </li>
            ))}
          </ol>
        )}
      </div>
      <div className="mt-2 flex flex-wrap justify-between gap-2 text-xs text-muted">
        <p aria-live="polite">{following ? text.following : text.paused}</p>
        <p>
          {text.shown(drawn.length, lines.length)} {filter === 'all' ? text.detailedHint : ''}
        </p>
      </div>
    </>
  )
}
