import {
  ArrowCounterClockwiseIcon,
  PuzzlePieceIcon,
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
  fetchAddons,
  fetchGroup,
  fetchUsers,
  queryClient,
  queryKeys,
  setGroupSource,
  updateGroup,
  userImageUrl,
  type Addon,
  type Group,
  type GroupPatch,
  type User,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { invalidateScope, replaceCachedAddon, sourcePath } from '@/features/sources/model'
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
  IconTile,
  InlineError,
  Modal,
  Notice,
  Row,
  RowList,
  SectionNav,
  SettingRow,
  Skeleton,
  SkeletonRows,
  TextLink,
  useToast,
} from '@/ui'
import { DraftForm, useDraftPart } from './draft'
import { Band, PageSection } from './rows'
import { groupSaved, refreshGroupIfGone, useSectionInView, UserState } from './shared'
import {
  AllowedHoursSection,
  BlockedGenresSection,
  LiveTvChannelsSection,
  ParentalControlSection,
  PermissionRows,
  PlaybackAccessSection,
  ResetDefaultsDialog,
  sectionTitle,
  VisibleLibrariesSection,
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
  'channels',
  'sources',
  'members',
] as const

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
  // Each save, discard or reset draws the sections again from the saved values.
  const [version, setVersion] = useState(0)
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

  const sections: [string, string][] = [
    ['name', t.groups.name],
    ['access', t.users.accessTitle],
    ['playback', t.users.playbackAccessTitle],
    ['parental', t.users.parentalTitle],
    ['libraries', t.users.visibleLibrariesTitle],
    ['genres', t.users.blockedGenresTitle],
    ['hours', t.users.allowedHoursTitle],
    ['channels', t.users.liveTvChannelsTitle],
    ['sources', t.groups.sourcesTitle],
    ['members', t.groups.membersTitle],
  ]
  const nav = (
    <SectionNav
      label={t.groups.sectionsLabel}
      current={current}
      items={sections.map(([id, label]) => ({ id, label, to: `#${id}` }))}
    />
  )

  const head = (
    <Band
      back={{ to: '/users/groups', label: t.groups.title }}
      leading={<IconTile icon={UsersThreeIcon} size="xl" />}
      title={group.name}
      facts={
        <>
          <span>{t.groups.memberCount(group.memberCount)}</span>
          <span>{t.groups.pageHelp}</span>
        </>
      }
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
    />
  )

  return (
    <>
      <PageLayout head={head} nav={nav}>
        <DraftForm target={target} onVersion={() => setVersion((count) => count + 1)}>
          {({ nameError }) => (
            <>
              <NameSection key={`name:${version}`} group={group} nameError={nameError} />
              <AccessSection group={group} />
              <PlaybackAccessSection key={`playback:${version}`} id="playback" target={target} />
              <ParentalControlSection key={`parental:${version}`} id="parental" target={target} />
              <VisibleLibrariesSection
                key={`libraries:${version}`}
                id="libraries"
                target={target}
              />
              <BlockedGenresSection key={`genres:${version}`} id="genres" target={target} />
              <AllowedHoursSection key={`hours:${version}`} id="hours" target={target} />
              <LiveTvChannelsSection key={`channels:${version}`} id="channels" target={target} />
              <SourcesSection group={group} />
              <MembersSection group={group} />
            </>
          )}
        </DraftForm>
      </PageLayout>
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
        onReset={() => setVersion((count) => count + 1)}
      />
    </>
  )
}

/** The group's name, saved with the dock. */
function NameSection({ group, nameError }: { group: Group; nameError: string | undefined }) {
  const { t } = useI18n()
  const [name, setName] = useState(group.name)
  useDraftPart('name', Number(name !== group.name), () => ({ name: name.trim() }))
  return (
    <PageSection id="name" title={t.groups.name}>
      <SettingRow
        kind="text"
        label={t.groups.name}
        help={t.groups.nameRule}
        value={name}
        saved={group.name}
        maxLength={64}
        error={nameError}
        onChange={setName}
      />
    </PageSection>
  )
}

/**
 * The permissions of Access the group gives its members, each saved as it changes. A member's
 * role, sign-in screen, Disabled and end date stay on their own page.
 */
