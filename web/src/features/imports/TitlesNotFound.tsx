import { LinkBreakIcon, LinkIcon } from '@phosphor-icons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import {
  ApiError,
  fetchImportTitleCounts,
  fetchImportTitles,
  linkImportTitle,
  queryClient,
  queryKeys,
  unlinkImportTitle,
  type ImportTitle,
  type ImportTitleLink,
  type ImportTitlesScope,
} from '@/api'
import { serviceNames } from '@/features/account/serviceNames'
import { errorMessage, formatEpisode } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Button,
  Field,
  FieldError,
  InlineError,
  Modal,
  SkeletonRows,
  TextInput,
  useToast,
} from '@/ui'

/** The episodes of a series named in its row; the others are counted. */
const shownPlaces = 6

/** The name of a source of titles not found: an app, a tracking service, an export or a server. */
export function sourceName(t: Messages, source: string): string {
  const named: Readonly<Record<string, string>> = {
    ...serviceNames,
    ...t.users.jellyfinImport.servers,
    ...t.account.appImport.apps,
    letterboxd: t.account.letterboxd.option,
  }
  return Object.hasOwn(named, source) ? named[source] : source
}

/** Why a title was not found, in the words of its source's import. */
function reasonLabel(t: Messages, source: string, reason: string): string {
  const reasons: Readonly<Record<string, string>> = Object.hasOwn(t.account.appImport.apps, source)
    ? t.account.appImport.reasons
    : source === 'letterboxd'
      ? t.account.letterboxd.reasons
      : Object.hasOwn(t.users.jellyfinImport.servers, source)
        ? t.users.jellyfinImport.reasons
        : t.importTitles.trackerReasons
  return Object.hasOwn(reasons, reason) ? reasons[reason] : reason
}

/** What the list says of its source's titles before them: its import's own words, when it has some. */
function sourceHelp(t: Messages, source: string): string {
  return Object.hasOwn(t.account.appImport.apps, source)
    ? t.account.appImport.unmatchedHelp
    : source === 'letterboxd'
      ? t.account.letterboxd.unmatchedHelp
      : t.users.jellyfinImport.unmatchedHelp
}

/** The title a link found: its name and year, else its identifier. */
function linkedName(link: ImportTitleLink): string {
  if (link.name === '') return link.imdb !== '' ? link.imdb : `tmdb:${link.tmdb}`
  return link.year === null ? link.name : `${link.name} (${link.year})`
}

/**
 * The button that opens the titles one source's imports did not find, for one account: named by
 * how many are not linked, else by how many are linked, and drawn only when there are some.
 * `stamp` follows the status of the import shown beside it, so that the counts are read again as
 * it changes.
 */
export function TitlesNotFound({
  scope,
  source,
  stamp,
}: {
  scope: ImportTitlesScope
  source: string
  stamp: string
}) {
  const { t } = useI18n()
  const text = t.importTitles
  const [open, setOpen] = useState(false)
  const counts = useQuery({
    queryKey: queryKeys.importTitleCounts(scope, stamp),
    queryFn: ({ signal }) => fetchImportTitleCounts(scope, signal),
    placeholderData: keepPreviousData,
  })
  const count = counts.data?.find((item) => item.source === source)
  if (count === undefined || count.notFound + count.linked === 0) return null
  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        icon={LinkIcon}
        className="self-start"
        onClick={() => setOpen(true)}
      >
        {count.notFound > 0 ? text.notFound(count.notFound) : text.linkedCount(count.linked)}
      </Button>
      <TitlesNotFoundDialog
        scope={scope}
        source={open ? source : null}
        onClose={() => setOpen(false)}
      />
    </>
  )
}

/**
 * The titles one source's imports did not find, for one account, in a dialog open while `source`
 * is set: each links to its IMDb or TMDB title, or has its link removed.
 */
export function TitlesNotFoundDialog({
  scope,
  source,
  onClose,
}: {
  scope: ImportTitlesScope
  source: string | null
  onClose: () => void
}) {
  const { t } = useI18n()
  return (
    <Modal
      open={source !== null}
      onClose={onClose}
      title={source === null ? '' : t.importTitles.title(sourceName(t, source))}
      width={704}
      footer={<Button onClick={onClose}>{t.users.close}</Button>}
    >
      {source !== null && <TitleList scope={scope} source={source} />}
    </Modal>
  )
}

/**
 * The list of a source's titles not found. The episodes of a series share its key, and its link:
 * they are one row.
 */
