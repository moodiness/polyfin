import {
  ArrowCounterClockwiseIcon,
  CheckCircleIcon,
  KeyIcon,
  ShareNetworkIcon,
  TrashIcon,
  UserCircleDashedIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState, type FormEvent, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router'
import {
  ApiError,
  deleteUser,
  fetchGroups,
  fetchMetadataLanguages,
  fetchUserDevices,
  fetchUserShareLinks,
  fetchUsers,
  fetchWebThemes,
  queryClient,
  queryKeys,
  revokeShareLink,
  signOutUserDevice,
  turnOffUserTwoStep,
  unblockUser,
  userImageUrl,
  type User,
  type UserWebTheme,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { ShareLinkList } from '@/features/account/SharedLinks'
import { errorMessage, formatDate, languageOptions } from '@/format'
import { useI18n } from '@/i18n'
import {
  Avatar,
  Badge,
  Button,
  ButtonLink,
  ConfirmDialog,
  EmptyState,
  Field,
  InlineError,
  Modal,
  Notice,
  OpenRow,
  RelativeTime,
  SectionNav,
  Select,
  SettingRow,
  Skeleton,
  SkeletonRows,
  TextInput,
  useToast,
} from '@/ui'
import DeviceList from './DeviceList'
import { DraftForm, useDraftPart } from './draft'
import { Band, PageSection, posterUrl, useWatching, WatchingLink } from './rows'
import {
  clockTime,
  refreshIfGone,
  storeUser,
  useSectionInView,
  UserState,
  useUserPatch,
} from './shared'
import {
  AllowedHoursSection,
  BlockedGenresSection,
  FollowedNotice,
  followedGroup,
  LiveTvChannelsSection,
  ParentalControlSection,
  PermissionRows,
  PlaybackAccessSection,
  ResetDefaultsDialog,
  VisibleLibrariesSection,
  type LimitsTarget,
} from './UserForms'

const sectionIds = [
  'profile',
  'access',
  'metadata-language',
  'web-theme',
  'audio-region',
  'playback',
  'parental',
  'libraries',
  'genres',
  'hours',
  'channels',
  'share-links',
  'devices',
] as const

/** `/users/:id`: one user's page, with every permission in its section. */
export default function UserDetail({ id }: { id: string }) {
  const { t } = useI18n()
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  const user = users.data?.find((item) => item.id === id)
  const back = { to: '/users', label: t.users.title }

  if (users.isPending || (user === undefined && users.isFetching)) {
    return (
      <PageLayout title={<Skeleton className="h-8 w-48" />} back={back}>
        <SkeletonRows rows={4} boxed label={t.common.loading} />
      </PageLayout>
    )
  }
  if (users.isError) {
    return (
      <PageLayout title={t.users.title} back={back}>
        <InlineError onRetry={() => void users.refetch()} retrying={users.isFetching}>
          {errorMessage(t, users.error)}
        </InlineError>
      </PageLayout>
    )
  }
  if (user === undefined) {
    return (
      <PageLayout title={t.users.notFound} back={back}>
        <EmptyState
          icon={UserCircleDashedIcon}
          title={t.users.notFound}
          action={<ButtonLink to="/users">{t.users.backToUsers}</ButtonLink>}
        >
          {t.users.notFoundHelp}
        </EmptyState>
      </PageLayout>
    )
  }
  return <UserPage user={user} />
}

/** The address of a title's poster, as Now playing loads it. */
function UserPage({ user }: { user: User }) {
  const { t, language } = useI18n()
  const self = useSessionUser()
  const toast = useToast()
  const navigate = useNavigate()
  const { hash } = useLocation()
  const current = useSectionInView(sectionIds, true)
  const watching = useWatching(user.id)
  const [artFailed, setArtFailed] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [resetting, setResetting] = useState(false)
  // Each save, discard or reset draws the sections again from the saved values.
  const [version, setVersion] = useState(0)
  const target: LimitsTarget = { kind: 'user', user }
  // A member's own values change only here, while the values of a group they follow change with
  // it: those sections are drawn again from each copy.
  const followed = followedGroup(user)
  const formsKey = followed === null ? `own:${version}` : `group:${JSON.stringify(user)}`

  // The content arrives after the first render: a #section address scrolls once it is drawn.
  useEffect(() => {
    if (hash) document.getElementById(hash.slice(1))?.scrollIntoView({ block: 'start' })
  }, [hash])

  const remove = useMutation({
    mutationFn: () => deleteUser(user.id),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: queryKeys.userDevices(user.id) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast(t.users.deleted(user.name))
      void navigate('/users')
    },
    onError: refreshIfGone,
  })

  const sections: [string, string][] = [
    ['profile', t.users.profileTitle],
    ['access', t.users.accessTitle],
    ['metadata-language', t.users.metadataLanguageTitle],
    ['web-theme', t.users.webThemeTitle],
    ['audio-region', t.users.audioRegionTitle],
    ['playback', t.users.playbackAccessTitle],
    ['parental', t.users.parentalTitle],
    ['libraries', t.users.visibleLibrariesTitle],
    ['genres', t.users.blockedGenresTitle],
    ['hours', t.users.allowedHoursTitle],
    ['channels', t.users.liveTvChannelsTitle],
    ['share-links', t.users.shareLinksTitle],
    ['devices', t.users.devicesTitle],
  ]
  const nav = (
    <SectionNav
      label={t.users.sectionsLabel}
      current={current}
      items={sections.map(([id, label]) => ({ id, label, to: `#${id}` }))}
    />
  )

  const item = watching?.item ?? null
  const art = item?.posterId && artFailed !== item.posterId ? item.posterId : null
  const head = (
    <Band
      back={{ to: '/users', label: t.users.title }}
      art={art === null ? null : posterUrl(art, 300)}
      leading={
        <Avatar
          name={user.name}
          image={userImageUrl(user)}
          size="xl"
          className="max-md:size-16 max-md:text-[1.75rem]"
        />
      }
      title={user.name}
      facts={
        <>
          <span>{user.isAdministrator ? t.users.administrator : t.users.member}</span>
          <UserState user={user} />
          {user.id === self.id && <Badge>{t.users.you}</Badge>}
          {user.groupName !== null && (
            <Badge>
              {user.followGroup
                ? t.groups.groupBadge(user.groupName)
                : t.groups.groupOwnBadge(user.groupName)}
            </Badge>
          )}
          <span>
            {t.users.lastSignInLabel}{' '}
            {user.lastLoginAt === null ? t.common.never : <RelativeTime iso={user.lastLoginAt} />}
          </span>
          {user.endsOn !== null && (
            <span>
              {(user.ended ? t.users.endedOn : t.users.endsOn)(formatDate(user.endsOn, language))}
            </span>
          )}
        </>
      }
      extra={
        watching !== null && (
          <WatchingLink session={watching} art={art} onArtError={() => setArtFailed(art)} />
        )
      }
      actions={
        <>
          {/* A member following their group has its values: the group's page resets them. */}
          {followed === null && (
            <Button
              variant="secondary"
              icon={ArrowCounterClockwiseIcon}
              onClick={() => setResetting(true)}
            >
              {t.resetDefaults.button}
            </Button>
          )}
          <Button
            variant="danger"
            icon={TrashIcon}
            onClick={() => {
              remove.reset()
              setDeleting(true)
            }}
          >
            {t.users.delete}
          </Button>
        </>
      }
    />
  )

  return (
    <>
      <PageLayout head={head} nav={nav}>
        <DraftForm target={target} onVersion={() => setVersion((count) => count + 1)}>
          {({ nameError }) => (
            <>
              <ProfileSection key={`profile:${version}`} user={user} nameError={nameError} />
              <AccessSection user={user} />
              <MetadataLanguageSection user={user} />
              <WebThemeSection user={user} />
              <AudioRegionSection user={user} />
              <PlaybackAccessSection key={`playback:${formsKey}`} id="playback" target={target} />
              <ParentalControlSection key={`parental:${formsKey}`} id="parental" target={target} />
              <VisibleLibrariesSection
                key={`libraries:${formsKey}`}
                id="libraries"
                target={target}
              />
              <BlockedGenresSection key={`genres:${formsKey}`} id="genres" target={target} />
              <AllowedHoursSection key={`hours:${formsKey}`} id="hours" target={target} />
              <LiveTvChannelsSection key={`channels:${formsKey}`} id="channels" target={target} />
              <ShareLinksSection user={user} />
              <PageSection id="devices" title={t.users.devicesTitle}>
                <DeviceList
                  queryKey={queryKeys.userDevices(user.id)}
                  load={(signal) => fetchUserDevices(user.id, signal)}
                  signOut={(deviceId) => signOutUserDevice(user.id, deviceId)}
                />
              </PageSection>
            </>
          )}
        </DraftForm>
      </PageLayout>
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        onConfirm={() => remove.mutate()}
        title={t.users.delete}
        confirmLabel={remove.isPending ? t.users.deleting : t.users.delete}
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {t.users.deleteConfirm(user.name)}
      </ConfirmDialog>
      <ResetDefaultsDialog
        target={target}
        open={resetting}
        onClose={() => setResetting(false)}
        onReset={() => setVersion((count) => count + 1)}
      />
    </>
  )
}

/** The line under a section's title saying its changes are saved at once. */
function SavedAtOnce({ children }: { children: ReactNode }) {
  return (
    <span className="flex items-start gap-2.5">
      <CheckCircleIcon size="1.25rem" aria-hidden="true" className="mt-0.5 shrink-0 text-ink-3" />
      <span>{children}</span>
    </span>
  )
}

/**
 * Name and password: the reset PIN asked for from an app, the block for wrong passwords and
 * two-step sign-in, which an administrator turns off for a user who lost their device; then the
 * name, saved with the dock, and the password, reset in a dialog.
 */
function ProfileSection({ user, nameError }: { user: User; nameError: string | undefined }) {
  const { t, language } = useI18n()
  const toast = useToast()
  const self = useSessionUser()
  const [name, setName] = useState(user.name)
  const [askingTwoStep, setAskingTwoStep] = useState(false)
  const [resettingPassword, setResettingPassword] = useState(false)
  useDraftPart('profile', Number(name !== user.name), () => ({ name: name.trim() }))
  const unblock = useMutation({
    mutationFn: () => unblockUser(user.id),
    onSuccess: (updated) => {
      storeUser(updated)
      toast(t.users.unblocked)
    },
    onError: refreshIfGone,
  })
  const twoStepOff = useMutation({
    mutationFn: () => turnOffUserTwoStep(user.id),
    onSuccess: (updated) => {
      storeUser(updated)
      setAskingTwoStep(false)
      toast(t.users.twoStepTurnedOff)
    },
    onError: refreshIfGone,
  })
  const pin = user.passwordResetPin

  return (
    <PageSection id="profile" title={t.users.profileTitle}>
      {pin !== null && (
        <Notice tone="warn">
          <p className="text-ink">
            {t.users.resetPinRequested}{' '}
            <code className="figures text-lead font-semibold tracking-widest select-all">
              {pin.pin}
            </code>
            {t.users.resetPinValidUntil(clockTime(pin.expiresAt, language))}
          </p>
          <p className="mt-1 text-ink-2">{t.users.resetPinHelp}</p>
        </Notice>
      )}
      {user.blockedUntil !== null && (
        <Notice
          tone="danger"
          action={
            <Button size="sm" loading={unblock.isPending} onClick={() => unblock.mutate()}>
              {unblock.isPending ? t.users.unblocking : t.users.unblock}
            </Button>
          }
        >
          <p className="text-ink">{t.users.blockedUntil(clockTime(user.blockedUntil, language))}</p>
          <p className="mt-1 text-ink-2">{t.users.blockedHelp}</p>
        </Notice>
      )}
      {unblock.isError && <Notice tone="danger">{errorMessage(t, unblock.error)}</Notice>}
      {user.twoStep && (
        <Notice
          action={
            self.id !== user.id && (
              <Button size="sm" onClick={() => setAskingTwoStep(true)}>
                {t.users.turnOffTwoStep}
              </Button>
            )
          }
        >
          <p className="text-ink">{t.users.twoStepOn}</p>
          {self.id !== user.id && <p className="mt-1 text-ink-2">{t.users.twoStepHelp}</p>}
        </Notice>
      )}
      <SettingRow
        kind="text"
        label={t.users.name}
        help={t.common.nameRule}
        value={name}
        saved={user.name}
        maxLength={64}
        error={nameError}
        onChange={setName}
      />
      <OpenRow
        label={t.users.password}
        value={t.users.resetPassword}
        help={`${t.common.passwordRule} ${t.users.resetPasswordHelp}`}
        onClick={() => setResettingPassword(true)}
      />
      <PasswordModal
        user={user}
        open={resettingPassword}
        onClose={() => setResettingPassword(false)}
      />
      <ConfirmDialog
        open={askingTwoStep}
        onClose={() => {
          setAskingTwoStep(false)
          twoStepOff.reset()
        }}
        onConfirm={() => twoStepOff.mutate()}
        title={t.users.turnOffTwoStepTitle(user.name)}
        confirmLabel={twoStepOff.isPending ? t.users.turningOffTwoStep : t.users.turnOffTwoStep}
        busy={twoStepOff.isPending}
        error={twoStepOff.isError ? errorMessage(t, twoStepOff.error) : undefined}
      >
        {t.users.turnOffTwoStepConfirm(user.name)}
      </ConfirmDialog>
    </PageSection>
  )
}

/** A new password for the user, set at once; it signs out their Jellyfin devices. */
function PasswordModal({
  user,
  open,
  onClose,
}: {
  user: User
  open: boolean
  onClose: () => void
}) {
  const { t } = useI18n()
  const toast = useToast()
  const formId = useId()
  const [password, setPassword] = useState('')
  const resetPassword = useUserPatch(user)
  const error = resetPassword.isError ? errorMessage(t, resetPassword.error) : undefined

  function close() {
    resetPassword.reset()
    setPassword('')
    onClose()
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title={t.users.resetPassword}
      width={520}
      footer={
        <>
          <Button variant="ghost" onClick={close}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            icon={KeyIcon}
            loading={resetPassword.isPending}
          >
            {resetPassword.isPending ? t.common.saving : t.users.resetPassword}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        className="p-5 sm:px-8"
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault()
          resetPassword.mutate(
            { password },
            {
              onSuccess: () => {
                toast(t.users.passwordReset)
                close()
              },
            },
          )
        }}
      >
        <Field
          label={t.users.newPassword}
          help={`${t.common.passwordRule} ${t.users.resetPasswordHelp}`}
          error={error}
        >
          <TextInput
            type="password"
            revealable
            value={password}
            onValue={(value) => {
              resetPassword.reset()
              setPassword(value)
            }}
            autoComplete="new-password"
            required
            invalid={error !== undefined}
          />
        </Field>
      </form>
    </Modal>
  )
}

