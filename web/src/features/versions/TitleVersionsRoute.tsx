import {
  ArrowsClockwiseIcon,
  ImagesSquareIcon,
  ProhibitIcon,
  PushPinIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { useParams } from 'react-router'
import {
  analyzeVersion,
  ApiError,
  askVersionsAgain,
  blockVersion,
  fetchTitleVersions,
  fetchVersionTitle,
  maxBlockReason,
  makeThumbnailsAgain,
  pinVersion,
  queryClient,
  queryKeys,
  unblockVersion,
  unpinVersion,
  type PlayDecision,
  type TitleVersions,
  type VersionBlock,
  type TitleVersionSource,
  type VersionStream,
  type VersionThumbnails,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage, formatBytes, formatEpisode } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  EmptyState,
  Field,
  InlineError,
  Modal,
  Notice,
  RelativeTime,
  Row,
  RowList,
  Segmented,
  SkeletonRows,
  StatusPill,
  TextInput,
  useToast,
  type StatusTone,
} from '@/ui'
import { HostErrorFile } from './HostErrorFile'
import { userQuery, UserPicker, useVersionsUser } from './parts'

/**
 * The waits before reading the versions again while addons are still asked, in milliseconds: each
 * answer the same as the one before waits the next one, and after the last the page stops reading
 * and says so. A new answer, the tab or the window shown again, or Check again start over.
 */
const pendingPolls = [2_000, 4_000, 8_000, 16_000, 30_000]

/** The wait before reading the versions again while a version's thumbnails wait or are made. */
const thumbnailsPoll = 5_000

/** Whether a version of the title waits for its thumbnails or has them made now. */
const makingThumbnails = (explained: TitleVersions | undefined) =>
  explained?.sources.some((source) =>
    source.streams.some(
      (stream) => stream.thumbnails?.state === 'waiting' || stream.thumbnails?.state === 'working',
    ),
  ) ?? false

/** The answers read one after the other the same as the last, counted once per read. */
type SameAnswers = { data: unknown; at: number; count: number }

/** Counts the answer read at `at` in `seen`, and returns how many in a row were the same. */
function sameAnswers(seen: SameAnswers, data: unknown, at: number): number {
  if (at !== seen.at) {
    seen.count = data === seen.data ? seen.count + 1 : 0
    seen.data = data
    seen.at = at
  }
  return seen.count
}

/**
 * `/versions/:id`: every stream each addon listed for a movie or an episode, as a user is offered
 * them, with the step that left it out or its place; the pinned version, the blocks and the last
 * plays.
 */
