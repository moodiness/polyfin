import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
  ApiError,
  fetchSettings,
  fetchThemeCopy,
  keepThemeCopy,
  queryClient,
  queryKeys,
  removeThemeCopy,
  type Settings,
  type ThemeCopy,
  type ThemeCopyUnsaved,
} from '@/api'
import { dateTime, errorMessage, formatBytes } from '@/format'
import { useI18n } from '@/i18n'
import { Button, ConfirmDialog, Notice, useToast } from '@/ui'

/** The texts the web player gets for the theme chosen: Polyfin's for LumaaGlaass, none for None. */
export function shownTexts(form: Settings): { css: string; js: string } {
  switch (form.webTheme) {
    case 'lumaaglaass':
      return form.lumaaGlaass
    case 'none':
      return { css: '', js: '' }
    default:
      return { css: form.customCss, js: form.customJs }
  }
}

/**
 * The button that turns the texts shown for LumaaGlaass into the Custom texts, to edit them; it
 * asks first when the Custom texts it replaces are not empty.
 */
export function EditAsCustom({
  form,
  update,
}: {
  form: Settings
  update: (patch: Partial<Settings>) => void
}) {
  const { t } = useI18n()
  const s = t.settings
  const [confirming, setConfirming] = useState(false)
  const copy = () => {
    setConfirming(false)
    const { css, js } = shownTexts(form)
    update({ webTheme: 'custom', customCss: css, customJs: js })
  }
  return (
    <>
      <Button
        onClick={() =>
          form.customCss.trim() === '' && form.customJs.trim() === '' ? copy() : setConfirming(true)
        }
      >
        {s.editAsCustom}
      </Button>
      <ConfirmDialog
        open={confirming}
        onClose={() => setConfirming(false)}
        onConfirm={copy}
        title={s.editAsCustomTitle}
        confirmLabel={s.editAsCustom}
      >
        {s.editAsCustomConfirm}
      </ConfirmDialog>
    </>
  )
}

/** The addresses not saved, each with why. */
function UnsavedList({ unsaved }: { unsaved: ThemeCopyUnsaved[] }) {
  const { t } = useI18n()
  const s = t.settings
  return (
    <ul className="mt-1 flex flex-col gap-1">
      {unsaved.map((entry) => (
        <li key={entry.address} className="min-w-0 text-small">
          <span className="font-mono break-all text-ink-2">{entry.address}</span>
          <span className="text-ink-3">
            {' '}
            · {s.themeCopyReasons[entry.reason]}
            {entry.detail ? ` (${entry.detail})` : ''}
          </span>
        </li>
      ))}
    </ul>
  )
}

/**
 * Keeping a copy of the files the Custom texts load from elsewhere: what the copy holds, what it
 * could not save, and its actions. The copy is made from the saved texts, so the actions wait
 * for the page's changes to be saved; the texts it rewrites become the form's.
 */
