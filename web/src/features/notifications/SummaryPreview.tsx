import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { fetchSummaryPreview, fetchUsers, type SummaryEvent } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Field, InlineError, Modal, Notice, Select, SkeletonRows } from '@/ui'

const summaries: SummaryEvent[] = ['weekly_summary', 'monthly_summary', 'yearly_summary']

/**
 * Settings › Notifications › Preview: a summary of the server or of a user, as an email target
 * would receive it now, in a sandboxed frame. Nothing is sent.
 */
export default function SummaryPreview() {
  const { t } = useI18n()
  const text = t.notifications.preview
  const [event, setEvent] = useState<SummaryEvent>('weekly_summary')
  const [user, setUser] = useState('')
  const [shown, setShown] = useState<{ event: SummaryEvent; user: string } | null>(null)
  const users = useQuery({ queryKey: ['users'], queryFn: ({ signal }) => fetchUsers(signal) })
  return (
    <div className="flex flex-col gap-4">
      <p className="max-w-[60ch] text-small text-ink-3">{text.help}</p>
      <div className="flex flex-wrap items-end gap-4">
        <Field label={text.summary}>
          <Select
            value={event}
            options={summaries.map((value) => ({ value, label: text.summaries[value] }))}
            onValue={setEvent}
            className="max-w-[14rem]"
          />
        </Field>
        <Field label={text.of}>
          <Select
            value={user}
            options={[
              { value: '', label: text.server },
              ...(users.data ?? []).map((u) => ({ value: u.id, label: u.name })),
            ]}
            onValue={setUser}
            className="max-w-[14rem]"
          />
        </Field>
        <Button onClick={() => setShown({ event, user })}>{text.show}</Button>
      </div>
      {shown !== null && <PreviewModal {...shown} onClose={() => setShown(null)} />}
    </div>
  )
}

function PreviewModal({
  event,
  user,
  onClose,
}: {
  event: SummaryEvent
  user: string
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.notifications.preview
  const preview = useQuery({
    queryKey: ['notifications', 'preview', event, user],
    queryFn: ({ signal }) => fetchSummaryPreview(event, user || undefined, signal),
  })
  return (
    <Modal open onClose={onClose} title={preview.data?.subject ?? text.title} width={760}>
      <div className="space-y-4 p-5">
        {preview.isPending ? (
          <SkeletonRows rows={4} label={text.loading} />
        ) : preview.isError ? (
          <InlineError onRetry={() => void preview.refetch()} retrying={preview.isFetching}>
            {errorMessage(t, preview.error)}
          </InlineError>
        ) : (
          <>
            {preview.data.empty && <Notice tone="info">{text.empty}</Notice>}
            <iframe
              title={text.frame}
              sandbox=""
              srcDoc={preview.data.html}
              className="h-[60vh] w-full rounded-panel border border-line-2 bg-white"
            />
          </>
        )}
      </div>
    </Modal>
  )
}
