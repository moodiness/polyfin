import { FileCodeIcon, PlusIcon, TelevisionIcon, UploadSimpleIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useRef, useState } from 'react'
import {
  ApiError,
  fetchDlnaDevices,
  fetchDlnaProfiles,
  fetchDlnaProfileXml,
  fetchUsers,
  forgetDlnaDevice,
  queryClient,
  queryKeys,
  removeDlnaProfile,
  saveDlnaProfile,
  updateDlnaDevice,
  type DlnaDevice,
  type DlnaDeviceChanges,
  type DlnaProfile,
  type User,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Badge,
  Button,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  IconTile,
  InlineError,
  Modal,
  RelativeTime,
  Select,
  SkeletonRows,
  StatusPill,
  TextInput,
  useToast,
} from '@/ui'
import CodeEditor from './CodeEditor'
import { FieldRow, SettingRow, SettingsGroup, SwitchRow } from './parts'
import type { SectionFormApi } from './SectionForm'

/** The largest profile the server takes, in bytes. */
const maxProfileBytes = 256 * 1024

/** The users a device may show the libraries of, after No one. */
function userOptions(t: Messages, users: readonly User[]) {
  return [
    { value: '', label: t.settings.dlna.noOne },
    ...users.map((user) => ({ value: user.id, label: user.name })),
  ]
}

/**
 * Settings › DLNA: sharing the libraries with the devices of the local network, the devices seen
 * with the user each shows, and the profiles they play with.
 */
export default function DlnaSection({ form, update, error, limits }: SectionFormApi) {
  const { t } = useI18n()
  const text = t.settings.dlna
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  return (
    <>
      <SettingsGroup>
        <SwitchRow
          anchor="dlna"
          label={text.share}
          help={text.shareHelp}
          checked={form.dlna}
          onChange={(dlna) => update({ dlna })}
        />
        <FieldRow
          anchor="dlna-name"
          label={text.name}
          help={text.nameHelp}
          error={error('dlna-name')}
        >
          <TextInput
            value={form.dlnaName}
            onValue={(dlnaName) => update({ dlnaName })}
            placeholder={form.serverName}
            maxLength={limits('dlnaName').max}
            autoComplete="off"
            className="max-w-sm"
          />
        </FieldRow>
        <FieldRow
          anchor="dlna-new-devices-user"
          label={text.newDevicesUser}
          help={text.newDevicesUserHelp}
          error={error('dlna-new-devices-user')}
        >
          <Select
            value={form.dlnaNewDevicesUser ?? ''}
            options={userOptions(t, users.data ?? [])}
            onValue={(id) => update({ dlnaNewDevicesUser: id === '' ? null : id })}
            disabled={users.isPending}
            className="max-w-xs"
          />
        </FieldRow>
        <SwitchRow
          anchor="dlna-mark-played"
          label={text.markPlayed}
          help={text.markPlayedHelp}
          checked={form.dlnaMarkPlayed}
          onChange={(dlnaMarkPlayed) => update({ dlnaMarkPlayed })}
        />
      </SettingsGroup>
      <SettingsGroup title={text.devices}>
        <SettingRow anchor="dlna-devices">
          <p className="mb-4 max-w-[60ch] text-small text-ink-3">{text.devicesHelp}</p>
          <Devices />
        </SettingRow>
      </SettingsGroup>
      <SettingsGroup title={text.profiles}>
        <SettingRow anchor="dlna-profiles">
          <p className="mb-4 max-w-[60ch] text-small text-ink-3">{text.profilesHelp}</p>
          <Profiles />
        </SettingRow>
      </SettingsGroup>
    </>
  )
}

