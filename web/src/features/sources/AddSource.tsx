import { BroadcastIcon, MusicNotesIcon, PuzzlePieceIcon } from '@phosphor-icons/react'
import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { installAddon, queryClient, queryKeys, type Addon, type Scope } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, Modal, Field, Segmented, TextInput, useToast } from '@/ui'
import { addedParts, IptvAddFlow } from './IptvForms'
import { invalidateScope } from './model'

type AddKind = 'stremio' | 'music' | 'iptv'

/**
 * The "Add a source" panel: a Stremio or music addon from its manifest address (the server tells
 * which it is), or an IPTV source in two steps. `onAdded` receives the new source, to select it.
 */
export function AddSourceModal({
  scope,
  open,
  onClose,
  onAdded,
}: {
  scope: Scope
  open: boolean
  onClose: () => void
  onAdded: (added: Addon) => void
}) {
  const { language, t } = useI18n()
  const toast = useToast()
  const text = t.sourceAdd
  const [kind, setKind] = useState<AddKind>('stremio')
  // A new key per opening starts the forms empty.
  const [round, setRound] = useState(0)

  function done(added: Addon, message: string) {
    toast(message)
    onAdded(added)
    setRound((n) => n + 1)
    onClose()
  }

  return (
    <Modal open={open} onClose={onClose} title={text.title} width={640}>
      <div key={round} className="flex flex-col gap-6 p-5">
        <Segmented<AddKind>
          label={text.kind}
          value={kind}
          onChange={setKind}
          options={[
            { value: 'stremio', label: text.kinds.stremio, icon: PuzzlePieceIcon },
            { value: 'music', label: text.kinds.music, icon: MusicNotesIcon },
            { value: 'iptv', label: text.kinds.iptv, icon: BroadcastIcon },
          ]}
          className="self-start max-sm:self-stretch"
        />
        {kind === 'iptv' ? (
          <IptvAddFlow
            scope={scope}
            onAdded={(added) =>
              done(
                added,
                added.source === null
                  ? text.installed(added.name)
                  : text.added(added.name, addedParts(t, language, added.source)),
              )
            }
          />
        ) : (
          <InstallForm
            key={kind}
            scope={scope}
            help={kind === 'music' ? text.musicHelp : text.stremioHelp}
            onInstalled={(added) => done(added, text.installed(added.name))}
          />
        )}
      </div>
    </Modal>
  )
}

function InstallForm({
  scope,
  help,
  onInstalled,
}: {
  scope: Scope
  help: string
  onInstalled: (added: Addon) => void
}) {
  const { t } = useI18n()
  const text = t.sourceAdd
  const [manifestUrl, setManifestUrl] = useState('')
  const mutation = useMutation({
    mutationFn: (url: string) => installAddon(scope, url),
    onSuccess: (installed) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (old) =>
        old === undefined ? old : [...old, installed],
      )
      onInstalled(installed)
    },
    onSettled: () => invalidateScope(scope),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate(manifestUrl.trim())
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <p className="text-small text-ink-3">{help}</p>
      <Field
        label={text.manifestUrl}
        help={text.manifestUrlHint}
        error={mutation.isError ? errorMessage(t, mutation.error) : undefined}
      >
        <TextInput
          type="url"
          inputMode="url"
          value={manifestUrl}
          onValue={(value) => {
            mutation.reset()
            setManifestUrl(value)
          }}
          placeholder={text.manifestUrlPlaceholder}
          autoComplete="off"
          spellCheck={false}
          required
          mono
          autoFocus
        />
      </Field>
      <div>
        <Button type="submit" variant="primary" loading={mutation.isPending}>
          {text.install}
        </Button>
      </div>
    </form>
  )
}
