import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { approveQuickConnect, lookupQuickConnect, queryClient, queryKeys } from '@/api'
import { useSessionUser } from '@/app/session'
import {
  buttonPrimary,
  buttonSecondary,
  Card,
  Notice,
  PageHeader,
  RelativeTime,
  TextField,
} from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

const codeLength = 6

export default function QuickConnectPage() {
  const { t } = useI18n()
  const user = useSessionUser()
  const [code, setCode] = useState('')
  const [approvedDevice, setApprovedDevice] = useState<string | null>(null)
  const complete = code.length === codeLength

  const lookup = useQuery({
    queryKey: ['quick-connect', code],
    queryFn: ({ signal }) => lookupQuickConnect(code, signal),
    enabled: complete && approvedDevice === null,
    retry: false,
    gcTime: 0,
  })

  const approve = useMutation({
    mutationFn: approveQuickConnect,
    onSuccess: () => {
      setApprovedDevice(lookup.data?.deviceName ?? '')
      void queryClient.invalidateQueries({ queryKey: queryKeys.myDevices })
    },
  })

  function reset() {
    setCode('')
    setApprovedDevice(null)
    approve.reset()
  }

  if (approvedDevice !== null) {
    return (
      <>
        <PageHeader title={t.quickConnect.title} description={t.quickConnect.description} />
        <Card>
          <Notice kind="success">{t.quickConnect.approved(approvedDevice, user.name)}</Notice>
          <button type="button" className={`${buttonSecondary} mt-4`} onClick={reset}>
            {t.quickConnect.another}
          </button>
        </Card>
      </>
    )
  }

  return (
    <>
      <PageHeader title={t.quickConnect.title} description={t.quickConnect.description} />
      <div className="max-w-lg space-y-6">
        <Card>
          <TextField
            label={t.quickConnect.code}
            hint={t.quickConnect.codeHint}
            value={code}
            onValue={(value) => {
              approve.reset()
              setCode(value.replace(/\D/g, '').slice(0, codeLength))
            }}
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}"
            maxLength={codeLength}
            className="font-mono tracking-[0.4em]"
          />
        </Card>

        {complete && lookup.isPending && (
          <p role="status" className="text-muted">
            {t.quickConnect.lookingUp}
          </p>
        )}
        {complete && lookup.isError && (
          <Notice kind="error">{errorMessage(t, lookup.error)}</Notice>
        )}
        {complete && lookup.data && (
          <Card title={t.quickConnect.requestTitle}>
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
              <dt className="text-muted">{t.quickConnect.device}</dt>
              <dd className="font-medium text-white">{lookup.data.deviceName}</dd>
              <dt className="text-muted">{t.quickConnect.app}</dt>
              <dd className="text-white">
                {lookup.data.appName} {lookup.data.appVersion}
              </dd>
              <dt className="text-muted">{t.quickConnect.requested}</dt>
              <dd className="text-white">
                <RelativeTime iso={lookup.data.requestedAt} />
              </dd>
            </dl>
            {approve.isError && (
              <div className="mt-4">
                <Notice kind="error">{errorMessage(t, approve.error)}</Notice>
              </div>
            )}
            <button
              type="button"
              className={`${buttonPrimary} mt-5`}
              disabled={approve.isPending}
              onClick={() => approve.mutate(code)}
            >
              {approve.isPending ? t.quickConnect.approving : t.quickConnect.approve}
            </button>
          </Card>
        )}
      </div>
    </>
  )
}
