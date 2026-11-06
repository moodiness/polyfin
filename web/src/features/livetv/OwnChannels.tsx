import {
  FilmSlateIcon,
  MagnifyingGlassIcon,
  PlusIcon,
  TelevisionSimpleIcon,
  TrashIcon,
  XIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router'
import {
  ApiError,
  createOwnChannel,
  deleteOwnChannel,
  fetchChannelChoices,
  fetchOwnChannel,
  fetchOwnChannels,
  queryClient,
  queryKeys,
  saveOwnChannel,
  searchChannelTitles,
  type ChannelChoices,
  type ChannelSource,
  type ChannelSourceKind,
  type OwnChannel,
  type OwnChannelDraft,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage, formatEpisode, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  Button,
  ButtonLink,
  ConfirmDialog,
  EmptyState,
  Field,
  FieldError,
  IconButton,
  InlineError,
  Row,
  RowList,
  SaveBar,
  SettingRow,
  SkeletonRows,
  StatusPill,
  TextInput,
  useToast,
} from '@/ui'

/** The page of a channel of the server's titles, `new` for one being made. */
export function ownChannelPath(id: string): string {
  return `/live-tv/channels/${id}`
}

const kinds: ChannelSourceKind[] = ['collection', 'smartlist', 'series', 'titles']

/** A time of the day in minutes, as an `<input type="time">` writes it. */
function clock(minutes: number): string {
  const m = Math.min(Math.max(minutes, 0), 24 * 60 - 1)
  return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`
}

function minutesOf(value: string, end: boolean): number {
  const [h, m] = value.split(':').map(Number)
  const minutes = (h || 0) * 60 + (m || 0)
  // A block that ends at midnight ends at the end of the day.
  return end && minutes === 0 ? 24 * 60 : minutes
}

/**
 * The channels Polyfin airs from the server's titles, on the Live TV page, each opening its page,
 * with what it airs now.
 */
export function OwnChannels() {
  const { t, language } = useI18n()
  const channels = useQuery({
    queryKey: queryKeys.ownChannels,
    queryFn: ({ signal }) => fetchOwnChannels(signal),
  })
  const add = (
    <ButtonLink to={ownChannelPath('new')} icon={PlusIcon} size="sm">
      {t.ownChannels.add}
    </ButtonLink>
  )
  let body
  if (channels.isPending) body = <SkeletonRows rows={2} label={t.ownChannels.loading} />
  else if (channels.isError)
    body = (
      <InlineError onRetry={() => void channels.refetch()} retrying={channels.isFetching}>
        {errorMessage(t, channels.error)}
      </InlineError>
    )
  else if (channels.data.length === 0)
    body = (
      <EmptyState icon={FilmSlateIcon} title={t.ownChannels.emptyTitle} action={add}>
        {t.ownChannels.empty}
      </EmptyState>
    )
  else
    body = (
      <RowList aria-label={t.ownChannels.label}>
        {channels.data.map((channel) => {
          const parts = [
            t.ownChannels.entries(channel.entries, channel.entries.toLocaleString(language)),
            formatSpan(channel.runtime, language),
          ]
          if (channel.blocks.length > 0) parts.push(t.ownChannels.blockCount(channel.blocks.length))
          const now = channel.airing[0]
          const state = !channel.enabled ? (
            <StatusPill tone="muted">{t.ownChannels.off}</StatusPill>
          ) : now ? (
            <span className="break-words">{t.ownChannels.now(now.name)}</span>
          ) : null
          return (
            <Row
              key={channel.id}
              leading={FilmSlateIcon}
              to={ownChannelPath(channel.id)}
              title={channel.number === null ? channel.name : `${channel.number} · ${channel.name}`}
              muted={!channel.enabled}
              meta={
                <>
                  <span>{parts.join(' · ')}</span>
                  {state && <PhoneState>{state}</PhoneState>}
                </>
              }
              trailing={state && <WideState>{state}</WideState>}
            />
          )
        })}
      </RowList>
    )
  return (
    <Block
      title={t.ownChannels.title}
      count={channels.data?.length}
      aside={channels.data && channels.data.length > 0 ? add : undefined}
    >
      <p className="mb-5 max-w-[72ch] text-small text-ink-2">{t.ownChannels.help}</p>
      {body}
    </Block>
  )
}

/** A row's state on the right from 768 px up. */
export function WideState({ children }: { children: ReactNode }) {
  return <span className="max-md:hidden">{children}</span>
}

/** A row's state on its own line under its second one on a phone, leaving the label room. */
export function PhoneState({ children }: { children: ReactNode }) {
  return <span className="mt-1.5 basis-full md:hidden">{children}</span>
}

const emptyDraft: OwnChannelDraft = {
  name: '',
  number: null,
  enabled: true,
  source: { kind: 'collection', items: [] },
  shuffle: false,
  blocks: [],
}

/** `/live-tv/channels/:id`: makes or changes a channel of the server's titles. */
export default function OwnChannelRoute() {
  const { t } = useI18n()
  const { id = 'new' } = useParams()
  const isNew = id === 'new'
  const back = { to: '/live-tv', label: t.ownChannels.back }
  const channel = useQuery({
    queryKey: queryKeys.ownChannel(id),
    queryFn: ({ signal }) => fetchOwnChannel(id, signal),
    enabled: !isNew,
  })
  const choices = useQuery({
    queryKey: queryKeys.channelChoices,
    queryFn: ({ signal }) => fetchChannelChoices(signal),
  })
  if (!isNew && channel.isError) {
    const missing = channel.error instanceof ApiError && channel.error.status === 404
    return (
      <PageLayout title={t.ownChannels.title} back={back}>
        {missing ? (
          <EmptyState icon={TelevisionSimpleIcon} title={t.ownChannels.notFound} />
        ) : (
          <InlineError onRetry={() => void channel.refetch()} retrying={channel.isFetching}>
            {errorMessage(t, channel.error)}
          </InlineError>
        )}
      </PageLayout>
    )
  }
  if ((!isNew && channel.isPending) || choices.isPending || choices.isError) {
    return (
      <PageLayout title={t.ownChannels.newTitle} back={back}>
        {choices.isError ? (
          <InlineError onRetry={() => void choices.refetch()} retrying={choices.isFetching}>
            {errorMessage(t, choices.error)}
          </InlineError>
        ) : (
          <SkeletonRows rows={4} label={t.ownChannels.loading} />
        )}
      </PageLayout>
    )
  }
  return (
    <Editor
      key={id}
      saved={isNew ? null : (channel.data ?? null)}
      choices={choices.data}
      back={back}
    />
  )
}

/** The fields of a saved channel that its form changes. */
function draftOf(saved: OwnChannel | null): OwnChannelDraft {
  if (saved === null) return emptyDraft
  return {
    name: saved.name,
    number: saved.number,
    enabled: saved.enabled,
    source: saved.source,
    shuffle: saved.shuffle,
    blocks: saved.blocks.map(({ start, end, source, shuffle }) => ({
      start,
      end,
      source,
      shuffle,
    })),
  }
}

/** How many of a channel's settings differ from the saved ones. */
function countChanges(draft: OwnChannelDraft, saved: OwnChannelDraft): number {
  const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)
  return [
    draft.name !== saved.name,
    draft.number !== saved.number,
    draft.enabled !== saved.enabled,
    !same(draft.source, saved.source),
    draft.shuffle !== saved.shuffle,
    !same(draft.blocks, saved.blocks),
  ].filter(Boolean).length
}

function Editor({
  saved,
  choices,
  back,
}: {
  saved: OwnChannel | null
  choices: ChannelChoices
  back: { to: string; label: string }
}) {
  const { t, language } = useI18n()
  const toast = useToast()
  const navigate = useNavigate()
  const [draft, setDraft] = useState<OwnChannelDraft>(() => draftOf(saved))
  const [rebuild, setRebuild] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const change = (patch: Partial<OwnChannelDraft>) => setDraft((d) => ({ ...d, ...patch }))
  const was = saved ? draftOf(saved) : null
  const changes = was ? countChanges(draft, was) + (rebuild ? 1 : 0) : 0

  const save = useMutation({
    mutationFn: () =>
      saved ? saveOwnChannel(saved.id, { ...draft, rebuild }) : createOwnChannel(draft),
    onSuccess: (channel) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownChannels })
      queryClient.setQueryData(queryKeys.ownChannel(channel.id), channel)
      setRebuild(false)
      toast(saved ? t.ownChannels.saved : t.ownChannels.created)
      if (!saved) void navigate(ownChannelPath(channel.id), { replace: true })
    },
  })
  const remove = useMutation({
    mutationFn: () => deleteOwnChannel(saved?.id ?? ''),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownChannels })
      toast(t.ownChannels.deleted)
      void navigate(back.to)
    },
  })
  const code = save.error instanceof ApiError ? save.error.code : null
  const nameError = code === 'invalid_own_channel_name' ? errorMessage(t, save.error) : undefined
  const numberError = code === 'invalid_channel_number' ? errorMessage(t, save.error) : undefined
  const generalError =
    save.isError && !nameError && !numberError ? errorMessage(t, save.error) : undefined
  const time = new Intl.DateTimeFormat(language, { timeStyle: 'short' })

  return (
    <PageLayout
      title={saved ? saved.name : t.ownChannels.newTitle}
      lede={t.ownChannels.lede}
      back={back}
      actions={
        saved && (
          <Button variant="danger" icon={TrashIcon} onClick={() => setDeleting(true)}>
            {t.ownChannels.delete}
          </Button>
        )
      }
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          save.mutate()
        }}
        className="flex flex-col gap-block"
      >
        <div className={rows}>
          <SettingRow
            kind="text"
            label={t.ownChannels.name}
            value={draft.name}
            maxLength={64}
            onChange={(name) => change({ name })}
            saved={was?.name}
            error={nameError}
          />
          <SettingRow
            kind="number"
            label={t.ownChannels.number}
            help={t.ownChannels.numberHelp}
            value={draft.number}
            min={1}
            max={9999}
            step={1}
            picker="well"
            onChange={(number) => change({ number })}
            saved={was ? was.number : undefined}
            error={numberError}
          />
          <SettingRow
            kind="switch"
            label={t.ownChannels.enabled}
            checked={draft.enabled}
            onChange={(enabled) => change({ enabled })}
            saved={was?.enabled}
          />
        </div>

        <Block title={t.ownChannels.programmes}>
          <p className={blockHelp}>{t.ownChannels.programmesHelp}</p>
          <div className={rows}>
            <SourcePicker
              source={draft.source}
              saved={was?.source}
              choices={choices}
              onChange={(source) => change({ source })}
            />
            <SettingRow
              kind="switch"
              label={t.ownChannels.shuffle}
              help={t.ownChannels.shuffleHelp}
              checked={draft.shuffle}
              onChange={(shuffle) => change({ shuffle })}
              saved={was?.shuffle}
            />
          </div>
        </Block>

        <Block title={t.ownChannels.blocks} count={draft.blocks.length}>
          <p className={blockHelp}>{t.ownChannels.blocksHelp}</p>
          <div className="flex flex-col gap-10">
            {draft.blocks.map((block, i) => {
              const set = (patch: Partial<OwnChannelDraft['blocks'][number]>) =>
                change({ blocks: draft.blocks.map((b, j) => (j === i ? { ...b, ...patch } : b)) })
              return (
                <fieldset key={i} className="min-w-0">
                  <legend className="mb-4 text-group text-ink">{t.ownChannels.block(i + 1)}</legend>
                  <div className="mb-4 flex flex-wrap items-end gap-3">
                    <Field label={t.ownChannels.from}>
                      <TextInput
                        type="time"
                        size="sm"
                        value={clock(block.start)}
                        onValue={(value) => set({ start: minutesOf(value, false) })}
                        className="w-[10rem]"
                      />
                    </Field>
                    <Field label={t.ownChannels.until}>
                      <TextInput
                        type="time"
                        size="sm"
                        value={clock(block.end === 24 * 60 ? 0 : block.end)}
                        onValue={(value) => set({ end: minutesOf(value, true) })}
                        className="w-[10rem]"
                      />
                    </Field>
                    <Button
                      variant="danger"
                      icon={TrashIcon}
                      onClick={() => change({ blocks: draft.blocks.filter((_, j) => j !== i) })}
                    >
                      {t.ownChannels.removeBlock(i + 1)}
                    </Button>
                  </div>
                  <div className={rows}>
                    <SourcePicker
                      source={block.source}
                      choices={choices}
                      onChange={(source) => set({ source })}
                    />
                    <SettingRow
                      kind="switch"
                      label={t.ownChannels.shuffle}
                      help={t.ownChannels.shuffleHelp}
                      checked={block.shuffle}
                      onChange={(shuffle) => set({ shuffle })}
                    />
                  </div>
                </fieldset>
              )
            })}
            <div>
              <Button
                icon={PlusIcon}
                disabled={draft.blocks.length >= 12}
                onClick={() =>
                  change({
                    blocks: [
                      ...draft.blocks,
                      {
                        start: 18 * 60,
                        end: 20 * 60,
                        source: { kind: 'series', items: [] },
                        shuffle: false,
                      },
                    ],
                  })
                }
              >
                {t.ownChannels.addBlock}
              </Button>
            </div>
          </div>
        </Block>

        {saved && (
          <Block title={t.ownChannels.airing}>
            <p className={blockHelp}>{t.ownChannels.airingHelp}</p>
            {saved.airing.length === 0 ? (
              <p className="mb-4 text-small text-ink-3">{t.ownChannels.nothing}</p>
            ) : (
              <RowList compact aria-label={t.ownChannels.airing} className="mb-4">
                {saved.airing.map((airing) => (
                  <Row
                    key={airing.start}
                    leading={
                      <span className={airingTime}>
                        {time.format(new Date(airing.start))}
                        {' – '}
                        {time.format(new Date(airing.end))}
                      </span>
                    }
                    title={airing.name}
                    meta={
                      airing.episode !== ''
                        ? `${formatEpisode(airing.season, airing.number)} · ${airing.episode}`
                        : undefined
                    }
                  />
                ))}
              </RowList>
            )}
            <div className={rows}>
              <SettingRow
                kind="switch"
                label={t.ownChannels.rebuild}
                help={t.ownChannels.rebuildHelp}
                checked={rebuild}
                onChange={setRebuild}
              />
            </div>
          </Block>
        )}

        {saved ? (
          <SaveBar
            dirty={changes > 0}
            saving={save.isPending}
            changes={changes}
            onDiscard={() => {
              setDraft(draftOf(saved))
              setRebuild(false)
              save.reset()
            }}
            error={generalError}
          />
        ) : (
          <div className="flex flex-col items-start gap-4">
            {generalError && <FieldError>{generalError}</FieldError>}
            <Button type="submit" variant="primary" loading={save.isPending}>
              {t.ownChannels.create}
            </Button>
          </div>
        )}
      </form>
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        onConfirm={() => remove.mutate()}
        title={t.ownChannels.deleteTitle(saved?.name ?? '')}
        confirmLabel={t.ownChannels.delete}
        tone="danger"
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {t.ownChannels.deleteBody}
      </ConfirmDialog>
    </PageLayout>
  )
}

/** The start and end of a programme at the start of its row. */
const airingTime = 'figures w-[7.75rem] shrink-0 text-micro text-ink-3 max-md:w-[6rem]'
/** Settings rows, 8 px apart. */
const rows = 'grid grid-cols-[minmax(0,1fr)] gap-2'
/** The help under a block's title. */
const blockHelp = 'mb-5 max-w-[72ch] text-small text-ink-2'

/** Chooses what a channel or a block is built from, as settings rows. */
function SourcePicker({
  source,
  saved,
  choices,
  onChange,
}: {
  source: ChannelSource
  /** The saved source, for the rows' "was" marks; none for a block. */
  saved?: ChannelSource
  choices: ChannelChoices
  onChange: (source: ChannelSource) => void
}) {
  const { t } = useI18n()
  const listed = source.kind === 'collection' ? choices.collections : choices.smartLists
  const chosen = source.items[0]?.id ?? ''
  return (
    <>
      <SettingRow
        kind="choice"
        label={t.ownChannels.kind}
        value={source.kind}
        options={kinds.map((value) => ({ value, label: t.ownChannels.kinds[value] }))}
        onChange={(kind) => onChange({ kind, items: [] })}
        saved={saved?.kind}
      />
      {(source.kind === 'collection' || source.kind === 'smartlist') &&
        (listed.length === 0 ? (
          <p className="px-[1.875rem] py-2 text-small text-ink-2 max-md:px-5">
            {source.kind === 'collection'
              ? t.ownChannels.noCollections
              : t.ownChannels.noSmartLists}
          </p>
        ) : (
          <SettingRow
            kind="choice"
            label={
              source.kind === 'collection' ? t.ownChannels.collection : t.ownChannels.smartList
            }
            value={chosen}
            options={[
              ...(chosen === ''
                ? [{ value: '', label: t.ownChannels.choose, disabled: true }]
                : []),
              ...listed.map((c) => ({ value: c.id, label: c.name })),
            ]}
            onChange={(id) => {
              const choice = listed.find((c) => c.id === id)
              if (choice)
                onChange({
                  kind: source.kind,
                  items: [{ id, name: choice.name, kind: source.kind }],
                })
            }}
            saved={saved?.kind === source.kind ? (saved.items[0]?.id ?? '') : undefined}
          />
        ))}
      {(source.kind === 'series' || source.kind === 'titles') && (
        <TitlePicker source={source} onChange={onChange} />
      )}
    </>
  )
}

/** Searches movies and series: a series replaces the one chosen, titles add up. */
function TitlePicker({
  source,
  onChange,
}: {
  source: ChannelSource
  onChange: (source: ChannelSource) => void
}) {
  const { t } = useI18n()
  const [query, setQuery] = useState('')
  const [asked, setAsked] = useState('')
  const found = useQuery({
    queryKey: queryKeys.channelTitles(asked),
    queryFn: ({ signal }) => searchChannelTitles(asked, signal),
    enabled: asked !== '',
    staleTime: 5 * 60_000,
  })
  const series = source.kind === 'series'
  const results = (found.data ?? []).filter((title) => !series || title.kind === 'series')
  function submit(event: FormEvent) {
    event.preventDefault()
    setAsked(query.trim())
  }
  return (
    <div className="flex flex-col gap-4 pt-4">
      <div className="flex flex-wrap items-end gap-2">
        <Field
          label={series ? t.ownChannels.searchSeries : t.ownChannels.searchTitles}
          className="min-w-[12.5rem] flex-1"
        >
          <TextInput
            value={query}
            onValue={setQuery}
            icon={MagnifyingGlassIcon}
            size="sm"
            onKeyDown={(event) => {
              if (event.key === 'Enter') submit(event)
            }}
          />
        </Field>
        <Button onClick={(event) => submit(event)} loading={found.isFetching}>
          {t.ownChannels.search}
        </Button>
      </div>
      {found.isError && <InlineError>{errorMessage(t, found.error)}</InlineError>}
      {asked !== '' && found.isSuccess && results.length === 0 && (
        <p className="text-small text-ink-3">{t.ownChannels.noResults}</p>
      )}
      {results.length > 0 && (
        <div className="max-h-[20rem] overflow-y-auto rounded-row">
          <RowList compact aria-label={t.ownChannels.searchTitles}>
            {results.map((title) => (
              <Row
                key={title.id}
                title={title.name}
                meta={[
                  title.year !== null ? String(title.year) : '',
                  title.kind === 'series' ? t.ownChannels.series : t.ownChannels.movie,
                ]
                  .filter(Boolean)
                  .join(' · ')}
                trailing={
                  <IconButton
                    label={t.ownChannels.pick(title.name)}
                    icon={PlusIcon}
                    size="sm"
                    onClick={() => {
                      const item = { id: title.id, name: title.name, kind: title.kind }
                      if (series) onChange({ kind: 'series', items: [item] })
                      else if (!source.items.some((i) => i.id === title.id))
                        onChange({ kind: 'titles', items: [...source.items, item] })
                    }}
                  />
                }
              />
            ))}
          </RowList>
        </div>
      )}
      <div className="flex flex-col gap-2.5">
        <p className="text-small font-semibold text-ink-2">{t.ownChannels.picked}</p>
        {source.items.length === 0 ? (
          <p className="text-small text-ink-3">{t.ownChannels.pickedNone}</p>
        ) : (
          <RowList compact aria-label={t.ownChannels.picked}>
            {source.items.map((item) => (
              <Row
                key={item.id}
                title={item.name}
                trailing={
                  <IconButton
                    label={t.ownChannels.remove(item.name)}
                    icon={XIcon}
                    size="sm"
                    onClick={() =>
                      onChange({ ...source, items: source.items.filter((i) => i.id !== item.id) })
                    }
                  />
                }
              />
            ))}
          </RowList>
        )}
      </div>
    </div>
  )
}