/**
 * What applies at once: role, sign-in screen, disabled, the end date, the group, and the user's
 * rights, which a group the user follows sets.
 */
function AccessSection({ user }: { user: User }) {
  const { t, language } = useI18n()
  const toast = useToast()
  const access = useUserPatch(user)
  // Joining a group and following it again replace the user's values: both are asked first.
  const [asked, setAsked] = useState<{ groupId: string; name: string } | 'follow' | null>(null)
  // Losing the permission, or being disabled, revokes the user's share links. A change waits for
  // the one before it, so that no row is turned off under the keyboard meanwhile.
  const change = (patch: Parameters<typeof access.mutate>[0]) => {
    if (access.isPending) return
    access.mutate(patch, {
      onSuccess: () => {
        setAsked(null)
        toast(t.users.updated)
        void queryClient.invalidateQueries({ queryKey: queryKeys.userShareLinks(user.id) })
      },
    })
  }
  const lastAdmin = access.error instanceof ApiError && access.error.code === 'last_administrator'
  const followed = followedGroup(user)

  return (
    <PageSection
      id="access"
      title={t.users.accessTitle}
      description={<SavedAtOnce>{`${t.users.accessHelp} ${t.users.lastAdminHelp}`}</SavedAtOnce>}
    >
      {access.isError && asked === null && (
        <Notice tone={lastAdmin ? 'warn' : 'danger'} live>
          {errorMessage(t, access.error)}
        </Notice>
      )}
      <SettingRow
        kind="switch"
        label={t.users.isAdministrator}
        help={
          user.groupName === null
            ? t.users.isAdministratorHelp
            : `${t.users.isAdministratorHelp} ${t.groups.administratorLeaves(user.groupName)}`
        }
        checked={user.isAdministrator}
        onChange={(isAdministrator) => change({ isAdministrator })}
      />
      <SettingRow
        kind="switch"
        label={t.users.showOnSignIn}
        help={t.users.showOnSignInHelp}
        checked={!user.isHidden}
        onChange={(shown) => change({ isHidden: !shown })}
      />
      <SettingRow
        kind="switch"
        label={t.users.isDisabled}
        help={t.users.isDisabledHelp}
        checked={user.isDisabled}
        onChange={(isDisabled) => change({ isDisabled })}
      />
      <SettingRow
        kind="switch"
        label={t.users.providerOnly}
        help={t.users.providerOnlyHelp}
        checked={user.providerOnly}
        onChange={(providerOnly) => change({ providerOnly })}
      />
      {user.isAdministrator ? (
        <OpenRow
          label={t.users.endTitle}
          value={t.users.endNever}
          note={t.users.endAdministratorHelp}
          disabled
        />
      ) : (
        <OpenRow
          label={t.users.endTitle}
          value={user.endsOn === null ? t.users.endNever : formatDate(user.endsOn, language)}
          help={t.users.endHelp}
        >
          <EndChoice
            key={user.endsOn ?? ''}
            user={user}
            pending={access.isPending}
            onSave={(endsOn) => change({ endsOn })}
          />
        </OpenRow>
      )}
      <GroupRow
        user={user}
        onChoose={(group) => {
          access.reset()
          if (group === null) change({ groupId: '' })
          else setAsked({ groupId: group.id, name: group.name })
        }}
      />
      {user.groupId !== null && (
        <SettingRow
          kind="switch"
          label={t.groups.followGroup}
          help={t.groups.followGroupHelp}
          checked={user.followGroup}
          onChange={(follow) => {
            access.reset()
            if (follow) setAsked('follow')
            else change({ followGroup: false })
          }}
        />
      )}
      {followed !== null && <FollowedNotice group={followed} />}
      <PermissionRows values={user} disabled={followed !== null} onChange={change} />
      <ConfirmDialog
        open={asked !== null}
        onClose={() => setAsked(null)}
        onConfirm={() =>
          asked === 'follow'
            ? change({ followGroup: true })
            : asked !== null && change({ groupId: asked.groupId })
        }
        title={asked === 'follow' ? t.groups.followAgainTitle : t.groups.joinTitle}
        confirmLabel={
          access.isPending
            ? t.common.saving
            : asked === 'follow'
              ? t.groups.followAgain
              : t.groups.join
        }
        tone="primary"
        busy={access.isPending}
        error={access.isError ? errorMessage(t, access.error) : undefined}
      >
        {asked === 'follow'
          ? t.groups.followAgainConfirm(user.name, user.groupName ?? '')
          : asked !== null && t.groups.joinConfirm(user.name, asked.name)}
      </ConfirmDialog>
    </PageSection>
  )
}

