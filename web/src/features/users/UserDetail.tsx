import {
  ArrowCounterClockwiseIcon,
  CheckCircleIcon,
  DotsThreeIcon,
  LinkIcon,
  ShareNetworkIcon,
  TrashIcon,
  UserCircleDashedIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import {
  ApiError,
  deleteUser,
  fetchGroups,
  fetchImportTitleCounts,
  fetchMetadataLanguages,
  fetchSettings,
  fetchStatus,
  fetchUserDevices,
  fetchUserMoonfin,
  fetchUserShareLinks,
  fetchUsers,
  fetchWebThemes,
  forgetUserMoonfin,
  groupSections,
  queryClient,
  queryKeys,
  revokeShareLink,
  signOutUserDevice,
  turnOffUserTwoStep,
  unblockUser,
  userImageUrl,
  type GroupSection,
  type User,
  type UserMoonfin,
  type UserWebTheme,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { ShareLinkList } from '@/features/account/SharedLinks'
import { moonfinOptions, MoonfinSyncRow } from '@/features/account/Moonfin'
import { UserMoonfinPin } from '@/features/account/MoonfinPin'
import { sourceName, TitlesNotFoundDialog } from '@/features/imports/TitlesNotFound'
import { errorMessage, languageOptions } from '@/format'
import { useI18n } from '@/i18n'
import {
  Avatar,
  Badge,
  Button,
  ButtonLink,
  ConfirmDialog,
  EmptyState,
  IconButton,
  InlineError,
  Menu,
  MenuItem,
  Notice,
  OpenRow,
  Select,
  SettingRow,
  Skeleton,
  SkeletonRows,
  TextInput,
  TextLink,
  useToast,
} from '@/ui'
import DeviceList from './DeviceList'
import PasswordModal from './PasswordModal'
import { DraftForm, useDraftPart } from './draft'
import {
  Band,
  PageSection,
  posterUrl,
  ShownSections,
  useSectionPages,
  useWatching,
  WatchingLink,
  type SectionGroup,
} from './rows'
import {
  accountEnd,
  clockTime,
  LastActivity,
  refreshIfGone,
  storeUser,
  UserState,
  useUserPatch,
} from './shared'
import {
  AllowedHoursSection,
  BlockedGenresSection,
  followedGroup,
  LiveTvChannelsSection,
  memberGroup,
  ParentalControlSection,
  PermissionRows,
  PlaybackAccessSection,
  ResetDefaultsDialog,
  sectionTitle,
  SectionValues,
  VisibleLibrariesSection,
  withOwnSection,
  type LimitsTarget,
} from './UserForms'

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

/** One user's page: their band, then their sections, a page each on a phone. */
function UserPage({ user }: { user: User }) {
  const { t, language } = useI18n()
  const self = useSessionUser()
  const toast = useToast()
  const navigate = useNavigate()
  const watching = useWatching(user.id)
  const [artFailed, setArtFailed] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [resetting, setResetting] = useState(false)
  // Each save, discard or reset draws the sections again from the saved values.
  const [version, setVersion] = useState(0)
  const target: LimitsTarget = { kind: 'user', user }
  // A section is drawn again when its values are kept or follow the group again, or the group
  // changes; one following the group also as the group's values reach it.
  const formsKey = (section: GroupSection) =>
    `${section}:${version}:${user.groupId}:${user.ownSections.includes(section)}`

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

  const userGroup = memberGroup(user)
  const following = groupSections.filter((section) => !user.ownSections.includes(section))
  // On a phone the list of sections says which ones follow the group.
  const held = (section: GroupSection) => ({
    id: section,
    label: sectionTitle(t, section),
    note:
      userGroup !== null && following.includes(section)
        ? t.groups.valuesFromGroup(userGroup.name)
        : undefined,
  })
  const sections: SectionGroup[] = [
    {
      id: 'account',
      label: t.users.navAccount,
      items: [{ id: 'profile', label: t.users.profileTitle }, held('access')],
    },
    {
      id: 'preferences',
      label: t.users.navPreferences,
      items: [
        { id: 'metadata-language', label: t.users.metadataLanguageTitle },
        { id: 'web-theme', label: t.users.webThemeTitle },
        { id: 'moonfin', label: t.users.moonfinTitle },
        { id: 'audio-region', label: t.users.audioRegionTitle },
      ],
    },
    {
      id: 'limits',
      label: t.users.navLimits,
      items: (['playback', 'parental', 'libraries', 'genres', 'hours', 'channels'] as const).map(
        held,
      ),
    },
    {
      id: 'activity',
      label: t.users.navActivity,
      items: [
        { id: 'share-links', label: t.users.shareLinksTitle },
        { id: 'import-titles', label: t.importTitles.sectionTitle },
        { id: 'devices', label: t.users.devicesTitle },
      ],
    },
  ]

  const end = accountEnd(user, t, language)
  const item = watching?.item ?? null
  const art = item?.posterId && artFailed !== item.posterId ? item.posterId : null
  const more = t.users.moreActions(user.name)
  const band = (
    <Band
      back={{ to: '/users', label: t.users.title }}
      art={art === null ? null : posterUrl(art, 300)}
      leading={<Avatar name={user.name} image={userImageUrl(user)} size="xl" />}
      title={user.name}
      facts={
        <>
          <span>{user.isAdministrator ? t.users.administrator : t.users.member}</span>
          <UserState user={user} />
          {user.id === self.id && <Badge>{t.users.you}</Badge>}
          <span>
            <LastActivity user={user} />
          </span>
          {end !== null && <span>{(user.ended ? t.users.endedOn : t.users.endsOn)(end)}</span>}
        </>
      }
      extra={
        <>
          {/* Said once for the page: each section it follows the group in carries the choice. */}
          {userGroup !== null && (
            // The link's own padding sets it apart on the line, and starts it flush when wrapped.
            <p className="mt-1.5 flex flex-wrap items-center gap-x-1 text-control text-ink-2">
              <span className="mr-3">
                {t.groups.follows(userGroup.name, following.length, groupSections.length)}
              </span>
              <TextLink to={`/users/groups/${userGroup.id}`} className="-ml-3">
                {t.groups.openGroup}
              </TextLink>
            </p>
          )}
          {watching !== null && (
            <WatchingLink session={watching} art={art} onArtError={() => setArtFailed(art)} />
          )}
        </>
      }
      actions={
        <Menu
          label={more}
          align="end"
          trigger={(props) => <IconButton {...props} label={more} icon={DotsThreeIcon} />}
        >
          {/* A member following their group everywhere has its values: its page resets them. */}
          {(userGroup === null || user.ownSections.length > 0) && (
            <MenuItem icon={ArrowCounterClockwiseIcon} onSelect={() => setResetting(true)}>
              {t.resetDefaults.button}
            </MenuItem>
          )}
          <MenuItem
            icon={TrashIcon}
            danger
            onSelect={() => {
              remove.reset()
              setDeleting(true)
            }}
          >
            {t.users.delete}
          </MenuItem>
        </Menu>
      }
    />
  )
  const pages = useSectionPages({
    label: t.users.sectionsLabel,
    name: user.name,
    groups: sections,
    band,
  })

  return (
    <>
      <PageLayout head={pages.head} nav={pages.nav}>
        <ShownSections shown={pages.shown}>
          <DraftForm target={target} onVersion={() => setVersion((count) => count + 1)}>
            {({ nameError }) => (
              <>
                <ProfileSection key={`profile:${version}`} user={user} nameError={nameError} />
                <AccessSection user={user} />
                <MetadataLanguageSection user={user} />
                <WebThemeSection user={user} />
                <MoonfinSection user={user} />
                <AudioRegionSection user={user} />
                <PlaybackAccessSection key={formsKey('playback')} id="playback" target={target} />
                <ParentalControlSection key={formsKey('parental')} id="parental" target={target} />
                <VisibleLibrariesSection
                  key={formsKey('libraries')}
                  id="libraries"
                  target={target}
                />
                <BlockedGenresSection key={formsKey('genres')} id="genres" target={target} />
                <AllowedHoursSection key={formsKey('hours')} id="hours" target={target} />
                <LiveTvChannelsSection key={formsKey('channels')} id="channels" target={target} />
                <ShareLinksSection user={user} />
                <ImportTitlesSection user={user} />
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
        </ShownSections>
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

/**
 * What applies at once: role, sign-in screen, whether the account can sign in, the end date, the
 * group, and the user's rights, which the group of a member sets while Access follows it. What a
 * tap cannot take back alone is asked first, naming its effects: the administrator's role either
 * way, turning sign-in off, joining a group and following it again.
 */
function AccessSection({ user }: { user: User }) {
  const { t, language } = useI18n()
  const toast = useToast()
  const access = useUserPatch(user)
  // The identity provider the server offers: the account's link to it shows while there is one.
  const provider = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
  }).data?.provider
  const [asked, setAsked] = useState<
    | { groupId: string; name: string }
    | 'access'
    | 'everywhere'
    | 'administrator'
    | 'demote'
    | 'disable'
    | null
  >(null)
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
  const userGroup = memberGroup(user)
  const accessOwn = user.ownSections.includes('access')
  const followed = followedGroup(user, 'access')
  // How many devices turning sign-in off signs out, read when asked.
  const devices = useQuery({
    queryKey: queryKeys.userDevices(user.id),
    queryFn: ({ signal }) => fetchUserDevices(user.id, signal),
    enabled: asked === 'disable',
  }).data?.length

  // The confirmation of what is asked: its title, its words, its button and what it applies.
  const confirm =
    asked === null
      ? null
      : asked === 'administrator'
        ? {
            title: t.users.makeAdministratorTitle(user.name),
            body: t.users.makeAdministratorConfirm(user.name, user.groupName),
            label: t.users.makeAdministrator,
            tone: 'primary' as const,
            apply: () => change({ isAdministrator: true }),
          }
        : asked === 'demote'
          ? {
              title: t.users.removeAdministratorTitle(user.name),
              body: t.users.removeAdministratorConfirm(user.name),
              label: t.users.removeAdministrator,
              tone: 'danger' as const,
              apply: () => change({ isAdministrator: false }),
            }
          : asked === 'disable'
            ? {
                title: t.users.disableTitle(user.name),
                body: t.users.disableConfirm(user.name, devices ?? null),
                label: t.users.disable,
                tone: 'danger' as const,
                apply: () => change({ isDisabled: true }),
              }
            : asked === 'access'
              ? {
                  title: t.groups.followSectionTitle(sectionTitle(t, 'access')),
                  body: t.groups.followSectionConfirm(
                    user.name,
                    userGroup?.name ?? '',
                    t.users.accessTitle,
                  ),
                  label: t.groups.followAgain,
                  tone: 'primary' as const,
                  apply: () =>
                    change({ ownSections: withOwnSection(user.ownSections, 'access', false) }),
                }
              : asked === 'everywhere'
                ? {
                    title: t.groups.followEverywhereTitle,
                    body: t.groups.followEverywhereConfirm(user.name, userGroup?.name ?? ''),
                    label: t.groups.followAgain,
                    tone: 'primary' as const,
                    apply: () => change({ ownSections: [] }),
                  }
                : {
                    title: t.groups.joinTitle,
                    body: t.groups.joinConfirm(user.name, asked.name),
                    label: t.groups.join,
                    tone: 'primary' as const,
                    apply: () => change({ groupId: asked.groupId }),
                  }
  const ask = (what: NonNullable<typeof asked>) => {
    access.reset()
    setAsked(what)
  }

  return (
    <PageSection
      id="access"
      title={t.users.accessTitle}
      description={<SavedAtOnce>{`${t.users.accessHelp} ${t.users.lastAdminHelp}`}</SavedAtOnce>}
      aside={
        userGroup !== null && (
          <SectionValues
            title={t.users.accessTitle}
            group={userGroup}
            own={accessOwn}
            onChange={(own) => {
              if (!own) ask('access')
              else {
                access.reset()
                change({ ownSections: withOwnSection(user.ownSections, 'access', true) })
              }
            }}
          />
        )
      }
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
        onChange={(isAdministrator) => ask(isAdministrator ? 'administrator' : 'demote')}
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
        label={t.users.canSignIn}
        help={t.users.canSignInHelp}
        checked={!user.isDisabled}
        onChange={(canSignIn) => (canSignIn ? change({ isDisabled: false }) : ask('disable'))}
      />
      <SettingRow
        kind="switch"
        label={t.users.providerOnly}
        help={t.users.providerOnlyHelp}
        checked={user.providerOnly}
        onChange={(providerOnly) => change({ providerOnly })}
      />
      {/* Linked, the account says so. Until then an administrator may have it linked at the next
          sign-in under its name, but for an administrator or an account with two-step sign-in,
          which link themselves under My account. */}
      {provider != null &&
        (user.providerLinked ? (
          <OpenRow
            label={t.users.providerLink}
            value={t.users.providerLinked}
            note={t.users.providerLinkedHelp}
            disabled
          />
        ) : (
          <SettingRow
            kind="switch"
            label={t.users.providerLinkAllowed}
            help={t.users.providerLinkAllowedHelp}
            checked={user.providerLinkAllowed}
            disabled={user.isAdministrator || user.twoStep}
            onChange={(providerLinkAllowed) => change({ providerLinkAllowed })}
          />
        ))}
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
          value={accountEnd(user, t, language) ?? t.users.endNever}
          help={t.users.endHelp}
        >
          <EndChoice
            key={user.endsAt ?? user.endsOn ?? ''}
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
      {user.ownSections.length > 0 && (
        <OpenRow
          label={t.groups.followEverywhere}
          help={t.groups.ownSections(
            user.ownSections.map((section) => sectionTitle(t, section)).join(', '),
          )}
          onClick={() => ask('everywhere')}
        />
      )}
      <PermissionRows
        values={user}
        readOnly={followed !== null}
        forGroup={false}
        onChange={change}
      />
      <ConfirmDialog
        open={confirm !== null}
        onClose={() => setAsked(null)}
        onConfirm={() => confirm?.apply()}
        title={confirm?.title ?? ''}
        confirmLabel={access.isPending ? t.common.saving : (confirm?.label ?? '')}
        tone={confirm?.tone}
        busy={access.isPending}
        error={access.isError ? errorMessage(t, access.error) : undefined}
      >
        {confirm?.body}
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

/**
 * When the account ends: a date, saved with its button, or Never, which clears it. Either replaces
 * the exact end an invite link set, which the date field leaves empty: only these buttons change it.
 */
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
      {(user.endsOn !== null || user.endsAt !== null) && (
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
        <Skeleton className="h-15 rounded-row" />
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
  if (themes.isPending) rows = <Skeleton className="h-15 rounded-row" />
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

/**
 * Whether Moonfin's companion serves the user: the server's default, naming whether it is on, On
 * or Off, saved as it changes, and whether they may change it themselves under My account; then
 * the settings Moonfin's apps synced for them, with Reset, and their sign-in PIN.
 */
function MoonfinSection({ user }: { user: User }) {
  const { t } = useI18n()
  const text = t.account.moonfin
  const toast = useToast()
  const patch = useUserPatch(user)
  const settings = useQuery({
    queryKey: queryKeys.settings,
    queryFn: ({ signal }) => fetchSettings(signal),
  })
  const sync = useQuery({
    queryKey: queryKeys.userMoonfin(user.id),
    queryFn: ({ signal }) => fetchUserMoonfin(user.id, signal),
  })
  const save = (change: Parameters<typeof patch.mutate>[0]) => {
    if (!patch.isPending) patch.mutate(change, { onSuccess: () => toast(t.users.updated) })
  }

  let choice
  if (settings.isPending) choice = <Skeleton className="h-15 rounded-row" />
  else if (settings.isError)
    choice = (
      <InlineError onRetry={() => void settings.refetch()} retrying={settings.isFetching}>
        {errorMessage(t, settings.error)}
      </InlineError>
    )
  else {
    const options = moonfinOptions(settings.data.moonfin, t)
    choice = (
      <OpenRow
        anchor="moonfin-choice"
        label={t.users.moonfinRow}
        value={options.find((option) => option.value === user.moonfin)?.label}
        help={t.users.moonfinHelp}
        error={patch.isError ? errorMessage(t, patch.error) : undefined}
      >
        <Select<UserMoonfin>
          size="sm"
          value={user.moonfin}
          options={options}
          onValue={(moonfin) => save({ moonfin })}
          disabled={patch.isPending}
          aria-label={t.users.moonfinRow}
          className="max-w-[26.25rem]"
        />
      </OpenRow>
    )
  }

  return (
    <PageSection id="moonfin" title={t.users.moonfinTitle} description={t.users.moonfinSectionHelp}>
      {choice}
      <SettingRow
        kind="switch"
        label={t.users.moonfinChange}
        help={t.users.moonfinChangeHelp}
        checked={!user.moonfinLocked}
        onChange={(change) => save({ moonfinLocked: !change })}
      />
      {sync.isPending ? (
        <Skeleton className="h-15 rounded-row" />
      ) : sync.isError ? (
        <InlineError onRetry={() => void sync.refetch()} retrying={sync.isFetching}>
          {errorMessage(t, sync.error)}
        </InlineError>
      ) : (
        <MoonfinSyncRow
          sync={sync.data}
          forget={() => forgetUserMoonfin(user.id)}
          onForgotten={() => {
            void queryClient.invalidateQueries({ queryKey: queryKeys.userMoonfin(user.id) })
            void queryClient.invalidateQueries({ queryKey: queryKeys.moonfinDefaults })
          }}
          button={text.resetUser}
          title={t.users.moonfinResetTitle(user.name)}
          confirm={t.users.moonfinResetConfirm}
        />
      )}
      <UserMoonfinPin id={user.id} name={user.name} />
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

/**
 * The titles the user's imports did not find, for an administrator: each source with how many are
 * not linked and linked, which opens its list, where they are linked for the user.
 */
function ImportTitlesSection({ user }: { user: User }) {
  const { t } = useI18n()
  const text = t.importTitles
  const [source, setSource] = useState<string | null>(null)
  const scope = { userId: user.id }
  const counts = useQuery({
    queryKey: queryKeys.importTitleCounts(scope),
    queryFn: ({ signal }) => fetchImportTitleCounts(scope, signal),
  })

  return (
    <PageSection id="import-titles" title={text.sectionTitle} description={text.sectionHelp}>
      {counts.isPending ? (
        <SkeletonRows rows={2} boxed label={t.common.loading} />
      ) : counts.isError ? (
        <InlineError onRetry={() => void counts.refetch()} retrying={counts.isFetching}>
          {errorMessage(t, counts.error)}
        </InlineError>
      ) : counts.data.length === 0 ? (
        <EmptyState icon={LinkIcon} title={text.sectionNone} />
      ) : (
        counts.data.map((count) => (
          <OpenRow
            key={count.source}
            label={sourceName(t, count.source)}
            value={[
              count.notFound > 0 ? text.notFound(count.notFound) : '',
              count.linked > 0 ? text.linkedCount(count.linked) : '',
            ]
              .filter(Boolean)
              .join(' · ')}
            onClick={() => setSource(count.source)}
          />
        ))
      )}
      <TitlesNotFoundDialog scope={scope} source={source} onClose={() => setSource(null)} />
    </PageSection>
  )
}