export function ThemeCopyPanel({
  dirty,
  adopt,
}: {
  dirty: boolean
  adopt: (patch: Partial<Settings>) => void
}) {
  const { t, language } = useI18n()
  const s = t.settings
  const toast = useToast()
  const [removing, setRemoving] = useState(false)
  const status = useQuery({
    queryKey: queryKeys.themeCopy,
    queryFn: ({ signal }) => fetchThemeCopy(signal),
  })
  // The copy rewrites the Custom texts on the server: the form takes them as saved.
  const settled = async (result: ThemeCopy, message: string) => {
    queryClient.setQueryData(queryKeys.themeCopy, result)
    const settings = await queryClient.fetchQuery({
      queryKey: queryKeys.settings,
      queryFn: ({ signal }) => fetchSettings(signal),
      staleTime: 0,
    })
    adopt({ customCss: settings.customCss, customJs: settings.customJs })
    toast(message, { tone: 'ok' })
  }
  const keep = useMutation({
    mutationFn: keepThemeCopy,
    onSuccess: (result) => settled(result, s.themeCopyKept),
  })
  const remove = useMutation({
    mutationFn: removeThemeCopy,
    onSuccess: async (result) => {
      setRemoving(false)
      await settled(result, s.themeCopyRemoved)
    },
  })
  const data = status.data
  if (data === undefined) {
    return status.isError ? <Notice tone="danger">{errorMessage(t, status.error)}</Notice> : null
  }
  const copy = data.copy
  // The addresses the texts load from elsewhere besides those the copy could not save: all of
  // them before a copy, those added since after.
  const outside = data.outside.filter(
    (address) => !copy?.unsaved.some((entry) => entry.address === address),
  )
  const failed = keep.error
  const busy = keep.isPending || remove.isPending
  return (
    <div className="flex flex-col gap-3">
      <p className="text-[15px] font-medium tracking-[-0.01em] text-ink">{s.themeCopy}</p>
      <p className="-mt-2 max-w-[60ch] text-small text-ink-3">
        {s.themeCopyHelp(
          data.limits.depth,
          formatBytes(data.limits.fileBytes, language),
          formatBytes(data.limits.totalBytes, language),
          data.limits.files,
        )}
      </p>
      {copy !== null && (
        <Notice tone="ok">
          {s.themeCopySaved(
            copy.files.length,
            formatBytes(copy.size, language),
            dateTime(copy.savedAt, language),
          )}
          <details className="mt-1">
            <summary className="cursor-pointer text-small text-ink-2">{s.themeCopyFiles}</summary>
            <ul className="mt-1 flex flex-col gap-1">
              {copy.files.map((file) => (
                <li key={file.name} className="min-w-0 text-small">
                  <span className="font-mono break-all text-ink-2">{file.address}</span>
                  <span className="text-ink-3">
                    {' '}
                    · {s.themeCopyKinds[file.kind]}, {formatBytes(file.size, language)}
                  </span>
                </li>
              ))}
            </ul>
          </details>
        </Notice>
      )}
      {copy !== null && copy.unsaved.length > 0 && (
        <Notice tone="warn">
          {s.themeCopyUnsaved(copy.unsaved.length)}
          <UnsavedList unsaved={copy.unsaved} />
        </Notice>
      )}
      {data.missing.length > 0 && (
        <Notice tone="danger">{s.themeCopyMissing(data.missing.length)}</Notice>
      )}
      {outside.length > 0 && (
        <Notice tone="info">
          {copy === null
            ? s.themeCopyOutside(outside.length)
            : s.themeCopyStillOutside(outside.length)}
          <ul className="mt-1 flex flex-col gap-1">
            {outside.map((address) => (
              <li key={address} className="font-mono text-small break-all text-ink-2">
                {address}
              </li>
            ))}
          </ul>
        </Notice>
      )}
      {copy === null && data.outside.length === 0 && (
        <p className="text-small text-ink-3">{s.themeCopyNothing}</p>
      )}
      {failed !== null && (
        <Notice tone="danger" live>
          {failed instanceof ApiError && failed.unsaved !== null ? (
            <>
              {s.themeCopyFailed}
              <UnsavedList unsaved={failed.unsaved} />
            </>
          ) : (
            errorMessage(t, failed)
          )}
        </Notice>
      )}
      {dirty && (copy !== null || data.outside.length > 0) && (
        <p className="text-small text-warn">{s.themeCopySaveFirst}</p>
      )}
      <div className="flex flex-wrap gap-2">
        {(copy !== null || data.outside.length > 0) && (
          <Button
            variant={copy === null ? 'primary' : 'secondary'}
            loading={keep.isPending}
            disabled={dirty || remove.isPending}
            onClick={() => {
              remove.reset()
              keep.mutate()
            }}
          >
            {copy === null ? s.themeCopyKeep : s.themeCopyUpdate}
          </Button>
        )}
        {copy !== null && (
          <Button
            variant="ghost"
            disabled={dirty || busy}
            onClick={() => {
              keep.reset()
              setRemoving(true)
            }}
          >
            {s.themeCopyRemove}
          </Button>
        )}
      </div>
      <ConfirmDialog
        open={removing}
        onClose={() => setRemoving(false)}
        onConfirm={() => remove.mutate()}
        title={s.themeCopyRemoveTitle}
        confirmLabel={s.themeCopyRemove}
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {s.themeCopyRemoveConfirm}
      </ConfirmDialog>
    </div>
  )
}
