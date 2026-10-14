import {
  ArrowLeftIcon,
  CheckIcon,
  CopyIcon,
  FilmSlateIcon,
  MagnifyingGlassIcon,
  ProhibitIcon,
  ShareNetworkIcon,
  TelevisionSimpleIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router'
import {
  ApiError,
  createShareLink,
  fetchShareEpisodes,
  fetchShareTitle,
  maxShareViewers,
  queryClient,
  queryKeys,
  revokeOwnShareLink,
  searchShareTitles,
  shareLinkHours,
  type AccountTitle,
  type NewShareLink,
  type ShareLink,
  type ShareLinkHours,
  type ShareLinkState,
} from '@/api'
import { errorMessage, formatEpisode } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  ConfirmDialog,
  cx,
  EmptyState,
  Field,
  InlineError,
  Modal,
  Notice,
  RelativeTime,
  Row,
  RowList,
  Select,
  Skeleton,
  StatusPill,
  TextInput,
  useToast,
  type StatusTone,
} from '@/ui'
import { TitlePoster, titleName } from './WatchingLists'

const stateTones: Record<ShareLinkState, StatusTone> = {
  active: 'ok',
  expired: 'muted',
  revoked: 'muted',
}

/** The id of My account's section, which the web player's Share and the notifications link to. */
const sharedLinksSection = 'shared-links'

/**
 * My account › Shared links: the user's links, each with Revoke, and Share a title while they may
 * share. `?share=<id>`, as the web player's Share opens the page, opens the form with that title.
 */
export default function SharedLinks({
  allowed,
  links,
}: {
  /** Whether the server allows share links and the user may share. */
  allowed: boolean
  links: ShareLink[]
}) {
  const { t } = useI18n()
  const text = t.shareLinks
  const [params, setParams] = useSearchParams()
  const preselect = params.get('share')
  const preselected = useQuery({
    queryKey: queryKeys.shareTitle(preselect ?? ''),
    queryFn: ({ signal }) => fetchShareTitle(preselect ?? '', signal),
    enabled: preselect !== null && allowed,
  })
  const [creating, setCreating] = useState(false)
  const fromPlayer = preselect !== null && preselected.data !== undefined

  function close() {
    setCreating(false)
    if (preselect === null) return
    // Once closed, the address no longer opens the form, and the list shows the new link.
    setParams(
      (current) => {
        current.delete('share')
        return current
      },
      { replace: true, preventScrollReset: true },
    )
    document.getElementById(sharedLinksSection)?.scrollIntoView({ block: 'start' })
  }

  const share = allowed && (
    <Button icon={ShareNetworkIcon} onClick={() => setCreating(true)}>
      {text.share}
    </Button>
  )

  return (
    <div className="space-y-3">
      {!allowed && <Notice tone="warn">{text.off}</Notice>}
      {preselect !== null && preselected.isError && (
        <Notice tone="danger" live>
          {errorMessage(t, preselected.error)}
        </Notice>
      )}
      {links.length === 0 ? (
        <EmptyState icon={ShareNetworkIcon} title={text.empty} action={share || undefined}>
          {text.emptyHelp}
        </EmptyState>
      ) : (
        <>
          {share && <div className="flex justify-end">{share}</div>}
          <ShareLinkList
            links={links}
            queryKey={queryKeys.ownShareLinks}
            revoke={revokeOwnShareLink}
          />
        </>
      )}
      {(creating || fromPlayer) && (
        <ShareTitleModal
          key={fromPlayer ? preselected.data.id : ''}
          initial={fromPlayer ? preselected.data : null}
          onClose={close}
        />
      )}
    </div>
  )
}

/**
 * Share links, each with its title, when it expires, how many may watch at once and how many
 * times it was played, and Revoke while it is active: the user's own on My account, or a user's
 * on their page for an administrator. `queryKey` is the list's, refreshed after a revocation.
 */
