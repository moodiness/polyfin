import {
  FilmSlateIcon,
  FolderIcon,
  GlobeIcon,
  HardDrivesIcon,
  LinkBreakIcon,
  MagnifyingGlassIcon,
  MusicNotesIcon,
  TelevisionSimpleIcon,
  VideoCameraIcon,
} from '@phosphor-icons/react'
import { Fragment, useId, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  addLocalFolder,
  fetchAddons,
  fetchUnmatchedFiles,
  linkLocalFile,
  queryClient,
  queryKeys,
  setLocalEpisodes,
  unlinkLocalFile,
  updateLocalFolder,
  type Addon,
  type DiscKind,
  type FolderKind,
  type FolderShare,
  type LocalFolder,
  type MatchMethod,
  type UnmatchedFile,
} from '@/api'
import { errorMessage, formatBytes } from '@/format'
import { useI18n, type Messages } from '@/i18n'
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
  SecretField,
  Segmented,
  Skeleton,
  Switch,
  TextInput,
  useToast,
} from '@/ui'
import { invalidateScope, replaceCachedAddon } from './model'
import TitleSearch from './TitleSearch'

/** The ways files are matched, in the order a folder's figures tell them. */
const matchMethods: MatchMethod[] = ['name', 'nfo', 'search', 'link']

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

/** Where an address leads: an SMB share, a WebDAV folder, or else a path in the container. */
export function shareOf(path: string): FolderShare {
  const address = path.trim().toLowerCase()
  if (address.startsWith('smb://')) return 'smb'
  if (address.startsWith('http://') || address.startsWith('https://')) return 'webdav'
  return ''
}

type Location = 'local' | 'smb' | 'webdav'

