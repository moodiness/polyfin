import { FilmSlateIcon, LinkBreakIcon, TelevisionSimpleIcon } from '@phosphor-icons/react'
import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  addLocalFolder,
  fetchAddons,
  fetchUnmatchedFiles,
  linkLocalFile,
  queryClient,
  queryKeys,
  unlinkLocalFile,
  updateLocalFolder,
  type Addon,
  type FolderKind,
  type LocalFolder,
  type UnmatchedFile,
} from '@/api'
import { errorMessage, formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Button,
  cx,
  Field,
  FieldError,
  IconButton,
  InlineError,
  Notice,
  PanelSection,
  Segmented,
  Skeleton,
  TextInput,
  useToast,
} from '@/ui'
import { invalidateScope, replaceCachedAddon } from './model'

/**
 * Keeps the server's sources fresh while a folder's scan goes on, every 2 seconds, so that its
 * figures and unmatched files follow; the list they come from is shared with the page.
 */
export function useFolderScanFollowed(addon: Addon) {
  const scanning = addon.folder?.scanning ?? false
  useQuery({
    queryKey: queryKeys.addons('shared'),
    queryFn: ({ signal }) => fetchAddons('shared', signal),
    refetchInterval: scanning ? 2000 : false,
    enabled: scanning,
  })
}

/** The form adding a local folder to the server's sources. */
export function LocalFolderAddForm({ onAdded }: { onAdded: (added: Addon) => void }) {
  const { t } = useI18n()
  const text = t.localFolders
  const [name, setName] = useState('')
  const [path, setPath] = useState('')
  const [kind, setKind] = useState<FolderKind>('movies')
  const mutation = useMutation({
    mutationFn: () => addLocalFolder({ name: name.trim(), path: path.trim(), kind }),
    onSuccess: (added) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons('shared'), (old) =>
        old === undefined ? old : [...old, added],
      )
      onAdded(added)
    },
    onSettled: () => invalidateScope('shared'),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate()
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <p className="text-small text-ink-3">{text.help}</p>
      <Field label={text.name} help={text.nameHint}>
        <TextInput value={name} onValue={setName} maxLength={64} required autoFocus />
      </Field>
      <Field label={text.path} help={text.pathHint}>
        <TextInput
          value={path}
          onValue={setPath}
          placeholder={text.pathPlaceholder}
          autoComplete="off"
          spellCheck={false}
          required
          mono
        />
      </Field>
      <Field label={text.kind} help={text.kindHelp[kind]}>
        <Segmented<FolderKind>
          label={text.kind}
          value={kind}
          onChange={setKind}
          options={[
            { value: 'movies', label: text.kinds.movies, icon: FilmSlateIcon },
            { value: 'shows', label: text.kinds.shows, icon: TelevisionSimpleIcon },
          ]}
          className="self-start"
        />
      </Field>
      {mutation.isError && <FieldError>{errorMessage(t, mutation.error)}</FieldError>}
      <div>
        <Button type="submit" variant="primary" loading={mutation.isPending}>
          {text.add}
        </Button>
      </div>
    </form>
  )
}

/** The form changing a folder's name or path; what it holds stays. */
export function LocalFolderEditForm({
  addon,
  folder,
  onSaved,
  onCancel,
}: {
  addon: Addon
  folder: LocalFolder
  onSaved: (updated: Addon) => void
  onCancel: () => void
}) {
  const { t } = useI18n()
  const text = t.localFolders
  const [name, setName] = useState(addon.name)
  const [path, setPath] = useState(folder.path)
  const mutation = useMutation({
    mutationFn: () => updateLocalFolder(addon.id, { name: name.trim(), path: path.trim() }),
    onSuccess: onSaved,
    onSettled: () => invalidateScope('shared'),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate()
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <Field label={text.name} help={text.nameHint}>
        <TextInput value={name} onValue={setName} maxLength={64} required />
      </Field>
      <Field label={text.path} help={path.trim() !== folder.path ? text.movedHint : text.pathHint}>
        <TextInput
          value={path}
          onValue={setPath}
          autoComplete="off"
          spellCheck={false}
          required
          mono
        />
      </Field>
      <p className="text-small text-ink-3">
        {text.holds}: {text.kinds[folder.kind]}. {text.kindFixed}
      </p>
      {mutation.isError && <FieldError>{errorMessage(t, mutation.error)}</FieldError>}
      <div className="flex gap-2">
        <Button type="submit" variant="primary" loading={mutation.isPending}>
          {text.save}
        </Button>
        <Button onClick={onCancel}>{text.cancel}</Button>
      </div>
    </form>
  )
}

/**
 * A folder's figures: its files, those matched and the others, its last scan, and why it cannot be
 * read when it cannot.
 */
export function LocalFolderFigures({ folder }: { folder: LocalFolder }) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const figures = [
    { label: text.files, value: folder.files, warn: false },
    { label: text.matched, value: folder.matched, warn: false },
    { label: text.unmatched, value: folder.unmatched, warn: folder.unmatched > 0 },
  ]
  return (
    <PanelSection>
      <dl className="grid grid-cols-3 gap-y-4">
        {figures.map((figure, i) => (
          <div
            key={figure.label}
            className={cx('min-w-0 px-4 first:pl-0', i > 0 && 'border-l border-line')}
          >
            <dt className="text-small text-ink-3">{figure.label}</dt>
            <dd
              className={cx(
                'figures mt-1.5 text-[19px] font-medium',
                figure.warn ? 'text-warn' : 'text-ink',
              )}
            >
              {figure.value.toLocaleString(language)}
            </dd>
          </div>
        ))}
      </dl>
      {folder.error !== '' && (
        <Notice tone="danger" className="mt-5">
          <strong className="font-medium">{text.errorTitle}.</strong>{' '}
          {Object.hasOwn(text.errors, folder.error) ? text.errors[folder.error] : t.errors.generic}{' '}
          {folder.files > 0 && text.keptNote}
        </Notice>
      )}
    </PanelSection>
  )
}

