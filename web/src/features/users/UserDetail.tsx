import {
  ArrowCounterClockwiseIcon,
  ClockIcon,
  DevicesIcon,
  FilmSlateIcon,
  IdentificationCardIcon,
  KeyIcon,
  PaletteIcon,
  PlayCircleIcon,
  ProhibitIcon,
  ShareNetworkIcon,
  ShieldCheckIcon,
  TrashIcon,
  UserCircleDashedIcon,
  BooksIcon,
  SpeakerHighIcon,
  TranslateIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState, type FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router'
import {
  ApiError,
  deleteUser,
  fetchGroups,
  fetchUserDevices,
  fetchUserShareLinks,
  fetchMetadataLanguages,
  fetchWebThemes,
  fetchUsers,
  queryClient,
  queryKeys,
  revokeShareLink,
  signOutUserDevice,
  turnOffUserTwoStep,
  unblockUser,
  userImageUrl,
  type User,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { ShareLinkList } from '@/features/account/SharedLinks'
import { MetadataLanguageSelect } from '@/features/account/MetadataLanguage'
import { WebThemeSelect } from '@/features/account/WebTheme'
import { AudioRegionSelect } from '@/features/account/AudioRegion'
import { errorMessage, formatDate } from '@/format'
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
  Notice,
  Panel,
  PanelSection,
  RelativeTime,
  SectionNav,
  Select,
  Skeleton,
  SkeletonRows,
  StatusPill,
  TextInput,
  useToast,
} from '@/ui'
import DeviceList from './DeviceList'
import {
  clockTime,
  refreshIfGone,
  storeUser,
  SwitchRow,
  useRestrictions,
  useSectionInView,
  useUserPatch,
} from './shared'
import {
  AllowedHoursForm,
  BlockedGenresForm,
  FollowedNotice,
  followedGroup,
  ParentalControlForm,
  PermissionRows,
  PlaybackAccessForm,
  VisibleLibrariesForm,
  ResetDefaultsDialog,
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
  'share-links',
  'devices',
] as const

const sectionClass =
  'scroll-mt-[calc(var(--spacing-topbar)+32px)] max-md:scroll-mt-[calc(var(--spacing-topbar)+64px)]'

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

function UserPage({ user }: { user: User }) {
  const { t, language } = useI18n()
  const self = useSessionUser()
  const toast = useToast()
  const navigate = useNavigate()
  const { hash } = useLocation()
  const restrictions = useRestrictions(user)
  const current = useSectionInView(sectionIds, true)
  const [deleting, setDeleting] = useState(false)
  const [resetting, setResetting] = useState(false)
  // Each reset draws the forms again from the new values.
  const [resets, setResets] = useState(0)
  const target: LimitsTarget = { kind: 'user', user }
  // The forms keep what is typed until saved. A member's own values are only changed there, while
  // the values of a group they follow change with it: those forms are drawn again from each copy.
  const followed = followedGroup(user)
  const formsKey = followed === null ? `own:${resets}` : `group:${JSON.stringify(user)}`

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

  const nav = (
    <SectionNav
      label={t.users.sectionsLabel}
      current={current}
      items={[
        {
          id: 'profile',
          label: t.users.profileTitle,
          icon: IdentificationCardIcon,
          to: '#profile',
        },
        { id: 'access', label: t.users.accessTitle, icon: ShieldCheckIcon, to: '#access' },
        {
          id: 'metadata-language',
          label: t.users.metadataLanguageTitle,
          icon: TranslateIcon,
          to: '#metadata-language',
        },
        {
          id: 'web-theme',
          label: t.users.webThemeTitle,
          icon: PaletteIcon,
          to: '#web-theme',
        },
        {
          id: 'audio-region',
          label: t.users.audioRegionTitle,
          icon: SpeakerHighIcon,
          to: '#audio-region',
        },
        {
          id: 'playback',
          label: t.users.playbackAccessTitle,
          icon: PlayCircleIcon,
          to: '#playback',
        },
        { id: 'parental', label: t.users.parentalTitle, icon: FilmSlateIcon, to: '#parental' },
        {
          id: 'libraries',
          label: t.users.visibleLibrariesTitle,
          icon: BooksIcon,
          to: '#libraries',
        },
        { id: 'genres', label: t.users.blockedGenresTitle, icon: ProhibitIcon, to: '#genres' },
        { id: 'hours', label: t.users.allowedHoursTitle, icon: ClockIcon, to: '#hours' },
        {
          id: 'share-links',
          label: t.users.shareLinksTitle,
          icon: ShareNetworkIcon,
          to: '#share-links',
        },
        { id: 'devices', label: t.users.devicesTitle, icon: DevicesIcon, to: '#devices' },
      ]}
    />
  )

  const lede = (
    <span className="flex flex-wrap items-center gap-2">
      <Avatar name={user.name} image={userImageUrl(user)} size="md" />
      {user.id === self.id && <Badge>{t.users.you}</Badge>}
      <Badge tone={user.isAdministrator ? 'accent' : 'neutral'}>
        {user.isAdministrator ? t.users.administrator : t.users.member}
      </Badge>
      {user.isDisabled ? (
        <StatusPill tone="muted">{t.users.disabled}</StatusPill>
      ) : user.ended ? (
        <StatusPill tone="muted">{t.users.ended}</StatusPill>
      ) : user.blockedUntil !== null ? (
        <StatusPill tone="warn">
          {t.users.blockedUntil(clockTime(user.blockedUntil, language))}
        </StatusPill>
      ) : (
        <StatusPill tone="ok">{t.users.active}</StatusPill>
      )}
      {user.groupName !== null && (
        <Badge>
          {user.followGroup
            ? t.groups.groupBadge(user.groupName)
            : t.groups.groupOwnBadge(user.groupName)}
        </Badge>
      )}
      <span className="text-ink-3">
        {t.users.lastSignInLabel}{' '}
        {user.lastLoginAt === null ? t.common.never : <RelativeTime iso={user.lastLoginAt} />}
        {user.endsOn !== null &&
          ` · ${(user.ended ? t.users.endedOn : t.users.endsOn)(formatDate(user.endsOn, language))}`}
        {restrictions.length > 0 && ` · ${restrictions.join(' · ')}`}
      </span>
    </span>
  )

  return (
    <>
      <PageLayout
        title={user.name}
        lede={lede}
        back={{ to: '/users', label: t.users.title }}
        nav={nav}
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
      >
        <ProfilePanel user={user} />
        <AccessPanel user={user} />
        <MetadataLanguagePanel user={user} />
        <WebThemePanel user={user} />
        <AudioRegionPanel user={user} />
        <PlaybackAccessForm key={`playback:${formsKey}`} id="playback" target={target} />
        <ParentalControlForm key={`parental:${formsKey}`} id="parental" target={target} />
        <VisibleLibrariesForm key={`libraries:${formsKey}`} id="libraries" target={target} />
        <BlockedGenresForm key={`genres:${formsKey}`} id="genres" target={target} />
        <AllowedHoursForm key={`hours:${formsKey}`} id="hours" target={target} />
        <ShareLinksSection user={user} />
        <section id="devices" aria-labelledby="devices-title" className={sectionClass}>
          <h2 id="devices-title" className="mb-4 text-h3 text-ink">
            {t.users.devicesTitle}
          </h2>
          <DeviceList
            queryKey={queryKeys.userDevices(user.id)}
            load={(signal) => fetchUserDevices(user.id, signal)}
            signOut={(deviceId) => signOutUserDevice(user.id, deviceId)}
          />
        </section>
      </PageLayout>
      {/* Outside the layout, whose block spacing would push the dialog down. */}
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
        onReset={() => setResets((count) => count + 1)}
      />
    </>
  )
}

/**
 * Name, password, the reset PIN asked for from an app, the block for wrong passwords, and two-step
 * sign-in, which an administrator turns off for a user who lost their device.
 */
function ProfilePanel({ user }: { user: User }) {
  const { t, language } = useI18n()
  const toast = useToast()
  const self = useSessionUser()
  const [name, setName] = useState(user.name)
  const [password, setPassword] = useState('')
  const [askingTwoStep, setAskingTwoStep] = useState(false)
  const rename = useUserPatch(user)
  const resetPassword = useUserPatch(user)
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

  const nameError = rename.isError ? errorMessage(t, rename.error) : undefined
  const passwordError = resetPassword.isError ? errorMessage(t, resetPassword.error) : undefined
  const pin = user.passwordResetPin

  return (
    <div id="profile" className={sectionClass}>
      <Panel title={t.users.profileTitle} flush>
        {(pin !== null || user.blockedUntil !== null || user.twoStep) && (
          <PanelSection>
            <div className="space-y-3">
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
                  <p className="text-ink">
                    {t.users.blockedUntil(clockTime(user.blockedUntil, language))}
                  </p>
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
            </div>
          </PanelSection>
        )}
        <PanelSection>
          <form
            noValidate
            className="flex flex-wrap items-start gap-3"
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault()
              rename.mutate({ name: name.trim() }, { onSuccess: () => toast(t.users.renamed) })
            }}
          >
            <Field
              label={t.users.name}
              help={t.common.nameRule}
              error={nameError}
              className="min-w-0 flex-1 basis-64"
            >
              <TextInput
                value={name}
                onValue={(value) => {
                  rename.reset()
                  setName(value)
                }}
                autoComplete="off"
                maxLength={64}
                required
                invalid={nameError !== undefined}
              />
            </Field>
            <Button type="submit" className="sm:mt-[26px]" loading={rename.isPending}>
              {rename.isPending ? t.common.saving : t.users.rename}
            </Button>
          </form>
        </PanelSection>
        <PanelSection>
          <form
            noValidate
            className="flex flex-wrap items-start gap-3"
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault()
              resetPassword.mutate(
                { password },
                {
                  onSuccess: () => {
                    setPassword('')
                    toast(t.users.passwordReset)
                  },
                },
              )
            }}
          >
            <Field
              label={t.users.newPassword}
              help={`${t.common.passwordRule} ${t.users.resetPasswordHelp}`}
              error={passwordError}
              className="min-w-0 flex-1 basis-64"
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
                invalid={passwordError !== undefined}
              />
            </Field>
            <Button
              type="submit"
              icon={KeyIcon}
              className="sm:mt-[26px]"
              loading={resetPassword.isPending}
            >
              {resetPassword.isPending ? t.common.saving : t.users.resetPassword}
            </Button>
          </form>
        </PanelSection>
      </Panel>
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
    </div>
  )
}