/** The form adding a local folder or a network share to the server's sources. */
export function LocalFolderAddForm({ onAdded }: { onAdded: (added: Addon) => void }) {
  const { t } = useI18n()
  const text = t.localFolders
  const [location, setLocation] = useState<Location>('local')
  const [name, setName] = useState('')
  const [path, setPath] = useState('')
  const [user, setUser] = useState('')
  const [password, setPassword] = useState('')
  const [kind, setKind] = useState<FolderKind>('movies')
  const [useArtwork, setUseArtwork] = useState(true)
  const share = location === 'local' ? null : location
  const mutation = useMutation({
    mutationFn: () =>
      addLocalFolder({
        name: name.trim(),
        path: path.trim(),
        kind,
        ...(share === null ? {} : { user: user.trim(), password }),
        ...(kind === 'music' || kind === 'homevideos' ? {} : { useArtwork }),
      }),
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
      <Field label={text.location} help={share === null ? text.help : text.shareHelp[share]}>
        <Segmented<Location>
          label={text.location}
          value={location}
          onChange={(next) => {
            mutation.reset()
            setLocation(next)
            setPath('')
          }}
          options={[
            { value: 'local', label: text.locations.local, icon: FolderIcon },
            { value: 'smb', label: text.locations.smb, icon: HardDrivesIcon },
            { value: 'webdav', label: text.locations.webdav, icon: GlobeIcon },
          ]}
          className="self-start max-sm:self-stretch"
        />
      </Field>
      <Field label={text.name} help={text.nameHint}>
        <TextInput value={name} onValue={setName} maxLength={64} required autoFocus />
      </Field>
      <Field
        label={share === null ? text.path : text.address}
        help={share === null ? text.pathHint : text.addressHint[share]}
      >
        <TextInput
          key={location}
          type={share === null ? 'text' : 'url'}
          inputMode={share === null ? undefined : 'url'}
          value={path}
          onValue={setPath}
          placeholder={share === null ? text.pathPlaceholder : text.addressPlaceholder[share]}
          autoComplete="off"
          spellCheck={false}
          required
          mono
        />
      </Field>
      {share !== null && (
        <>
          <Field label={text.user} help={text.userHint[share]}>
            <TextInput
              value={user}
              onValue={setUser}
              maxLength={256}
              autoComplete="off"
              spellCheck={false}
            />
          </Field>
          <Field label={text.password} help={text.passwordHint}>
            <TextInput
              type="password"
              revealable
              value={password}
              onValue={setPassword}
              maxLength={1024}
              autoComplete="new-password"
            />
          </Field>
        </>
      )}
      <Field label={text.kind} help={text.kindHelp[kind]}>
        <Segmented<FolderKind>
          label={text.kind}
          value={kind}
          onChange={setKind}
          options={[
            { value: 'movies', label: text.kinds.movies, icon: FilmSlateIcon },
            { value: 'shows', label: text.kinds.shows, icon: TelevisionSimpleIcon },
            { value: 'music', label: text.kinds.music, icon: MusicNotesIcon },
            { value: 'homevideos', label: text.kinds.homevideos, icon: VideoCameraIcon },
          ]}
          className="self-start"
        />
      </Field>
      {kind !== 'music' && kind !== 'homevideos' && (
        <Field label={text.useArtwork} help={text.useArtworkHelp}>
          <Switch checked={useArtwork} onChange={setUseArtwork} stateText />
        </Field>
      )}
      {mutation.isError && <FieldError>{errorMessage(t, mutation.error)}</FieldError>}
      <div>
        <Button type="submit" variant="primary" loading={mutation.isPending}>
          {text.add}
        </Button>
      </div>
    </form>
  )
}

/**
 * The form changing a folder's name, path or address, a share's user and password, and whether a
 * movies or shows folder's titles show its artwork; what it holds stays.
 */
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
  const [user, setUser] = useState(folder.user)
  // undefined keeps the stored password, '' removes it.
  const [password, setPassword] = useState<string | undefined>(undefined)
  const [useArtwork, setUseArtwork] = useState(folder.useArtwork)
  const share = shareOf(path)
  const mutation = useMutation({
    mutationFn: () =>
      updateLocalFolder(addon.id, {
        name: name.trim(),
        path: path.trim(),
        ...(share === '' ? {} : { user: user.trim(), password }),
        ...(folder.kind === 'music' || folder.kind === 'homevideos' ? {} : { useArtwork }),
      }),
    onSuccess: onSaved,
    onSettled: () => invalidateScope('shared'),
  })
  const moved = path.trim() !== folder.path

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate()
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <Field label={text.name} help={text.nameHint}>
        <TextInput value={name} onValue={setName} maxLength={64} required />
      </Field>
      <Field
        label={share === '' ? text.path : text.address}
        help={
          share === ''
            ? moved
              ? text.movedHint
              : text.pathHint
            : moved
              ? text.movedShareHint
              : text.addressHint[share]
        }
      >
        <TextInput
          value={path}
          onValue={setPath}
          autoComplete="off"
          spellCheck={false}
          required
          mono
        />
      </Field>
      {share !== '' && (
        <>
          <Field label={text.user} help={text.userHint[share]}>
            <TextInput
              value={user}
              onValue={setUser}
              maxLength={256}
              autoComplete="off"
              spellCheck={false}
            />
          </Field>
          <SecretField
            label={text.password}
            help={text.passwordHint}
            saved={folder.passwordSet}
            value={password}
            onValue={setPassword}
          />
        </>
      )}
      {folder.kind !== 'music' && folder.kind !== 'homevideos' && (
        <Field label={text.useArtwork} help={text.useArtworkHelp}>
          <Switch checked={useArtwork} onChange={setUseArtwork} stateText />
        </Field>
      )}
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
 * A folder's figures: its files, those matched, how, and the others, its last scan, and why it
 * cannot be read when it cannot. A music folder's are its tracks, albums and artists, and its
 * tracks left out; a home videos folder's its videos, by where their dates come from.
 */
export function LocalFolderFigures({ folder }: { folder: LocalFolder }) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const figures =
    folder.kind === 'music'
      ? [
          { label: text.tracks, value: folder.files, warn: false },
          { label: text.albums, value: folder.albums, warn: false },
          { label: text.artists, value: folder.artists, warn: false },
          { label: text.leftOut, value: folder.unmatched, warn: folder.unmatched > 0 },
        ]
      : folder.kind === 'homevideos'
        ? [
            { label: text.videos, value: folder.files, warn: false },
            { label: text.datedBy.nfo, value: folder.datedBy.nfo, warn: false },
            { label: text.datedBy.created, value: folder.datedBy.created, warn: false },
            { label: text.datedBy.file, value: folder.datedBy.file, warn: false },
          ]
        : [
            { label: text.files, value: folder.files, warn: false },
            { label: text.matched, value: folder.matched, warn: false },
            { label: text.unmatched, value: folder.unmatched, warn: folder.unmatched > 0 },
          ]
  const methods = matchMethods
    .filter((method) => folder.matchedBy[method] > 0)
    .map((method) => text.matchMethods[method](folder.matchedBy[method].toLocaleString(language)))
  const errors = folder.share === '' ? text.errors : text.shareErrors
  return (
    <PanelSection>
      <dl className={cx('grid gap-y-4', figures.length === 4 ? 'grid-cols-4' : 'grid-cols-3')}>
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
      {methods.length > 0 && (
        <p className="mt-4 text-small text-ink-3">{text.matchedBy(methods)}</p>
      )}
      {folder.error !== '' && (
        <Notice tone="danger" className="mt-5">
          <strong className="font-medium">
            {folder.share === '' ? text.errorTitle : text.errorTitleShare}.
          </strong>{' '}
          {Object.hasOwn(errors, folder.error) ? errors[folder.error] : t.errors.generic}{' '}
          {folder.files > 0 && text.keptNote}
        </Notice>
      )}
    </PanelSection>
  )
}

