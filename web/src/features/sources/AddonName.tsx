import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { updateAddon, type Addon, type Scope } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { SaveBar, SettingRow, useToast } from '@/ui'
import { invalidateScope, replaceCachedAddon } from './model'

/** The most characters a name given to an addon may have, as the server checks it. */
const nameMaxLength = 64

/** Whether an addon can be given a name of its own: a Stremio or Eclipse addon. */
export const isRenamable = (addon: Addon) => addon.kind === 'stremio' || addon.kind === 'eclipse'

/** The Name row of an addon being edited, saved through the page's unsaved dock. */
export type AddonNameDraft = {
  /** What the row shows: the name typed, else the one saved ('' for the manifest's). */
  value: string
  saved: string
  changed: boolean
  set: (value: string) => void
  discard: () => void
  /** Saves the name typed, trimmed; '' gives back the manifest's. */
  save: () => void
  saving: boolean
  error: string | undefined
}

/**
 * The name given to a Stremio or Eclipse addon, as a draft for its Name row. Saving updates the
 * scope's cached addons and confirms with a toast.
 */
export function useAddonName(scope: Scope, addon: Addon): AddonNameDraft {
  const { t } = useI18n()
  const toast = useToast()
  const saved = addon.chosenName ?? ''
  const [draft, setDraft] = useState<string | null>(null)
  const mutation = useMutation({
    mutationFn: (name: string) => updateAddon(scope, addon.id, { name }),
    onSuccess: (updated) => {
      replaceCachedAddon(scope, updated)
      setDraft(null)
      toast(t.sources.nameSaved(updated.name))
    },
    onSettled: () => invalidateScope(scope),
  })
  const value = draft ?? saved
  return {
    value,
    saved,
    changed: value !== saved,
    set: (next) => {
      mutation.reset()
      setDraft(next)
    },
    discard: () => {
      mutation.reset()
      setDraft(null)
    },
    save: () => mutation.mutate(value.trim()),
    saving: mutation.isPending,
    error: mutation.isError ? errorMessage(t, mutation.error) : undefined,
  }
}

/**
 * An addon's Name row: the name shown wherever Polyfin names it, its manifest's as the placeholder,
 * and what its manifest calls it once it has a name of its own.
 */
export function AddonNameRow({ addon, draft }: { addon: Addon; draft: AddonNameDraft }) {
  const { t } = useI18n()
  const text = t.sources
  const renamed = addon.chosenName !== null && addon.chosenName !== addon.manifestName
  return (
    <SettingRow
      kind="text"
      label={text.name}
      detail={renamed ? text.manifestCalls(addon.manifestName) : undefined}
      help={text.nameHelp}
      value={draft.value}
      saved={draft.saved}
      onChange={draft.set}
      placeholder={addon.manifestName}
      maxLength={nameMaxLength}
      error={draft.error}
    />
  )
}

/** The Name row in a form of its own, for an addon whose page has no other setting to save. */
export function AddonNameForm({
  addon,
  draft,
  className,
}: {
  addon: Addon
  draft: AddonNameDraft
  className?: string
}) {
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (draft.changed) draft.save()
  }
  return (
    <form onSubmit={submit} noValidate className={className}>
      <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
        <AddonNameRow addon={addon} draft={draft} />
      </div>
      <SaveBar
        dirty={draft.changed}
        changes={draft.changed ? 1 : 0}
        saving={draft.saving}
        onDiscard={draft.discard}
      />
    </form>
  )
}
