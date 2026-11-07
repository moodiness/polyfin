import {
  CheckCircleIcon,
  DotsThreeIcon,
  KeyIcon,
  ProhibitIcon,
  SignOutIcon,
} from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import {
  ApiError,
  fetchUserDevices,
  queryClient,
  queryKeys,
  signOutUserDevice,
  type User,
} from '@/api'
import { useSessionUser } from '@/app/session'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { ConfirmDialog, IconButton, Menu, MenuItem, MenuSeparator, useToast } from '@/ui'
import PasswordModal from './PasswordModal'
import { useUserPatch } from './shared'

/**
 * The "…" of a user's row in the list: Disable or Enable, Reset password, and Sign out of all
 * devices, so that none of them opens the user's page. Disable and the sign-out are asked first;
 * an administrator never gets Disable on their own row.
 */
export default function UserActions({ user }: { user: User }) {
  const { t } = useI18n()
  const toast = useToast()
  const self = useSessionUser()
  const [asked, setAsked] = useState<'disable' | 'signOut' | null>(null)
  const [resetting, setResetting] = useState(false)
  const access = useUserPatch(user)
  const signOut = useMutation({
    // One device after another, as the user's page signs them out; one already gone is skipped.
    mutationFn: async () => {
      const devices = await fetchUserDevices(user.id)
      for (const device of devices) {
        await signOutUserDevice(user.id, device.id).catch((error: unknown) => {
          if (!(error instanceof ApiError && error.status === 404)) throw error
        })
      }
      return devices.length
    },
    onSuccess: (count) => {
      setAsked(null)
      toast(t.users.signedOutAll(count))
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.userDevices(user.id) })
    },
  })

  // Disabling revokes the user's share links, as the switch on their page does.
  function setDisabled(isDisabled: boolean) {
    access.mutate(
      { isDisabled },
      {
        onSuccess: () => {
          setAsked(null)
          toast((isDisabled ? t.users.disabledNow : t.users.enabledNow)(user.name))
          void queryClient.invalidateQueries({ queryKey: queryKeys.userShareLinks(user.id) })
        },
        onError: (error) => {
          if (!isDisabled) toast(errorMessage(t, error), { tone: 'danger' })
        },
      },
    )
  }

  const label = t.users.moreActions(user.name)
  return (
    <>
      <Menu
        label={label}
        align="end"
        trigger={(props) => <IconButton {...props} size="sm" label={label} icon={DotsThreeIcon} />}
      >
        {user.isDisabled ? (
          <MenuItem
            icon={CheckCircleIcon}
            disabled={access.isPending}
            onSelect={() => setDisabled(false)}
          >
            {t.users.enable}
          </MenuItem>
        ) : (
          user.id !== self.id && (
            <MenuItem
              icon={ProhibitIcon}
              onSelect={() => {
                access.reset()
                setAsked('disable')
              }}
            >
              {t.users.disable}
            </MenuItem>
          )
        )}
        <MenuItem icon={KeyIcon} onSelect={() => setResetting(true)}>
          {t.users.resetPassword}
        </MenuItem>
        <MenuSeparator />
        <MenuItem
          icon={SignOutIcon}
          danger
          onSelect={() => {
            signOut.reset()
            setAsked('signOut')
          }}
        >
          {t.users.signOutAll}
        </MenuItem>
      </Menu>
      <ConfirmDialog
        open={asked === 'disable'}
        onClose={() => setAsked(null)}
        onConfirm={() => setDisabled(true)}
        title={t.users.disableTitle(user.name)}
        confirmLabel={access.isPending ? t.users.disabling : t.users.disable}
        busy={access.isPending}
        error={access.isError ? errorMessage(t, access.error) : undefined}
      >
        {t.users.isDisabledHelp}
      </ConfirmDialog>
      <ConfirmDialog
        open={asked === 'signOut'}
        onClose={() => setAsked(null)}
        onConfirm={() => signOut.mutate()}
        title={t.users.signOutAll}
        confirmLabel={signOut.isPending ? t.users.signingOutAll : t.users.signOutAll}
        busy={signOut.isPending}
        error={signOut.isError ? errorMessage(t, signOut.error) : undefined}
      >
        {t.users.signOutAllConfirm(user.name)}
      </ConfirmDialog>
      <PasswordModal user={user} open={resetting} onClose={() => setResetting(false)} />
    </>
  )
}