/**
 * The files of a folder no title was matched to, each linkable to an IMDb identifier, and the links
 * made; a music folder's tracks left out of its albums, which no link links.
 */
export function UnmatchedFiles({ addon, folder }: { addon: Addon; folder: LocalFolder }) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const toast = useToast()
  const unmatched = useQuery({
    // The scan's progress changes the list: the figures key it. The list shown stays until the
    // next one comes, so that its rows, and an identifier being typed in one, stay as the scan
    // goes; never another folder's, as the page can move to another source without being drawn
    // anew.
    queryKey: [...queryKeys.unmatched(addon.id), folder.files, folder.unmatched, folder.scanning],
    queryFn: ({ signal }) => fetchUnmatchedFiles(addon.id, signal),
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === addon.id ? previous : undefined,
  })
  const unlink = useMutation({
    mutationFn: (unit: string) => unlinkLocalFile(addon.id, unit),
    onSuccess: (updated, unit) => {
      replaceCachedAddon('shared', updated)
      toast(text.unlinked(unitName(unit, folder, text.discTitle)))
    },
    onSettled: () => {
      invalidateScope('shared')
      void queryClient.invalidateQueries({ queryKey: queryKeys.unmatched(addon.id) })
    },
  })
  const removeEpisode = useMutation({
    mutationFn: ({ path }: { path: string; name: string }) =>
      setLocalEpisodes(addon.id, [{ path, episode: '' }]),
    onSuccess: (updated, { name }) => {
      replaceCachedAddon('shared', updated)
      toast(text.episodeRemoved(name))
    },
    onSettled: () => {
      invalidateScope('shared')
      void queryClient.invalidateQueries({ queryKey: queryKeys.unmatched(addon.id) })
    },
  })

  return (
    <>
      <PanelSection
        title={folder.kind === 'music' ? text.leftOutTitle : text.unmatchedTitle}
        description={folder.kind === 'music' ? text.leftOutHelp : text.unmatchedHelp}
      >
        {unmatched.isPending ? (
          <Skeleton className="h-24 rounded-panel" />
        ) : unmatched.isError ? (
          <InlineError onRetry={() => void unmatched.refetch()} retrying={unmatched.isRefetching}>
            {errorMessage(t, unmatched.error)}
          </InlineError>
        ) : unmatched.data.files.length === 0 ? (
          <p className="text-control text-ink-3">
            {folder.kind === 'music' ? text.noLeftOut : text.noUnmatched}
          </p>
        ) : (
          <>
            <ul className="flex flex-col divide-y divide-line">
              {unmatched.data.files.map((file, i, files) => {
                const disc = file.discTitle?.discPath
                return (
                  <Fragment key={file.path}>
                    {disc !== undefined && files[i - 1]?.discTitle?.discPath !== disc && (
                      <DiscSuggestion
                        addon={addon}
                        folder={folder}
                        titles={files.filter((other) => other.discTitle?.discPath === disc)}
                      />
                    )}
                    <UnmatchedRow addon={addon} folder={folder} file={file} />
                  </Fragment>
                )
              })}
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
      {(folder.links.length > 0 || folder.episodes.length > 0) && (
        <PanelSection title={text.linksTitle}>
          <ul className="flex flex-col divide-y divide-line">
            {folder.links.map((link) => (
              <li key={link.unit} className="flex items-center justify-between gap-4 py-2">
                <span className="min-w-0">
                  <span className="block font-mono text-small break-all text-ink-2">
                    {unitName(link.unit, folder, text.discTitle)}
                  </span>
                  <Badge tone="accent">{link.imdbId}</Badge>
                </span>
                <IconButton
                  label={text.unlinkLabel(unitName(link.unit, folder, text.discTitle))}
                  icon={LinkBreakIcon}
                  size="sm"
                  disabled={unlink.isPending}
                  onClick={() => unlink.mutate(link.unit)}
                />
              </li>
            ))}
            {folder.episodes.map((given) => {
              const name = titleName(given.disc, given.discPath, given.number, folder, text)
              return (
                <li key={given.path} className="flex items-center justify-between gap-4 py-2">
                  <span className="min-w-0">
                    <span className="block font-mono text-small break-all text-ink-2">{name}</span>
                    <Badge tone="accent">{given.episode}</Badge>
                  </span>
                  <IconButton
                    label={text.removeEpisodeLabel(name)}
                    icon={LinkBreakIcon}
                    size="sm"
                    disabled={removeEpisode.isPending}
                    onClick={() => removeEpisode.mutate({ path: given.path, name })}
                  />
                </li>
              )
            })}
          </ul>
          {unlink.isError && <FieldError>{errorMessage(t, unlink.error)}</FieldError>}
          {removeEpisode.isError && <FieldError>{errorMessage(t, removeEpisode.error)}</FieldError>}
        </PanelSection>
      )}
    </>
  )
}

/**
 * One unmatched file: its path, what its name was read as and why it is unmatched, its link form,
 * and the search of its title. A title of a show's disc has its episode form instead, and the link
 * form too while its show is unmatched.
 */
function UnmatchedRow({
  addon,
  folder,
  file,
}: {
  addon: Addon
  folder: LocalFolder
  file: UnmatchedFile
}) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const toast = useToast()
  const [imdbId, setImdbId] = useState('')
  const [searching, setSearching] = useState(false)
  const searchId = useId()
  const link = useMutation({
    mutationFn: (id: string) => linkLocalFile(addon.id, file.path, id),
    onSuccess: (updated, id) => {
      replaceCachedAddon('shared', updated)
      toast(text.linked(unitName(file.unit, folder, text.discTitle), id.toLowerCase()))
    },
    onSettled: () => {
      invalidateScope('shared')
      void queryClient.invalidateQueries({ queryKey: queryKeys.unmatched(addon.id) })
    },
  })
  const title = [file.title, file.year === null ? '' : `(${file.year})`].filter(Boolean).join(' ')

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    link.mutate(imdbId.trim())
  }

  return (
    <li className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3 py-3">
      <div className="min-w-0 flex-1 basis-[260px]">
        <p className="font-mono text-small break-all text-ink">{file.path}</p>
        <p className="mt-1 text-small text-ink-3">
          {[
            text.reasons[file.reason] ?? file.reason,
            title !== '' ? text.read(title) : '',
            file.discTitle
              ? titleName(file.disc, file.discTitle.discPath, file.discTitle.number, folder, text)
              : file.disc
                ? text.disc[file.disc](unitName(file.unit, folder, text.discTitle))
                : file.unit !== file.path
                  ? text.showFolder(file.unit)
                  : '',
            file.discTitle?.length ? text.minutes(Math.round(file.discTitle.length / 60)) : '',
            formatBytes(file.size, language),
          ]
            .filter(Boolean)
            .join(' · ')}
        </p>
        {link.isError && <FieldError>{errorMessage(t, link.error)}</FieldError>}
      </div>
      {file.discTitle && <EpisodeForm addon={addon} folder={folder} file={file} />}
      {folder.kind !== 'music' && (file.discTitle === null || file.reason !== 'disc_episode') && (
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
          <Button
            variant="ghost"
            size="sm"
            icon={MagnifyingGlassIcon}
            aria-expanded={searching}
            aria-controls={searching ? searchId : undefined}
            onClick={() => setSearching(!searching)}
          >
            {text.findTitle}
          </Button>
        </form>
      )}
      {folder.kind !== 'music' && searching && (
        <div id={searchId} className="basis-full">
          <TitleSearch
            folderId={addon.id}
            path={file.path}
            title={file.title}
            year={file.year}
            linking={link.isPending}
            onPick={(id) => {
              link.reset()
              link.mutate(id)
            }}
          />
        </div>
      )}
    </li>
  )
}