/**
 * The group the user belongs to, or No group, chosen from the list. An administrator cannot join
 * one, as their permissions differ.
 */
function GroupRow({
  user,
  onChoose,
}: {
  user: User
  onChoose: (group: { id: string; name: string } | null) => void
}) {
  const { t } = useI18n()
  const groups = useQuery({
    queryKey: queryKeys.groups,
    queryFn: ({ signal }) => fetchGroups(signal),
    enabled: !user.isAdministrator,
  })
  const current = user.groupName ?? t.groups.noGroup
  if (user.isAdministrator) {
    return (
      <OpenRow
        label={t.groups.group}
        value={current}
        note={t.groups.administratorNoGroup}
        disabled
      />
    )
  }
  const options = [
    { value: '', label: t.groups.noGroup },
    ...(groups.data ?? []).map((group) => ({ value: group.id, label: group.name })),
  ]
  // The user's group shows while the list loads.
  if (user.groupId !== null && !options.some((option) => option.value === user.groupId)) {
    options.push({ value: user.groupId, label: user.groupName ?? '' })
  }

  return (
    <OpenRow
      label={t.groups.group}
      value={current}
      help={t.groups.groupHelp}
      error={groups.isError ? errorMessage(t, groups.error) : undefined}
    >
      <Select
        size="sm"
        value={user.groupId ?? ''}
        options={options}
        onValue={(id) => {
          if (id === (user.groupId ?? '')) return
          const name = options.find((option) => option.value === id)?.label ?? ''
          onChoose(id === '' ? null : { id, name })
        }}
        aria-label={t.groups.group}
        className="max-w-[26.25rem]"
      />
    </OpenRow>
  )
}