/** The devices seen, the latest first, each with its user, block, profile and Forget. */
function Devices() {
  const { t } = useI18n()
  const text = t.settings.dlna
  const devices = useQuery({
    queryKey: queryKeys.dlnaDevices,
    queryFn: ({ signal }) => fetchDlnaDevices(signal),
  })
  const profiles = useQuery({
    queryKey: queryKeys.dlnaProfiles,
    queryFn: ({ signal }) => fetchDlnaProfiles(signal),
  })
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  const failed = [devices, profiles, users].find((query) => query.isError)
  if (failed) {
    return (
      <InlineError onRetry={() => void failed.refetch()} retrying={failed.isFetching}>
        {errorMessage(t, failed.error)}
      </InlineError>
    )
  }
  if (devices.data === undefined || profiles.data === undefined || users.data === undefined) {
    return <SkeletonRows rows={2} label={t.common.loading} />
  }
  if (devices.data.length === 0) {
    return <EmptyState icon={TelevisionIcon} title={text.devicesEmpty} />
  }
  return (
    <ul
      aria-label={text.devicesLabel}
      className="overflow-hidden rounded-panel border border-line-2 bg-s1"
    >
      {devices.data.map((device) => (
        <DeviceRow key={device.id} device={device} profiles={profiles.data} users={users.data} />
      ))}
    </ul>
  )
}

function DeviceRow({
  device,
  profiles,
  users,
}: {
  device: DlnaDevice
  profiles: readonly DlnaProfile[]
  users: readonly User[]
}) {
  const { t } = useI18n()
  const text = t.settings.dlna
  const toast = useToast()
  const headingId = useId()
  const [forgetting, setForgetting] = useState(false)
  // What a device is called: the name it announces, else its model, else its address.
  const title = device.name || device.model || device.address
  const change = useMutation({
    mutationFn: (changes: DlnaDeviceChanges) => updateDlnaDevice(device.id, changes),
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.dlnaDevices, (old: DlnaDevice[] | undefined) =>
        old?.map((current) => (current.id === saved.id ? saved : current)),
      )
      toast(text.deviceSaved(title), { tone: 'ok' })
    },
    onError: (error) => {
      toast(errorMessage(t, error), { tone: 'danger' })
      void queryClient.invalidateQueries({ queryKey: queryKeys.dlnaDevices })
    },
  })
  const forget = useMutation({
    mutationFn: () => forgetDlnaDevice(device.id),
    onSuccess: () => {
      setForgetting(false)
      toast(text.forgotten(title))
    },
    onSettled: (_data, error) => {
      if (error === null || (error instanceof ApiError && error.status === 404)) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.dlnaDevices })
      }
    },
  })
  // While a change is saved, the controls show it.
  const shown = change.isPending ? { ...device, ...change.variables } : device
  const model = [device.manufacturer, device.model]
    .filter((part) => part !== '' && part !== title)
    .join(' ')
  return (
    <li
      aria-labelledby={headingId}
      className="grid grid-cols-[40px_minmax(0,1fr)_auto] items-start gap-x-4 px-[22px] py-5 max-sm:grid-cols-[40px_minmax(0,1fr)] max-sm:gap-x-3.5 max-sm:px-4 [&+&]:border-t [&+&]:border-line"
    >
      <IconTile icon={TelevisionIcon} />
      <div className="min-w-0">
        <h4
          id={headingId}
          className="flex min-h-10 flex-wrap items-center gap-x-2.5 text-[15px] font-semibold tracking-[-0.01em] break-all text-ink"
        >
          {title}
          {device.blocked && <StatusPill tone="danger">{text.blocked}</StatusPill>}
        </h4>
        {model !== '' && (
          <p className="-mt-1.5 text-control text-ink-2">
            <span className="sr-only">{text.model} </span>
            {model}
          </p>
        )}
        <p className="mt-1 text-small text-ink-3">
          {title !== device.address && (
            <>
              <span className="sr-only">{text.address} </span>
              <span className="figures">{device.address}</span>
              {' · '}
            </>
          )}
          {text.lastSeen} <RelativeTime iso={device.lastSeen} />
        </p>
      </div>
      <div className="flex h-10 items-center justify-end max-sm:col-span-full max-sm:mt-3 max-sm:h-auto">
        <Button
          variant="ghost"
          aria-label={`${text.forget}: ${title}`}
          onClick={() => {
            forget.reset()
            setForgetting(true)
          }}
        >
          {text.forget}
        </Button>
      </div>
      <div className="col-start-2 -col-end-1 mt-4 grid items-end gap-x-4 gap-y-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] max-sm:col-span-full">
        <Field label={text.showsLibrariesOf}>
          <Select
            value={shown.userId ?? ''}
            options={userOptions(t, users)}
            onValue={(id) => change.mutate({ userId: id === '' ? null : id })}
            disabled={change.isPending}
          />
        </Field>
        <Field label={text.profile}>
          <Select
            value={shown.profile}
            options={[
              {
                value: '',
                label:
                  device.profile === ''
                    ? text.automaticProfile(device.matchedProfile)
                    : text.automatic,
              },
              ...profiles.map((profile) => ({ value: profile.id, label: profile.name })),
            ]}
            onValue={(profile) => change.mutate({ profile })}
            disabled={change.isPending}
          />
        </Field>
        <Checkbox
          label={text.blocked}
          checked={shown.blocked}
          onChange={(blocked) => change.mutate({ blocked })}
          disabled={change.isPending}
          className="h-10"
        />
      </div>
      <ConfirmDialog
        open={forgetting}
        onClose={() => setForgetting(false)}
        onConfirm={() => forget.mutate()}
        title={text.forgetTitle(title)}
        confirmLabel={text.forget}
        tone="danger"
        busy={forget.isPending}
        error={forget.isError ? errorMessage(t, forget.error) : undefined}
      >
        {text.forgetBody}
      </ConfirmDialog>
    </li>
  )
}