/**
 * What applies at once: role, sign-in screen, disabled, the end date, the group, and the user's
 * rights, which a group the user follows sets.
 */
function AccessPanel({ user }: { user: User }) {
  const { t } = useI18n()
  const toast = useToast()
  const access = useUserPatch(user)
  // Joining a group and following it again replace the user's values: both are asked first.
  const [asked, setAsked] = useState<{ groupId: string; name: string } | 'follow' | null>(null)
  // Losing the permission, or being disabled, revokes the user's share links.
  const change = (patch: Parameters<typeof access.mutate>[0]) =>
    access.mutate(patch, {
      onSuccess: () => {
        setAsked(null)
        toast(t.users.updated)
        void queryClient.invalidateQueries({ queryKey: queryKeys.userShareLinks(user.id) })
      },
    })
  const lastAdmin = access.error instanceof ApiError && access.error.code === 'last_administrator'
  const followed = followedGroup(user)

  return (
    <div id="access" className={sectionClass}>
      <Panel
        title={t.users.accessTitle}
        description={`${t.users.accessHelp} ${t.users.lastAdminHelp}`}
      >
        {access.isError && asked === null && (
          <div className="mb-2">
            <Notice tone={lastAdmin ? 'warn' : 'danger'} live>
              {errorMessage(t, access.error)}
            </Notice>
          </div>
        )}
        <div className="-my-4">
          <SwitchRow
            title={t.users.isAdministrator}
            help={
              user.groupName === null
                ? t.users.isAdministratorHelp
                : `${t.users.isAdministratorHelp} ${t.groups.administratorLeaves(user.groupName)}`
            }
            checked={user.isAdministrator}
            disabled={access.isPending}
            onChange={(isAdministrator) => change({ isAdministrator })}
          />
          <SwitchRow
            title={t.users.showOnSignIn}
            help={t.users.showOnSignInHelp}
            checked={!user.isHidden}
            disabled={access.isPending}
            onChange={(shown) => change({ isHidden: !shown })}
          />
          <SwitchRow
            title={t.users.isDisabled}
            help={t.users.isDisabledHelp}
            checked={user.isDisabled}
            disabled={access.isPending}
            onChange={(isDisabled) => change({ isDisabled })}
          />
          <SwitchRow
            title={t.users.providerOnly}
            help={t.users.providerOnlyHelp}
            checked={user.providerOnly}
            disabled={access.isPending}
            onChange={(providerOnly) => change({ providerOnly })}
          />
          <EndRow
            key={user.endsOn ?? ''}
            user={user}
            pending={access.isPending}
            onSave={(endsOn) => change({ endsOn })}
          />
          <GroupRow
            user={user}
            pending={access.isPending}
            onChoose={(group) => {
              access.reset()
              if (group === null) change({ groupId: '' })
              else setAsked({ groupId: group.id, name: group.name })
            }}
          />
          {user.groupId !== null && (
            <SwitchRow
              title={t.groups.followGroup}
              help={t.groups.followGroupHelp}
              checked={user.followGroup}
              disabled={access.isPending}
              onChange={(follow) => {
                access.reset()
                if (follow) setAsked('follow')
                else change({ followGroup: false })
              }}
            />
          )}
          {followed !== null && (
            <div className="border-t border-line py-4">
              <FollowedNotice group={followed} />
            </div>
          )}
          <PermissionRows
            values={user}
            disabled={access.isPending || followed !== null}
            onChange={change}
          />
        </div>
      </Panel>
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
    </div>
  )
}

