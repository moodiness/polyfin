import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { useBlocker } from 'react-router'
import { useMutation } from '@tanstack/react-query'
import { ApiError, queryClient, queryKeys, saveSettings, type Settings } from '@/api'
import type { SettingsSectionId } from '@/app/navigation'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { RangeText } from '@/i18n/en/settings'
import { ConfirmDialog, SaveBar, useToast } from '@/ui'
import { numberLimits, rangeText } from './bounds'
import { settingEntries } from './catalog'

/** What a section page draws its fields from. */
export type SectionFormApi = {
  form: Settings
  /** The settings as saved: a changed row shows "was …" from them. */
  saved: Settings
  update: (patch: Partial<Settings>) => void
  /** The error about the setting at this anchor, shown under it: an empty number, or the save's. */
  error: (anchor: string) => string | undefined
  /**
   * The value and change handler of the number row at this anchor (`SettingRow` kind `number`).
   * An empty well keeps the last number in `form`, shows "Enter a number" under it and blocks
   * the save until it is filled.
   */
  number: (
    anchor: string,
    value: number,
    set: (value: number) => void,
  ) => { value: number | null; onChange: (value: number | null) => void }
  /**
   * The bounds and default of a setting, by its name in Settings, as its help text shows them;
   * `scale` converts the server's unit to the field's.
   */
  range: (name: keyof Settings, scale?: number) => RangeText
  /** The smallest and largest values of a setting's number field, from the server's bounds. */
  limits: (name: keyof Settings, scale?: number) => { min?: number; max?: number }
  /**
   * The same for a number row, which needs both ends: 0 and no top when the server gives none.
   */
  minMax: (name: keyof Settings, scale?: number) => { min: number; max: number }
  /** Whether the form holds changes not saved yet. */
  dirty: boolean
  /**
   * Takes settings the server changed itself, outside the form, such as texts an action
   * rewrote, as saved, keeping the other changes not saved yet.
   */
  adopt: (patch: Partial<Settings>) => void
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
  const { t, language } = useI18n()
  const toast = useToast()
  const [saved, setSaved] = useState(initial)
  const [form, setForm] = useState(initial)
  /** The anchors of the number fields left empty. */
  const [empty, setEmpty] = useState<ReadonlySet<string>>(new Set())
  /**
   * The number each well held before its first change since the last save: a well left empty
   * puts it back in the form, so that it counts as one change, not as an empty well and a changed
   * value.
   */
  const before = useRef(new Map<string, number>())
  const changes = changedCount(form, saved, empty)
  const dirty = changes > 0

  const mutation = useMutation({
    mutationFn: saveSettings,
    onSuccess: (result) => {
      queryClient.setQueryData(queryKeys.settings, result)
      setSaved(result)
      setForm(result)
      before.current.clear()
      toast(t.settings.saved, { tone: 'ok' })
      // The server name is part of the public status.
      void queryClient.invalidateQueries({ queryKey: queryKeys.status })
      // Library names in apps follow the server language.
      void queryClient.invalidateQueries({ queryKey: queryKeys.scopes })
      // Email targets can be added once an SMTP server is saved.
      void queryClient.invalidateQueries({ queryKey: ['notifications'] })
      // The themes users are given follow the server's and its Custom texts.
      void queryClient.invalidateQueries({ queryKey: queryKeys.webThemes })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownWebTheme })
      // A user's age limit is picked among the rating country's ratings.
      void queryClient.invalidateQueries({ queryKey: queryKeys.parentalRatings })
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
    if (empty.size > 0) {
      event.currentTarget.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
      return
    }
    mutation.mutate({
      ...form,
      serverName: form.serverName.trim(),
      traktClientId: form.traktClientId.trim(),
      simklClientId: form.simklClientId.trim(),
      lastFmApiKey: form.lastFmApiKey.trim(),
    })
  }

  return (
    <form onSubmit={submit} noValidate>
      {children({
        form,
        saved,
        update,
        error: (anchor) =>
          empty.has(anchor)
            ? t.common.enterNumber
            : failedEntry?.anchor === anchor
              ? failureText
              : undefined,
        number: (anchor, value, set) => ({
          value: empty.has(anchor) ? null : value,
          onChange: (next) => {
            if (!before.current.has(anchor)) before.current.set(anchor, value)
            setEmpty((current) => {
              const updated = new Set(current)
              if (next === null) updated.add(anchor)
              else updated.delete(anchor)
              return updated
            })
            if (next === null) {
              mutation.reset()
              set(before.current.get(anchor) ?? value)
            } else set(next)
          },
        }),
        range: (name, scale) => rangeText(form.bounds[name], language, scale),
        limits: (name, scale) => numberLimits(form.bounds[name], scale),
        minMax: (name, scale) => {
          const { min, max } = numberLimits(form.bounds[name], scale)
          return { min: min ?? 0, max: max ?? Number.MAX_SAFE_INTEGER }
        },
        dirty,
        adopt: (patch) => {
          setSaved((current) => ({ ...current, ...patch }))
          setForm((current) => ({ ...current, ...patch }))
          queryClient.setQueryData<Settings>(queryKeys.settings, (current) =>
            current === undefined ? current : { ...current, ...patch },
          )
        },
      })}
      <SaveBar
        dirty={dirty}
        changes={changes}
        saving={mutation.isPending}
        onDiscard={() => {
          mutation.reset()
          setEmpty(new Set())
          before.current.clear()
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

/**
 * How many settings the form changes: each setting that differs from the saved one, and each
 * number well left empty (which puts back the number it held before its first change).
 */
function changedCount(form: Settings, saved: Settings, empty: ReadonlySet<string>): number {
  let count = empty.size
  for (const key of Object.keys(form) as (keyof Settings)[]) {
    if (JSON.stringify(form[key]) !== JSON.stringify(saved[key])) count++
  }
  return count
}
