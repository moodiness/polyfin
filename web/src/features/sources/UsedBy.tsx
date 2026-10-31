import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchGroups,
  fetchUsers,
  queryKeys,
  updateAddon,
  type Addon,
  type GroupSummary,
  type UsedBy,
  type User,
} from '@/api'
import { useSessionUser } from '@/app/session'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Checkbox, Field, FieldError, Notice, Segmented, useToast } from '@/ui'
import { invalidateScope, replaceCachedAddon } from './model'

/** A server source used by everyone: the default of a new one. */
export const everyone: UsedBy = { mode: 'everyone', groups: [], users: [] }

/** Whether two settings are the same, whatever the order of their lists. */
const sameUsedBy = (a: UsedBy, b: UsedBy) =>
  a.mode === b.mode &&
  [...a.groups].sort().join() === [...b.groups].sort().join() &&
  [...a.users].sort().join() === [...b.users].sort().join()

/** The groups and the users a source may be limited to, as the admin app lists them. */
function useAccounts() {
  const groups = useQuery({
    queryKey: queryKeys.groups,
    queryFn: ({ signal }) => fetchGroups(signal),
  })
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  return { groups: groups.data ?? [], users: users.data ?? [] }
}

/** Whether a user uses a source set to `usedBy`: listed, or their group is. */
export const usesSource = (usedBy: UsedBy, user: Pick<User, 'id' | 'groupId'>) =>
  usedBy.mode === 'everyone' ||
  usedBy.users.includes(user.id) ||
  (user.groupId !== null && usedBy.groups.includes(user.groupId))

/**
 * The names a source's Used by lists, groups then users, sorted; those deleted meanwhile are left
 * out, as the server drops them.
 */
export function usedByNames(usedBy: UsedBy, groups: GroupSummary[], users: User[]): string[] {
  const named = (ids: string[], list: { id: string; name: string }[]) =>
    list
      .filter((entry) => ids.includes(entry.id))
      .map((entry) => entry.name)
      .sort((a, b) => a.localeCompare(b))
  return [...named(usedBy.groups, groups), ...named(usedBy.users, users)]
}

/**
 * Who uses a server source: Everyone, or only the groups and users ticked. It warns when the
 * signed-in administrator is left out, and when no one is ticked.
 */
export function UsedByPicker({
  value,
  onChange,
  disabled = false,
}: {
  value: UsedBy
  onChange: (value: UsedBy) => void
  disabled?: boolean
}) {
  const { t } = useI18n()
  const text = t.usedBy
  const self = useSessionUser()
  const { groups, users } = useAccounts()
  const me = users.find((user) => user.id === self.id)
  const toggle = (list: 'groups' | 'users', id: string, checked: boolean) =>
    onChange({
      ...value,
      [list]: checked ? [...value[list], id] : value[list].filter((other) => other !== id),
    })
  return (
    <div className="flex flex-col gap-4">
      <Segmented<UsedBy['mode']>
        label={text.title}
        value={value.mode}
        onChange={(mode) => onChange({ ...value, mode })}
        options={[
          { value: 'everyone', label: text.everyone },
          { value: 'listed', label: text.listed },
        ]}
        className="self-start max-sm:self-stretch"
      />
      {value.mode === 'listed' && (
        <>
          <div className="grid gap-6 sm:grid-cols-2">
            <fieldset className="flex min-w-0 flex-col gap-2" disabled={disabled}>
              <legend className="mb-2 text-small font-medium text-ink-2">{text.groups}</legend>
              {groups.length === 0 && <p className="text-small text-ink-3">{text.noGroups}</p>}
              {groups.map((group) => (
                <Checkbox
                  key={group.id}
                  label={group.name}
                  aside={text.members(group.memberCount)}
                  checked={value.groups.includes(group.id)}
                  onChange={(checked) => toggle('groups', group.id, checked)}
                  disabled={disabled}
                />
              ))}
            </fieldset>
            <fieldset className="flex min-w-0 flex-col gap-2" disabled={disabled}>
              <legend className="mb-2 text-small font-medium text-ink-2">{text.users}</legend>
              {users.map((user) => (
                <Checkbox
                  key={user.id}
                  label={user.name}
                  help={
                    user.groupId !== null
                      ? groups.find((g) => g.id === user.groupId)?.name
                      : undefined
                  }
                  checked={value.users.includes(user.id)}
                  onChange={(checked) => toggle('users', user.id, checked)}
                  disabled={disabled}
                />
              ))}
            </fieldset>
          </div>
          {value.groups.length === 0 && value.users.length === 0 ? (
            <Notice tone="warn">{text.nobody}</Notice>
          ) : (
            me !== undefined &&
            !usesSource(value, me) && <Notice tone="warn">{text.selfLeftOut}</Notice>
          )}
          <p className="text-small text-ink-3">{text.ownAddons}</p>
        </>
      )}
    </div>
  )
}

/**
 * A server source's Used by on its page: the setting, saved with its button, and the activity log
 * records the change.
 */
export function SourceUsedBy({ addon }: { addon: Addon }) {
  const { t } = useI18n()
  const text = t.usedBy
  const toast = useToast()
  const [draft, setDraft] = useState<UsedBy>(addon.usedBy)
  const save = useMutation({
    mutationFn: (usedBy: UsedBy) => updateAddon('shared', addon.id, { usedBy }),
    onSuccess: (updated) => {
      replaceCachedAddon('shared', updated)
      setDraft(updated.usedBy)
      toast(text.saved(updated.name))
    },
    onSettled: () => invalidateScope('shared'),
  })
  const changed = !sameUsedBy(draft, addon.usedBy)
  return (
    <Field label={text.title} help={text.help}>
      <div className="flex flex-col gap-4">
        <UsedByPicker
          value={draft}
          onChange={(value) => {
            save.reset()
            setDraft(value)
          }}
          disabled={save.isPending}
        />
        {save.isError && <FieldError>{errorMessage(t, save.error)}</FieldError>}
        {changed && (
          <div className="flex gap-2">
            <Button variant="primary" loading={save.isPending} onClick={() => save.mutate(draft)}>
              {text.save}
            </Button>
            <Button
              variant="secondary"
              disabled={save.isPending}
              onClick={() => setDraft(addon.usedBy)}
            >
              {text.cancel}
            </Button>
          </div>
        )}
      </div>
    </Field>
  )
}

/** A server source's Used by in the list: nothing for everyone, else who uses it. */
export function UsedByChip({ usedBy }: { usedBy: UsedBy }) {
  const { t } = useI18n()
  const { groups, users } = useAccounts()
  if (usedBy.mode === 'everyone') return null
  const names = usedByNames(usedBy, groups, users)
  return (
    <span className="truncate text-small text-ink-3">
      {names.length === 0 ? t.usedBy.chipNone : t.usedBy.chip(names.join(', '))}
    </span>
  )
}