/**
 * The group the user belongs to, or No group, chosen from the list. An administrator cannot join
 * one, as their permissions differ.
 */
function GroupRow({
  user,
  pending,
  onChoose,
}: {
  user: User
  pending: boolean
  onChoose: (group: { id: string; name: string } | null) => void
}) {
  const { t } = useI18n()
  const titleId = useId()
  const helpId = useId()
  const groups = useQuery({
    queryKey: queryKeys.groups,
    queryFn: ({ signal }) => fetchGroups(signal),
    enabled: !user.isAdministrator,
  })
  const options = [
    { value: '', label: t.groups.noGroup },
    ...(groups.data ?? []).map((group) => ({ value: group.id, label: group.name })),
  ]
  // The user's group shows while the list loads.
  if (user.groupId !== null && !options.some((option) => option.value === user.groupId)) {
    options.push({ value: user.groupId, label: user.groupName ?? '' })
  }

  return (
    <div className="flex items-start justify-between gap-6 border-t border-line py-4 max-sm:flex-col max-sm:gap-3">
      <div className="min-w-0">
        <p id={titleId} className="text-control font-medium text-ink">
          {t.groups.group}
        </p>
        <p id={helpId} className="mt-1 text-small text-ink-3">
          {user.isAdministrator ? t.groups.administratorNoGroup : t.groups.groupHelp}
        </p>
        {groups.isError && (
          <p className="mt-1 text-small text-danger">{errorMessage(t, groups.error)}</p>
        )}
      </div>
      <Select
        value={user.groupId ?? ''}
        options={options}
        onValue={(id) => {
          if (id === (user.groupId ?? '')) return
          const name = options.find((option) => option.value === id)?.label ?? ''
          onChoose(id === '' ? null : { id, name })
        }}
        disabled={pending || user.isAdministrator}
        aria-label={t.groups.group}
        aria-describedby={helpId}
        className="w-56 shrink-0 max-sm:w-full"
      />
    </div>
  )
}

