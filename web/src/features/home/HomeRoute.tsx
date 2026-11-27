import { useEffect, useState, type Ref } from 'react'
import { ArrowSquareOutIcon, CaretRightIcon, CheckCircleIcon } from '@phosphor-icons/react'
import { useSearchParams } from 'react-router'
import { queryClient, type Health, type HealthProblem, type ServerActions } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { problemLink, problemText, useHealth, useHealthProblems } from '@/features/system/problems'
import { errorMessage, formatBytes, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import {
  AlertTile,
  Block,
  ButtonLink,
  EmptyState,
  ExternalButtonLink,
  InlineError,
  ProgressBar,
  Row,
  RowList,
  SkeletonRows,
  StatusPill,
  TextLink,
  useOnScreen,
} from '@/ui'
import { GetStarted } from './GetStarted'
import { MemberHome } from './MemberHome'
import { useLiveSessions } from './liveSessions'
import { Bandwidth, PlayingNow } from './NowPlaying'
import { RecentActivity } from './RecentActivity'
import { RoomLight } from './RoomLight'
import {
  ActionDialog,
  DesktopInstall,
  NoticeUpdateButton,
  ServerActionButtons,
  useServerActions,
  type ServerAction,
} from './ServerActions'

/** `/`: the administrator overview, or a member's own Home. */
export default function HomeRoute() {
  const user = useSessionUser()
  if (!user.isAdministrator) return <MemberHome name={user.name} />
  return <Overview name={user.name} />
}

/**
 * The administrator's Home: the greeting and its summary, Playing now as art tiles whose lit one
 * tints the top of the page, then two columns: To look at and Recent activity on the left;
 * Bandwidth, Get started and Server state on the right. In one column below lg, Recent activity
 * comes last. When every load fails, one page error stands in their place.
 */
function Overview({ name }: { name: string }) {
  const { t } = useI18n()
  const text = t.dashboard.overview
  const sessions = useLiveSessions()
  // The health, which Server state and the conversions under Bandwidth show, is followed while
  // either is on screen.
  const [bandwidthRef, bandwidthOnScreen] = useOnScreen<HTMLElement>()
  const [serverRef, serverOnScreen] = useOnScreen<HTMLElement>()
  const health = useHealth(bandwidthOnScreen || serverOnScreen)
  const problems = useHealthProblems()
  const transcoder = health.data?.transcoder
  const actions = useServerActions()
  const [open, setOpen] = useState<ServerAction | null>(null)
  useActionFromAddress(actions.data?.updateTool, setOpen)
  // The lit tile's poster tints the room; before any tile is lit, the first one's does.
  const [litId, setLitId] = useState<string | null>(null)
  const playing = sessions.data ?? []
  const lit = playing.find((session) => session.id === litId) ?? playing[0]
  const posters = [
    ...new Set(
      playing.flatMap((session) => (session.item?.posterId ? [session.item.posterId] : [])),
    ),
  ]
  // When Polyfin does not answer, or fails every load, Home says so once, in place of its blocks:
  // the two it always reads both failed before answering. Try again reads all it shows again.
  const failed =
    sessions.isError &&
    sessions.data === undefined &&
    problems.isError &&
    problems.data === undefined

  return (
    <>
      <RoomLight posters={posters} lit={lit?.item?.posterId || null} />
      <PageLayout
        greeting
        title={text.greeting(new Date().getHours(), name)}
        lede={
          sessions.data !== undefined && problems.data !== undefined
            ? text.summary(sessions.data.length, problems.data.length)
            : text.description
        }
      >
        {failed ? (
          <InlineError
            page
            error={sessions.error}
            onRetry={() => void queryClient.invalidateQueries()}
            retrying={sessions.isFetching || problems.isFetching}
          />
        ) : (
          <>
            <PlayingNow onLight={setLitId} />
            <div className="grid items-start gap-10 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] max-xl:gap-8 max-lg:gap-10 max-md:mt-8 max-md:gap-8">
              <div className="flex flex-col gap-10 max-lg:contents max-md:gap-8">
                <ToLookAt
                  health={health.data}
                  problems={problems.data ?? []}
                  loading={problems.isPending}
                  error={problems.error}
                  retrying={problems.isFetching}
                  onRetry={() => void problems.refetch()}
                  actions={actions.data}
                  onOpen={setOpen}
                />
                <div className="max-lg:order-last">
                  <RecentActivity />
                </div>
              </div>
              <div className="flex flex-col gap-10 max-lg:contents max-md:gap-8">
                <Bandwidth
                  ref={bandwidthRef}
                  playing={playing.length > 0}
                  conversions={
                    transcoder
                      ? { count: transcoder.conversions, limit: transcoder.limit }
                      : undefined
                  }
                />
                <GetStarted />
                <ServerState ref={serverRef} actions={actions.data} onOpen={setOpen} />
              </div>
            </div>
          </>
        )}
      </PageLayout>
      {open !== null && actions.data && (
        <ActionDialog action={open} actions={actions.data} onClose={() => setOpen(null)} />
      )}
    </>
  )
}

/**
 * `/?action=restart` and `/?action=update`, which the search opens, open that action's
 * confirmation once the server's actions are known, then leave the address, so that closing it
 * does not open it again. Update now only opens while an update tool is set.
 */
function useActionFromAddress(
  updateTool: boolean | undefined,
  open: (action: ServerAction) => void,
) {
  const [params, setParams] = useSearchParams()
  const asked = params.get('action')
  useEffect(() => {
    if (asked === null || updateTool === undefined) return
    if (asked === 'restart' || (asked === 'update' && updateTool)) open(asked)
    setParams(
      (current) => {
        current.delete('action')
        return current
      },
      { replace: true },
    )
  }, [asked, updateTool, open, setParams])
}

/**
 * To look at: a new version of Polyfin, then each problem Health finds, as alert tiles with the
 * app's own words and a way to the page that fixes it.
 */
function ToLookAt({
  health,
  problems,
  loading,
  error,
  retrying,
  onRetry,
  actions,
  onOpen,
}: {
  health: Health | undefined
  problems: HealthProblem[]
  loading: boolean
  error: Error | null
  retrying: boolean
  onRetry: () => void
  actions: ServerActions | undefined
  onOpen: (action: ServerAction) => void
}) {
  const { language, t } = useI18n()
  const text = t.dashboard.overview
  const update = health?.update
  return (
    // No count while the problems failed to load: 0 would say there is nothing to look at.
    <Block
      title={text.problems}
      count={loading || (error !== null && problems.length === 0) ? undefined : problems.length}
    >
      {loading ? (
        <div role="status" aria-label={t.common.loading}>
          <SkeletonRows rows={1} />
        </div>
      ) : error !== null && problems.length === 0 ? (
        <InlineError onRetry={onRetry} retrying={retrying}>
          {errorMessage(t, error)}
        </InlineError>
      ) : problems.length === 0 && !update ? (
        <EmptyState icon={CheckCircleIcon} title={text.problemsEmpty}>
          {text.problemsEmptyHelp}
        </EmptyState>
      ) : (
        <ul className="flex flex-col gap-2">
          {update && health && (
            <li>
              <AlertTile
                tone="info"
                kind={text.updateKind}
                title={text.update(update.version, health.process.version)}
                actions={
                  <>
                    {actions?.desktopUpdate && (
                      <DesktopInstall actions={actions} update={actions.desktopUpdate} />
                    )}
                    {actions?.updateTool && <NoticeUpdateButton onOpen={onOpen} />}
                    <ExternalButtonLink
                      href={update.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      size="sm"
                      iconEnd={ArrowSquareOutIcon}
                    >
                      {text.releaseNotes}
                    </ExternalButtonLink>
                  </>
                }
              />
            </li>
          )}
          {problems.map((problem) => {
            // Its button names where it leads, as on Health: "Open the source"…
            const opens = problem.to === '' ? null : (problemLink(t, problem.to) ?? text.openHealth)
            return (
              <li key={problem.key}>
                <AlertTile
                  tone={problem.tone === 'error' ? 'danger' : 'warn'}
                  title={problemText(t, language, problem)}
                  actions={
                    opens !== null && (
                      <ButtonLink to={problem.to} size="sm" iconEnd={CaretRightIcon}>
                        {opens}
                      </ButtonLink>
                    )
                  }
                />
              </li>
            )
          })}
        </ul>
      )}
    </Block>
  )
}

/**
 * Server state: the graphics card, the cache and the database as rows that open Health, then how
 * long Polyfin has run and its version, with Restart (and Update now) at its foot. The details
 * (encoders, the server's ID) are Health's.
 */
function ServerState({
  actions,
  onOpen,
  ref,
}: {
  actions: ServerActions | undefined
  onOpen: (action: ServerAction) => void
  /** The block, which Home watches to follow the health while it is on screen. */
  ref: Ref<HTMLElement>
}) {
  const { language, t } = useI18n()
  const text = t.dashboard.overview
  const health = useHealth()
  const data = health.data

  return (
    <Block
      ref={ref}
      title={text.serverTitle}
      aside={<TextLink to="/system/health">{text.openHealth}</TextLink>}
    >
      {health.isError && data === undefined ? (
        <InlineError onRetry={() => void health.refetch()} retrying={health.isFetching}>
          {errorMessage(t, health.error)}
        </InlineError>
      ) : data === undefined ? (
        <div role="status" aria-label={t.common.loading}>
          <SkeletonRows rows={3} />
        </div>
      ) : (
        <RowList aria-label={text.serverTitle}>
          <Row
            to="/system/health#transcoder"
            title={text.graphics}
            trailing={
              data.transcoder?.hardware ? (
                <>
                  <span className="text-ink">
                    {t.dashboard.live.gpuMethod(data.transcoder.hardware.method)}
                  </span>
                  {data.transcoder.hardware.toneMapping && (
                    <span className="text-micro text-ink-3">{text.hdrSupported}</span>
                  )}
                </>
              ) : (
                <>
                  <span className="text-ink">{text.graphicsNone}</span>
                  <span className="max-w-[15rem] text-micro text-ink-3 max-md:max-w-full">
                    {text.graphicsCpu}
                  </span>
                </>
              )
            }
          />
          <Row
            to="/system/health#cache"
            title={text.cache}
            trailing={
              data.cache === null ? (
                <span>{text.cacheOff}</span>
              ) : (
                <>
                  <span className="figures">
                    {formatBytes(data.cache.used, language)}{' '}
                    {data.cache.limit > 0
                      ? text.cacheOf(formatBytes(data.cache.limit, language))
                      : `· ${text.cacheNoLimit}`}
                  </span>
                  {data.cache.limit > 0 && (
                    <ProgressBar
                      size="sm"
                      variant={data.cache.used / data.cache.limit > 0.9 ? 'warn' : 'neutral'}
                      value={data.cache.used / data.cache.limit}
                      label={text.cacheUnit}
                      className="w-[6rem] max-md:w-[5rem]"
                    />
                  )}
                </>
              )
            }
          />
          <Row
            to="/system/health#database"
            title={text.database}
            trailing={
              <>
                <StatusPill tone={data.database.reachable ? 'ok' : 'danger'}>
                  {data.database.reachable ? t.status.databaseReady : t.status.databaseUnavailable}
                </StatusPill>
                {data.database.size !== null && (
                  <span className="figures text-micro text-ink-3">
                    {text.databaseSize(formatBytes(data.database.size, language))}
                  </span>
                )}
              </>
            }
          />
        </RowList>
      )}
      <div className="mt-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-2.5 pl-4 max-md:flex-col max-md:items-stretch max-md:pl-0">
        <div className="min-w-0 flex-1 basis-[14rem] text-small text-ink-3 max-md:basis-auto">
          {data !== undefined && (
            <p className="figures text-lead">
              {text.uptime}{' '}
              <strong className="font-semibold text-ink">
                {formatSpan(
                  (Date.parse(data.checkedAt) - Date.parse(data.process.startedAt)) / 1000,
                  language,
                )}
              </strong>
              {' · '}
              {text.version}{' '}
              <strong className="font-semibold text-ink">{data.process.version}</strong>
            </p>
          )}
        </div>
        {actions && (
          <div className="flex flex-wrap items-center gap-2">
            <ServerActionButtons actions={actions} onOpen={onOpen} />
          </div>
        )}
      </div>
    </Block>
  )
}
