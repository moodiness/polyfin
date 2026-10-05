import { UserIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { fetchUsers, queryKeys } from '@/api'
import { registerPaletteSource } from '@/app/palette/registry'

// Every user, for administrators: the same query as the Users page, so its cache serves both.
registerPaletteSource('users', ({ t, user }) => {
  const users = useQuery({
    queryKey: queryKeys.users,
    queryFn: ({ signal }) => fetchUsers(signal),
    enabled: user.isAdministrator,
  })
  if (!user.isAdministrator) return []
  return (users.data ?? []).map((item) => ({
    id: `users:${item.id}`,
    group: 'users' as const,
    label: item.name,
    hint: item.isAdministrator ? t.nav.administrator : t.nav.member,
    icon: UserIcon,
    to: `/users/${item.id}`,
  }))
})