/**
 * The user's metadata language: The server's, or one the server's addons were given, saved as it
 * changes. Without a language given to any addon it says where one is given.
 */
function MetadataLanguagePanel({ user }: { user: User }) {
  const { t } = useI18n()
  const toast = useToast()
  const patch = useUserPatch(user)
  const languages = useQuery({
    queryKey: queryKeys.metadataLanguages,
    queryFn: ({ signal }) => fetchMetadataLanguages(signal),
  })

  let body
  if (languages.isPending) body = <SkeletonRows rows={1} label={t.common.loading} />
  else if (languages.isError)
    body = (
      <InlineError onRetry={() => void languages.refetch()} retrying={languages.isFetching}>
        {errorMessage(t, languages.error)}
      </InlineError>
    )
  else
    body = (
      <div className="space-y-4">
        <Field
          label={t.users.metadataLanguageTitle}
          hideLabel
          error={patch.isError ? errorMessage(t, patch.error) : undefined}
        >
          <MetadataLanguageSelect
            value={user.metadataLanguage}
            languages={languages.data}
            onValue={(metadataLanguage) =>
              patch.mutate({ metadataLanguage }, { onSuccess: () => toast(t.users.updated) })
            }
            disabled={patch.isPending}
          />
        </Field>
        {languages.data.length === 0 && user.metadataLanguage === '' && (
          <Notice>{t.users.metadataLanguageNone}</Notice>
        )}
      </div>
    )
  return (
    <div id="metadata-language" className={sectionClass}>
      <Panel title={t.users.metadataLanguageTitle} description={t.users.metadataLanguageHelp}>
        {body}
      </Panel>
    </div>
  )
}