export function ShareLinkList({
  links,
  queryKey,
  revoke,
}: {
  links: ShareLink[]
  queryKey: readonly unknown[]
  revoke: (id: string) => Promise<ShareLink>
}) {
  const { t } = useI18n()
  const text = t.shareLinks
  const toast = useToast()
  const [asked, setAsked] = useState<ShareLink | null>(null)
  const revocation = useMutation({
    mutationFn: (link: ShareLink) => revoke(link.id),
    onSuccess: () => {
      setAsked(null)
      toast(text.revoked, { tone: 'ok' })
    },
    onSettled: (_data, error) => {
      if (error === null || (error instanceof ApiError && error.status === 404)) {
        void queryClient.invalidateQueries({ queryKey })
      }
    },
  })

  return (
    <>
      <RowList aria-label={t.account.sections.sharedLinks}>
        {links.map((link) => (
          <Row
            key={link.id}
            leading={link.item.kind === 'episode' ? TelevisionSimpleIcon : FilmSlateIcon}
            title={link.item.name}
            muted={link.state !== 'active'}
            meta={
              <span className="truncate">
                {link.state === 'revoked' && link.revokedAt !== null ? (
                  <>
                    {text.revokedLabel} <RelativeTime iso={link.revokedAt} />
                  </>
                ) : (
                  <>
                    {link.state === 'active' ? text.expiresLabel : text.expiredLabel}{' '}
                    <RelativeTime iso={link.expiresAt} />
                  </>
                )}
                {` · ${text.viewers(link.maxViewers)} · ${text.plays(link.plays)}`}
              </span>
            }
            trailing={
              <span className="flex items-center gap-3">
                <StatusPill tone={stateTones[link.state]}>{text.states[link.state]}</StatusPill>
                {link.state === 'active' && (
                  <Button
                    size="sm"
                    variant="danger"
                    icon={ProhibitIcon}
                    aria-label={text.revokeLabel(link.item.name)}
                    loading={revocation.isPending && revocation.variables.id === link.id}
                    onClick={() => {
                      revocation.reset()
                      setAsked(link)
                    }}
                  >
                    {text.revoke}
                  </Button>
                )}
              </span>
            }
          />
        ))}
      </RowList>
      <ConfirmDialog
        open={asked !== null}
        onClose={() => setAsked(null)}
        onConfirm={() => asked && revocation.mutate(asked)}
        title={text.revokeTitle}
        confirmLabel={revocation.isPending ? text.revoking : text.revoke}
        busy={revocation.isPending}
        error={revocation.isError ? errorMessage(t, revocation.error) : undefined}
      >
        {text.revokeConfirm}
      </ConfirmDialog>
    </>
  )
}

/**
 * The form making a share link, in a floating panel: the title, found in the user's libraries
 * unless given, when the link expires (1 day by default) and how many may watch at once (1 by
 * default). Once made, it shows the link, which the server gives this one time only.
 */
function ShareTitleModal({
  initial,
  onClose,
}: {
  initial: AccountTitle | null
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.shareLinks
  const formId = useId()
  const [title, setTitle] = useState(initial)
  const [hours, setHours] = useState<ShareLinkHours>(24)
  const [viewers, setViewers] = useState(1)
  const mutation = useMutation({
    mutationFn: createShareLink,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.ownShareLinks }),
  })

  if (mutation.isSuccess) return <NewLink link={mutation.data} onClose={onClose} />

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (title === null) return
    mutation.mutate({ itemId: title.id, expiresInHours: hours, maxViewers: viewers })
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={text.share}
      width={560}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            icon={ShareNetworkIcon}
            disabled={title === null}
            loading={mutation.isPending}
          >
            {mutation.isPending ? text.creating : text.submit}
          </Button>
        </>
      }
    >
      <div className="space-y-5 p-5">
        {title === null ? (
          <TitlePicker onPick={setTitle} />
        ) : (
          <div className="flex flex-col gap-2">
            <p className="text-control font-medium text-ink">{text.chosen}</p>
            <div className="flex items-center gap-3 rounded-field border border-line-2 bg-s1 p-2">
              <TitlePoster title={title} />
              <span className="min-w-0 flex-1 truncate text-control text-ink">
                {titleName(title)}
                {title.kind === 'movie' && title.year > 0 && (
                  <span className="text-ink-3"> ({title.year})</span>
                )}
              </span>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  mutation.reset()
                  setTitle(null)
                }}
              >
                {text.change}
              </Button>
            </div>
          </div>
        )}
        <form id={formId} onSubmit={submit} noValidate className="space-y-5">
          <Field label={text.expires} help={text.expiresHelp}>
            <Select
              value={hours}
              onValue={setHours}
              options={shareLinkHours.map((value) => ({ value, label: text.after(value) }))}
            />
          </Field>
          <Field label={text.viewersLabel} help={text.viewersHelp}>
            <Select
              value={viewers}
              onValue={setViewers}
              options={Array.from({ length: maxShareViewers }, (_, index) => ({
                value: index + 1,
                label: String(index + 1),
              }))}
            />
          </Field>
          {mutation.isError && <Notice tone="danger">{errorMessage(t, mutation.error)}</Notice>}
        </form>
      </div>
    </Modal>
  )
}

/**
 * The search of a title to share in the user's libraries: movies and series, searched on the
 * button or Enter. A series lists its episodes, one of which is shared.
 */