/** The files of a folder no title was matched to, each linkable to an IMDb identifier, and the links made. */
export function UnmatchedFiles({ addon, folder }: { addon: Addon; folder: LocalFolder }) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const toast = useToast()
  const unmatched = useQuery({
    // The scan's progress changes the list: the figures key it.
    queryKey: [...queryKeys.unmatched(addon.id), folder.files, folder.unmatched, folder.scanning],
    queryFn: ({ signal }) => fetchUnmatchedFiles(addon.id, signal),
  })
  const unlink = useMutation({
    mutationFn: (unit: string) => unlinkLocalFile(addon.id, unit),
    onSuccess: (updated, unit) => {
      replaceCachedAddon('shared', updated)
      toast(text.unlinked(unit))
    },
    onSettled: () => {
      invalidateScope('shared')
      void queryClient.invalidateQueries({ queryKey: queryKeys.unmatched(addon.id) })
    },
  })

  return (
    <>
      <PanelSection title={text.unmatchedTitle} description={text.unmatchedHelp}>
        {unmatched.isPending ? (
          <Skeleton className="h-24 rounded-panel" />
        ) : unmatched.isError ? (
          <InlineError onRetry={() => void unmatched.refetch()} retrying={unmatched.isRefetching}>
            {errorMessage(t, unmatched.error)}
          </InlineError>
        ) : unmatched.data.files.length === 0 ? (
          <p className="text-control text-ink-3">{text.noUnmatched}</p>
        ) : (
          <>
            <ul className="flex flex-col divide-y divide-line">
              {unmatched.data.files.map((file) => (
                <UnmatchedRow key={file.path} addon={addon} file={file} />
              ))}
            </ul>
            {unmatched.data.total > unmatched.data.files.length && (
              <p className="mt-3 text-small text-ink-3">
                {text.shownOf(
                  unmatched.data.files.length.toLocaleString(language),
                  unmatched.data.total.toLocaleString(language),
                )}
              </p>
            )}
          </>
        )}
      </PanelSection>
      {folder.links.length > 0 && (
        <PanelSection title={text.linksTitle}>
          <ul className="flex flex-col divide-y divide-line">
            {folder.links.map((link) => (
              <li key={link.unit} className="flex items-center justify-between gap-4 py-2">
                <span className="min-w-0">
                  <span className="block font-mono text-small break-all text-ink-2">
                    {link.unit}
                  </span>
                  <Badge tone="accent">{link.imdbId}</Badge>
                </span>
                <IconButton
                  label={text.unlinkLabel(link.unit)}
                  icon={LinkBreakIcon}
                  size="sm"
                  disabled={unlink.isPending}
                  onClick={() => unlink.mutate(link.unit)}
                />
              </li>
            ))}
          </ul>
          {unlink.isError && <FieldError>{errorMessage(t, unlink.error)}</FieldError>}
        </PanelSection>
      )}
    </>
  )
}

/** One unmatched file: its path, what its name was read as and why it is unmatched, and its link form. */
function UnmatchedRow({ addon, file }: { addon: Addon; file: UnmatchedFile }) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const toast = useToast()
  const [imdbId, setImdbId] = useState('')
  const link = useMutation({
    mutationFn: () => linkLocalFile(addon.id, file.path, imdbId.trim()),
    onSuccess: (updated) => {
      replaceCachedAddon('shared', updated)
      toast(text.linked(file.unit, imdbId.trim().toLowerCase()))
    },
    onSettled: () => {
      invalidateScope('shared')
      void queryClient.invalidateQueries({ queryKey: queryKeys.unmatched(addon.id) })
    },
  })
  const title = [file.title, file.year === null ? '' : `(${file.year})`].filter(Boolean).join(' ')

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    link.mutate()
  }

  return (
    <li className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3 py-3">
      <div className="min-w-0 flex-1 basis-[260px]">
        <p className="font-mono text-small break-all text-ink">{file.path}</p>
        <p className="mt-1 text-small text-ink-3">
          {[
            text.reasons[file.reason] ?? file.reason,
            title !== '' ? text.read(title) : '',
            file.unit !== file.path ? text.showFolder(file.unit) : '',
            formatBytes(file.size, language),
          ]
            .filter(Boolean)
            .join(' · ')}
        </p>
        {link.isError && <FieldError>{errorMessage(t, link.error)}</FieldError>}
      </div>
      <form onSubmit={submit} noValidate className="flex items-end gap-2">
        <Field label={text.linkLabel(file.path)} hideLabel>
          <TextInput
            value={imdbId}
            onValue={(value) => {
              link.reset()
              setImdbId(value)
            }}
            placeholder={text.imdbPlaceholder}
            autoComplete="off"
            spellCheck={false}
            size="sm"
            mono
            className="w-[150px]"
          />
        </Field>
        <Button type="submit" size="sm" loading={link.isPending} disabled={imdbId.trim() === ''}>
          {text.link}
        </Button>
      </form>
    </li>
  )
}
