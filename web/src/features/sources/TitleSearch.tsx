import { FilmSlateIcon, MagnifyingGlassIcon, TelevisionSimpleIcon } from '@phosphor-icons/react'
import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { queryKeys, searchFolderTitles, type FolderTitle } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, cx, Field, InlineError, Skeleton, TextInput } from '@/ui'

/**
 * The search of an unmatched file's title in the metadata addons: what its name was read as, and
 * its year, which the administrator can change. A search starts on the button or Enter, never as
 * they type; picking a title links the file with it, as typing its identifier does.
 */
export default function TitleSearch({
  folderId,
  path,
  title,
  year,
  linking,
  onPick,
}: {
  folderId: string
  /** The file's path, which names the fields for screen readers. */
  path: string
  title: string
  year: number | null
  /** Whether a link is being made, during which no other title can be picked. */
  linking: boolean
  onPick: (imdbId: string) => void
}) {
  const { t } = useI18n()
  const text = t.localFolders
  const [query, setQuery] = useState(title)
  const [yearText, setYearText] = useState(year === null ? '' : String(year))
  const [asked, setAsked] = useState<{ query: string; year: number | null } | null>(null)
  const search = useQuery({
    queryKey: queryKeys.folderTitles(folderId, asked?.query ?? '', asked?.year ?? null),
    queryFn: ({ signal }) =>
      searchFolderTitles(folderId, asked?.query ?? '', asked?.year ?? null, signal),
    enabled: asked !== null,
    staleTime: 5 * 60_000,
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = { query: query.trim(), year: yearText === '' ? null : Number(yearText) }
    if (next.query === '') return
    // The same search again asks the addons again.
    if (asked?.query === next.query && asked.year === next.year) void search.refetch()
    else setAsked(next)
  }

  return (
    <section
      aria-label={text.searchLabel(path)}
      className="mt-3 rounded-field border border-line bg-s1 p-3"
    >
      <form onSubmit={submit} noValidate className="flex flex-wrap items-end gap-2">
        <Field label={text.searchQuery} className="min-w-[180px] flex-1">
          <TextInput
            value={query}
            onValue={setQuery}
            autoComplete="off"
            spellCheck={false}
            size="sm"
          />
        </Field>
        <Field label={text.searchYear}>
          <TextInput
            value={yearText}
            onValue={(value) => setYearText(value.replace(/\D/g, '').slice(0, 4))}
            inputMode="numeric"
            autoComplete="off"
            size="sm"
            className="w-[80px]"
          />
        </Field>
        <Button
          type="submit"
          size="sm"
          icon={MagnifyingGlassIcon}
          loading={search.isFetching}
          disabled={query.trim() === ''}
        >
          {text.search}
        </Button>
      </form>
      <div aria-live="polite" className="mt-3 empty:hidden">
        {asked === null ? null : search.isPending ? (
          <div className="flex flex-col gap-2" aria-label={text.searching}>
            <Skeleton className="h-12 rounded-field" />
            <Skeleton className="h-12 rounded-field" />
          </div>
        ) : search.isError ? (
          <InlineError onRetry={() => void search.refetch()} retrying={search.isRefetching}>
            {errorMessage(t, search.error)}
          </InlineError>
        ) : search.data.titles.length === 0 ? (
          <p className="text-small text-ink-3">{text.noTitles}</p>
        ) : (
          <ul className="flex flex-col gap-1" aria-label={text.titlesFound(asked.query)}>
            {search.data.titles.map((found) => (
              <li key={found.imdbId}>
                <TitleChoice title={found} disabled={linking} onPick={() => onPick(found.imdbId)} />
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}

/** A title found: its poster, name, year and kind, as a button that links the file with it. */
function TitleChoice({
  title,
  disabled,
  onPick,
}: {
  title: FolderTitle
  disabled: boolean
  onPick: () => void
}) {
  const { t } = useI18n()
  const text = t.localFolders
  const [posterFailed, setPosterFailed] = useState(false)
  const KindIcon = title.kind === 'show' ? TelevisionSimpleIcon : FilmSlateIcon
  const details = [title.year === null ? '' : String(title.year), text.titleKinds[title.kind]]
    .filter(Boolean)
    .join(' · ')
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onPick}
      aria-label={text.pickTitle(title.name, details)}
      className={cx(
        'flex w-full cursor-pointer items-center gap-3 rounded-field p-1.5 text-left',
        'transition-colors duration-160 hover:bg-s3 focus-visible:bg-s3',
        'disabled:cursor-not-allowed disabled:opacity-45',
      )}
    >
      {title.posterId !== null && !posterFailed ? (
        <img
          src={`/Items/${encodeURIComponent(title.posterId)}/Images/Primary?maxHeight=96&quality=85`}
          alt=""
          loading="lazy"
          onError={() => setPosterFailed(true)}
          className="aspect-[2/3] w-8 shrink-0 rounded-check object-cover ring-1 ring-ink/8"
        />
      ) : (
        <span
          aria-hidden="true"
          className="grid aspect-[2/3] w-8 shrink-0 place-items-center rounded-check border border-line-2 bg-s2 text-ink-3"
        >
          <KindIcon size={14} />
        </span>
      )}
      <span className="min-w-0">
        <span className="block truncate text-control text-ink">{title.name}</span>
        <span className="block text-small text-ink-3">
          {details} · <span className="font-mono">{title.imdbId}</span>
        </span>
      </span>
    </button>
  )
}
