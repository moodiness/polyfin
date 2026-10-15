import {
  ArrowSquareInIcon,
  CaretRightIcon,
  DeviceMobileIcon,
  DownloadSimpleIcon,
  FileZipIcon,
  LinkIcon,
  PlusIcon,
  UsersIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
  fetchGroups,
  fetchUsers,
  queryClient,
  queryKeys,
  turnOffDownloads,
  userImageUrl,
  type User,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { errorMessage, formatDate } from '@/format'
import { useI18n } from '@/i18n'
import {
  Avatar,
  Badge,
  Block,
  Button,
  ButtonLink,
  ConfirmDialog,
  EmptyState,
  InlineError,
  Row,
  RowList,
  Select,
  SkeletonRows,
  StatusPill,
  RelativeTime,
  useToast,
} from '@/ui'
import CreateUserModal from './CreateUserModal'
import { CreateInviteModal, InviteLinks } from './InviteLinks'
import { clockTime, useRestrictions, UsersTabs } from './shared'

/** The users the list shows: every one, those in no group, or the members of one group. */
const allUsers = 'all'
const noGroup = 'none'

/**
 * `/users`: every account, each opening its page, the button to create one, the ones importing
 * accounts from a Jellyfin server and a user's watch data from an app, the one turning downloads
 * off for everyone, and the invite links, with the button to create one.
 */
export default function UsersList() {
  const { t } = useI18n()
  const toast = useToast()
  const [creating, setCreating] = useState(false)
  const [inviting, setInviting] = useState(false)
  const [stoppingDownloads, setStoppingDownloads] = useState(false)
  const [shownGroup, setShownGroup] = useState(allUsers)
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  const groups = useQuery({
    queryKey: queryKeys.groups,
    queryFn: ({ signal }) => fetchGroups(signal),
  })
  const shown = (users.data ?? []).filter((user) =>
    shownGroup === allUsers
      ? true
      : shownGroup === noGroup
        ? user.groupId === null
        : user.groupId === shownGroup,
  )
  const downloadsOff = useMutation({
    mutationFn: turnOffDownloads,
    onSuccess: (changed) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      void queryClient.invalidateQueries({ queryKey: queryKeys.groups })
      setStoppingDownloads(false)
      toast(t.users.downloadsTurnedOff(changed.length), { tone: 'ok' })
    },
  })

  const create = (
    <Button variant="primary" icon={PlusIcon} onClick={() => setCreating(true)}>
      {t.users.createTitle}
    </Button>
  )
  const actions = (
    <span className="flex flex-wrap gap-2">
      <Button
        icon={DownloadSimpleIcon}
        onClick={() => {
          downloadsOff.reset()
          setStoppingDownloads(true)
        }}
      >
        {t.users.turnOffDownloads}
      </Button>
      <ButtonLink to="/users/jellyfin-import" icon={ArrowSquareInIcon}>
        {t.users.jellyfinImport.open}
      </ButtonLink>
      <ButtonLink to="/users/app-import" icon={DeviceMobileIcon}>
        {t.users.appImport.open}
      </ButtonLink>
      <ButtonLink to="/users/letterboxd-import" icon={FileZipIcon}>
        {t.users.letterboxdImport.open}
      </ButtonLink>
      <Button icon={LinkIcon} onClick={() => setInviting(true)}>
        {t.invites.create}
      </Button>
      {create}
    </span>
  )

  return (
    <>
      <UsersTabs />
      <PageLayout title={t.users.title} lede={t.users.description} actions={actions}>
        <Block
          title={t.users.listTitle}
          count={users.data === undefined ? undefined : shown.length}
          aside={
            (groups.data?.length ?? 0) > 0 && (
              <Select
                value={shownGroup}
                onValue={setShownGroup}
                aria-label={t.groups.filter}
                className="w-48"
                options={[
                  { value: allUsers, label: t.groups.allGroups },
                  { value: noGroup, label: t.groups.noGroup },
                  ...(groups.data ?? []).map((group) => ({ value: group.id, label: group.name })),
                ]}
              />
            )
          }
        >
          {users.isPending ? (
            <SkeletonRows rows={3} boxed label={t.common.loading} />
          ) : users.isError ? (
            <InlineError onRetry={() => void users.refetch()} retrying={users.isFetching}>
              {errorMessage(t, users.error)}
            </InlineError>
          ) : users.data.length === 0 ? (
            <EmptyState icon={UsersIcon} title={t.users.empty} action={create}>
              {t.users.emptyHelp}
            </EmptyState>
          ) : shown.length === 0 ? (
            <EmptyState icon={UsersIcon} title={t.groups.noneShown} />
          ) : (
            <RowList aria-label={t.users.listTitle}>
              {shown.map((user) => (
                <UserRow key={user.id} user={user} />
              ))}
            </RowList>
          )}
        </Block>
        <InviteLinks onCreate={() => setInviting(true)} />
      </PageLayout>
      {/* Outside the layout, whose block spacing would push the panel down. */}
      <CreateUserModal
        open={creating}
        onClose={() => setCreating(false)}
        groups={groups.data ?? []}
      />
      <CreateInviteModal
        open={inviting}
        onClose={() => setInviting(false)}
        users={users.data ?? []}
        groups={groups.data ?? []}
      />
      <ConfirmDialog
        open={stoppingDownloads}
        onClose={() => setStoppingDownloads(false)}
        onConfirm={() => downloadsOff.mutate()}
        title={t.users.turnOffDownloads}
        confirmLabel={
          downloadsOff.isPending ? t.users.turningOffDownloads : t.users.turnOffDownloads
        }
        busy={downloadsOff.isPending}
        error={downloadsOff.isError ? errorMessage(t, downloadsOff.error) : undefined}
      >
        {t.users.turnOffDownloadsConfirm}
      </ConfirmDialog>
    </>
  )
}

function UserRow({ user }: { user: User }) {
  const { t, language } = useI18n()
  const self = useSessionUser()
  const restrictions = useRestrictions(user)
  const lastSignIn =
    user.lastLoginAt === null ? t.common.never : <RelativeTime iso={user.lastLoginAt} />

  return (
    <Row
      to={`/users/${user.id}`}
      leading={<Avatar name={user.name} image={userImageUrl(user)} size="lg" />}
      title={user.name}
      titleAside={
        <>
          {user.id === self.id && <Badge>{t.users.you}</Badge>}
          {user.isAdministrator && <Badge tone="accent">{t.users.administrator}</Badge>}
          {user.passwordResetPin !== null && <Badge tone="warn">{t.users.pinRequested}</Badge>}
          {user.groupName !== null && (
            <Badge>
              {user.followGroup
                ? t.groups.groupBadge(user.groupName)
                : t.groups.groupOwnBadge(user.groupName)}
            </Badge>
          )}
        </>
      }
      meta={
        <span className="truncate">
          {t.users.lastSignInLabel} {lastSignIn}
          {user.endsOn !== null && (
            <span>
              {' · '}
              {(user.ended ? t.users.endedOn : t.users.endsOn)(formatDate(user.endsOn, language))}
            </span>
          )}
          {restrictions.length > 0 && <span> · {restrictions.join(' · ')}</span>}
        </span>
      }
      trailing={
        <span className="flex items-center gap-3">
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
          <CaretRightIcon size={16} aria-hidden="true" className="text-ink-3 max-sm:hidden" />
        </span>
      }
    />
  )
}