export default function TitleVersionsRoute() {
  const { t } = useI18n()
  const text = t.versions
  const { id = '' } = useParams()
  const [user, setUser] = useVersionsUser()
  const toast = useToast()
  const title = useQuery({
    queryKey: queryKeys.versionTitle(user, id),
    queryFn: ({ signal }) => fetchVersionTitle(user, id, signal),
  })
  // The same answer keeps the same data, React Query sharing what did not change.
  const seen = useRef<SameAnswers>({ data: undefined, at: 0, count: 0 })
  const versions = useQuery({
    queryKey: queryKeys.titleVersions(id, user),
    queryFn: ({ signal }) => fetchTitleVersions(id, user, signal),
    refetchInterval: (query) =>
      (query.state.data?.pending ?? 0) > 0
        ? (pendingPolls[sameAnswers(seen.current, query.state.data, query.state.dataUpdatedAt)] ??
          false)
        : makingThumbnails(query.state.data)
          ? thumbnailsPoll
          : false,
  })
  const stopped =
    (versions.data?.pending ?? 0) > 0 &&
    sameAnswers(seen.current, versions.data, versions.dataUpdatedAt) >= pendingPolls.length
  const checkAgain = () => {
    seen.current.data = undefined
    void versions.refetch({ cancelRefetch: false })
  }
  // Shown again, the tab or the window starts over, reading at once while addons are asked.
  useEffect(() => {
    const key = queryKeys.titleVersions(id, user)
    const shown = () => {
      if (document.hidden) return
      seen.current.data = undefined
      if ((queryClient.getQueryData<TitleVersions>(key)?.pending ?? 0) === 0) return
      void queryClient.refetchQueries({ queryKey: key, exact: true }, { cancelRefetch: false })
    }
    window.addEventListener('focus', shown)
    document.addEventListener('visibilitychange', shown)
    return () => {
      window.removeEventListener('focus', shown)
      document.removeEventListener('visibilitychange', shown)
    }
  }, [id, user])
  const ask = useMutation({
    mutationFn: () => askVersionsAgain(id, user),
    onSuccess: () => {
      toast(text.askedAgain, { tone: 'ok' })
      void queryClient.invalidateQueries({ queryKey: queryKeys.titleVersions(id, user) })
    },
    onError: (error) =>
      toast(
        error instanceof ApiError && error.status === 429
          ? text.askedLately
          : errorMessage(t, error),
        { tone: 'danger' },
      ),
  })

  const name =
    title.data === undefined
      ? text.title
      : title.data.kind === 'episode'
        ? `${title.data.seriesName} · ${formatEpisode(title.data.season, title.data.episode)} · ${title.data.name}`
        : title.data.name
  const notFound =
    (title.error instanceof ApiError && title.error.status === 404) ||
    (versions.error instanceof ApiError && versions.error.status === 404)

  return (
    <PageLayout
      title={name}
      back={{ to: `/versions${userQuery(user)}`, label: text.backToSearch }}
      actions={
        <Button icon={ArrowsClockwiseIcon} loading={ask.isPending} onClick={() => ask.mutate()}>
          {text.askAgain}
        </Button>
      }
    >
      <UserPicker user={user} onUser={setUser} />
      {notFound ? (
        <Notice tone="warn">{text.notFound}</Notice>
      ) : versions.isPending ? (
        <SkeletonRows rows={4} />
      ) : versions.isError ? (
        <InlineError onRetry={() => void versions.refetch()} retrying={versions.isFetching}>
          {errorMessage(t, versions.error)}
        </InlineError>
      ) : (
        <Explained
          id={id}
          user={user}
          explained={versions.data}
          stopped={stopped}
          checking={versions.isFetching}
          onCheckAgain={checkAgain}
        />
      )}
    </PageLayout>
  )
}

/** The blocks of a title's versions explained. */
function Explained({
  id,
  user,
  explained,
  stopped,
  checking,
  onCheckAgain,
}: {
  id: string
  user: string
  explained: TitleVersions
  /** The page stopped reading the versions again while addons are still asked. */
  stopped: boolean
  checking: boolean
  onCheckAgain: () => void
}) {
  const { t } = useI18n()
  const text = t.versions
  const criteria = explained.criteria
    .map(
      (criterion) =>
        t.settings.versionOrder.criteria[
          criterion as keyof Messages['settings']['versionOrder']['criteria']
        ] ?? criterion,
    )
    .join(', ')
  return (
    <div className="flex flex-col gap-block">
      <Block title={text.order}>
        <div className="flex max-w-[72ch] flex-col gap-1.5 text-body text-ink-2">
          <p>{criteria === '' ? text.orderNone : text.orderBy(criteria)}</p>
          {explained.qualityGroup > 0 && <p>{text.group(explained.qualityGroup)}</p>}
          {explained.keptAll && <Notice tone="info">{text.keptAll}</Notice>}
          {explained.closest && <Notice tone="info">{text.closest}</Notice>}
          {explained.pending > 0 &&
            (stopped ? (
              <Notice
                tone="info"
                live
                action={
                  <Button
                    size="sm"
                    icon={ArrowsClockwiseIcon}
                    loading={checking}
                    onClick={onCheckAgain}
                  >
                    {text.checkAgain}
                  </Button>
                }
              >
                {text.pendingStopped(explained.pending)}
              </Notice>
            ) : (
              <Notice tone="info" live>
                {text.pending(explained.pending)}
              </Notice>
            ))}
        </div>
      </Block>
      <PinBlock id={id} explained={explained} />
      <Block title={text.sources}>
        {explained.sources.length === 0 ? (
          <EmptyState title={text.noSources} />
        ) : (
          <div className="flex flex-col gap-5">
            {sectionNames(explained.sources, text).map(([source, name]) => (
              <SourceStreams
                key={`${source.id}-${source.embedded}`}
                id={id}
                user={user}
                source={source}
                name={name}
              />
            ))}
          </div>
        )}
      </Block>
      <TitleBlocks id={id} user={user} blocks={explained.blocks} />
      <Decisions explained={explained} />
    </div>
  )
}