/**
 * The user's web player theme: The server's, or one the server offers, saved as it changes, and
 * whether they may change it themselves under My account.
 */
function WebThemePanel({ user }: { user: User }) {
  const { t } = useI18n()
  const toast = useToast()
  const patch = useUserPatch(user)
  const themes = useQuery({
    queryKey: queryKeys.webThemes,
    queryFn: ({ signal }) => fetchWebThemes(signal),
  })

  let body
  if (themes.isPending) body = <SkeletonRows rows={2} label={t.common.loading} />
  else if (themes.isError)
    body = (
      <InlineError onRetry={() => void themes.refetch()} retrying={themes.isFetching}>
        {errorMessage(t, themes.error)}
      </InlineError>
    )
  else
    body = (
      <div>
        <Field
          label={t.users.webThemeTitle}
          hideLabel
          error={patch.isError ? errorMessage(t, patch.error) : undefined}
        >
          <WebThemeSelect
            value={user.webTheme}
            themes={themes.data}
            onValue={(webTheme) =>
              patch.mutate({ webTheme }, { onSuccess: () => toast(t.users.updated) })
            }
            disabled={patch.isPending}
          />
        </Field>
        <div className="mt-4">
          <SwitchRow
            title={t.users.webThemeChange}
            help={t.users.webThemeChangeHelp}
            checked={!user.webThemeLocked}
            onChange={(change) =>
              patch.mutate({ webThemeLocked: !change }, { onSuccess: () => toast(t.users.updated) })
            }
            disabled={patch.isPending}
          />
        </div>
      </div>
    )
  return (
    <div id="web-theme" className={sectionClass}>
      <Panel title={t.users.webThemeTitle} description={t.users.webThemeHelp}>
        {body}
      </Panel>
    </div>
  )
}

/** The user's audio region: None, or one of the regions offered, saved as it changes. */
function AudioRegionPanel({ user }: { user: User }) {
  const { t } = useI18n()
  const toast = useToast()
  const patch = useUserPatch(user)
  return (
    <div id="audio-region" className={sectionClass}>
      <Panel title={t.users.audioRegionTitle} description={t.users.audioRegionHelp}>
        <Field
          label={t.users.audioRegionTitle}
          hideLabel
          error={patch.isError ? errorMessage(t, patch.error) : undefined}
        >
          <AudioRegionSelect
            value={user.audioRegion}
            regions={user.audioRegions}
            onValue={(audioRegion) =>
              patch.mutate({ audioRegion }, { onSuccess: () => toast(t.users.updated) })
            }
            disabled={patch.isPending}
          />
        </Field>
      </Panel>
    </div>
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
    <section id="share-links" aria-labelledby="share-links-title" className={sectionClass}>
      <h2 id="share-links-title" className="text-h3 text-ink">
        {t.users.shareLinksTitle}
      </h2>
      <p className="mt-1.5 mb-4 max-w-[60ch] text-small text-ink-2">{t.users.shareLinksHelp}</p>
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
    </section>
  )
}

/**
 * When the account ends: a date, saved with its button, or Never, which clears it. An
 * administrator's account never ends.
 */
function EndRow({
  user,
  pending,
  onSave,
}: {
  user: User
  pending: boolean
  onSave: (endsOn: string | null) => void
}) {
  const { t } = useI18n()
  const helpId = useId()
  const [end, setEnd] = useState(user.endsOn ?? '')

  return (
    <div className="flex items-start justify-between gap-6 border-t border-line py-4 max-sm:flex-col max-sm:gap-3">
      <div className="min-w-0">
        <p className="text-control font-medium text-ink">{t.users.endTitle}</p>
        <p id={helpId} className="mt-1 text-small text-ink-3">
          {user.isAdministrator ? t.users.endAdministratorHelp : t.users.endHelp}
        </p>
      </div>
      {!user.isAdministrator && (
        <div className="flex shrink-0 items-center gap-2">
          <TextInput
            type="date"
            size="sm"
            value={end}
            onValue={setEnd}
            aria-label={t.users.endDate}
            aria-describedby={helpId}
            disabled={pending}
            className="w-40"
          />
          <Button
            size="sm"
            disabled={pending || end === '' || end === user.endsOn}
            onClick={() => onSave(end)}
          >
            {t.users.saveEnd}
          </Button>
          {user.endsOn !== null && (
            <Button size="sm" variant="ghost" disabled={pending} onClick={() => onSave(null)}>
              {t.users.endNever}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
