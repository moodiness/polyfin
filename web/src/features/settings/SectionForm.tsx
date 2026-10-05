import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { useBlocker } from 'react-router'
import { useMutation } from '@tanstack/react-query'
import { ApiError, queryClient, queryKeys, saveSettings, type Settings } from '@/api'
import type { SettingsSectionId } from '@/app/navigation'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { ConfirmDialog, SaveBar, useToast } from '@/ui'
import { settingEntries } from './catalog'

/** What a section page draws its fields from. */
export type SectionFormApi = {
  form: Settings
  update: (patch: Partial<Settings>) => void
  /** The save error about the setting at this anchor, shown under it. */
  error: (anchor: string) => string | undefined
  /** Counts the saves, so that secret fields start over (back to the dots) after each one. */
  saves: number
}

/**
 * The form of one settings section: its fields, one save bar that saves them, and a guard
 * (in-app navigation and closing the tab) while changes are not saved.
 */
export default function SectionForm({
  section,
  initial,
  children,
}: {
  section: SettingsSectionId
  initial: Settings
  children: (api: SectionFormApi) => ReactNode
}) {
  const { t } = useI18n()
  const toast = useToast()
  const [saved, setSaved] = useState(initial)
  const [form, setForm] = useState(initial)
  const [saves, setSaves] = useState(0)
  const dirty = JSON.stringify(form) !== JSON.stringify(saved)

  const mutation = useMutation({
    mutationFn: saveSettings,
    onSuccess: (result) => {
      queryClient.setQueryData(queryKeys.settings, result)
      setSaved(result)
      setForm(result)
      setSaves((count) => count + 1)
      toast(t.settings.saved, { tone: 'ok' })
      // The server name is part of the public status.
      void queryClient.invalidateQueries({ queryKey: queryKeys.status })
      // Library names in apps follow the server language.
      void queryClient.invalidateQueries({ queryKey: queryKeys.scopes })
    },
  })

  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      dirty && currentLocation.pathname !== nextLocation.pathname,
  )
  useEffect(() => {
    if (!dirty) return
    const warn = (event: BeforeUnloadEvent) => event.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])

  function update(patch: Partial<Settings>) {
    mutation.reset()
    setForm((current) => ({ ...current, ...patch }))
  }

  // A save error goes under the setting of this section it is about, else in the save bar.
  const failure = mutation.error
  const failedEntry =
    failure instanceof ApiError
      ? settingEntries.find(
          (entry) => entry.section === section && entry.codes?.includes(failure.code),
        )
      : undefined
  const failureText = failure === null ? undefined : errorMessage(t, failure)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate({
      ...form,
      serverName: form.serverName.trim(),
      traktClientId: form.traktClientId.trim(),
      simklClientId: form.simklClientId.trim(),
      // The order POLYFIN_SEGMENTS gives is not saved, so that it keeps following the variable.
      segmentOrder:
        form.segmentOrder.join() === form.segmentOrderDefault.join() ? [] : form.segmentOrder,
    })
  }

  return (
    <form onSubmit={submit} noValidate>
      {children({
        form,
        update,
        saves,
        error: (anchor) => (failedEntry?.anchor === anchor ? failureText : undefined),
      })}
      <SaveBar
        dirty={dirty}
        saving={mutation.isPending}
        onDiscard={() => {
          mutation.reset()
          setForm(saved)
        }}
        error={failedEntry === undefined ? failureText : undefined}
      />
      <ConfirmDialog
        open={blocker.state === 'blocked'}
        onClose={() => blocker.reset?.()}
        onConfirm={() => blocker.proceed?.()}
        title={t.settingsPage.leaveTitle}
        confirmLabel={t.settingsPage.leave}
        cancelLabel={t.settingsPage.stay}
        tone="danger"
      >
        {t.settingsPage.leaveBody}
      </ConfirmDialog>
    </form>
  )
}
