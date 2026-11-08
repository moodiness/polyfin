import { useQuery } from '@tanstack/react-query'
import { fetchAddonPreferences, queryKeys } from '@/api'

/**
 * The signed-in user's addon preferences, and whether their own sources and smart lists are used:
 * not while the server or their own permission turned them off, nor under parental control or
 * blocked genres, which keep them to the server's sources. They are then kept, and may be turned
 * off or deleted, but not added or changed. `used` is undefined until the preferences are read.
 */
export function useOwnSources(enabled = true) {
  const preferences = useQuery({
    queryKey: queryKeys.addonPreferences,
    queryFn: ({ signal }) => fetchAddonPreferences(signal),
    enabled,
  })
  const used =
    preferences.data === undefined
      ? undefined
      : preferences.data.personalAddons && !preferences.data.parentalControl
  return { preferences, used }
}
