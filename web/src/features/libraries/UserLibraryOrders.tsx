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
    <div className="mt-5 max-w-[72ch]">
      <p className="text-small text-ink-2">{u.help}</p>
      <div className="mt-3 flex flex-wrap items-center gap-3">
        <Button
          onClick={() => {
            apply.reset()
            setConfirming(true)
          }}
          disabled={dirty}
        >
          {u.apply}
        </Button>
        {dirty && <span className="text-small text-ink-3">{u.saveFirst}</span>}
      </div>
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