/** What the profile panel shows: a profile's XML, or the XML of one being added or replaced. */
type ProfilePanel =
  | { kind: 'view'; profile: DlnaProfile }
  | { kind: 'edit'; profile: DlnaProfile | null; xml: string }

/** The built-in profiles, then the added ones, each with its XML, and Add a profile. */
function Profiles() {
  const { t } = useI18n()
  const text = t.settings.dlna
  const [panel, setPanel] = useState<ProfilePanel | null>(null)
  const profiles = useQuery({
    queryKey: queryKeys.dlnaProfiles,
    queryFn: ({ signal }) => fetchDlnaProfiles(signal),
  })
  if (profiles.isPending) return <SkeletonRows rows={3} label={t.common.loading} />
  if (profiles.isError) {
    return (
      <InlineError onRetry={() => void profiles.refetch()} retrying={profiles.isFetching}>
        {errorMessage(t, profiles.error)}
      </InlineError>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      <ul
        aria-label={text.profilesLabel}
        className="overflow-hidden rounded-panel border border-line-2 bg-s1"
      >
        {profiles.data.map((profile) => (
          <ProfileRow
            key={profile.id}
            profile={profile}
            onView={() => setPanel({ kind: 'view', profile })}
          />
        ))}
      </ul>
      <div>
        <Button icon={PlusIcon} onClick={() => setPanel({ kind: 'edit', profile: null, xml: '' })}>
          {text.addProfile}
        </Button>
      </div>
      {panel?.kind === 'view' && (
        <ProfileXml
          profile={panel.profile}
          onReplace={(xml) => setPanel({ kind: 'edit', profile: panel.profile, xml })}
          onClose={() => setPanel(null)}
        />
      )}
      {panel?.kind === 'edit' && (
        <ProfileEditor profile={panel.profile} initial={panel.xml} onClose={() => setPanel(null)} />
      )}
    </div>
  )
}

function ProfileRow({ profile, onView }: { profile: DlnaProfile; onView: () => void }) {
  const { t } = useI18n()
  const text = t.settings.dlna
  const toast = useToast()
  const [removing, setRemoving] = useState(false)
  const remove = useMutation({
    mutationFn: () => removeDlnaProfile(profile.id),
    onSuccess: () => {
      setRemoving(false)
      toast(text.profileRemoved(profile.name))
    },
    onSettled: (_data, error) => {
      // Devices forced to it go back to automatic.
      if (error === null || (error instanceof ApiError && error.status === 404)) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.dlna })
      }
    },
  })
  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 px-[22px] py-3.5 max-sm:px-4 [&+&]:border-t [&+&]:border-line">
      <div className="min-w-0 flex-1">
        <p className="text-[14.5px] font-medium break-words text-ink">{profile.name}</p>
        {profile.createdAt !== undefined && (
          <p className="text-small text-ink-3">
            {text.added} <RelativeTime iso={profile.createdAt} />
          </p>
        )}
      </div>
      <div className="flex items-center gap-1.5">
        {profile.builtin ? (
          <Badge>{text.builtin}</Badge>
        ) : (
          <Button
            variant="ghost"
            size="sm"
            aria-label={`${text.removeProfile}: ${profile.name}`}
            onClick={() => {
              remove.reset()
              setRemoving(true)
            }}
          >
            {text.removeProfile}
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          icon={FileCodeIcon}
          aria-label={`${text.viewXml}: ${profile.name}`}
          onClick={onView}
        >
          {text.viewXml}
        </Button>
      </div>
      <ConfirmDialog
        open={removing}
        onClose={() => setRemoving(false)}
        onConfirm={() => remove.mutate()}
        title={text.removeTitle(profile.name)}
        confirmLabel={text.removeProfile}
        tone="danger"
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {text.removeBody}
      </ConfirmDialog>
    </li>
  )
}

