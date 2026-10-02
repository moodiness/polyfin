import { useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, queryClient, type Device } from '@/api'
import { ConfirmButton, Loading, Notice, RelativeTime } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

/** Jellyfin devices signed in as one user, each with a confirmed Sign out action. */
export default function DeviceList({
  queryKey,
  load,
  signOut,
}: {
  queryKey: readonly unknown[]
  load: (signal: AbortSignal) => Promise<Device[]>
  signOut: (deviceId: string) => Promise<void>
}) {
  const { t } = useI18n()
  const devices = useQuery({ queryKey, queryFn: ({ signal }) => load(signal) })
  const mutation = useMutation({
    mutationFn: (device: Device) => signOut(device.id),
    onSettled: (_data, error) => {
      // A 404 means the device is already gone: refresh either way.
      if (error === null || (error instanceof ApiError && error.status === 404)) {
        void queryClient.invalidateQueries({ queryKey })
      }
    },
  })

  if (devices.isPending) return <Loading />
  if (devices.isError) return <Notice kind="error">{errorMessage(t, devices.error)}</Notice>

  return (
    <div className="space-y-3">
      {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
      {mutation.isSuccess && (
        <Notice kind="success">{t.devices.signedOut(mutation.variables.deviceName)}</Notice>
      )}
      {devices.data.length === 0 ? (
        <p className="text-sm text-muted">{t.devices.empty}</p>
      ) : (
        <ul className="divide-y divide-line rounded-xl border border-line">
          {devices.data.map((device) => (
            <li
              key={device.id}
              className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="min-w-0 text-sm">
                <p className="font-medium break-words text-white">{device.deviceName}</p>
                <p className="text-muted">
                  {device.client} {device.clientVersion}
                </p>
                <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 text-muted">
                  <dt>{t.devices.lastActivity}</dt>
                  <dd className="text-zinc-200">
                    <RelativeTime iso={device.lastActivityAt} />
                  </dd>
                  <dt>{t.devices.address}</dt>
                  <dd className="font-mono break-all text-zinc-200">{device.remoteAddress}</dd>
                </dl>
              </div>
              <div className="shrink-0">
                <ConfirmButton
                  label={t.devices.signOut}
                  busyLabel={t.devices.signingOut}
                  message={t.devices.signOutConfirm(device.deviceName)}
                  busy={mutation.isPending && mutation.variables.id === device.id}
                  onConfirm={() => mutation.mutate(device)}
                />
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
