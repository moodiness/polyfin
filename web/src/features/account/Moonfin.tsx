import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
  fetchOwnMoonfin,
  forgetOwnMoonfinSettings,
  queryClient,
  queryKeys,
  saveOwnMoonfin,
  type MoonfinSync,
  type OwnMoonfin as OwnMoonfinAnswer,
  type UserMoonfin,
} from '@/api'
import { dateTime, errorMessage, relativeTime } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Button,
  ConfirmDialog,
  InlineError,
  Notice,
  SettingRow,
  SkeletonRows,
  useToast,
} from '@/ui'

/** The choices of Moonfin's companion: the server's default, naming whether it is on, On, Off. */
export function moonfinOptions(serverDefault: boolean, t: Messages) {
  const text = t.account.moonfin
  return [
    { value: '' as const, label: text.server(serverDefault ? t.ui.on : t.ui.off) },
    { value: 'on' as const, label: t.ui.on },
    { value: 'off' as const, label: t.ui.off },
  ]
}

/**
 * The settings Moonfin's apps synced for an account, as a row: how many, in which profiles, and
 * when and by which app they were last saved, or that none were synced yet; with a button that
 * forgets them once confirmed.
 */
export function MoonfinSyncRow({
  sync,
  forget,
  onForgotten,
  button,
  title,
  confirm,
}: {
  sync: MoonfinSync
  forget: () => Promise<void>
  onForgotten: () => void
  button: string
  title: string
  confirm: string
}) {
  const { t, language } = useI18n()
  const text = t.account.moonfin
  const toast = useToast()
  const [asking, setAsking] = useState(false)
  const reset = useMutation({
    mutationFn: forget,
    onSuccess: () => {
      setAsking(false)
      onForgotten()
      toast(text.resetDone)
    },
  })
  const profiles = new Intl.ListFormat(language, { type: 'conjunction' }).format(
    sync.profiles.map((profile) => text.profiles[profile]),
  )
  const when =
    sync.updatedAt === null ? null : relativeTime(sync.updatedAt, language, t.time.justNow)

  return (
    <>
      <div
        className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2 rounded-row bg-s1 px-4 py-3"
        data-testid="moonfin-sync"
      >
        <div className="min-w-0 flex-1 basis-60">
          <p className="text-h3 text-ink">{text.sync}</p>
          {sync.synced ? (
            <>
              <p className="mt-0.5 text-small text-ink-2">
                {profiles === ''
                  ? text.settings(sync.settings)
                  : text.settingsIn(text.settings(sync.settings), profiles)}
              </p>
              {sync.updatedAt !== null && when !== null && (
                <p
                  className="mt-0.5 text-small text-ink-2"
                  title={dateTime(sync.updatedAt, language)}
                >
                  {sync.updatedBy === ''
                    ? text.lastSaved(when)
                    : text.lastSavedBy(when, sync.updatedBy)}
                </p>
              )}
            </>
          ) : (
            <p className="mt-0.5 text-small text-ink-2">{text.notSynced}</p>
          )}
        </div>
        <Button
          variant="secondary"
          icon={ArrowCounterClockwiseIcon}
          disabled={!sync.synced}
          onClick={() => {
            reset.reset()
            setAsking(true)
          }}
        >
          {button}
        </Button>
      </div>
      <ConfirmDialog
        open={asking}
        onClose={() => setAsking(false)}
        onConfirm={() => reset.mutate()}
        title={title}
        confirmLabel={button}
        busy={reset.isPending}
        error={reset.isError ? errorMessage(t, reset.error) : undefined}
      >
        {confirm}
      </ConfirmDialog>
    </>
  )
}

/**
 * My account › Moonfin: whether Moonfin's companion serves the user, a setting row saved as it
 * changes, read only, with why, while the administrator keeps them from changing it; then the
 * settings Moonfin's apps synced for them, with Reset my synced settings.
 */
export function OwnMoonfin() {
  const { t } = useI18n()
  const text = t.account.moonfin
  const toast = useToast()
  const current = useQuery({
    queryKey: queryKeys.ownMoonfin,
    queryFn: ({ signal }) => fetchOwnMoonfin(signal),
  })
  const save = useMutation({
    mutationFn: saveOwnMoonfin,
    onSuccess: (saved) => {
      queryClient.setQueryData<OwnMoonfinAnswer>(queryKeys.ownMoonfin, saved)
      // An administrator's own page under Users shows their choice.
      void queryClient.invalidateQueries({ queryKey: queryKeys.users })
      toast(text.saved)
    },
  })

  if (current.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (current.isError) {
    return (
      <InlineError onRetry={() => void current.refetch()} retrying={current.isFetching}>
        {errorMessage(t, current.error)}
      </InlineError>
    )
  }
  const { moonfin, locked, serverDefault, sync } = current.data
  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
      <SettingRow<UserMoonfin>
        kind="choice"
        anchor="moonfin-choice"
        label={text.label}
        help={text.help}
        value={moonfin}
        options={moonfinOptions(serverDefault, t)}
        onChange={(chosen) => save.mutate(chosen)}
        disabled={locked || save.isPending}
        error={save.isError ? errorMessage(t, save.error) : undefined}
      >
        {locked ? <Notice>{t.errors.moonfin_locked}</Notice> : undefined}
      </SettingRow>
      <MoonfinSyncRow
        sync={sync}
        forget={forgetOwnMoonfinSettings}
        onForgotten={() => {
          void queryClient.invalidateQueries({ queryKey: queryKeys.ownMoonfin })
          // An administrator's own page under Users shows their synced settings.
          void queryClient.invalidateQueries({ queryKey: queryKeys.usersMoonfin })
        }}
        button={text.reset}
        title={text.resetTitle}
        confirm={text.resetConfirm}
      />
    </div>
  )
}