function AccessSection({ group }: { group: Group }) {
  const { t } = useI18n()
  const toast = useToast()
  const access = useGroupPatch(group)

  return (
    <PageSection id="access" title={t.users.accessTitle} description={t.groups.accessHelp}>
      {access.isError && (
        <Notice tone="danger" live>
          {errorMessage(t, access.error)}
        </Notice>
      )}
      <PermissionRows
        values={group}
        disabled={false}
        onChange={(patch) => {
          // A change waits for the one before it.
          if (!access.isPending) {
            access.mutate(patch, { onSuccess: () => toast(t.groups.updated) })
          }
        }}
      />
    </PageSection>
  )
}

/**
 * The server's sources the group gets, one switch each, saved at once: a switch lists the group in
 * its source's Used by, or takes it off, through a request of its own, so that quick switches do
 * not undo each other. A source used by everyone is on and locked, with a link to its page, where
 * that changes. Members' own sources are theirs alone and not listed.
 */
function SourcesSection({ group }: { group: Group }) {
  const { t } = useI18n()
  const sources = useQuery({
    queryKey: queryKeys.addons('shared'),
    queryFn: ({ signal }) => fetchAddons('shared', signal),
  })

  return (
    <PageSection id="sources" title={t.groups.sourcesTitle} description={t.groups.sourcesHelp}>
      {sources.isPending ? (
        <SkeletonRows rows={3} boxed label={t.common.loading} />
      ) : sources.isError ? (
        <InlineError onRetry={() => void sources.refetch()} retrying={sources.isFetching}>
          {errorMessage(t, sources.error)}
        </InlineError>
      ) : sources.data.length === 0 ? (
        <EmptyState
          icon={PuzzlePieceIcon}
          title={t.groups.noSources}
          action={<ButtonLink to="/sources">{t.sources.add}</ButtonLink>}
        >
          {t.groups.noSourcesHelp}
        </EmptyState>
      ) : (
        <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
          {sources.data.map((source) => (
            <SourceSwitch key={source.id} group={group} source={source} />
          ))}
        </div>
      )}
    </PageSection>
  )
}

/** One server source on a group's page: on when the group, or everyone, gets it. */
function SourceSwitch({ group, source }: { group: Group; source: Addon }) {
  const { t } = useI18n()
  const toast = useToast()
  const save = useMutation({
    mutationFn: (uses: boolean) => setGroupSource(group.id, source.id, uses),
    onSuccess: (updated, uses) => {
      replaceCachedAddon('shared', updated)
      toast(uses ? t.groups.sourceGiven(updated.name) : t.groups.sourceTaken(updated.name))
    },
    onError: refreshGroupIfGone,
    // The list again: a source used by everyone meanwhile, or removed, is refused.
    onSettled: () => invalidateScope('shared'),
  })
  const everyone = source.usedBy.mode === 'everyone'
  const gets = source.usedBy.groups.includes(group.id)
  const kind = t.sources.kindName[source.kind]

  return (
    <SettingRow
      kind="switch"
      label={source.name}
      detail={everyone ? `${kind} · ${t.groups.sourceEveryone}` : kind}
      checked={everyone || (save.isPending ? save.variables === true : gets)}
      disabled={everyone}
      error={save.isError ? errorMessage(t, save.error) : undefined}
      onChange={(uses) => {
        // A change waits for the one before it.
        if (!save.isPending) {
          save.reset()
          save.mutate(uses)
        }
      }}
    >
      {everyone && (
        <TextLink to={sourcePath('shared', source.id)} className="-ml-3">
          {t.groups.changeWhoUses}
        </TextLink>
      )}
    </SettingRow>
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
    <PageSection
      id="members"
      title={t.groups.membersTitle}
      description={t.groups.membersHelp}
      aside={members.length > 0 && add}
    >
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
              leading={<Avatar name={member.name} image={userImageUrl(member)} size="md" />}
              title={member.name}
              titleAside={
                member.ownSections.length > 0 && <Badge tone="warn">{t.groups.ownSettings}</Badge>
              }
              meta={
                member.ownSections.length === 0
                  ? t.groups.followsHelp
                  : t.groups.ownSections(
                      member.ownSections.map((section) => sectionTitle(t, section)).join(', '),
                    )
              }
              trailing={<UserState user={member} />}
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
    </PageSection>
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
      <form id={formId} onSubmit={submit} noValidate className="space-y-4 p-5">
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
