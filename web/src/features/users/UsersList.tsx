import { CaretRightIcon, PlusIcon, UsersIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { fetchUsers, queryKeys, type User } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  EmptyState,
  InlineError,
  Row,
  RowList,
  SkeletonRows,
  StatusPill,
  RelativeTime,
} from '@/ui'
import CreateUserDrawer from './CreateUserDrawer'
import { clockTime, UserAvatar, useRestrictions } from './shared'

/** `/users`: every account, each opening its page, and the button to create one. */
export default function UsersList() {
  const { t } = useI18n()
  const [creating, setCreating] = useState(false)
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })

  const create = (
    <Button variant="primary" icon={PlusIcon} onClick={() => setCreating(true)}>
      {t.users.createTitle}
    </Button>
  )

  return (
    <>
      <PageLayout title={t.users.title} lede={t.users.description} actions={create}>
        <Block title={t.users.listTitle} count={users.data?.length}>
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
          ) : (
            <RowList aria-label={t.users.listTitle}>
              {users.data.map((user) => (
                <UserRow key={user.id} user={user} />
              ))}
            </RowList>
          )}
        </Block>
      </PageLayout>
      {/* Outside the layout, whose block spacing would push the drawer down. */}
      <CreateUserDrawer open={creating} onClose={() => setCreating(false)} />
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
      leading={<UserAvatar user={user} />}
      title={user.name}
      titleAside={
        <>
          {user.id === self.id && <Badge>{t.users.you}</Badge>}
          {user.isAdministrator && <Badge tone="accent">{t.users.administrator}</Badge>}
          {user.passwordResetPin !== null && <Badge tone="warn">{t.users.pinRequested}</Badge>}
        </>
      }
      meta={
        <span className="truncate">
          {t.users.lastSignInLabel} {lastSignIn}
          {restrictions.length > 0 && <span> · {restrictions.join(' · ')}</span>}
        </span>
      }
      trailing={
        <span className="flex items-center gap-3">
          {user.isDisabled ? (
            <StatusPill tone="muted">{t.users.disabled}</StatusPill>
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
