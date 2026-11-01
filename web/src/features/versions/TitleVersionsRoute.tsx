import { ArrowsClockwiseIcon, ProhibitIcon, PushPinIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import { useParams } from 'react-router'
import {
  analyzeVersion,
  ApiError,
  askVersionsAgain,
  blockVersion,
  fetchTitleVersions,
  fetchVersionTitle,
  maxBlockReason,
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
import { userQuery, UserPicker, useVersionsUser } from './parts'

/** How long the page waits before reading the versions again while addons are still asked. */
const pendingPoll = 2000

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
  const versions = useQuery({
    queryKey: queryKeys.titleVersions(id, user),
    queryFn: ({ signal }) => fetchTitleVersions(id, user, signal),
    refetchInterval: (query) => ((query.state.data?.pending ?? 0) > 0 ? pendingPoll : false),
  })
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
        <Explained id={id} user={user} explained={versions.data} />
      )}
    </PageLayout>
  )
}

/** The blocks of a title's versions explained. */
function Explained({
  id,
  user,
  explained,
}: {
  id: string
  user: string
  explained: TitleVersions
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
    <div className="flex flex-col gap-block max-md:gap-11">
      <Block title={text.order}>
        <div className="flex flex-col gap-2 text-control text-ink-2">
          <p>{criteria === '' ? text.orderNone : text.orderBy(criteria)}</p>
          {explained.qualityGroup > 0 && <p>{text.group(explained.qualityGroup)}</p>}
          {explained.keptAll && <Notice tone="info">{text.keptAll}</Notice>}
          {explained.closest && <Notice tone="info">{text.closest}</Notice>}
          {explained.pending > 0 && (
            <Notice tone="info" live>
              {text.pending(explained.pending)}
            </Notice>
          )}
        </div>
      </Block>
      <PinBlock id={id} explained={explained} />
      <Block title={text.sources}>
        {explained.sources.length === 0 ? (
          <EmptyState title={text.noSources} />
        ) : (
          <div className="flex flex-col gap-8">
            {explained.sources.map((source) => (
              <SourceStreams
                key={`${source.id}-${source.embedded}`}
                id={id}
                user={user}
                source={source}
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
        <p className="max-w-2xl text-small text-ink-3">{text.noPin}</p>
      ) : (
        <RowList>
          <li>
            <Row
              leading={PushPinIcon}
              title={<span className="break-all">{pin.name}</span>}
              titleAside={pin.missing ? <Badge tone="warn">{text.pinMissing}</Badge> : undefined}
              meta={
                <span className="flex min-w-0 flex-col whitespace-normal break-words">
                  {[pin.filename, pin.size > 0 ? formatBytes(pin.size, language) : '']
                    .filter(Boolean)
                    .join(' · ')}
                  <span className="block">
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
          </li>
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

/** A source's streams, each with what became of it and the actions on its version. */
function SourceStreams({
  id,
  user,
  source,
}: {
  id: string
  user: string
  source: TitleVersionSource
}) {
  const { t, language } = useI18n()
  const text = t.versions
  const toast = useToast()
  const [blocking, setBlocking] = useState<VersionStream | null>(null)
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
  const state: Record<TitleVersionSource['state'], StatusTone> = {
    known: 'ok',
    stale: 'warn',
    asked: 'muted',
  }
  return (
    <section aria-label={source.name} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2.5">
        <h3 className="text-control font-medium text-ink">{source.name}</h3>
        {source.embedded && <Badge>{text.embedded}</Badge>}
        <StatusPill tone={state[source.state]}>{text.sourceStates[source.state]}</StatusPill>
      </div>
      {source.streams.length === 0 ? (
        <p className="text-small text-ink-3">{text.noStreams}</p>
      ) : (
        <RowList aria-label={source.name}>
          {source.streams.map((stream, index) => {
            const explanation = why(t, stream)
            const labels = tags(stream.labels, t)
            const known = tags(stream.facts, t)
            const isVersion =
              stream.version !== '' && !['duplicate', 'mirror', 'blocked'].includes(stream.step)
            return (
              <li key={`${index}-${stream.version}`}>
                <Row
                  title={<span className="break-words">{stream.name || stream.filename}</span>}
                  titleAside={
                    <>
                      {stream.step === 'offered' ? (
                        <Badge tone="accent">{text.steps.offered(stream.place)}</Badge>
                      ) : (
                        <StatusPill tone={stepTones[stream.step] ?? 'muted'}>
                          {text.steps[stream.step]}
                        </StatusPill>
                      )}
                      {stream.pinned && (
                        <Badge tone="ok">
                          <PushPinIcon size={12} aria-hidden="true" className="mr-1" />
                          {text.pinned}
                        </Badge>
                      )}
                      {stream.analyzed && <Badge>{text.analyzed}</Badge>}
                    </>
                  }
                  meta={
                    <span className="flex min-w-0 flex-col whitespace-normal break-words">
                      {[
                        stream.filename,
                        stream.size > 0 ? formatBytes(stream.size, language) : '',
                        stream.torrent ? 'torrent' : '',
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                      {labels.length > 0 && (
                        <span className="block">
                          {text.labelsAsRead} {labels.join(' · ')}
                        </span>
                      )}
                      {stream.analyzed && known.length > 0 && known.join() !== labels.join() && (
                        <span className="block">
                          {text.facts} {known.join(' · ')}
                        </span>
                      )}
                      {stream.facts.labelRules.length > 0 && (
                        <span className="block">
                          {text.labelRules(stream.facts.labelRules, stream.facts.score)}
                        </span>
                      )}
                      {explanation !== '' && (
                        <span className="block text-ink-2">{explanation}</span>
                      )}
                      {stream.unstarted && (
                        <span className="block text-ink-2">{text.unstarted}</span>
                      )}
                    </span>
                  }
                  trailing={
                    stream.step === 'blocked' && stream.block !== null ? (
                      <Button
                        size="sm"
                        loading={unblock.isPending && unblock.variables === stream.block.id}
                        onClick={() => stream.block !== null && unblock.mutate(stream.block.id)}
                      >
                        {text.unblock}
                      </Button>
                    ) : isVersion ? (
                      <div className="flex flex-wrap justify-end gap-2">
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
                        <Button
                          size="sm"
                          variant="ghost"
                          icon={ProhibitIcon}
                          onClick={() => setBlocking(stream)}
                        >
                          {text.block}
                        </Button>
                      </div>
                    ) : undefined
                  }
                />
              </li>
            )
          })}
        </RowList>
      )}
      {blocking !== null && (
        <BlockModal
          id={id}
          user={user}
          stream={blocking}
          onClose={() => setBlocking(null)}
          onBlocked={refresh}
        />
      )}
    </section>
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
      <form id={formId} onSubmit={submit} noValidate className="space-y-5 p-5">
        <p className="text-control break-words text-ink">{stream.name || stream.filename}</p>
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
        <p className="text-small text-ink-3">{text.noTitleBlocks}</p>
      ) : (
        <RowList>
          {blocks.map((block) => (
            <li key={block.id}>
              <Row
                leading={ProhibitIcon}
                title={<span className="break-all">{block.name}</span>}
                meta={
                  <span className="flex min-w-0 flex-col whitespace-normal break-words">
                    {[block.filename, block.size > 0 ? formatBytes(block.size, language) : '']
                      .filter(Boolean)
                      .join(' · ')}
                    <span className="block">
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
            </li>
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
      <p className="mb-4 max-w-2xl text-small text-ink-3">{text.decisionsHelp}</p>
      {explained.decisions.length === 0 ? (
        <EmptyState title={text.noDecisions} />
      ) : (
        <RowList>
          {explained.decisions.map((decision, index) => (
            <li key={`${decision.at}-${index}`}>
              <Decision decision={decision} />
            </li>
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
        <span className="flex min-w-0 flex-col whitespace-normal break-words">
          {text.decisionBy(decision.user, decision.device, decision.app)} ·{' '}
          <RelativeTime iso={decision.at} />
        </span>
      }
    >
      <ol className="mt-2 flex list-decimal flex-col gap-1 pl-5 text-small text-ink-2">
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
