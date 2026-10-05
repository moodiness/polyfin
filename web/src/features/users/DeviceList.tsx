import { BrowserIcon, DevicesIcon, SignOutIcon, TelevisionIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { ApiError, queryClient, type Device } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  ConfirmDialog,
  EmptyState,
  InlineError,
  Notice,
  Row,
  RowList,
  SkeletonRows,
  useToast,
} from '@/ui'
import { RelativeTime } from './shared'

export type DeviceListProps = {
  /** The query key of the list; it is refreshed after a sign-out. */
  queryKey: readonly unknown[]
  /** Loads the devices. */
  load: (signal: AbortSignal) => Promise<Device[]>
  /** Signs one device out. */
  signOut: (deviceId: string) => Promise<void>
}

/** A rough kind of device from its name and app, for the row's icon only. */
function deviceIcon(device: Device) {
  const words = `${device.deviceName} ${device.client}`.toLowerCase()
  if (/\btv\b|television|télé/.test(words)) return TelevisionIcon
  if (/web|browser|firefox|chrome|safari|edge/.test(words)) return BrowserIcon
  return DevicesIcon
}

/**
 * The Jellyfin devices signed in as one user, each with a Sign out action asked in a dialog.
 * Used on a user's page and on My account.
 */
export default function DeviceList({ queryKey, load, signOut }: DeviceListProps) {
  const { t } = useI18n()
  const toast = useToast()
  const [asked, setAsked] = useState<Device | null>(null)
  const devices = useQuery({ queryKey, queryFn: ({ signal }) => load(signal) })
  const mutation = useMutation({
    mutationFn: (device: Device) => signOut(device.id),
    onSuccess: (_data, device) => {
      setAsked(null)
      toast(t.devices.signedOut(device.deviceName))
    },
    onSettled: (_data, error) => {
      // A 404 means the device is already gone: refresh either way.
      if (error === null || (error instanceof ApiError && error.status === 404)) {
        void queryClient.invalidateQueries({ queryKey })
      }
    },
  })

  if (devices.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (devices.isError) {
    return (
      <InlineError onRetry={() => void devices.refetch()} retrying={devices.isFetching}>
        {errorMessage(t, devices.error)}
      </InlineError>
    )
  }

  return (
    <div className="space-y-3">
      {mutation.isError && asked === null && (
        <Notice tone="danger">{errorMessage(t, mutation.error)}</Notice>
      )}
      {devices.data.length === 0 ? (
        <EmptyState icon={DevicesIcon} title={t.devices.empty}>
          {t.users.devicesEmptyHelp}
        </EmptyState>
      ) : (
        <RowList aria-label={t.users.devicesTitle}>
          {devices.data.map((device) => (
            <Row
              key={device.id}
              leading={deviceIcon(device)}
              title={device.deviceName}
              meta={
                <span className="truncate">
                  {device.client} {device.clientVersion}
                  {/* On a phone the time moves here, as the right column hides. */}
                  <span className="sm:hidden">
                    {', '}
                    <RelativeTime iso={device.lastActivityAt} />
                  </span>
                </span>
              }
              trailing={
                <div className="flex items-center gap-4">
                  <div className="text-right text-small max-sm:hidden">
                    <p className="text-ink-2">
                      <span className="sr-only">{t.devices.lastActivity} </span>
                      <RelativeTime iso={device.lastActivityAt} />
                    </p>
                    <p className="figures text-micro break-all text-ink-3">
                      <span className="sr-only">{t.devices.address} </span>
                      {device.remoteAddress}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    icon={SignOutIcon}
                    aria-label={`${t.devices.signOut}: ${device.deviceName}`}
                    loading={mutation.isPending && mutation.variables.id === device.id}
                    onClick={() => {
                      mutation.reset()
                      setAsked(device)
                    }}
                  >
                    {t.devices.signOut}
                  </Button>
                </div>
              }
            />
          ))}
        </RowList>
      )}
      <ConfirmDialog
        open={asked !== null}
        onClose={() => setAsked(null)}
        onConfirm={() => asked && mutation.mutate(asked)}
        title={t.devices.signOut}
        confirmLabel={t.devices.signOut}
        busy={mutation.isPending}
        error={mutation.isError ? errorMessage(t, mutation.error) : undefined}
      >
        {asked && t.devices.signOutConfirm(asked.deviceName)}
      </ConfirmDialog>
    </div>
  )
}
