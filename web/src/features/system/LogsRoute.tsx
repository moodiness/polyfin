import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import {
  BugIcon,
  DownloadSimpleIcon,
  EraserIcon,
  InfoIcon,
  MagnifyingGlassIcon,
  PauseIcon,
  PlayIcon,
  ScrollIcon,
  WarningCircleIcon,
  WarningIcon,
  type Icon,
} from '@phosphor-icons/react'
import { ApiError, fetchLogLines, logDownloadUrl, sessionEnded } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import {
  Button,
  ExternalButtonLink,
  cx,
  EmptyState,
  Field,
  InlineError,
  Select,
  StatusPill,
  TextInput,
  type StatusTone,
} from '@/ui'
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

/** Each level as a state: an icon and the word, in the color of a warning or an error. */
const levelLooks: Record<Level, { tone: StatusTone; icon: Icon }> = {
  DEBUG: { tone: 'muted', icon: BugIcon },
  INFO: { tone: 'muted', icon: InfoIcon },
  WARN: { tone: 'warn', icon: WarningIcon },
  ERROR: { tone: 'danger', icon: WarningCircleIcon },
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

/** `/system/logs`: the server's log, followed live, filtered by level and words. */
export default function LogsRoute() {
  const { language, t } = useI18n()
  const text = t.system.logs
  const [lines, setLines] = useState<Line[]>([])
  const [following, setFollowing] = useState(true)
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [loaded, setLoaded] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const next = useRef(0)
  const view = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)

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
        setLoaded(true)
        if (page.lines.length > 0 || restarted) {
          const added = page.lines.map((line, i) => parse(line, first + i))
          setLines((current) => (restarted ? added : [...current, ...added].slice(-keptLines)))
        }
      } catch (caught) {
        if (controller.signal.aborted) return
        if (caught instanceof ApiError && caught.status === 401) {
          sessionEnded()
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
  }, [following, attempt])

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
    <PageLayout
      title={text.title}
      lede={text.description}
      actions={
        <>
          <Button
            icon={following ? PauseIcon : PlayIcon}
            aria-pressed={following}
            onClick={() => setFollowing((value) => !value)}
          >
            {following ? text.pause : text.follow}
          </Button>
          <ExternalButtonLink
            href={logDownloadUrl}
            download="polyfin.log"
            icon={DownloadSimpleIcon}
          >
            {text.download}
          </ExternalButtonLink>
        </>
      }
    >
      <section aria-label={text.region}>
        <div
          role="search"
          aria-label={text.filters}
          className="mb-4 flex flex-col gap-3 md:flex-row md:items-end"
        >
          <Field label={text.level} className="md:w-60">
            <Select
              value={filter}
              onValue={setFilter}
              options={(Object.keys(filters) as Filter[]).map((key) => ({
                value: key,
                label: text.levels[key],
              }))}
            />
          </Field>
          <Field label={text.search} className="md:max-w-sm md:flex-1">
            <TextInput
              type="search"
              icon={MagnifyingGlassIcon}
              value={search}
              onValue={setSearch}
            />
          </Field>
          <Button
            variant="ghost"
            icon={EraserIcon}
            className="md:ml-auto"
            onClick={() => setLines([])}
            disabled={lines.length === 0}
          >
            {text.clear}
          </Button>
        </div>
        {error !== null && (
          <InlineError className="mb-3" onRetry={() => setAttempt((value) => value + 1)}>
            {errorMessage(t, error)}
          </InlineError>
        )}
        {/* The tile, and inside it the lines scrolling, faded where they pass its edges. */}
        <div className="rounded-panel bg-s1 p-2 max-md:rounded-[24px]">
          <div
            ref={view}
            role="log"
            aria-label={text.region}
            aria-live="off"
            aria-busy={!loaded}
            tabIndex={0}
            onScroll={(event) => {
              const element = event.currentTarget
              atBottom.current =
                element.scrollHeight - element.scrollTop - element.clientHeight < 24
            }}
            className="focus-lit h-[65dvh] min-h-80 overflow-auto rounded-[20px] [mask-image:linear-gradient(to_bottom,transparent,#000_14px,#000_calc(100%-14px),transparent)] [--lift:1] max-md:rounded-[16px]"
          >
            {!loaded && error === null ? (
              <div aria-hidden="true" className="space-y-5 px-4 py-4">
                {[70, 55, 82, 64, 48, 76, 60].map((width, index) => (
                  <span
                    key={index}
                    className="block h-4 animate-pulse rounded-md bg-s3 motion-reduce:animate-none"
                    style={{ width: `${width}%` }}
                  />
                ))}
              </div>
            ) : drawn.length === 0 ? (
              <EmptyState
                icon={ScrollIcon}
                title={lines.length === 0 ? text.empty : text.noMatch}
                className="bg-transparent"
              >
                {lines.length === 0 ? text.emptyHint : text.noMatchHint}
              </EmptyState>
            ) : (
              <ol className="grid grid-cols-[minmax(0,1fr)] gap-0.5 py-3">
                {drawn.map((line) => (
                  <li
                    key={line.seq}
                    className="grid min-h-14 grid-cols-[5.5rem_7rem_minmax(0,1fr)] items-center gap-x-4 rounded-[18px] px-4 py-2 hover:bg-s2 max-md:min-h-0 max-md:grid-cols-[auto_minmax(0,1fr)] max-md:gap-y-1.5 max-md:px-3 max-md:py-3"
                  >
                    <span className="text-small text-ink-2 figures">{line.time}</span>
                    <StatusPill
                      tone={levelLooks[line.level].tone}
                      icon={levelLooks[line.level].icon}
                    >
                      {line.level}
                    </StatusPill>
                    <span className="min-w-0 font-mono text-[15px] leading-6 whitespace-pre-wrap text-ink [overflow-wrap:anywhere] max-md:col-span-2 max-md:text-[14px]">
                      {line.text}
                    </span>
                  </li>
                ))}
              </ol>
            )}
          </div>
        </div>
        <div className="mt-3 flex flex-wrap justify-between gap-x-6 gap-y-2 text-small text-ink-2">
          <p aria-live="polite" className="inline-flex items-center gap-2">
            <span
              aria-hidden="true"
              className={cx(
                'size-2 shrink-0 rounded-full',
                following ? 'bg-ok ring-4 ring-ok/14' : 'bg-ink-3',
              )}
            />
            {following ? text.following : text.paused}
          </p>
          <p className="figures">
            {text.shown(drawn.length, lines.length)} {filter === 'all' ? text.detailedHint : ''}
          </p>
        </div>
      </section>
    </PageLayout>
  )
}
