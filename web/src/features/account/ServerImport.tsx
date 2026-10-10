import { ArrowSquareInIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import {
  ApiError,
  queryClient,
  queryKeys,
  startOwnImport,
  type JellyfinImportStatus,
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

/**
 * The signed-in user imports their own watch history from a Jellyfin or Emby server, signing in
 * there with their own name and password, which only live in this form's state. Their running or
 * last import shows above the form; another import running on the server refuses theirs.
 */
export default function ServerImport({ current }: { current: JellyfinImportStatus | null }) {
  const { t } = useI18n()
  const text = t.account.serverImport
  const servers = t.users.jellyfinImport.servers
  const toast = useToast()
  const [kind, setKind] = useState<'jellyfin' | 'emby'>('jellyfin')
  const [address, setAddress] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const start = useMutation({
    mutationFn: startOwnImport,
    onSuccess: (started) => {
      queryClient.setQueryData<OwnImport>(queryKeys.ownImport, { enabled: true, import: started })
      setPassword('')
      toast(text.started)
    },
  })
  const running = current?.state === 'running'

  const code = start.error instanceof ApiError ? start.error.code : null
  const message = start.isError ? serverError(t, kind, start.error) : undefined
  const addressError = code !== null && addressCodes.includes(code) ? message : undefined
  const passwordError = code !== null && accountCodes.includes(code) ? message : undefined
  const otherError = start.isError && !addressError && !passwordError ? message : undefined

  function change(apply: () => void) {
    start.reset()
    apply()
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    start.mutate({ kind, address: address.trim(), name: name.trim(), password })
  }

  return (
    <div className="flex flex-col gap-6">
      {current !== null && <ImportProgress current={current} own />}
      {!running && (
        <Panel as="div" flush>
          <form onSubmit={submit} noValidate>
            <div className="flex max-w-lg flex-col gap-5 px-6 py-6 max-sm:px-4">
              <div className="flex flex-col gap-2">
                <span aria-hidden className="text-control font-medium text-ink">
                  {text.serverKind}
                </span>
                <Segmented
                  label={text.serverKind}
                  value={kind}
                  options={(['jellyfin', 'emby'] as const).map((value) => ({
                    value,
                    label: servers[value],
                  }))}
                  onChange={(value) => change(() => setKind(value))}
                  className="self-start"
                />
              </div>
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
        </Panel>
      )}
    </div>
  )
}