/** The version pinned for the title, with Unpin; missing when no addon lists its file now. */
function PinBlock({ id, explained }: { id: string; explained: TitleVersions }) {
  const { t, language } = useI18n()
  const text = t.versions
  const toast = useToast()
  const unpin = useMutation({
    mutationFn: () => unpinVersion(id),
    onSuccess: () => {
      toast(text.unpinDone, { tone: 'ok' })
      void queryClient.invalidateQueries({ queryKey: ['title-versions', id] })
    },
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
  })
  const pin = explained.pin
  return (
    <Block title={text.pin}>
      {pin === null ? (
        <p className="max-w-[72ch] text-small text-ink-2">{text.noPin}</p>
      ) : (
        <RowList compact aria-label={text.pin}>
          <Row
            leading={PushPinIcon}
            title={<span className="break-all">{pin.name}</span>}
            titleAside={pin.missing ? <Badge tone="warn">{text.pinMissing}</Badge> : undefined}
            meta={
              <span className="flex min-w-0 flex-col break-words">
                <span>
                  {[pin.filename, pin.size > 0 ? formatBytes(pin.size, language) : '']
                    .filter(Boolean)
                    .join(' · ')}
                </span>
                <span>
                  {text.pinnedBy(pin.by)} · <RelativeTime iso={pin.at} />
                </span>
              </span>
            }
            trailing={
              <Button size="sm" loading={unpin.isPending} onClick={() => unpin.mutate()}>
                {text.unpin}
              </Button>
            }
          />
        </RowList>
      )}
    </Block>
  )
}

const stepTones: Record<string, StatusTone> = {
  nothing: 'muted',
  torrents: 'muted',
  duplicate: 'muted',
  mirror: 'muted',
  blocked: 'danger',
  failed: 'danger',
  hidden: 'warn',
  group: 'warn',
}

/** What left a stream out, in words: the rule, the failure, or what the step means. */
function why(t: Messages, stream: VersionStream): string {
  const text = t.versions
  switch (stream.step) {
    case 'hidden': {
      const rules = text.rules
      switch (stream.rule) {
        case '':
          return rules.none
        case 'elsewhere':
          return rules.elsewhere(stream.detail)
        case 'larger':
          return rules.larger(stream.detail)
        case 'codec':
          return rules.codec(
            t.settings.versionCodecs[
              stream.detail as keyof Messages['settings']['versionCodecs']
            ] ?? stream.detail,
          )
        case 'release':
          return rules.release(
            t.settings.releaseKinds[stream.detail as keyof Messages['settings']['releaseKinds']] ??
              stream.detail,
          )
        case 'words':
          return rules.words(stream.detail)
        case 'label':
          return rules.label(stream.detail)
      }
      return rules.other(stream.rule, stream.detail)
    }
    case 'failed':
      return stream.detail
    case 'blocked':
      return stream.block === null ? '' : text.blockedBy(stream.block.by, stream.block.reason)
    case 'nothing':
    case 'torrents':
    case 'duplicate':
    case 'mirror':
    case 'group':
      return text.stepHelp[stream.step]
  }
  return ''
}

