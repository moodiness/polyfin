import { ArrowSquareInIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import {
  ApiError,
  isLetterboxdImport,
  queryClient,
  queryKeys,
  startOwnImport,
  startOwnLetterboxdImport,
  type OwnImport,
} from '@/api'
import {
  accountCodes,
  addressCodes,
  ImportProgress,
  serverError,
} from '@/features/users/JellyfinImport'
import { useI18n } from '@/i18n'
import { Button, Field, Notice, Panel, PanelFooter, Segmented, TextInput, useToast } from '@/ui'
import { LetterboxdForm, LetterboxdProgress } from './LetterboxdImport'
import { PlexImportForm } from './PlexImport'

type Kind = 'jellyfin' | 'emby' | 'plex' | 'letterboxd'

/**
 * The signed-in user imports their own watch history from a Jellyfin or Emby server, signing in
 * there with their own name and password, which only live in this form's state, from a Plex server,
 * signing in to Plex on Plex's own page, or from their Letterboxd export file. Their latest import,
 * running or ended, shows above the form; another import running on the server, or into their
 * account, refuses theirs.
 */
export default function ServerImport({ own }: { own: OwnImport }) {
  const { t } = useI18n()
  const text = t.account.serverImport
  const servers = t.users.jellyfinImport.servers
  const toast = useToast()
  const [kind, setKind] = useState<Kind>('jellyfin')
  const [address, setAddress] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const start = useMutation({
    mutationFn: startOwnImport,
    onSuccess: (started) => {
      queryClient.setQueryData<OwnImport>(
        queryKeys.ownImport,
        (data) => data && { ...data, import: started },
      )
      setPassword('')
      toast(text.started)
    },
  })
  const current = own.import
  const running = current?.state === 'running'
  const kinds: readonly Kind[] = own.letterboxd
    ? ['jellyfin', 'emby', 'plex', 'letterboxd']
    : ['jellyfin', 'emby', 'plex']

  const code = start.error instanceof ApiError ? start.error.code : null
  const server = kind === 'letterboxd' || kind === 'plex' ? 'jellyfin' : kind
  const message = start.isError ? serverError(t, server, start.error) : undefined
  const addressError = code !== null && addressCodes.includes(code) ? message : undefined
  const passwordError = code !== null && accountCodes.includes(code) ? message : undefined
  const otherError = start.isError && !addressError && !passwordError ? message : undefined

  function change(apply: () => void) {
    start.reset()
    apply()
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (kind === 'letterboxd' || kind === 'plex') return
    start.mutate({ kind, address: address.trim(), name: name.trim(), password })
  }

  const picker = (
    <div className="flex flex-col gap-2">
      <span aria-hidden className="text-control font-medium text-ink">
        {text.serverKind}
      </span>
      <Segmented
        label={text.serverKind}
        value={kind}
        options={kinds.map((value) => ({
          value,
          label: value === 'letterboxd' ? t.account.letterboxd.option : servers[value],
        }))}
        onChange={(value) => change(() => setKind(value))}
        className="self-start"
      />
    </div>
  )

  return (
    <div className="flex flex-col gap-6">
      {current !== null &&
        (isLetterboxdImport(current) ? (
          <LetterboxdProgress current={current} own />
        ) : (
          <ImportProgress current={current} own />
        ))}
      {!running && (
        <Panel as="div" flush>
          {kind === 'letterboxd' ? (
            <LetterboxdForm
              header={picker}
              searchable={own.searchable}
              own
              start={startOwnLetterboxdImport}
              onStarted={(started) =>
                queryClient.setQueryData<OwnImport>(
                  queryKeys.ownImport,
                  (data) => data && { ...data, import: started },
                )
              }
            />
          ) : kind === 'plex' ? (
            <PlexImportForm
              header={picker}
              onStarted={(started) => {
                queryClient.setQueryData<OwnImport>(
                  queryKeys.ownImport,
                  (data) => data && { ...data, import: started },
                )
                toast(text.started)
              }}
            />
          ) : (
            <form onSubmit={submit} noValidate>
              <div className="flex max-w-lg flex-col gap-5 px-6 py-6 max-sm:px-4">
                {picker}
                <Field label={text.address} help={text.addressHelp} error={addressError}>
                  <TextInput
                    type="url"
                    inputMode="url"
                    value={address}
                    onValue={(value) => change(() => setAddress(value))}
                    placeholder="http://192.168.1.10:8096"
                    autoComplete="off"
                    spellCheck={false}
                    mono
                    required
                  />
                </Field>
                <Field label={text.name}>
                  <TextInput
                    value={name}
                    onValue={(value) => change(() => setName(value))}
                    autoComplete="off"
                    spellCheck={false}
                    required
                  />
                </Field>
                <Field label={text.password} help={text.passwordHelp} error={passwordError}>
                  <TextInput
                    type="password"
                    revealable
                    value={password}
                    onValue={(value) => change(() => setPassword(value))}
                    autoComplete="off"
                  />
                </Field>
                {otherError && (
                  <Notice tone="danger" live>
                    {otherError}
                  </Notice>
                )}
              </div>
              <PanelFooter>
                <Button
                  type="submit"
                  variant="primary"
                  icon={ArrowSquareInIcon}
                  loading={start.isPending}
                  disabled={address.trim() === '' || name.trim() === ''}
                >
                  {start.isPending ? text.starting : text.start}
                </Button>
              </PanelFooter>
            </form>
          )}
        </Panel>
      )}
    </div>
  )
}
