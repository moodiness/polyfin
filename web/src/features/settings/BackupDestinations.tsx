import { CloudArrowUpIcon, PlusIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import {
  ApiError,
  createBackupDestination,
  deleteBackupDestination,
  fetchBackupDestinations,
  queryClient,
  queryKeys,
  testBackupDestination,
  updateBackupDestination,
  type BackupDestination,
  type BackupDestinationDraft,
  type BackupDestinationKind,
} from '@/api'
import { dateTime, errorMessage, relativeTime } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  IconTile,
  InlineError,
  Modal,
  Notice,
  NumberInput,
  SecretField,
  Select,
  SkeletonRows,
  StatusPill,
  Textarea,
  TextInput,
  useToast,
} from '@/ui'
import { destinationStatus } from '@/features/system/BackupStatus'

/** The letters in each kind's tile. */
const monograms: Record<BackupDestinationKind, string> = { s3: 'S3', sftp: 'SF', webdav: 'WD' }

/** The bounds of Backups to keep, as the server checks them. */
const kept = { min: 1, max: 90, byDefault: 7 }

/**
 * The places backups are sent to besides the folder (Settings › Backups): each with its last
 * upload, Test, Edit and Delete, and Add.
 */
export default function BackupDestinations() {
  const { t } = useI18n()
  const text = t.backups.destinations
  const [editing, setEditing] = useState<BackupDestination | 'new' | null>(null)
  const destinations = useQuery({
    queryKey: queryKeys.backupDestinations,
    queryFn: ({ signal }) => fetchBackupDestinations(signal),
  })
  if (destinations.isPending) return <SkeletonRows rows={2} label={text.loading} />
  if (destinations.isError) {
    return (
      <InlineError onRetry={() => void destinations.refetch()} retrying={destinations.isFetching}>
        {errorMessage(t, destinations.error)}
      </InlineError>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      {destinations.data.destinations.length === 0 ? (
        <EmptyState icon={CloudArrowUpIcon} title={text.empty}>
          {text.emptyHint}
        </EmptyState>
      ) : (
        <ul className="overflow-hidden rounded-panel border border-line-2 bg-s1">
          {destinations.data.destinations.map((destination) => (
            <DestinationRow
              key={destination.id}
              destination={destination}
              onEdit={() => setEditing(destination)}
            />
          ))}
        </ul>
      )}
      <div>
        <Button icon={PlusIcon} onClick={() => setEditing('new')}>
          {text.add}
        </Button>
      </div>
      {editing !== null && (
        <DestinationModal
          destination={editing === 'new' ? null : editing}
          kinds={destinations.data.kinds}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  )
}

/** Puts a destination as the server answered it into the list, and reads the backups again. */
function store(destination: BackupDestination) {
  void queryClient.invalidateQueries({ queryKey: queryKeys.backup })
  queryClient.setQueryData(
    queryKeys.backupDestinations,
    (old: { destinations: BackupDestination[] } | undefined) =>
      old && {
        ...old,
        destinations: old.destinations.some((current) => current.id === destination.id)
          ? old.destinations.map((current) =>
              current.id === destination.id ? destination : current,
            )
          : [...old.destinations, destination],
      },
  )
}

function DestinationRow({
  destination,
  onEdit,
}: {
  destination: BackupDestination
  onEdit: () => void
}) {
  const { language, t } = useI18n()
  const text = t.backups.destinations
  const toast = useToast()
  const [confirming, setConfirming] = useState(false)
  const test = useMutation({
    mutationFn: () => testBackupDestination(destination.id),
    onSuccess: (result) => {
      store(result.destination)
      toast(
        result.ok
          ? text.testOk(destination.name)
          : `${text.testFailed(destination.name)} ${result.error}`,
        { tone: result.ok ? 'ok' : 'danger' },
      )
    },
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
  })
  const remove = useMutation({
    mutationFn: () => deleteBackupDestination(destination.id),
    onSuccess: () => {
      setConfirming(false)
      toast(text.deleted(destination.name))
      void queryClient.invalidateQueries({ queryKey: queryKeys.backupDestinations })
      void queryClient.invalidateQueries({ queryKey: queryKeys.backup })
    },
  })
  const status = destinationStatus(t, destination)
  const headingId = useId()
  return (
    <li
      aria-labelledby={headingId}
      className="grid grid-cols-[40px_minmax(0,1fr)_auto] items-start gap-x-4 px-[22px] py-5 max-sm:grid-cols-[40px_minmax(0,1fr)] max-sm:gap-x-3.5 max-sm:px-4 [&+&]:border-t [&+&]:border-line"
    >
      <IconTile letters={monograms[destination.kind]} />
      <div className="min-w-0">
        <h3
          id={headingId}
          className="flex h-10 items-center font-mono text-[15px] font-semibold tracking-[-0.01em] break-all text-ink"
        >
          {destination.name}
        </h3>
        <p className="-mt-1.5 text-control text-ink-2">
          {text.kinds[destination.kind]} · {text.keeps(destination.kept)}
        </p>
        <p className="mt-1 text-small text-ink-3">
          {text.lastUpload}{' '}
          {destination.ranAt === null ? (
            text.none
          ) : (
            <time dateTime={destination.ranAt} title={dateTime(destination.ranAt, language)}>
              {relativeTime(destination.ranAt, language, t.time.justNow)}
            </time>
          )}
          {destination.newestFile !== '' && (
            <>
              {' · '}
              {text.newest} <code className="font-mono">{destination.newestFile}</code>
            </>
          )}
        </p>
      </div>
      <div className="flex h-10 items-center justify-end gap-1.5 max-sm:col-span-full max-sm:mt-3 max-sm:h-auto max-sm:flex-wrap">
        <StatusPill tone={status.tone} className="max-sm:mr-auto">
          {status.label}
        </StatusPill>
        <Button
          variant="secondary"
          loading={test.isPending}
          disabled={destination.unreadable}
          title={text.testHelp}
          onClick={() => test.mutate()}
        >
          {test.isPending ? text.testing : text.test}
        </Button>
        <Button variant="ghost" onClick={onEdit}>
          {text.edit}
        </Button>
        <Button variant="ghost" onClick={() => setConfirming(true)}>
          {text.delete}
        </Button>
      </div>
      {(destination.unreadable || destination.error !== '' || destination.problem === 'stale') && (
        <div className="col-start-2 -col-end-1 mt-4 max-sm:col-span-full">
          <Notice tone={status.tone === 'warn' ? 'warn' : 'danger'}>
            {destination.unreadable
              ? text.unreadable
              : destination.error !== ''
                ? destination.error
                : text.staleHelp}
          </Notice>
        </div>
      )}
      <ConfirmDialog
        open={confirming}
        onClose={() => {
          setConfirming(false)
          remove.reset()
        }}
        onConfirm={() => remove.mutate()}
        title={text.deleteTitle(destination.name)}
        confirmLabel={text.delete}
        tone="danger"
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {text.deleteBody}
      </ConfirmDialog>
    </li>
  )
}

/** The codes shown under each field. */
const fieldCodes = {
  address: ['invalid_destination_address'],
  bucket: ['invalid_bucket'],
  folder: ['invalid_destination_folder'],
  region: ['invalid_region'],
  port: ['invalid_port'],
  username: ['invalid_destination_user'],
  secret: ['invalid_destination_secret'],
  privateKey: ['invalid_private_key'],
  kept: ['invalid_backups_kept'],
}

/** The form to add a destination, or to change one, in a floating panel. */
function DestinationModal({
  destination,
  kinds,
  onClose,
}: {
  destination: BackupDestination | null
  kinds: BackupDestinationKind[]
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.backups.destinations
  const toast = useToast()
  const formId = useId()
  const [kind, setKind] = useState<BackupDestinationKind>(destination?.kind ?? 's3')
  const [address, setAddress] = useState(destination?.address ?? '')
  const [port, setPort] = useState<number | null>(destination?.port || 22)
  const [bucket, setBucket] = useState(destination?.bucket ?? '')
  const [folder, setFolder] = useState(destination?.folder ?? '')
  const [region, setRegion] = useState(destination?.region ?? '')
  const [username, setUsername] = useState(destination?.username ?? '')
  // Secrets: undefined keeps the saved one.
  const [secret, setSecret] = useState<string | undefined>(undefined)
  const [privateKey, setPrivateKey] = useState('')
  const [removeKey, setRemoveKey] = useState(false)
  const [forgetHostKey, setForgetHostKey] = useState(false)
  const [keep, setKeep] = useState<number | null>(destination?.kept ?? kept.byDefault)

  const mutation = useMutation({
    mutationFn: (draft: BackupDestinationDraft) =>
      destination === null
        ? createBackupDestination(draft)
        : updateBackupDestination(destination.id, draft),
    onSuccess: (saved) => {
      store(saved)
      toast(destination === null ? text.created(saved.name) : text.saved(saved.name))
      onClose()
    },
  })
  const code = mutation.error instanceof ApiError ? mutation.error.code : null
  const message = mutation.isError ? errorMessage(t, mutation.error) : undefined
  const fieldError = (field: keyof typeof fieldCodes) =>
    code !== null && fieldCodes[field].includes(code) ? message : undefined
  const shownElsewhere = Object.values(fieldCodes).some(
    (codes) => code !== null && codes.includes(code),
  )
  const otherError = mutation.isError && !shownElsewhere ? message : undefined

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const draft: BackupDestinationDraft = {
      address: address.trim(),
      username: username.trim(),
      kept: keep ?? kept.byDefault,
    }
    if (destination === null) draft.kind = kind
    if (secret !== undefined) draft.secret = secret
    switch (kind) {
      case 's3':
        Object.assign(draft, {
          bucket: bucket.trim(),
          folder: folder.trim(),
          region: region.trim(),
        })
        break
      case 'sftp':
        Object.assign(draft, { port: port ?? 22, folder: folder.trim() })
        if (privateKey.trim() !== '') draft.privateKey = privateKey
        else if (removeKey) draft.privateKey = ''
        if (forgetHostKey) draft.hostKey = ''
        break
    }
    mutation.mutate(draft)
  }

  const change =
    <T,>(set: (value: T) => void) =>
    (value: T) => {
      mutation.reset()
      set(value)
    }
  const textField = (
    label: string,
    help: string | undefined,
    value: string,
    set: (value: string) => void,
    field: keyof typeof fieldCodes,
    placeholder?: string,
  ) => (
    <Field label={label} help={help} error={fieldError(field)}>
      <TextInput
        value={value}
        onValue={change(set)}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
        mono
        invalid={fieldError(field) !== undefined}
      />
    </Field>
  )
  const secretField = (label: string, help?: string) => (
    <SecretField
      label={label}
      help={help}
      saved={destination?.secretSet ?? false}
      value={secret}
      onValue={change(setSecret)}
      removable={kind !== 's3'}
      error={fieldError('secret')}
    />
  )

  return (
    <Modal
      open
      onClose={onClose}
      title={destination === null ? text.addTitle : text.editTitle(destination.name)}
      width={560}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={mutation.isPending}>
            {destination === null
              ? mutation.isPending
                ? text.creating
                : text.create
              : mutation.isPending
                ? text.saving
                : text.save}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-5 p-5">
        {destination === null && (
          <Field label={text.kind} help={text.kindHelp[kind]}>
            <Select
              value={kind}
              options={kinds.map((value) => ({ value, label: text.kinds[value] }))}
              onValue={change(setKind)}
              className="max-w-xs"
            />
          </Field>
        )}
        {destination?.unreadable && <Notice tone="danger">{text.unreadable}</Notice>}
        {kind === 's3' && (
          <>
            {textField(
              text.endpoint,
              text.endpointHelp,
              address,
              setAddress,
              'address',
              'https://s3.example.org',
            )}
            {textField(text.bucket, undefined, bucket, setBucket, 'bucket')}
            {textField(text.prefix, text.prefixHelp, folder, setFolder, 'folder', 'polyfin')}
            {textField(text.region, text.regionHelp, region, setRegion, 'region', 'us-east-1')}
            {textField(text.accessKey, undefined, username, setUsername, 'username')}
            {secretField(text.secretKey)}
          </>
        )}
        {kind === 'sftp' && (
          <>
            {textField(text.host, text.hostHelp, address, setAddress, 'address', 'nas.lan')}
            <Field label={text.port} error={fieldError('port')}>
              <NumberInput
                value={port}
                onValue={change(setPort)}
                min={1}
                max={65535}
                step={1}
                invalid={fieldError('port') !== undefined}
              />
            </Field>
            {textField(text.user, undefined, username, setUsername, 'username')}
            {secretField(text.password, text.sftpPasswordHelp)}
            <Field
              label={text.privateKey}
              help={text.privateKeyHelp}
              error={fieldError('privateKey')}
            >
              <Textarea
                value={privateKey}
                onValue={change(setPrivateKey)}
                placeholder={
                  destination?.privateKeySet && !removeKey ? text.privateKeySaved : undefined
                }
                rows={4}
                mono
                spellCheck={false}
                autoComplete="off"
                invalid={fieldError('privateKey') !== undefined}
              />
            </Field>
            {destination?.privateKeySet && (
              <Checkbox
                label={text.privateKeyRemove}
                checked={removeKey}
                onChange={change(setRemoveKey)}
              />
            )}
            {textField(
              text.sftpFolder,
              text.sftpFolderHelp,
              folder,
              setFolder,
              'folder',
              '/backups/polyfin',
            )}
            {destination !== null && (
              <Field label={text.hostKey} help={text.hostKeyForgetHelp}>
                <div className="flex flex-wrap items-center gap-3">
                  <code className="font-mono text-small break-all text-ink-2">
                    {destination.hostKey === '' || forgetHostKey
                      ? forgetHostKey
                        ? text.hostKeyForgotten
                        : text.hostKeyNone
                      : destination.hostKey}
                  </code>
                  {destination.hostKey !== '' && !forgetHostKey && (
                    <Button variant="secondary" size="sm" onClick={() => setForgetHostKey(true)}>
                      {text.hostKeyForget}
                    </Button>
                  )}
                </div>
              </Field>
            )}
          </>
        )}
        {kind === 'webdav' && (
          <>
            {textField(
              text.webdavAddress,
              text.webdavAddressHelp,
              address,
              setAddress,
              'address',
              'https://cloud.example.org/dav/polyfin/',
            )}
            {textField(text.user, undefined, username, setUsername, 'username')}
            {secretField(text.password, text.webdavPasswordHelp)}
          </>
        )}
        <Field
          label={text.kept}
          help={text.keptHelp(kept.min, kept.max, kept.byDefault)}
          error={fieldError('kept')}
        >
          <NumberInput
            value={keep}
            onValue={change(setKeep)}
            min={kept.min}
            max={kept.max}
            step={1}
            invalid={fieldError('kept') !== undefined}
          />
        </Field>
        {otherError && <Notice tone="danger">{otherError}</Notice>}
      </form>
    </Modal>
  )
}