/** A profile's XML, read only; an added profile can be replaced from there. */
function ProfileXml({
  profile,
  onReplace,
  onClose,
}: {
  profile: DlnaProfile
  onReplace: (xml: string) => void
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.settings.dlna
  const xml = useQuery({
    queryKey: queryKeys.dlnaProfileXml(profile.id),
    queryFn: ({ signal }) => fetchDlnaProfileXml(profile.id, signal),
  })
  return (
    <Modal
      open
      onClose={onClose}
      title={`${text.profileXml} · ${profile.name}`}
      width={760}
      footer={
        <>
          {!profile.builtin && xml.data !== undefined && (
            <Button variant="secondary" onClick={() => onReplace(xml.data)}>
              {text.replaceProfile}
            </Button>
          )}
          <Button variant="primary" onClick={onClose}>
            {t.ui.close}
          </Button>
        </>
      }
    >
      <div className="p-5">
        {xml.isPending ? (
          <SkeletonRows rows={4} boxed={false} label={t.common.loading} />
        ) : xml.isError ? (
          <InlineError onRetry={() => void xml.refetch()} retrying={xml.isFetching}>
            {errorMessage(t, xml.error)}
          </InlineError>
        ) : (
          <pre className="overflow-x-auto rounded-row bg-s2 p-4 font-mono text-micro leading-relaxed whitespace-pre text-ink">
            {xml.data}
          </pre>
        )}
      </div>
    </Modal>
  )
}

/** Adds a profile (`profile` null) or replaces an added one, from XML pasted or opened. */
function ProfileEditor({
  profile,
  initial,
  onClose,
}: {
  profile: DlnaProfile | null
  initial: string
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.settings.dlna
  const toast = useToast()
  const file = useRef<HTMLInputElement>(null)
  const [xml, setXml] = useState(initial)
  const save = useMutation({
    mutationFn: () => saveDlnaProfile(xml, profile?.id),
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.dlna })
      toast(profile === null ? text.profileAdded(saved.name) : text.profileReplaced(saved.name), {
        tone: 'ok',
      })
      onClose()
    },
  })
  const edit = (value: string) => {
    save.reset()
    setXml(value)
  }
  // A profile refused says why, as the server tells it.
  const failure = save.isError ? errorMessage(t, save.error) : undefined
  const reason = save.error instanceof ApiError ? save.error.detail : null
  return (
    <Modal
      open
      onClose={onClose}
      title={profile === null ? text.addProfile : `${text.replaceProfile} · ${profile.name}`}
      width={760}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            loading={save.isPending}
            disabled={xml.trim() === ''}
            onClick={() => save.mutate()}
          >
            {profile === null
              ? save.isPending
                ? text.adding
                : text.add
              : save.isPending
                ? text.replacing
                : text.replaceProfile}
          </Button>
        </>
      }
    >
      <div className="space-y-4 p-5">
        <input
          ref={file}
          type="file"
          accept=".xml,application/xml,text/xml"
          className="sr-only"
          tabIndex={-1}
          onChange={(event) => {
            const chosen = event.target.files?.[0]
            event.target.value = ''
            if (chosen) void chosen.text().then(edit)
          }}
        />
        <Button variant="secondary" icon={UploadSimpleIcon} onClick={() => file.current?.click()}>
          {text.openFile}
        </Button>
        <CodeEditor
          label={text.profileXml}
          help={text.profileXmlHelp}
          value={xml}
          onValue={edit}
          maxBytes={maxProfileBytes}
          error={failure && (reason ? `${failure} ${reason}` : failure)}
        />
      </div>
    </Modal>
  )
}