function TitleList({ scope, source }: { scope: ImportTitlesScope; source: string }) {
  const { t } = useI18n()
  const text = t.importTitles
  const titles = useQuery({
    queryKey: queryKeys.importTitleList(scope, source),
    queryFn: ({ signal }) => fetchImportTitles(scope, source, signal),
  })
  const rows = new Map<string, ImportTitle[]>()
  for (const title of titles.data ?? [])
    rows.set(title.key, [...(rows.get(title.key) ?? []), title])

  return (
    <div className="flex flex-col gap-4 p-5">
      <p className="text-small text-ink-3">
        {sourceHelp(t, source)} {text.help}
      </p>
      {titles.isPending ? (
        <SkeletonRows rows={3} boxed label={t.common.loading} />
      ) : titles.isError ? (
        <InlineError onRetry={() => void titles.refetch()} retrying={titles.isFetching}>
          {errorMessage(t, titles.error)}
        </InlineError>
      ) : rows.size === 0 ? (
        <p className="text-control text-ink-3">{text.none}</p>
      ) : (
        <ul className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
          {[...rows].map(([key, group]) => (
            <TitleRow key={key} scope={scope} source={source} titles={group} />
          ))}
        </ul>
      )}
    </div>
  )
}

/**
 * One title not found, with the episodes of it the import met: its name, else its identifiers,
 * its kind, its episodes and why it was not found; then its link form, or the title it is linked
 * to with Remove the link. Enter in the field links.
 */
function TitleRow({
  scope,
  source,
  titles,
}: {
  scope: ImportTitlesScope
  source: string
  titles: ImportTitle[]
}) {
  const { t } = useI18n()
  const text = t.importTitles
  const toast = useToast()
  const [target, setTarget] = useState('')
  const first = titles[0]
  const name = first.name === '' ? first.ids : first.name
  const heading = first.year === null ? name : `${name} (${first.year})`
  // Linking adds what the titles would import, and changes the counts: everything kept is read again.
  const refresh = () =>
    void queryClient.invalidateQueries({ queryKey: queryKeys.importTitles(scope) })
  const link = useMutation({
    mutationFn: (value: string) => linkImportTitle(scope, source, first.key, value),
    onSuccess: (linked) => {
      setTarget('')
      toast(text.linked(heading, linkedName(linked)))
    },
    onSettled: refresh,
  })
  const unlink = useMutation({
    mutationFn: () => unlinkImportTitle(scope, source, first.key),
    onSuccess: () => toast(text.unlinked(heading)),
    onSettled: refresh,
  })
  const failed = link.error ?? unlink.error
  const linkErrors: Readonly<Record<string, string>> = text.errors
  const message =
    failed instanceof ApiError && Object.hasOwn(linkErrors, failed.code)
      ? linkErrors[failed.code]
      : failed !== null
        ? errorMessage(t, failed)
        : undefined

  const places = titles
    .filter((title) => title.kind === 'episode')
    .sort((a, b) => a.season - b.season || a.episode - b.episode)
    .map((title) => formatEpisode(title.season, title.episode))
  const reasons = [...new Set(titles.map((title) => reasonLabel(t, source, title.reason)))]
  const details = [
    t.users.jellyfinImport.kinds[first.kind],
    places.length > shownPlaces
      ? `${places.slice(0, shownPlaces).join(', ')} ${text.morePlaces(places.length - shownPlaces)}`
      : places.join(', '),
    ...reasons,
  ].filter(Boolean)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    unlink.reset()
    link.mutate(target.trim())
  }

  return (
    <li className="flex flex-wrap items-start justify-between gap-x-5 gap-y-2 rounded-row bg-s1 py-2.5 pr-3 pl-4">
      <div className="min-w-0 flex-1 basis-[16.25rem]">
        <p className="hyphenate text-control font-medium text-ink">{heading}</p>
        {first.name !== '' && first.ids !== '' && (
          <p className="font-mono text-small break-all text-ink-3">{first.ids}</p>
        )}
        <p className="mt-1 text-small text-ink-3">{details.join(' · ')}</p>
        {first.link !== null && (
          <>
            <p className="mt-2 text-small text-ink-2">
              {text.linkedTo(linkedName(first.link), first.link.linkedBy?.name ?? null)}
            </p>
            <p className="text-small text-ink-3">{text.marksStay}</p>
          </>
        )}
        {message !== undefined && <FieldError>{message}</FieldError>}
      </div>
      {first.link === null ? (
        <form onSubmit={submit} noValidate className="flex w-full flex-wrap items-end gap-2">
          <Field
            label={text.fieldLabel(heading)}
            hideLabel
            help={text.fieldHint}
            helpFirst
            className="min-w-[13rem] flex-1"
          >
            <TextInput
              value={target}
              onValue={(value) => {
                link.reset()
                setTarget(value)
              }}
              placeholder={text.placeholder}
              autoComplete="off"
              spellCheck={false}
              size="sm"
              mono
              className="w-full"
            />
          </Field>
          <Button type="submit" size="sm" loading={link.isPending} disabled={target.trim() === ''}>
            {text.link}
          </Button>
        </form>
      ) : (
        <Button
          variant="ghost"
          size="sm"
          icon={LinkBreakIcon}
          loading={unlink.isPending}
          onClick={() => {
            link.reset()
            unlink.mutate()
          }}
        >
          {text.unlink}
        </Button>
      )}
    </li>
  )
}
