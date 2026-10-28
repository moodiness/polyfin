import {
  CloudIcon,
  FilmSlateIcon,
  FolderIcon,
  GlobeIcon,
  HardDrivesIcon,
  LinkBreakIcon,
  MagnifyingGlassIcon,
  MusicNotesIcon,
  PlugsConnectedIcon,
  TelevisionSimpleIcon,
  TerminalWindowIcon,
  VideoCameraIcon,
} from '@phosphor-icons/react'
import { Fragment, useId, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  addLocalFolder,
  fetchAddons,
  fetchLooserMatches,
  fetchRcloneRemotes,
  fetchStatus,
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
  type ServerSystem,
  type UnmatchedFile,
} from '@/api'
import { dateTime, errorMessage, formatBytes } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Badge,
  Button,
  Checkbox,
  ConfirmDialog,
  cx,
  Field,
  FieldError,
  IconButton,
  InlineError,
  Notice,
  NumberInput,
  PanelSection,
  SecretField,
  Segmented,
  Skeleton,
  SuggestInput,
  Switch,
  Textarea,
  TextInput,
  useToast,
} from '@/ui'
import { folderFailure, invalidateScope, replaceCachedAddon } from './model'
import TitleSearch from './TitleSearch'

/** The ways files are matched, in the order a folder's figures tell them. */
const matchMethods: MatchMethod[] = [
  'name',
  'nfo',
  'search',
  'year',
  'numerals',
  'yearNumerals',
  'link',
]

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

/**
 * Where an address leads: an SMB share, a WebDAV folder, an S3 bucket, an SFTP folder, an rclone
 * remote, or else a path in the container.
 */
export function shareOf(path: string): FolderShare {
  const address = path.trim().toLowerCase()
  if (address.startsWith('smb://')) return 'smb'
  if (address.startsWith('http://') || address.startsWith('https://')) return 'webdav'
  if (address.startsWith('s3://')) return 's3'
  if (address.startsWith('sftp://')) return 'sftp'
  if (address.startsWith('rclone://')) return 'rclone'
  return ''
}

/** Where the server runs, which names its local folders: in its container until it tells. */
export function useServerSystem(): ServerSystem {
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
  })
  return status.data?.system ?? 'container'
}

/** Which texts name a local folder: the container's, or a computer's. */
export const computer = (system: ServerSystem) =>
  system === 'container' ? 'container' : 'computer'

type Location = 'local' | 'smb' | 'webdav' | 's3' | 'sftp' | 'rclone'

/** The shares read through the source cache, which have fields of their own. */
type RemoteShare = 's3' | 'sftp' | 'rclone'

const isRemote = (share: FolderShare | null): share is RemoteShare =>
  share === 's3' || share === 'sftp' || share === 'rclone'

/** How often a kind of remote share is listed again when the form leaves it empty. */
const defaultPollMinutes: Record<RemoteShare, number> = { s3: 30, sftp: 2, rclone: 2 }

/**
 * An rclone folder's address, rclone://host:port/path, as its form shows it: the address of the
 * rclone instance, reached over http://, and the path in the remote.
 */
