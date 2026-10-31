import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { fetchUsers, queryKeys } from '@/api'
import { useSessionUser } from '@/app/session'
import { useI18n } from '@/i18n'
import { Field, Select } from '@/ui'

/**
 * The user Content › Versions explains versions for, from `?user=`: empty for the administrator
 * signed in. Links between its pages keep it.
 */
export function useVersionsUser(): [string, (user: string) => void] {
  const [params, setParams] = useSearchParams()
  const user = params.get('user') ?? ''
  const setUser = (next: string) =>
    setParams(
      (current) => {
        const updated = new URLSearchParams(current)
        if (next === '') updated.delete('user')
        else updated.set('user', next)
        return updated
      },
      { replace: true },
    )
  return [user, setUser]
}

/** `?user=…` to add to a link of the Versions pages, empty for the administrator signed in. */
export const userQuery = (user: string) => (user === '' ? '' : `?${new URLSearchParams({ user })}`)

/** Picks the user the Versions pages explain versions for. */
export function UserPicker({ user, onUser }: { user: string; onUser: (user: string) => void }) {
  const { t } = useI18n()
  const me = useSessionUser()
  const users = useQuery({ queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) })
  const options = [
    { value: '', label: t.versions.you(me.name) },
    ...(users.data ?? [])
      .filter((other) => other.id !== me.id)
      .map((other) => ({ value: other.id, label: other.name })),
  ]
  return (
    <Field label={t.versions.asUser} help={t.versions.asUserHelp} className="max-w-sm">
      <Select value={user} options={options} onValue={onUser} />
    </Field>
  )
}
