import {
  ArrowCounterClockwiseIcon,
  BooksIcon,
  CaretRightIcon,
  ClockIcon,
  FilmSlateIcon,
  IdentificationCardIcon,
  PlayCircleIcon,
  ProhibitIcon,
  ShieldCheckIcon,
  TrashIcon,
  UserPlusIcon,
  UsersThreeIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useState, type FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router'
import {
  addGroupMembers,
  ApiError,
  deleteGroup,
  fetchGroup,
  fetchUsers,
  queryClient,
  queryKeys,
  updateGroup,
  userImageUrl,
  type Group,
  type GroupPatch,
  type User,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Avatar,
  Badge,
  Button,
  ButtonLink,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  InlineError,
  Modal,
  Notice,
  Panel,
  Row,
  RowList,
  SectionNav,
  Skeleton,
  SkeletonRows,
  TextInput,
  useToast,
} from '@/ui'
import { groupSaved, refreshGroupIfGone, useSectionInView } from './shared'
import {
  AllowedHoursForm,
  BlockedGenresForm,
  ParentalControlForm,
  PermissionRows,
  PlaybackAccessForm,
  ResetDefaultsDialog,
  VisibleLibrariesForm,
  type LimitsTarget,
} from './UserForms'

const sectionIds = [
  'name',
  'access',
  'playback',
  'parental',
  'libraries',
  'genres',
  'hours',
  'members',
] as const

const sectionClass =
  'scroll-mt-[calc(var(--spacing-topbar)+32px)] max-md:scroll-mt-[calc(var(--spacing-topbar)+64px)]'

/** `/users/groups/:id`: one group's page, with the sections its members follow and its members. */
export default function GroupDetail({ id }: { id: string }) {
  const { t } = useI18n()
  const group = useQuery({
    queryKey: queryKeys.group(id),
    queryFn: ({ signal }) => fetchGroup(id, signal),
  })
  const back = { to: '/users/groups', label: t.groups.title }

  if (group.isPending) {
    return (
      <PageLayout title={<Skeleton className="h-8 w-48" />} back={back}>
        <SkeletonRows rows={4} boxed label={t.common.loading} />
      </PageLayout>
    )
  }
  if (group.isError) {
    const gone = group.error instanceof ApiError && group.error.status === 404
    return (
      <PageLayout title={gone ? t.groups.notFound : t.groups.title} back={back}>
        {gone ? (
          <EmptyState
            icon={UsersThreeIcon}
            title={t.groups.notFound}
            action={<ButtonLink to="/users/groups">{t.groups.backToGroups}</ButtonLink>}
          >
            {t.groups.notFoundHelp}
          </EmptyState>
        ) : (
          <InlineError onRetry={() => void group.refetch()} retrying={group.isFetching}>
            {errorMessage(t, group.error)}
          </InlineError>
        )}
      </PageLayout>
    )
  }
  return <GroupPage group={group.data} />
}

/** A PATCH of a group, keeping its page, the list and its members up to date. */
function useGroupPatch(group: Group) {
  return useMutation({
    mutationFn: (patch: GroupPatch) => updateGroup(group.id, patch),
    onSuccess: groupSaved,
    onError: refreshGroupIfGone,
  })
}

function GroupPage({ group }: { group: Group }) {
  const { t } = useI18n()
  const toast = useToast()
  const navigate = useNavigate()
  const { hash } = useLocation()
  const current = useSectionInView(sectionIds, true)
  const [deleting, setDeleting] = useState(false)
  const [resetting, setResetting] = useState(false)
  // The forms keep what is typed until saved: each reset draws them again from the new values.
  const [resets, setResets] = useState(0)
  const target: LimitsTarget = { kind: 'group', group }

  // The content arrives after the first render: a #section address scrolls once it is drawn.
  useEffect(() => {
    if (hash) document.getElementById(hash.slice(1))?.scrollIntoView({ block: 'start' })
  }, [hash])

  const remove = useMutation({
    mutationFn: () => deleteGroup(group.id),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: queryKeys.group(group.id) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.groups })
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      void queryClient.invalidateQueries({ queryKey: queryKeys.invites })
      toast(t.groups.deleted(group.name))
      void navigate('/users/groups')
    },
    onError: refreshGroupIfGone,
  })

  const nav = (
    <SectionNav
      label={t.groups.sectionsLabel}
      current={current}
      items={[
        { id: 'name', label: t.groups.name, icon: IdentificationCardIcon, to: '#name' },
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
        { id: 'members', label: t.groups.membersTitle, icon: UsersThreeIcon, to: '#members' },
      ]}
    />
  )

  return (
    <>
      <PageLayout
        title={group.name}
        lede={`${t.groups.memberCount(group.memberCount)} · ${t.groups.pageHelp}`}
        back={{ to: '/users/groups', label: t.groups.title }}
        nav={nav}
        actions={
          <>
            <Button
              variant="secondary"
              icon={ArrowCounterClockwiseIcon}
              onClick={() => setResetting(true)}
            >
              {t.resetDefaults.button}
            </Button>
            <Button
              variant="danger"
              icon={TrashIcon}
              onClick={() => {
                remove.reset()
                setDeleting(true)
              }}
            >
              {t.groups.delete}
            </Button>
          </>
        }
      >
        <NamePanel group={group} />
        <AccessPanel group={group} />
        <PlaybackAccessForm key={`playback:${resets}`} id="playback" target={target} />
        <ParentalControlForm key={`parental:${resets}`} id="parental" target={target} />
        <VisibleLibrariesForm key={`libraries:${resets}`} id="libraries" target={target} />
        <BlockedGenresForm key={`genres:${resets}`} id="genres" target={target} />
        <AllowedHoursForm key={`hours:${resets}`} id="hours" target={target} />
        <MembersSection group={group} />
      </PageLayout>
      {/* Outside the layout, whose block spacing would push the dialog down. */}
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        onConfirm={() => remove.mutate()}
        title={t.groups.deleteTitle(group.name)}
        confirmLabel={remove.isPending ? t.groups.deleting : t.groups.delete}
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {t.groups.deleteConfirm}
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

/** The group's name, changed with Rename. */
function NamePanel({ group }: { group: Group }) {
  const { t } = useI18n()
  const toast = useToast()
  const [name, setName] = useState(group.name)
  const rename = useGroupPatch(group)
  const code = rename.error instanceof ApiError ? rename.error.code : null
  // The shared name codes speak of a user and of the user name rule: a group's name has its own.
  const nameError =
    code === 'name_taken'
      ? t.groups.nameTaken
      : code === 'invalid_name'
        ? t.groups.nameRule
        : rename.isError
          ? errorMessage(t, rename.error)
          : undefined

  return (
    <div id="name" className={sectionClass}>
      <Panel title={t.groups.name}>
        <form
          noValidate
          className="flex flex-wrap items-start gap-3"
          onSubmit={(event: FormEvent<HTMLFormElement>) => {
            event.preventDefault()
            rename.mutate({ name: name.trim() }, { onSuccess: () => toast(t.groups.renamed) })
          }}
        >
          <Field
            label={t.groups.name}
            help={t.groups.nameRule}
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
            {rename.isPending ? t.common.saving : t.groups.rename}
          </Button>
        </form>
      </Panel>
    </div>
  )
}

/**
 * The permissions of Access the group gives its members, each saved as it changes. A member's
 * role, sign-in screen, Disabled and end date stay on their own page.
 */
function AccessPanel({ group }: { group: Group }) {
  const { t } = useI18n()
  const toast = useToast()
  const access = useGroupPatch(group)

  return (
    <div id="access" className={sectionClass}>
      <Panel title={t.users.accessTitle} description={t.groups.accessHelp}>
        {access.isError && (
          <div className="mb-2">
            <Notice tone="danger" live>
              {errorMessage(t, access.error)}
            </Notice>
          </div>
        )}
        <div className="-my-4">
          <PermissionRows
            values={group}
            disabled={access.isPending}
            onChange={(patch) => access.mutate(patch, { onSuccess: () => toast(t.groups.updated) })}
          />
        </div>
      </Panel>
    </div>
  )
}

/** The group's members, each opening their page, marked when they keep their own settings. */
function MembersSection({ group }: { group: Group }) {
  const { t } = useI18n()
  const [adding, setAdding] = useState(false)
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  const members = (users.data ?? []).filter((user) => user.groupId === group.id)

  const add = (
    <Button icon={UserPlusIcon} onClick={() => setAdding(true)} disabled={!users.isSuccess}>
      {t.groups.addMembers}
    </Button>
  )

  return (
    <section id="members" aria-labelledby="members-title" className={sectionClass}>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 id="members-title" className="text-h3 text-ink">
            {t.groups.membersTitle}
          </h2>
          <p className="mt-1.5 max-w-[60ch] text-small text-ink-2">{t.groups.membersHelp}</p>
        </div>
        {add}
      </div>
      {users.isPending ? (
        <SkeletonRows rows={2} boxed label={t.common.loading} />
      ) : users.isError ? (
        <InlineError onRetry={() => void users.refetch()} retrying={users.isFetching}>
          {errorMessage(t, users.error)}
        </InlineError>
      ) : members.length === 0 ? (
        <EmptyState icon={UsersThreeIcon} title={t.groups.noMembers} action={add}>
          {t.groups.noMembersHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={t.groups.membersTitle}>
          {members.map((member) => (
            <Row
              key={member.id}
              to={`/users/${member.id}`}
              leading={<Avatar name={member.name} image={userImageUrl(member)} size="lg" />}
              title={member.name}
              titleAside={!member.followGroup && <Badge tone="warn">{t.groups.ownSettings}</Badge>}
              meta={member.followGroup ? t.groups.followsHelp : t.groups.ownSettingsHelp}
              trailing={
                <CaretRightIcon size={16} aria-hidden="true" className="text-ink-3 max-sm:hidden" />
              }
            />
          ))}
        </RowList>
      )}
      <AddMembersModal
        open={adding}
        onClose={() => setAdding(false)}
        group={group}
        users={users.data ?? []}
      />
    </section>
  )
}

/**
 * Picks users to put in the group, following it. Administrators cannot join one and are not
 * offered; a user in another group moves to this one.
 */
function AddMembersModal({
  open,
  onClose,
  group,
  users,
}: {
  open: boolean
  onClose: () => void
  group: Group
  users: readonly User[]
}) {
  const { t } = useI18n()
  const toast = useToast()
  const formId = useId()
  const [picked, setPicked] = useState<string[]>([])
  const candidates = users.filter((user) => !user.isAdministrator && user.groupId !== group.id)
  const mutation = useMutation({
    mutationFn: (userIds: string[]) => addGroupMembers(group.id, userIds),
    onSuccess: (_data, userIds) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      void queryClient.invalidateQueries({ queryKey: queryKeys.groups })
      toast(t.groups.membersAdded(userIds.length))
      close()
    },
    onError: (error) => {
      // A user deleted, or made an administrator, since the list was read.
      if (error instanceof ApiError && (error.status === 404 || error.status === 409)) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      }
    },
  })

  function close() {
    mutation.reset()
    setPicked([])
    onClose()
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (picked.length > 0) mutation.mutate(picked)
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title={t.groups.addMembersTitle(group.name)}
      width={480}
      footer={
        <>
          <Button variant="ghost" onClick={close}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            loading={mutation.isPending}
            disabled={picked.length === 0}
          >
            {mutation.isPending ? t.groups.adding : t.groups.add}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-5 p-5">
        <p className="text-control text-ink-2">{t.groups.addMembersHelp}</p>
        {candidates.length === 0 ? (
          <EmptyState title={t.groups.noCandidates}>{t.groups.noCandidatesHelp}</EmptyState>
        ) : (
          <div className="space-y-3">
            {candidates.map((user) => (
              <Checkbox
                key={user.id}
                label={user.name}
                help={user.groupName === null ? undefined : t.groups.movesFrom(user.groupName)}
                checked={picked.includes(user.id)}
                onChange={(checked) => {
                  mutation.reset()
                  setPicked(checked ? [...picked, user.id] : picked.filter((id) => id !== user.id))
                }}
              />
            ))}
          </div>
        )}
        {mutation.isError && <Notice tone="danger">{errorMessage(t, mutation.error)}</Notice>}
      </form>
    </Modal>
  )
}