/** What a stream's labels, or what the rules know, tell of it, as short tags. */
function tags(facts: VersionStream['facts'] | VersionStream['labels'], t: Messages): string[] {
  return [
    facts.height > 0 ? `${facts.height}p` : '',
    facts.codec === ''
      ? ''
      : (t.settings.versionCodecs[facts.codec as keyof Messages['settings']['versionCodecs']] ??
        facts.codec),
    facts.range === 'hdr' ? 'HDR' : facts.range === 'sdr' ? 'SDR' : '',
    ...facts.languages.map((language) => language.toUpperCase()),
    facts.multi ? t.versions.multi : '',
    facts.tier === ''
      ? ''
      : (t.versions.tiers[facts.tier as keyof Messages['versions']['tiers']] ?? facts.tier),
    facts.audio.lossless ? t.versions.lossless : '',
    facts.audio.object ? t.versions.objectAudio : '',
    channelLayout(facts.audio.channels),
    facts.edition === ''
      ? ''
      : (t.settings.versionEditions[
          facts.edition as keyof Messages['settings']['versionEditions']
        ] ?? facts.edition),
    'perMinute' in facts && facts.perMinute > 0
      ? t.versions.perMinute(Math.round(facts.perMinute / 1e6))
      : '',
  ].filter(Boolean)
}

/** Audio channels as a layout: 6 is 5.1, 8 is 7.1; empty when unknown. */
function channelLayout(channels: number): string {
  if (channels <= 0) return ''
  if (channels >= 6) return `${channels - 1}.1`
  return `${channels}.0`
}

/**
 * The name of each source's section, unique on the page: a source also listed in the title's
 * description says so, and a name met again (a member's own source named as a server's) takes
 * its place among those of that name.
 */
function sectionNames(
  sources: TitleVersionSource[],
  text: Messages['versions'],
): [TitleVersionSource, string][] {
  const seen = new Map<string, number>()
  return sources.map((source) => {
    const name = source.embedded ? text.embeddedSource(source.name) : source.name
    const place = (seen.get(name) ?? 0) + 1
    seen.set(name, place)
    return [source, place === 1 ? name : text.repeatedSource(name, place)]
  })
}

/** What became of a stream: its place among the versions offered, or the step that left it out. */
function StreamStep({ stream }: { stream: VersionStream }) {
  const { t } = useI18n()
  const text = t.versions
  return stream.step === 'offered' ? (
    <Badge tone="accent">{text.steps.offered(stream.place)}</Badge>
  ) : (
    <StatusPill tone={stepTones[stream.step] ?? 'muted'}>{text.steps[stream.step]}</StatusPill>
  )
}