/** When the account ends: a date, saved with its button, or Never, which clears it. */
function EndChoice({
  user,
  pending,
  onSave,
}: {
  user: User
  pending: boolean
  onSave: (endsOn: string | null) => void
}) {
  const { t } = useI18n()
  const [end, setEnd] = useState(user.endsOn ?? '')
  const savable = !pending && end !== '' && end !== user.endsOn

  return (
    <div className="flex flex-wrap items-center gap-3">
      <TextInput
        type="date"
        size="sm"
        value={end}
        onValue={setEnd}
        aria-label={t.users.endDate}
        onKeyDown={(event) => {
          // Enter saves the date rather than the page.
          if (event.key !== 'Enter') return
          event.preventDefault()
          if (savable) onSave(end)
        }}
        className="w-48"
      />
      <Button size="sm" disabled={!savable} onClick={() => onSave(end)}>
        {t.users.saveEnd}
      </Button>
      {user.endsOn !== null && (
        <Button size="sm" variant="ghost" disabled={pending} onClick={() => onSave(null)}>
          {t.users.endNever}
        </Button>
      )}
    </div>
  )
}

/**
 * The user's metadata language: The server's, or one the server's addons were given, saved as it
 * changes. Without a language given to any addon it says where one is given.
 */
function MetadataLanguageSection({ user }: { user: User }) {
  const { t, language } = useI18n()
  const toast = useToast()
  const patch = useUserPatch(user)
  const languages = useQuery({
    queryKey: queryKeys.metadataLanguages,
    queryFn: ({ signal }) => fetchMetadataLanguages(signal),
  })
  const value = user.metadataLanguage
  const codes =
    languages.data === undefined
      ? []
      : value === '' || languages.data.includes(value)
        ? languages.data
        : [...languages.data, value]
  const options = [
    { value: '', label: t.account.metadataLanguage.server },
    ...languageOptions(codes, language),
  ]

  return (
    <PageSection id="metadata-language" title={t.users.metadataLanguageTitle}>
      {languages.isPending ? (
        <Skeleton className="h-[5.125rem] rounded-row max-md:h-[4.25rem]" />
      ) : languages.isError ? (
        <InlineError onRetry={() => void languages.refetch()} retrying={languages.isFetching}>
          {errorMessage(t, languages.error)}
        </InlineError>
      ) : (
        <OpenRow
          label={t.users.metadataLanguageRow}
          value={options.find((option) => option.value === value)?.label ?? value}
          help={t.users.metadataLanguageHelp}
          error={patch.isError ? errorMessage(t, patch.error) : undefined}
          note={
            languages.data.length === 0 && value === '' ? t.users.metadataLanguageNone : undefined
          }
        >
          <Select
            size="sm"
            value={value}
            options={options}
            onValue={(metadataLanguage) =>
              patch.mutate({ metadataLanguage }, { onSuccess: () => toast(t.users.updated) })
            }
            disabled={patch.isPending}
            aria-label={t.users.metadataLanguageRow}
            className="max-w-[26.25rem]"
          />
        </OpenRow>
      )}
    </PageSection>
  )
}