/**
 * The episode form of a title of a show's disc: its episodes, typed as a file's name gives them,
 * the suggested ones shown as its placeholder.
 */
function EpisodeForm({
  addon,
  folder,
  file,
}: {
  addon: Addon
  folder: LocalFolder
  file: UnmatchedFile
}) {
  const { t } = useI18n()
  const text = t.localFolders
  const toast = useToast()
  const [episode, setEpisode] = useState(file.discTitle?.episode ?? '')
  const name = file.discTitle
    ? titleName(file.disc, file.discTitle.discPath, file.discTitle.number, folder, text)
    : file.path
  const give = useMutation({
    mutationFn: (value: string) =>
      setLocalEpisodes(addon.id, [{ path: file.path, episode: value }]),
    onSuccess: (updated) => {
      replaceCachedAddon('shared', updated)
      toast(text.episodesGiven(1))
    },
    onSettled: () => {
      invalidateScope('shared')
      void queryClient.invalidateQueries({ queryKey: queryKeys.unmatched(addon.id) })
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    give.mutate(episode.trim())
  }

  return (
    <div className="flex flex-col items-end gap-1">
      <form onSubmit={submit} noValidate className="flex items-end gap-2">
        <Field label={text.episodeLabel(name)} hideLabel>
          <TextInput
            value={episode}
            onValue={(value) => {
              give.reset()
              setEpisode(value)
            }}
            placeholder={file.discTitle?.suggested ?? text.episodePlaceholder}
            autoComplete="off"
            spellCheck={false}
            size="sm"
            mono
            className="w-[150px]"
          />
        </Field>
        <Button type="submit" size="sm" loading={give.isPending} disabled={episode.trim() === ''}>
          {text.giveEpisode}
        </Button>
      </form>
      {give.isError && <FieldError>{errorMessage(t, give.error)}</FieldError>}
    </div>
  )
}

/**
 * The episodes Polyfin suggests for a show's disc, whose unmatched titles are titles, in order, and
 * the button that gives them all at once; nothing when none is suggested.
 */
function DiscSuggestion({
  addon,
  folder,
  titles,
}: {
  addon: Addon
  folder: LocalFolder
  titles: UnmatchedFile[]
}) {
  const { t } = useI18n()
  const text = t.localFolders
  const toast = useToast()
  const suggested = titles.flatMap((file) =>
    file.discTitle?.suggested ? [{ path: file.path, episode: file.discTitle.suggested }] : [],
  )
  const confirm = useMutation({
    mutationFn: () => setLocalEpisodes(addon.id, suggested),
    onSuccess: (updated) => {
      replaceCachedAddon('shared', updated)
      toast(text.episodesGiven(suggested.length))
    },
    onSettled: () => {
      invalidateScope('shared')
      void queryClient.invalidateQueries({ queryKey: queryKeys.unmatched(addon.id) })
    },
  })
  const first = titles[0]
  if (suggested.length === 0 || !first?.discTitle) return null
  const disc = first.disc
    ? text.disc[first.disc](unitName(first.discTitle.discPath, folder, text.discTitle))
    : first.discTitle.discPath
  return (
    <li className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 py-3">
      <div className="min-w-0 flex-1 basis-[260px]">
        <p className="text-small text-ink-2">
          {text.suggestion(disc, suggested[0].episode, suggested[suggested.length - 1].episode)}
        </p>
        {confirm.isError && <FieldError>{errorMessage(t, confirm.error)}</FieldError>}
      </div>
      <Button
        size="sm"
        variant="primary"
        loading={confirm.isPending}
        onClick={() => confirm.mutate()}
      >
        {text.confirmSuggestion}
      </Button>
    </li>
  )
}

/**
 * A title of a show's disc as the admin app names it: its disc, of kind, and its number, such as
 * `DVD: Show/Season 1/Disc 2 · title 3`.
 */
function titleName(
  kind: DiscKind | '' | null,
  discPath: string,
  number: number,
  folder: LocalFolder,
  text: Messages['localFolders'],
): string {
  const titled = text.discTitle(unitName(discPath, folder, text.discTitle), number)
  return kind ? text.disc[kind](titled) : titled
}

/**
 * A unit as the admin app names it. "." is a disc at the folder's root: it bears the
 * folder's own name, the last element of its path or share address, as the scan reads its title.
 * A Blu-ray disc's title other than its main one is the disc's unit, # and its playlist's number,
 * named through titled.
 */
function unitName(
  unit: string,
  folder: LocalFolder,
  titled: (disc: string, title: number) => string,
): string {
  const title = /^(.*)#(\d{5})$/.exec(unit)
  if (title) return titled(unitName(title[1], folder, titled), Number(title[2]))
  if (unit !== '.') return unit
  const last =
    folder.path
      .replace(/[\\/]+$/, '')
      .split(/[\\/]/)
      .pop() ?? ''
  if (shareOf(folder.path) === '') return last || unit
  try {
    return decodeURIComponent(last) || unit
  } catch {
    return last || unit
  }
}
