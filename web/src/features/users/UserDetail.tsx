import {
  ClockIcon,
  DevicesIcon,
  FilmSlateIcon,
  IdentificationCardIcon,
  KeyIcon,
  PlayCircleIcon,
  ProhibitIcon,
  ShieldCheckIcon,
  TrashIcon,
  UserCircleDashedIcon,
  BooksIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useState, type FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router'
import {
  ApiError,
  deleteUser,
  fetchUserDevices,
  fetchUsers,
  queryClient,
  queryKeys,
  signOutUserDevice,
  unblockUser,
  userImageUrl,
  type User,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { errorMessage } from '@/format'
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
  ParentalControlForm,
  PlaybackAccessForm,
  VisibleLibrariesForm,
} from './UserForms'

const sectionIds = [
  'profile',
  'access',
  'playback',
  'parental',
  'libraries',
  'genres',
  'hours',
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
      ) : user.blockedUntil !== null ? (
        <StatusPill tone="warn">
          {t.users.blockedUntil(clockTime(user.blockedUntil, language))}
        </StatusPill>
      ) : (
        <StatusPill tone="ok">{t.users.active}</StatusPill>
      )}
      <span className="text-ink-3">
        {t.users.lastSignInLabel}{' '}
        {user.lastLoginAt === null ? t.common.never : <RelativeTime iso={user.lastLoginAt} />}
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
        }
      >
        <ProfilePanel user={user} />
        <AccessPanel user={user} />
        <PlaybackAccessForm id="playback" user={user} />
        <ParentalControlForm id="parental" user={user} />
        <VisibleLibrariesForm id="libraries" user={user} />
        <BlockedGenresForm id="genres" user={user} />
        <AllowedHoursForm id="hours" user={user} />
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
    </>
  )
}

/** Name, password, the reset PIN asked for from an app, and the block for wrong passwords. */
function ProfilePanel({ user }: { user: User }) {
  const { t, language } = useI18n()
  const toast = useToast()
  const [name, setName] = useState(user.name)
  const [password, setPassword] = useState('')
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

  const nameError = rename.isError ? errorMessage(t, rename.error) : undefined
  const passwordError = resetPassword.isError ? errorMessage(t, resetPassword.error) : undefined
  const pin = user.passwordResetPin

  return (
    <div id="profile" className={sectionClass}>
      <Panel title={t.users.profileTitle} flush>
        {(pin !== null || user.blockedUntil !== null) && (
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
    </div>
  )
}

/** The switches that apply at once: role, sign-in screen, disabled, and the user's rights. */
function AccessPanel({ user }: { user: User }) {
  const { t } = useI18n()
  const toast = useToast()
  const access = useUserPatch(user)
  const change = (patch: Parameters<typeof access.mutate>[0]) =>
    access.mutate(patch, { onSuccess: () => toast(t.users.updated) })
  const lastAdmin = access.error instanceof ApiError && access.error.code === 'last_administrator'

  return (
    <div id="access" className={sectionClass}>
      <Panel
        title={t.users.accessTitle}
        description={`${t.users.accessHelp} ${t.users.lastAdminHelp}`}
      >
        {access.isError && (
          <div className="mb-2">
            <Notice tone={lastAdmin ? 'warn' : 'danger'} live>
              {errorMessage(t, access.error)}
            </Notice>
          </div>
        )}
        <div className="-my-4">
          <SwitchRow
            title={t.users.isAdministrator}
            help={t.users.isAdministratorHelp}
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
            title={t.users.canTranscode}
            help={t.users.canTranscodeHelp}
            checked={user.transcoding}
            disabled={access.isPending}
            onChange={(transcoding) => change({ transcoding })}
          />
          <SwitchRow
            title={t.users.canDownload}
            help={t.users.canDownloadHelp}
            checked={user.downloads}
            disabled={access.isPending}
            onChange={(downloads) => change({ downloads })}
          />
          <SwitchRow
            title={t.users.canAddAddons}
            help={t.users.canAddAddonsHelp}
            checked={user.personalAddons}
            disabled={access.isPending}
            onChange={(personalAddons) => change({ personalAddons })}
          />
          <SwitchRow
            title={t.users.canManageCollections}
            help={t.users.canManageCollectionsHelp}
            checked={user.collectionManagement}
            disabled={access.isPending}
            onChange={(collectionManagement) => change({ collectionManagement })}
          />
          <SwitchRow
            title={t.users.canManageSubtitles}
            help={t.users.canManageSubtitlesHelp}
            checked={user.subtitleManagement}
            disabled={access.isPending}
            onChange={(subtitleManagement) => change({ subtitleManagement })}
          />
        </div>
      </Panel>
    </div>
  )
}