/**
 * The user's web player theme: The server's, or one the server offers, saved as it changes, and
 * whether they may change it themselves under My account.
 */
function WebThemeSection({ user }: { user: User }) {
  const { t } = useI18n()
  const text = t.account.webTheme
  const toast = useToast()
  const patch = useUserPatch(user)
  const themes = useQuery({
    queryKey: queryKeys.webThemes,
    queryFn: ({ signal }) => fetchWebThemes(signal),
  })
  const value = user.webTheme
  const save = (change: Parameters<typeof patch.mutate>[0]) => {
    if (!patch.isPending) patch.mutate(change, { onSuccess: () => toast(t.users.updated) })
  }

  let rows
  if (themes.isPending) rows = <Skeleton className="h-[5.125rem] rounded-row max-md:h-[4.25rem]" />
  else if (themes.isError)
    rows = (
      <InlineError onRetry={() => void themes.refetch()} retrying={themes.isFetching}>
        {errorMessage(t, themes.error)}
      </InlineError>
    )
  else {
    // A theme no longer offered, such as Custom once its texts are emptied, stays listed while it
    // is the one chosen.
    const offered =
      value === '' || themes.data.themes.includes(value)
        ? themes.data.themes
        : [...themes.data.themes, value]
    const options: { value: UserWebTheme; label: string }[] = [
      { value: '', label: text.server(text.choices[themes.data.serverTheme]) },
      ...offered.map((theme) => ({ value: theme, label: text.choices[theme] })),
    ]
    rows = (
      <>
        <OpenRow
          label={t.users.webThemeRow}
          value={options.find((option) => option.value === value)?.label}
          help={t.users.webThemeHelp}
          error={patch.isError ? errorMessage(t, patch.error) : undefined}
        >
          <Select<UserWebTheme>
            size="sm"
            value={value}
            options={options}
            onValue={(webTheme) => save({ webTheme })}
            disabled={patch.isPending}
            aria-label={t.users.webThemeRow}
            className="max-w-[26.25rem]"
          />
        </OpenRow>
        <SettingRow
          kind="switch"
          label={t.users.webThemeChange}
          help={t.users.webThemeChangeHelp}
          checked={!user.webThemeLocked}
          onChange={(change) => save({ webThemeLocked: !change })}
        />
      </>
    )
  }
  return (
    <PageSection id="web-theme" title={t.users.webThemeTitle}>
      {rows}
    </PageSection>
  )
}

