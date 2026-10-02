import { QueryClient } from '@tanstack/react-query'

export type Status = {
  name: string
  version: string
  serverId: string
  database: 'ready' | 'unavailable'
}

export async function fetchStatus(signal?: AbortSignal): Promise<Status> {
  const response = await fetch(`${import.meta.env.BASE_URL}api/status`, {
    headers: { Accept: 'application/json' },
    signal,
  })
  if (!response.ok) throw new Error(`GET /admin/api/status failed with HTTP ${response.status}`)
  return (await response.json()) as Status
}

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Fail fast so an unreachable server is reported within seconds, not after long backoff.
      retry: 1,
    },
  },
})
