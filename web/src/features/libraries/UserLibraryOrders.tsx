import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { applyLibraryOrder, libraryOrders } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, ConfirmDialog, useToast } from '@/ui'

/**
 * Under the server's libraries: says that a user who ordered them in the web player's Settings ›
 * Home keeps that order, and offers Apply this order to every user, which forgets those orders once
 * confirmed, after saying how many users keep one. The order applied is the saved one, so the
 * action waits while the list has unsaved changes.
 */
export default function UserLibraryOrders({ dirty }: { dirty: boolean }) {
  const { t } = useI18n()
  const u = t.libraries.userOrders
  const toast = useToast()
  const [confirming, setConfirming] = useState(false)
  // Counted each time the dialog opens: users may have saved an order since.
  const count = useQuery({
    queryKey: ['scopes', 'shared', 'libraries', 'user-orders'],
    queryFn: libraryOrders,
    enabled: confirming,
    staleTime: 0,
    gcTime: 0,
  })
  const apply = useMutation({
    mutationFn: applyLibraryOrder,
    onSuccess: ({ users }) => {
      setConfirming(false)
      toast(u.applied(users), { tone: 'ok' })
    },
  })
  const users = count.data?.users
  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-x-5 gap-y-2.5 rounded-row bg-s1 py-3 pr-3 pl-4 max-md:pr-3 max-md:pl-3.5">
      <div className="max-w-[66ch] min-w-0 flex-[1_1_20rem] text-small text-ink-2">
        <p>{u.help}</p>
        {dirty && <p className="mt-1 text-ink-3">{u.saveFirst}</p>}
      </div>
      <Button
        onClick={() => {
          apply.reset()
          setConfirming(true)
        }}
        disabled={dirty}
        className="max-md:w-full"
      >
        {u.apply}
      </Button>
      <ConfirmDialog
        open={confirming}
        onClose={() => setConfirming(false)}
        onConfirm={() => apply.mutate()}
        title={u.title}
        confirmLabel={u.apply}
        tone="primary"
        busy={apply.isPending || count.isPending}
        error={
          apply.isError
            ? errorMessage(t, apply.error)
            : count.isError
              ? errorMessage(t, count.error)
              : undefined
        }
      >
        {users === undefined ? u.counting : users === 0 ? u.none : u.some(users)}
      </ConfirmDialog>
    </div>
  )
}