function TitlePicker({ onPick }: { onPick: (title: AccountTitle) => void }) {
  const { t } = useI18n()
  const text = t.shareLinks
  const kinds = t.account.watching.kinds
  const [term, setTerm] = useState('')
  const [asked, setAsked] = useState<string | null>(null)
  const [series, setSeries] = useState<AccountTitle | null>(null)
  const search = useQuery({
    queryKey: queryKeys.shareTitles(asked ?? ''),
    queryFn: ({ signal }) => searchShareTitles(asked ?? '', signal),
    enabled: asked !== null,
  })
  const episodes = useQuery({
    queryKey: queryKeys.shareEpisodes(series?.id ?? ''),
    queryFn: ({ signal }) => fetchShareEpisodes(series?.id ?? '', signal),
    enabled: series !== null,
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = term.trim()
    if (next === '') return
    if (next === asked) void search.refetch()
    else setAsked(next)
  }

  if (series !== null) {
    return (
      <section aria-label={text.episodesOf(series.name)} className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-3">
          <p className="truncate text-control font-medium text-ink">
            {text.episodesOf(series.name)}
          </p>
          <Button size="sm" variant="ghost" icon={ArrowLeftIcon} onClick={() => setSeries(null)}>
            {text.back}
          </Button>
        </div>
        {episodes.isPending ? (
          <Skeleton className="h-24 rounded-field" />
        ) : episodes.isError ? (
          <InlineError onRetry={() => void episodes.refetch()} retrying={episodes.isFetching}>
            {errorMessage(t, episodes.error)}
          </InlineError>
        ) : episodes.data.length === 0 ? (
          <p className="text-small text-ink-3">{text.noEpisodes}</p>
        ) : (
          <ul className="flex max-h-72 flex-col gap-0.5 overflow-y-auto rounded-field border border-line bg-s1 p-1">
            {episodes.data.map((episode) => (
              <li key={episode.id}>
                <button
                  type="button"
                  onClick={() => onPick(episode)}
                  className={cx(
                    'flex w-full cursor-pointer items-baseline gap-3 rounded-field px-2.5 py-2 text-left',
                    'transition-colors duration-160 hover:bg-s3 focus-visible:bg-s3',
                  )}
                >
                  <span className="figures shrink-0 text-small text-ink-3">
                    {formatEpisode(episode.season, episode.episode)}
                  </span>
                  <span className="min-w-0 truncate text-control text-ink">{episode.name}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <form onSubmit={submit} noValidate className="flex items-start gap-2">
        <Field label={text.search} help={text.searchHelp} className="flex-1">
          <TextInput
            value={term}
            onValue={setTerm}
            icon={MagnifyingGlassIcon}
            autoComplete="off"
            spellCheck={false}
            autoFocus
          />
        </Field>
        <Button
          type="submit"
          className="mt-[26px]"
          loading={search.isFetching}
          disabled={term.trim() === ''}
        >
          {text.searchButton}
        </Button>
      </form>
      <div aria-live="polite" className="empty:hidden">
        {asked === null ? null : search.isPending ? (
          <div className="flex flex-col gap-2" aria-label={text.searching}>
            <Skeleton className="h-12 rounded-field" />
            <Skeleton className="h-12 rounded-field" />
          </div>
        ) : search.isError ? (
          <InlineError onRetry={() => void search.refetch()} retrying={search.isRefetching}>
            {errorMessage(t, search.error)}
          </InlineError>
        ) : search.data.length === 0 ? (
          <p className="text-small text-ink-3">{text.noTitles}</p>
        ) : (
          <ul
            className="flex max-h-72 flex-col gap-1 overflow-y-auto"
            aria-label={text.titlesFound(asked)}
          >
            {search.data.map((found) => {
              const details = [kinds[found.kind] ?? found.kind, found.year > 0 ? found.year : '']
                .filter(Boolean)
                .join(' · ')
              return (
                <li key={found.id}>
                  <button
                    type="button"
                    onClick={() => (found.kind === 'series' ? setSeries(found) : onPick(found))}
                    aria-label={text.pickTitle(found.name, details)}
                    className={cx(
                      'flex w-full cursor-pointer items-center gap-3 rounded-field p-1.5 text-left',
                      'transition-colors duration-160 hover:bg-s3 focus-visible:bg-s3',
                    )}
                  >
                    <TitlePoster title={found} />
                    <span className="min-w-0">
                      <span className="block truncate text-control text-ink">{found.name}</span>
                      <span className="block text-small text-ink-3">{details}</span>
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </div>
  )
}

/** The address of a link just made, to copy: the server gives it this one time only. */
function NewLink({ link, onClose }: { link: NewShareLink; onClose: () => void }) {
  const { t } = useI18n()
  const text = t.shareLinks
  const [copy, setCopy] = useState<'idle' | 'copied' | 'failed'>('idle')
  // The guest's page is at the server's root, beside the admin app.
  const address = link.url ?? `${window.location.origin}/share/${link.token}`

  async function copyLink() {
    try {
      // The clipboard is missing outside secure contexts, such as plain http on a local network.
      await navigator.clipboard.writeText(address)
      setCopy('copied')
    } catch {
      setCopy('failed')
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={text.createdTitle}
      width={560}
      footer={
        <>
          <Button onClick={onClose}>{text.done}</Button>
          <Button
            variant="primary"
            icon={copy === 'copied' ? CheckIcon : CopyIcon}
            onClick={() => void copyLink()}
          >
            {text.copy}
          </Button>
        </>
      }
    >
      <div className="space-y-4 p-5">
        <Notice tone="warn" live>
          {text.createdNotice(link.item.name)}
        </Notice>
        <code className="block rounded-field border border-line-2 bg-bg px-3.5 py-2.5 font-mono text-control break-all text-ink select-all">
          {address}
        </code>
        <div aria-live="polite">
          {copy === 'copied' && <Notice tone="ok">{text.copied}</Notice>}
          {copy === 'failed' && <Notice tone="danger">{text.copyFailed}</Notice>}
        </div>
      </div>
    </Modal>
  )
}
