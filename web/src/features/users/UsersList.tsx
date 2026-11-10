import {
  ArrowSquareInIcon,
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
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Avatar,
  Badge,
  Block,
  Button,
  ConfirmDialog,
  EmptyState,
  InlineError,
  Row,
  RowList,
  Select,
  SkeletonRows,
  useToast,
} from '@/ui'
import CreateUserModal from './CreateUserModal'
import { CreateInviteModal, InviteLinks } from './InviteLinks'
import { accountEnd, LastActivity, UserState, useRestrictions, UsersArea } from './shared'
import UserActions from './UserActions'

/** The users the list shows: every one, those in no group, or the members of one group. */
const allUsers = 'all'
const noGroup = 'none'

/**
 * `/users`: every account as a row opening its page, with a "…" of quick actions beside it, and
 * the buttons creating a user and an invite link; then the invite links; then the imports of
 * accounts and watch data from another server, an app or Letterboxd, and the one turning
 * downloads off for everyone.
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
  // Two actions in the head, which wrap on a phone; the imports and downloads have their rows.
  const actions = (
    <>
      <Button icon={LinkIcon} onClick={() => setInviting(true)}>
        {t.invites.create}
      </Button>
      {create}
    </>
  )

  return (
    <UsersArea>
      <PageLayout title={t.users.title} lede={t.users.description} actions={actions}>
        <Block
          title={t.users.listTitle}
          count={users.data === undefined ? undefined : shown.length}
          aside={
            (groups.data?.length ?? 0) > 0 && (
              <Select
                size="sm"
                value={shownGroup}
                onValue={setShownGroup}
                aria-label={t.groups.filter}
                className="w-56 max-w-full"
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
        <Block title={t.users.moreTitle}>
          <RowList aria-label={t.users.moreTitle}>
            <Row
              to="/users/jellyfin-import"
              leading={ArrowSquareInIcon}
              title={t.users.jellyfinImport.open}
              meta={t.users.jellyfinImport.summary}
            />
            <Row
              to="/users/app-import"
              leading={DeviceMobileIcon}
              title={t.users.appImport.open}
              meta={t.users.appImport.summary}
            />
            <Row
              to="/users/letterboxd-import"
              leading={FileZipIcon}
              title={t.users.letterboxdImport.open}
              meta={t.users.letterboxdImport.summary}
            />
            <Row
              onClick={() => {
                downloadsOff.reset()
                setStoppingDownloads(true)
              }}
              leading={DownloadSimpleIcon}
              title={t.users.turnOffDownloads}
              meta={t.users.turnOffDownloadsSummary}
            />
          </RowList>
        </Block>
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
    </UsersArea>
  )
}

/**
 * A user as a row: the avatar, the name with its tags, the last activity, and the state; then,
 * outside its link, the "…" of quick actions. As on a source's row, whose controls sit beside its
 * link, the row draws no chevron. On a phone the state leads the second line, and the tags keep
 * to one line under the name when they do not fit beside it.
 */
function UserRow({ user }: { user: User }) {
  const { t, language } = useI18n()
  const self = useSessionUser()
  const restrictions = useRestrictions(user)
  const end = accountEnd(user, t, language)
  const tags = [
    user.id === self.id && <Badge key="you">{t.users.you}</Badge>,
    user.isAdministrator && (
      <Badge key="administrator" tone="accent">
        {t.users.administrator}
      </Badge>
    ),
    user.passwordResetPin !== null && (
      <Badge key="pin" tone="warn">
        {t.users.pinRequested}
      </Badge>
    ),
    user.groupName !== null && (
      <Badge key="group">
        {user.ownSections.length === 0
          ? t.groups.groupBadge(user.groupName)
          : t.groups.groupOwnBadge(user.groupName)}
      </Badge>
    ),
  ].filter(Boolean)

  return (
    <Row
      to={`/users/${user.id}`}
      chevron={false}
      leading={<Avatar name={user.name} image={userImageUrl(user)} size="md" />}
      title={user.name}
      titleAside={
        tags.length > 0 && (
          <span className="flex min-w-0 flex-wrap items-center gap-1.5">{tags}</span>
        )
      }
      meta={
        <span className="min-w-0">
          {/* On a phone the state leads this line, leaving the name its width. */}
          <span className="mr-2 inline-flex align-middle sm:hidden">
            <UserState user={user} />
          </span>
          <LastActivity user={user} />
          {end !== null && (
            <span>
              {' · '}
              {(user.ended ? t.users.endedOn : t.users.endsOn)(end)}
            </span>
          )}
          {restrictions.length > 0 && <span> · {restrictions.join(' · ')}</span>}
        </span>
      }
      trailing={
        <span className="max-sm:hidden">
          <UserState user={user} />
        </span>
      }
      actions={<UserActions user={user} />}
    />
  )
}