/** A source's streams, each with what became of it and the actions on its version. */
function SourceStreams({
  id,
  user,
  source,
  name,
}: {
  id: string
  user: string
  source: TitleVersionSource
  /** The section's name, unique on the page (sectionNames). */
  name: string
}) {
  const { t, language } = useI18n()
  const text = t.versions
  const toast = useToast()
  const [blocking, setBlocking] = useState<VersionStream | null>(null)
  // The Block button that opened the dialog, which gets the focus back when it closes.
  const opener = useRef<HTMLElement | null>(null)
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['title-versions', id] })
    void queryClient.invalidateQueries({ queryKey: queryKeys.versionBlocks })
  }
  const failed = (error: unknown) => toast(errorMessage(t, error), { tone: 'danger' })
  const pin = useMutation({
    mutationFn: (version: string) => pinVersion(id, version, user),
    onSuccess: () => {
      toast(text.pinDone, { tone: 'ok' })
      refresh()
    },
    onError: failed,
  })
  const analyze = useMutation({
    mutationFn: (version: string) => analyzeVersion(id, version, user),
    onSuccess: ({ failure }) => {
      toast(failure === '' ? text.analyzeDone : text.analysisFailed(failure), {
        tone: failure === '' ? 'ok' : 'danger',
      })
      refresh()
    },
    onError: failed,
  })
  const unblock = useMutation({
    mutationFn: unblockVersion,
    onSuccess: () => {
      toast(text.unblockDone, { tone: 'ok' })
      refresh()
    },
    onError: failed,
  })
  const makeAgain = useMutation({
    mutationFn: (version: string) => makeThumbnailsAgain(id, version),
    onSuccess: (thumbnails, version) => {
      // Waiting at once, as the server answers, before the versions are read again.
      queryClient.setQueryData<TitleVersions>(queryKeys.titleVersions(id, user), (explained) =>
        explained === undefined
          ? explained
          : {
              ...explained,
              sources: explained.sources.map((each) => ({
                ...each,
                streams: each.streams.map((stream) =>
                  stream.version === version && stream.thumbnails !== null
                    ? { ...stream, thumbnails }
                    : stream,
                ),
              })),
            },
      )
      toast(text.makeAgainDone, { tone: 'ok' })
      void queryClient.invalidateQueries({ queryKey: queryKeys.titleVersions(id, user) })
    },
    onError: failed,
  })
  const state: Record<TitleVersionSource['state'], StatusTone> = {
    known: 'ok',
    stale: 'warn',
    failed: 'warn',
    asked: 'muted',
  }
  return (
    <section aria-label={name} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h3 className="text-group text-ink">{source.name}</h3>
        {source.embedded && <Badge>{text.embedded}</Badge>}
        <StatusPill tone={state[source.state]}>{text.sourceStates[source.state]}</StatusPill>
      </div>
      {source.streams.length === 0 ? (
        <p className="text-small text-ink-3">{text.noStreams}</p>
      ) : (
        <RowList compact aria-label={name}>
          {source.streams.map((stream, index) => {
            const explanation = why(t, stream)
            const labels = tags(stream.labels, t)
            const known = tags(stream.facts, t)
            const isVersion =
              stream.version !== '' && !['duplicate', 'mirror', 'blocked'].includes(stream.step)
            const blocked = stream.step === 'blocked' ? stream.block : null
            return (
              <Row
                key={`${index}-${stream.version}`}
                title={<span className="break-words">{stream.name || stream.filename}</span>}
                titleAside={
                  <span className="flex flex-wrap gap-1.5">
                    {stream.pinned && (
                      <Badge tone="ok" icon={PushPinIcon}>
                        {text.pinned}
                      </Badge>
                    )}
                    {stream.analyzed && <Badge>{text.analyzed}</Badge>}
                    {/* On a phone the step moves here, as the right column hides. */}
                    <span className="sm:hidden">
                      <StreamStep stream={stream} />
                    </span>
                  </span>
                }
                meta={
                  <span className="flex min-w-0 flex-col break-words">
                    <span>
                      {[
                        stream.filename,
                        stream.size > 0 ? formatBytes(stream.size, language) : '',
                        stream.torrent ? 'torrent' : '',
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    </span>
                    {labels.length > 0 && (
                      <span>
                        {text.labelsAsRead} {labels.join(' · ')}
                      </span>
                    )}
                    {stream.analyzed && known.length > 0 && known.join() !== labels.join() && (
                      <span>
                        {text.facts} {known.join(' · ')}
                      </span>
                    )}
                    {stream.facts.labelRules.length > 0 && (
                      <span>{text.labelRules(stream.facts.labelRules, stream.facts.score)}</span>
                    )}
                    {explanation !== '' && <span className="text-ink-2">{explanation}</span>}
                    {stream.unstarted && <span className="text-ink-2">{text.unstarted}</span>}
                    {isVersion && stream.thumbnails !== null && (
                      <ThumbnailsState thumbnails={stream.thumbnails} />
                    )}
                    <HostErrorFile stream={stream} />
                  </span>
                }
                trailing={
                  <span className="max-sm:hidden">
                    <StreamStep stream={stream} />
                  </span>
                }
              >
                {blocked !== null ? (
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      loading={unblock.isPending && unblock.variables === blocked.id}
                      onClick={() => unblock.mutate(blocked.id)}
                    >
                      {text.unblock}
                    </Button>
                  </div>
                ) : isVersion ? (
                  <div className="flex flex-wrap gap-2">
                    {stream.step === 'offered' && !stream.pinned && (
                      <Button
                        size="sm"
                        icon={PushPinIcon}
                        loading={pin.isPending && pin.variables === stream.version}
                        onClick={() => pin.mutate(stream.version)}
                      >
                        {text.pinAction}
                      </Button>
                    )}
                    {!stream.torrent && (
                      <Button
                        size="sm"
                        loading={analyze.isPending && analyze.variables === stream.version}
                        onClick={() => analyze.mutate(stream.version)}
                      >
                        {text.analyze}
                      </Button>
                    )}
                    {stream.thumbnails !== null && (
                      <Button
                        size="sm"
                        icon={ImagesSquareIcon}
                        loading={makeAgain.isPending && makeAgain.variables === stream.version}
                        onClick={() => makeAgain.mutate(stream.version)}
                      >
                        {text.makeAgain}
                      </Button>
                    )}
                    <Button
                      size="sm"
                      variant="ghost"
                      icon={ProhibitIcon}
                      onClick={(event) => {
                        opener.current = event.currentTarget
                        setBlocking(stream)
                      }}
                    >
                      {text.block}
                    </Button>
                  </div>
                ) : undefined}
              </Row>
            )
          })}
        </RowList>
      )}
      {blocking !== null && (
        <BlockModal
          id={id}
          user={user}
          stream={blocking}
          onClose={() => {
            setBlocking(null)
            // Once the dialog is gone: the page under it takes no focus while it is open.
            requestAnimationFrame(() => opener.current?.focus())
          }}
          onBlocked={refresh}
        />
      )}
    </section>
  )
}

const thumbnailTones: Record<VersionThumbnails['state'], StatusTone> = {
  none: 'muted',
  waiting: 'muted',
  working: 'live',
  made: 'ok',
  cannot: 'warn',
  failed: 'danger',
}

/** How a local folder's version stands for its scrubbing thumbnails, with when it last did. */
function ThumbnailsState({ thumbnails }: { thumbnails: VersionThumbnails }) {
  const { t } = useI18n()
  const text = t.versions
  const { state, reason, count, at } = thumbnails
  const word =
    state === 'made'
      ? text.thumbnailStates.made(count)
      : state === 'cannot'
        ? (text.thumbnailsCannot[reason as keyof typeof text.thumbnailsCannot] ??
          text.thumbnailsCannot.unreadable)
        : text.thumbnailStates[state]
  return (
    <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
      <span className="text-ink-2">{text.thumbnails}</span>
      <StatusPill tone={thumbnailTones[state]}>{word}</StatusPill>
      {at !== null && state !== 'waiting' && state !== 'working' && <RelativeTime iso={at} />}
    </span>
  )
}

/** Blocks a stream's file, for the title or every title, with a reason. */
function BlockModal({
  id,
  user,
  stream,
  onClose,
  onBlocked,
}: {
  id: string
  user: string
  stream: VersionStream
  onClose: () => void
  onBlocked: () => void
}) {
  const { t } = useI18n()
  const text = t.versions
  const toast = useToast()
  const formId = useId()
  const [scope, setScope] = useState<'title' | 'everywhere'>('title')
  const [reason, setReason] = useState('')
  const block = useMutation({
    mutationFn: () =>
      blockVersion(id, {
        version: stream.version,
        user,
        everywhere: scope === 'everywhere',
        reason: reason.trim(),
      }),
    onSuccess: () => {
      toast(text.blockDone, { tone: 'ok' })
      onBlocked()
      onClose()
    },
  })
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    block.mutate()
  }
  return (
    <Modal
      open
      onClose={onClose}
      title={text.blockTitle}
      width={560}
      footer={
        <>
          <Button onClick={onClose}>{t.common.cancel}</Button>
          <Button
            type="submit"
            form={formId}
            variant="danger"
            icon={ProhibitIcon}
            loading={block.isPending}
          >
            {text.block}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-4 px-5 pb-5">
        <p className="text-body break-words text-ink">{stream.name || stream.filename}</p>
        <Field label={text.blockScope} help={text.scopeHelp}>
          <Segmented
            label={text.blockScope}
            value={scope}
            onChange={setScope}
            options={[
              { value: 'title', label: text.scopeTitle },
              { value: 'everywhere', label: text.scopeEverywhere },
            ]}
          />
        </Field>
        <Field label={text.reason} help={text.reasonHelp(maxBlockReason)}>
          <TextInput
            value={reason}
            onValue={setReason}
            maxLength={maxBlockReason}
            autoComplete="off"
          />
        </Field>
        {block.isError && <Notice tone="danger">{errorMessage(t, block.error)}</Notice>}
      </form>
    </Modal>
  )
}

/** The blocks of the title's own files, those no addon lists now included. */
function TitleBlocks({ id, user, blocks }: { id: string; user: string; blocks: VersionBlock[] }) {
  const { t, language } = useI18n()
  const text = t.versions
  const toast = useToast()
  const unblock = useMutation({
    mutationFn: unblockVersion,
    onSuccess: () => {
      toast(text.unblockDone, { tone: 'ok' })
      void queryClient.invalidateQueries({ queryKey: queryKeys.titleVersions(id, user) })
    },
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
  })
  return (
    <Block title={text.titleBlocks} count={blocks.length}>
      {blocks.length === 0 ? (
        <p className="text-small text-ink-2">{text.noTitleBlocks}</p>
      ) : (
        <RowList compact aria-label={text.titleBlocks}>
          {blocks.map((block) => (
            <Row
              key={block.id}
              leading={ProhibitIcon}
              title={<span className="break-all">{block.name}</span>}
              meta={
                <span className="flex min-w-0 flex-col break-words">
                  <span>
                    {[block.filename, block.size > 0 ? formatBytes(block.size, language) : '']
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                  <span>
                    {text.blockedBy(block.by, block.reason)} · <RelativeTime iso={block.at} />
                  </span>
                </span>
              }
              trailing={
                <Button
                  size="sm"
                  loading={unblock.isPending && unblock.variables === block.id}
                  onClick={() => unblock.mutate(block.id)}
                >
                  {text.unblock}
                </Button>
              }
            />
          ))}
        </RowList>
      )}
    </Block>
  )
}

/** The title's last plays: who, where, what each tried and the version chosen. */
function Decisions({ explained }: { explained: TitleVersions }) {
  const { t } = useI18n()
  const text = t.versions
  return (
    <Block title={text.decisions} count={explained.decisions.length}>
      <p className="mb-3 max-w-[72ch] text-small text-ink-2">{text.decisionsHelp}</p>
      {explained.decisions.length === 0 ? (
        <EmptyState title={text.noDecisions} />
      ) : (
        <RowList compact aria-label={text.decisions}>
          {explained.decisions.map((decision, index) => (
            <Decision key={`${decision.at}-${index}`} decision={decision} />
          ))}
        </RowList>
      )}
    </Block>
  )
}

function Decision({ decision }: { decision: PlayDecision }) {
  const { t } = useI18n()
  const text = t.versions
  const chosen = decision.tries.find((tried) => tried.version === decision.chosen)
  return (
    <Row
      title={
        decision.chosen === ''
          ? text.noneChosen
          : [chosen?.name ?? decision.chosen, text.deliveries[decision.delivery] ?? '']
              .filter(Boolean)
              .join(' · ')
      }
      titleAside={decision.picked ? <Badge>{text.picked}</Badge> : undefined}
      meta={
        <span className="min-w-0 break-words">
          {text.decisionBy(decision.user, decision.device, decision.app)} ·{' '}
          <RelativeTime iso={decision.at} />
        </span>
      }
    >
      <ol className="-mt-2 flex list-decimal flex-col gap-1 pl-5 text-small text-ink-2">
        {decision.tries.map((tried, index) => (
          <li key={`${tried.version}-${index}`}>
            <span className="break-words text-ink">{tried.name}</span>:{' '}
            {text.outcomes[tried.outcome] ?? tried.outcome}
            {tried.reason !== '' && <span className="text-ink-3"> ({tried.reason})</span>}
          </li>
        ))}
      </ol>
    </Row>
  )
}