function rcloneParts(path: string): { instance: string; folder: string } {
  const rest = path.trim().replace(/^rclone:\/\//i, '')
  const slash = rest.indexOf('/')
  const host = slash < 0 ? rest : rest.slice(0, slash)
  return {
    instance: host === '' ? '' : `http://${host}`,
    folder: slash < 0 ? '' : rest.slice(slash),
  }
}

/** The address of an rclone folder, from the rclone instance's address and the path in the remote. */
function rcloneAddress(instance: string, folder: string): string {
  const host = instance
    .trim()
    .replace(/^[a-z][a-z0-9+.-]*:\/\//i, '')
    .replace(/\/.*$/, '')
  return `rclone://${host}/${folder.trim().replace(/^\/+/, '')}`
}

/**
 * Where a folder's stored secrets go, its password, private key and passphrase: its kind of share
 * with the scheme and server of its address, or of an S3 bucket's endpoint. The server keeps them
 * only while this stays the same, else those the form gives again.
 */
function secretsServer(share: FolderShare, address: string, endpoint: string): string {
  const target = (share === 's3' ? endpoint : address).trim()
  const server = /^([a-z][a-z0-9+.-]*):\/\/([^/?#]*)/i.exec(target)
  return `${share} ${server === null ? '' : `${server[1]}://${server[2]}`.toLowerCase()}`
}

/** What an S3 bucket, an SFTP folder or an rclone remote is read with beside its address. */
type RemoteDraft = {
  endpoint: string
  region: string
  /** A new private key; '' keeps the one saved, as removeKey does not ask to remove it. */
  privateKey: string
  removeKey: boolean
  /** undefined keeps the stored passphrase, '' removes it. */
  passphrase: string | undefined
  rcloneRemote: string
  rcloneInstance: string
  rcloneFolder: string
  /** null for the kind's default. */
  pollMinutes: number | null
}

function remoteDraft(folder?: LocalFolder): RemoteDraft {
  const { instance, folder: rcloneFolder } = rcloneParts(
    folder?.share === 'rclone' ? folder.path : '',
  )
  return {
    endpoint: folder?.endpoint ?? '',
    region: folder?.region ?? '',
    privateKey: '',
    removeKey: false,
    passphrase: undefined,
    rcloneRemote: folder?.rcloneRemote ?? '',
    rcloneInstance: instance,
    rcloneFolder,
    pollMinutes: folder?.pollMinutes ?? null,
  }
}

/** The fields of a remote share's request: those of its kind, those changed when editing one. */
function remoteFields(share: RemoteShare, draft: RemoteDraft, editing: boolean) {
  const poll = draft.pollMinutes === null ? {} : { pollMinutes: draft.pollMinutes }
  switch (share) {
    case 's3':
      return { endpoint: draft.endpoint.trim(), region: draft.region.trim(), ...poll }
    case 'rclone':
      return { rcloneRemote: draft.rcloneRemote.trim(), ...poll }
    case 'sftp':
      return {
        ...(draft.privateKey.trim() !== ''
          ? { privateKey: draft.privateKey }
          : editing && draft.removeKey
            ? { privateKey: '' }
            : {}),
        ...(draft.passphrase === undefined || (!editing && draft.passphrase === '')
          ? {}
          : { passphrase: draft.passphrase }),
        ...poll,
      }
  }
}

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
  const [readExtras, setReadExtras] = useState(true)
  const [draft, setDraft] = useState<RemoteDraft>(() => remoteDraft())
  const share = location === 'local' ? null : location
  const system = useServerSystem()
  const mutation = useMutation({
    mutationFn: () =>
      addLocalFolder({
        name: name.trim(),
        path:
          share === 'rclone'
            ? rcloneAddress(draft.rcloneInstance, draft.rcloneFolder)
            : path.trim(),
        kind,
        ...(share === null ? {} : { user: user.trim(), password }),
        ...(isRemote(share) ? remoteFields(share, draft, false) : {}),
        ...(kind === 'music' || kind === 'homevideos' ? {} : { useArtwork, readExtras }),
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
      <Field
        label={text.location}
        help={share === null ? text.help[computer(system)] : text.shareHelp[share]}
      >
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
            { value: 's3', label: text.locations.s3, icon: CloudIcon },
            { value: 'sftp', label: text.locations.sftp, icon: TerminalWindowIcon },
            { value: 'rclone', label: text.locations.rclone, icon: PlugsConnectedIcon },
          ]}
          className="self-start flex-wrap max-sm:self-stretch"
        />
      </Field>
      <Field label={text.name} help={text.nameHint}>
        <TextInput value={name} onValue={setName} maxLength={64} required autoFocus />
      </Field>
      {share !== 'rclone' && (
        <Field
          label={share === null ? text.path[computer(system)] : text.address}
          help={share === null ? text.pathHint[system] : text.addressHint[share]}
        >
          <TextInput
            key={location}
            type={share === null ? 'text' : 'url'}
            inputMode={share === null ? undefined : 'url'}
            value={path}
            onValue={setPath}
            placeholder={
              share === null ? text.pathPlaceholder[system] : text.addressPlaceholder[share]
            }
            autoComplete="off"
            spellCheck={false}
            required
            mono
          />
        </Field>
      )}
      {share === 's3' && <S3Endpoint draft={draft} onDraft={setDraft} />}
      {share === 'rclone' && <RcloneInstance draft={draft} onDraft={setDraft} />}
      {share !== null && (
        <>
          <Field label={share === 's3' ? text.accessKey : text.user} help={text.userHint[share]}>
            <TextInput
              value={user}
              onValue={setUser}
              maxLength={256}
              autoComplete="off"
              spellCheck={false}
            />
          </Field>
          <Field
            label={share === 's3' ? text.secretKey : text.password}
            help={share === 'sftp' ? text.sftpPasswordHint : text.passwordHint}
          >
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
      {share === 'sftp' && <SFTPKey draft={draft} onDraft={setDraft} />}
      {share === 'rclone' && (
        <RcloneRemote draft={draft} onDraft={setDraft} user={user} password={password} />
      )}
      {isRemote(share) && <PollField share={share} draft={draft} onDraft={setDraft} />}
      <KindField kind={kind} onChange={setKind} />
      {kind !== 'music' && kind !== 'homevideos' && (
        <Field label={text.useArtwork} help={text.useArtworkHelp}>
          <Switch checked={useArtwork} onChange={setUseArtwork} stateText />
        </Field>
      )}
      {kind !== 'music' && kind !== 'homevideos' && (
        <Field label={text.readExtras} help={text.readExtrasHelp}>
          <Switch checked={readExtras} onChange={setReadExtras} stateText />
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
 * The form changing a folder's name, path or address, a share's user and password, whether a
 * movies or shows folder's titles show its artwork, and what it holds. A change of what it holds
 * is asked first: what follows the files and what is dropped.
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
  const [readExtras, setReadExtras] = useState(folder.readExtras)
  const [kind, setKind] = useState<FolderKind>(folder.kind)
  const [confirming, setConfirming] = useState(false)
  const [draft, setDraft] = useState<RemoteDraft>(() => remoteDraft(folder))
  const share = shareOf(path)
  const retyped = kind !== folder.kind
  const address =
    share === 'rclone' ? rcloneAddress(draft.rcloneInstance, draft.rcloneFolder) : path.trim()
  const mutation = useMutation({
    mutationFn: () =>
      updateLocalFolder(addon.id, {
        name: name.trim(),
        path: address,
        ...(share === '' ? {} : { user: user.trim(), password }),
        ...(isRemote(share) ? remoteFields(share, draft, true) : {}),
        ...(kind === 'music' || kind === 'homevideos' ? {} : { useArtwork, readExtras }),
        ...(retyped ? { kind } : {}),
      }),
    onSuccess: (updated) => {
      setConfirming(false)
      onSaved(updated)
    },
    onSettled: () => invalidateScope('shared'),
  })
  const moved = address !== folder.path
  const system = useServerSystem()
  // On another server the stored secrets are dropped on save, unless given again.
  const rekeyed =
    share !== '' &&
    secretsServer(share, address, draft.endpoint) !==
      secretsServer(folder.share, folder.path, folder.endpoint)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (retyped) {
      setConfirming(true)
      return
    }
    mutation.mutate()
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <Field label={text.name} help={text.nameHint}>
        <TextInput value={name} onValue={setName} maxLength={64} required />
      </Field>
      {share !== 'rclone' && (
        <Field
          label={share === '' ? text.path[computer(system)] : text.address}
          help={
            share === ''
              ? moved
                ? text.movedHint
                : text.pathHint[system]
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
      )}
      {share === 's3' && <S3Endpoint draft={draft} onDraft={setDraft} />}
      {share === 'rclone' && <RcloneInstance draft={draft} onDraft={setDraft} moved={moved} />}
      {share !== '' && (
        <>
          <Field label={share === 's3' ? text.accessKey : text.user} help={text.userHint[share]}>
            <TextInput
              value={user}
              onValue={setUser}
              maxLength={256}
              autoComplete="off"
              spellCheck={false}
            />
          </Field>
          <SecretField
            label={share === 's3' ? text.secretKey : text.password}
            help={share === 'sftp' ? text.sftpPasswordHint : text.passwordHint}
            saved={folder.passwordSet && !rekeyed}
            status={
              rekeyed && folder.passwordSet && password === undefined
                ? text.secretNotSent
                : undefined
            }
            value={password}
            onValue={setPassword}
          />
        </>
      )}
      {share === 'sftp' && (
        <SFTPKey
          draft={draft}
          onDraft={setDraft}
          folder={folder}
          addonId={addon.id}
          rekeyed={rekeyed}
        />
      )}
      {share === 'rclone' && (
        <RcloneRemote
          draft={draft}
          onDraft={setDraft}
          user={user}
          password={password}
          folder={rekeyed ? undefined : addon.id}
        />
      )}
      {isRemote(share) && <PollField share={share} draft={draft} onDraft={setDraft} />}
      <KindField kind={kind} onChange={setKind} />
      {kind !== 'music' && kind !== 'homevideos' && (
        <Field label={text.useArtwork} help={text.useArtworkHelp}>
          <Switch checked={useArtwork} onChange={setUseArtwork} stateText />
        </Field>
      )}
      {kind !== 'music' && kind !== 'homevideos' && (
        <Field label={text.readExtras} help={text.readExtrasHelp}>
          <Switch checked={readExtras} onChange={setReadExtras} stateText />
        </Field>
      )}
      {mutation.isError && !confirming && (
        <FieldError>{errorMessage(t, mutation.error)}</FieldError>
      )}
      <div className="flex gap-2">
        <Button type="submit" variant="primary" loading={mutation.isPending && !confirming}>
          {text.save}
        </Button>
        <Button onClick={onCancel}>{text.cancel}</Button>
      </div>
      <ConfirmDialog
        open={confirming}
        onClose={() => {
          setConfirming(false)
          mutation.reset()
        }}
        onConfirm={() => mutation.mutate()}
        title={text.changeKindTitle(addon.name)}
        confirmLabel={text.changeKind}
        tone={folder.links.length > 0 || folder.episodes.length > 0 ? 'danger' : 'primary'}
        busy={mutation.isPending}
        error={mutation.isError ? errorMessage(t, mutation.error) : undefined}
      >
        <KindChange folder={folder} kind={kind} />
      </ConfirmDialog>
    </form>
  )
}

type DraftProps = { draft: RemoteDraft; onDraft: (draft: RemoteDraft) => void }

/** An S3 bucket's endpoint and region. */
function S3Endpoint({ draft, onDraft }: DraftProps) {
  const { t } = useI18n()
  const text = t.localFolders
  return (
    <>
      <Field label={text.endpoint} help={text.endpointHint}>
        <TextInput
          type="url"
          inputMode="url"
          value={draft.endpoint}
          onValue={(endpoint) => onDraft({ ...draft, endpoint })}
          placeholder="https://s3.example.org"
          autoComplete="off"
          spellCheck={false}
          required
          mono
        />
      </Field>
      <Field label={text.region} help={text.regionHint}>
        <TextInput
          value={draft.region}
          onValue={(region) => onDraft({ ...draft, region })}
          placeholder="us-east-1"
          maxLength={64}
          autoComplete="off"
          spellCheck={false}
          mono
        />
      </Field>
    </>
  )
}

/** The address of the rclone instance an rclone folder is read through, and the path in its remote. */
function RcloneInstance({ draft, onDraft, moved }: DraftProps & { moved?: boolean }) {
  const { t } = useI18n()
  const text = t.localFolders
  return (
    <>
      <Field label={text.rcloneInstance} help={text.rcloneInstanceHint}>
        <TextInput
          type="url"
          inputMode="url"
          value={draft.rcloneInstance}
          onValue={(rcloneInstance) => onDraft({ ...draft, rcloneInstance })}
          placeholder="http://rclone:5572"
          autoComplete="off"
          spellCheck={false}
          required
          mono
        />
      </Field>
      <Field label={text.rcloneFolder} help={moved ? text.movedShareHint : text.rcloneFolderHint}>
        <TextInput
          value={draft.rcloneFolder}
          onValue={(rcloneFolder) => onDraft({ ...draft, rcloneFolder })}
          placeholder="/movies"
          autoComplete="off"
          spellCheck={false}
          mono
        />
      </Field>
    </>
  )
}

/**
 * The remote of an rclone folder, typed or picked among those the rclone instance lists, asked
 * with the user and password of the form, or those stored for the folder when editing one.
 */
function RcloneRemote({
  draft,
  onDraft,
  user,
  password,
  folder,
}: DraftProps & { user: string; password: string | undefined; folder?: string }) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const listing = useMutation({
    mutationFn: () =>
      fetchRcloneRemotes({
        address: draft.rcloneInstance.trim(),
        user: user.trim(),
        ...(password === undefined ? { folder } : { password }),
      }),
  })
  const remotes = listing.data?.remotes ?? []
  const failure = listing.data?.error ?? ''
  const help =
    failure !== ''
      ? text.remotesFailed[failure]
      : listing.isSuccess
        ? remotes.length === 0
          ? text.noRemotes
          : text.remotesListed(remotes.length, remotes.length.toLocaleString(language))
        : text.rcloneRemoteHint
  return (
    <Field
      label={text.rcloneRemote}
      help={help}
      error={listing.isError ? errorMessage(t, listing.error) : undefined}
    >
      <div className="flex flex-wrap items-center gap-2">
        <SuggestInput
          value={draft.rcloneRemote}
          onValue={(rcloneRemote) => onDraft({ ...draft, rcloneRemote })}
          suggestions={remotes}
          onPick={(rcloneRemote) => onDraft({ ...draft, rcloneRemote })}
          maxLength={128}
          autoComplete="off"
          spellCheck={false}
          required
          mono
          className="max-w-xs"
        />
        <Button
          size="sm"
          onClick={() => listing.mutate()}
          loading={listing.isPending}
          disabled={draft.rcloneInstance.trim() === ''}
        >
          {text.listRemotes}
        </Button>
      </div>
    </Field>
  )
}

/**
 * An SFTP folder's private key and passphrase, and when editing one the host key saved, with the
 * one its server offers instead to accept. Moved to another server (rekeyed), the folder keeps
 * neither its key nor its passphrase nor its host key, unless the key and passphrase are given again.
 */
function SFTPKey({
  draft,
  onDraft,
  folder,
  addonId,
  rekeyed = false,
}: DraftProps & { folder?: LocalFolder; addonId?: string; rekeyed?: boolean }) {
  const { t } = useI18n()
  const text = t.localFolders
  const saved = folder?.privateKeySet === true && !rekeyed
  const left = folder?.privateKeySet === true && rekeyed
  return (
    <>
      <Field
        label={text.privateKey}
        help={left && draft.privateKey.trim() === '' ? text.privateKeyNotSent : text.privateKeyHint}
      >
        <Textarea
          value={draft.privateKey}
          onValue={(privateKey) => onDraft({ ...draft, privateKey })}
          placeholder={saved && !draft.removeKey ? text.privateKeySaved : undefined}
          rows={4}
          mono
          spellCheck={false}
          autoComplete="off"
        />
      </Field>
      {saved && (
        <Checkbox
          label={text.privateKeyRemove}
          checked={draft.removeKey}
          onChange={(removeKey) => onDraft({ ...draft, removeKey })}
        />
      )}
      {folder === undefined ? (
        <Field label={text.passphrase} help={text.passphraseHint}>
          <TextInput
            type="password"
            revealable
            value={draft.passphrase ?? ''}
            onValue={(passphrase) => onDraft({ ...draft, passphrase })}
            maxLength={1024}
            autoComplete="new-password"
          />
        </Field>
      ) : (
        <SecretField
          label={text.passphrase}
          help={text.passphraseHint}
          saved={folder.passphraseSet && !rekeyed}
          status={
            rekeyed && folder.passphraseSet && draft.passphrase === undefined
              ? text.secretNotSent
              : undefined
          }
          value={draft.passphrase}
          onValue={(passphrase) => onDraft({ ...draft, passphrase })}
        />
      )}
      {folder !== undefined && addonId !== undefined && !rekeyed && (
        <HostKey folder={folder} addonId={addonId} />
      )}
    </>
  )
}

/** The host key saved for an SFTP folder, and the one its server offers instead, to accept. */
function HostKey({ folder, addonId }: { folder: LocalFolder; addonId: string }) {
  const { t } = useI18n()
  const text = t.localFolders
  const toast = useToast()
  const accept = useMutation({
    mutationFn: () => updateLocalFolder(addonId, { acceptHostKey: true }),
    onSuccess: (updated) => {
      replaceCachedAddon('shared', updated)
      toast(text.hostKeyAccepted)
    },
    onSettled: () => invalidateScope('shared'),
  })
  return (
    <Field label={text.hostKey} help={text.hostKeyHint}>
      <div className="flex flex-col gap-3">
        <code className="font-mono text-small break-all text-ink-2">
          {folder.hostKey === '' ? text.hostKeyNone : folder.hostKey}
        </code>
        {folder.offeredHostKey !== '' && (
          <Notice tone="danger">
            <p>{text.hostKeyOffered(folder.offeredHostKey)}</p>
            <div className="mt-3">
              <Button
                size="sm"
                variant="danger"
                onClick={() => accept.mutate()}
                loading={accept.isPending}
              >
                {text.acceptHostKey}
              </Button>
            </div>
            {accept.isError && <InlineError>{errorMessage(t, accept.error)}</InlineError>}
          </Notice>
        )}
      </div>
    </Field>
  )
}

/** How often a remote share is listed again for changes, empty for its kind's default. */
function PollField({ share, draft, onDraft }: DraftProps & { share: RemoteShare }) {
  const { t } = useI18n()
  const text = t.localFolders
  return (
    <Field label={text.pollMinutes} help={text.pollMinutesHint[share]}>
      <NumberInput
        value={draft.pollMinutes}
        onValue={(pollMinutes) => onDraft({ ...draft, pollMinutes })}
        placeholder={String(defaultPollMinutes[share])}
        min={0}
        max={1440}
        step={1}
        suffix="min"
      />
    </Field>
  )
}

/** The choice of what a folder holds, with what each kind means. */
function KindField({ kind, onChange }: { kind: FolderKind; onChange: (kind: FolderKind) => void }) {
  const { t } = useI18n()
  const text = t.localFolders
  return (
    <Field label={text.kind} help={text.kindHelp[kind]}>
      <Segmented<FolderKind>
        label={text.kind}
        value={kind}
        onChange={onChange}
        options={[
          { value: 'movies', label: text.kinds.movies, icon: FilmSlateIcon },
          { value: 'shows', label: text.kinds.shows, icon: TelevisionSimpleIcon },
          { value: 'music', label: text.kinds.music, icon: MusicNotesIcon },
          { value: 'homevideos', label: text.kinds.homevideos, icon: VideoCameraIcon },
        ]}
        className="self-start"
      />
    </Field>
  )
}

/**
 * What a change of what a folder holds does, as the server does it: the folder is scanned again
 * as the new kind; between movies, shows and home videos what users did with a video follows it,
 * and home videos come back as the same titles within 30 days; to or from music nothing follows;
 * links made by hand and episodes given to discs are dropped.
 */
function KindChange({ folder, kind }: { folder: LocalFolder; kind: FolderKind }) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const music = folder.kind === 'music' || kind === 'music'
  const dropped = [
    ...(folder.links.length > 0
      ? [text.linksDropped(folder.links.length, folder.links.length.toLocaleString(language))]
      : []),
    ...(folder.episodes.length > 0
      ? [
          text.episodesDropped(
            folder.episodes.length,
            folder.episodes.length.toLocaleString(language),
          ),
        ]
      : []),
  ]
  return (
    <div className="flex flex-col gap-2">
      <p>{text.changeKindScan(text.kindNouns[folder.kind], text.kindNouns[kind])}</p>
      <p>{music ? text.changeKindNothing : text.changeKindFollows}</p>
      {folder.kind === 'homevideos' && <p>{text.changeKindBack}</p>}
      {dropped.length > 0 && <p>{text.changeKindDrops(dropped)}</p>}
    </div>
  )
}

/**
 * A folder's figures: its files, those matched, how, and the others, its last scan, and why it
 * cannot be read when it cannot. A music folder's are its tracks, albums and artists, its tracks
 * with a gain and those measured, and its tracks left out; a home videos folder's its videos, by
 * where their dates come from.
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
          { label: text.gained, value: folder.gained, warn: false },
          { label: text.measured, value: folder.measured, warn: false },
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
            ...(folder.readExtras || folder.extras > 0
              ? [{ label: text.extras, value: folder.extras, warn: false }]
              : []),
          ]
  const methods = matchMethods
    .filter((method) => folder.matchedBy[method] > 0)
    .map((method) => text.matchMethods[method](folder.matchedBy[method].toLocaleString(language)))
  const errors = folder.share === '' ? text.errors : text.shareErrors
  const failure = folderFailure(folder)
  // Figures wrap onto rows: only those after the first of their row are set apart.
  const columns = figures.length === 4 ? 4 : 3
  return (
    <PanelSection>
      <dl className={cx('grid gap-y-4', columns === 4 ? 'grid-cols-4' : 'grid-cols-3')}>
        {figures.map((figure, i) => (
          <div
            key={figure.label}
            className={cx('min-w-0 px-4', i % columns === 0 ? 'pl-0' : 'border-l border-line')}
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
      {failure !== '' && (
        <Notice tone="danger" className="mt-5">
          <strong className="font-medium">
            {folder.share === '' ? text.errorTitle : text.errorTitleShare}.
          </strong>{' '}
          {Object.hasOwn(errors, failure) ? errors[failure] : t.errors.generic}{' '}
          {folder.files > 0 && text.keptNote}
        </Notice>
      )}
      {folder.searchFailed > 0 && (
        <Notice tone="warn" className="mt-5">
          <strong className="font-medium">
            {text.searchFailedTitle(
              folder.searchFailed,
              folder.searchFailed.toLocaleString(language),
            )}
            .
          </strong>{' '}
          {folder.searchFailure !== null && folder.searchFailure.addon !== '' && (
            <>
              {text.searchFailedBy(
                folder.searchFailure.addon,
                text.searchFailures[folder.searchFailure.failure] ?? '',
              )}
              .{' '}
            </>
          )}
          {folder.searchRetryAt !== null &&
            `${text.searchRetry(dateTime(folder.searchRetryAt, language))} `}
          {text.searchHelp} {text.searchIdentifier(folder.kind === 'shows')}
        </Notice>
      )}
      {folder.noSearch > 0 && (
        <Notice tone="warn" className="mt-5">
          <strong className="font-medium">
            {text.noSearchTitle(folder.noSearch, folder.noSearch.toLocaleString(language))}.
          </strong>{' '}
          {text.noSearchHelp}
        </Notice>
      )}
    </PanelSection>
  )
}

/**
 * The files of a folder no title was matched to, each linkable to an IMDb identifier, those a looser
 * rule matched, and the links made; a music folder's tracks left out of its albums, which no link
 * links.
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
      <LooserMatches addon={addon} folder={folder} />
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
 * The units of a folder a looser rule matched, each with its rule, what its name was read as and
 * the title it was matched to, and its unlink, which leaves it to the exact rule.
 */
function LooserMatches({ addon, folder }: { addon: Addon; folder: LocalFolder }) {
  const { language, t } = useI18n()
  const text = t.localFolders
  const toast = useToast()
  const count = folder.matchedBy.year + folder.matchedBy.numerals + folder.matchedBy.yearNumerals
  const looser = useQuery({
    // The figures key the list, as they key the unmatched files.
    queryKey: [...queryKeys.looser(addon.id), folder.files, count, folder.scanning],
    queryFn: ({ signal }) => fetchLooserMatches(addon.id, signal),
    enabled: count > 0,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === addon.id ? previous : undefined,
  })
  const unlink = useMutation({
    mutationFn: (unit: string) => unlinkLocalFile(addon.id, unit),
    onSuccess: (updated, unit) => {
      replaceCachedAddon('shared', updated)
      toast(text.looserUnlinked(unitName(unit, folder, text.discTitle)))
    },
    onSettled: () => {
      invalidateScope('shared')
      void queryClient.invalidateQueries({ queryKey: queryKeys.unmatched(addon.id) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.looser(addon.id) })
    },
  })
  if (count === 0) return null

  return (
    <PanelSection title={text.looserTitle} description={text.looserHelp}>
      {looser.isPending ? (
        <Skeleton className="h-24 rounded-panel" />
      ) : looser.isError ? (
        <InlineError onRetry={() => void looser.refetch()} retrying={looser.isRefetching}>
          {errorMessage(t, looser.error)}
        </InlineError>
      ) : (
        <>
          <ul className="flex flex-col divide-y divide-line">
            {looser.data.units.map((match) => {
              const name = unitName(match.unit, folder, text.discTitle)
              const read = match.year === null ? match.title : `${match.title} (${match.year})`
              const title =
                match.titleYear === null ? match.name : `${match.name} (${match.titleYear})`
              return (
                <li key={match.unit} className="flex items-center justify-between gap-4 py-3">
                  <div className="min-w-0">
                    <p className="font-mono text-small break-all text-ink">{name}</p>
                    <p className="mt-1 text-small text-ink-3">
                      {[
                        text.looserRules[match.rule] ?? match.rule,
                        text.read(read),
                        text.matchedTo(title),
                        match.files > 1
                          ? text.fileCount(match.files, match.files.toLocaleString(language))
                          : '',
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    </p>
                    <Badge tone="warn">{match.imdbId}</Badge>
                  </div>
                  <IconButton
                    label={text.looserUnlinkLabel(name)}
                    icon={LinkBreakIcon}
                    size="sm"
                    disabled={unlink.isPending}
                    onClick={() => unlink.mutate(match.unit)}
                  />
                </li>
              )
            })}
          </ul>
          {looser.data.total > looser.data.units.length && (
            <p className="mt-3 text-small text-ink-3">
              {text.shownOf(
                looser.data.units.length.toLocaleString(language),
                looser.data.total.toLocaleString(language),
              )}
            </p>
          )}
        </>
      )}
      {unlink.isError && <FieldError>{errorMessage(t, unlink.error)}</FieldError>}
    </PanelSection>
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
            file.search !== null && file.search.addon !== ''
              ? text.searchFailedBy(
                  file.search.addon,
                  text.searchFailures[file.search.failure] ?? '',
                )
              : (text.reasons[file.reason] ?? file.reason),
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