/** The user's audio region: None, or one of the regions offered, saved as it changes. */
function AudioRegionSection({ user }: { user: User }) {
  const { t } = useI18n()
  const text = t.account.audioRegion
  const toast = useToast()
  const patch = useUserPatch(user)
  const options = [
    { value: '', label: text.none },
    ...user.audioRegions.map((region) => ({
      value: region,
      label: text.regions[region] ?? region,
    })),
  ]
  return (
    <PageSection id="audio-region" title={t.users.audioRegionTitle}>
      <OpenRow
        label={t.users.audioRegionRow}
        value={
          options.find((option) => option.value === user.audioRegion)?.label ?? user.audioRegion
        }
        help={t.users.audioRegionHelp}
        error={patch.isError ? errorMessage(t, patch.error) : undefined}
      >
        <Select
          size="sm"
          value={user.audioRegion}
          options={options}
          onValue={(audioRegion) =>
            patch.mutate({ audioRegion }, { onSuccess: () => toast(t.users.updated) })
          }
          disabled={patch.isPending}
          aria-label={t.users.audioRegionRow}
          className="max-w-[26.25rem]"
        />
      </OpenRow>
    </PageSection>
  )
}

/** The user's share links, for an administrator, each with Revoke while it is active. */
function ShareLinksSection({ user }: { user: User }) {
  const { t } = useI18n()
  const links = useQuery({
    queryKey: queryKeys.userShareLinks(user.id),
    queryFn: ({ signal }) => fetchUserShareLinks(user.id, signal),
  })

  return (
    <PageSection
      id="share-links"
      title={t.users.shareLinksTitle}
      description={t.users.shareLinksHelp}
    >
      {links.isPending ? (
        <SkeletonRows rows={2} boxed label={t.common.loading} />
      ) : links.isError ? (
        <InlineError onRetry={() => void links.refetch()} retrying={links.isFetching}>
          {errorMessage(t, links.error)}
        </InlineError>
      ) : links.data.length === 0 ? (
        <EmptyState icon={ShareNetworkIcon} title={t.shareLinks.userEmpty} />
      ) : (
        <ShareLinkList
          links={links.data}
          queryKey={queryKeys.userShareLinks(user.id)}
          revoke={revokeShareLink}
        />
      )}
    </PageSection>
  )
}
