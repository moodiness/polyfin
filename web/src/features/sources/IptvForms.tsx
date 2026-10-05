import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  addIptvSource,
  defaultIptvOptions,
  previewNewSource,
  queryClient,
  queryKeys,
  updateIptvSource,
  type Addon,
  type IptvAccount,
  type IptvOptions,
  type IptvSource,
  type IptvSourcePatch,
  type Scope,
} from '@/api'
import { optionsValid, SourceOptions } from '@/features/iptv/ImportOptions'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'
import { Button, Checkbox, cx, Field, FieldError, Segmented, TextInput } from '@/ui'
import { invalidateScope } from './model'

type Kind = 'm3u' | 'xtream'
type Account = { url: string; server: string; username: string; password: string }

const emptyAccount: Account = { url: '', server: '', username: '', password: '' }

/** The account fields of a source: a playlist address, or an Xtream server and its login. */
function AccountFields({
  kind,
  values,
  onChange,
  editing,
}: {
  kind: Kind
  values: Account
  onChange: (patch: Partial<Account>) => void
  editing: boolean
}) {
  const { t } = useI18n()
  const text = t.sourceAdd
  if (kind === 'm3u') {
    return (
      <Field label={text.playlistUrl} help={editing ? text.keepHint : text.playlistUrlHint}>
        <TextInput
          type="url"
          inputMode="url"
          value={values.url}
          onValue={(url) => onChange({ url })}
          placeholder="https://…/playlist.m3u"
          autoComplete="off"
          spellCheck={false}
          required={!editing}
          mono
        />
      </Field>
    )
  }
  return (
    <>
      <Field label={text.server} help={editing ? text.keepHint : text.serverHint}>
        <TextInput
          type="url"
          inputMode="url"
          value={values.server}
          onValue={(server) => onChange({ server })}
          placeholder="https://…:8080"
          autoComplete="off"
          spellCheck={false}
          required={!editing}
          mono
        />
      </Field>
      <Field label={text.username}>
        <TextInput
          value={values.username}
          onValue={(username) => onChange({ username })}
          autoComplete="off"
          spellCheck={false}
          required={!editing}
        />
      </Field>
      <Field label={text.password} help={editing ? text.passwordKeepHint : undefined}>
        <TextInput
          type="password"
          revealable
          value={values.password}
          onValue={(password) => onChange({ password })}
          autoComplete="new-password"
          required={!editing}
        />
      </Field>
    </>
  )
}

/** What a source just added brings: its channels, movies and series, as imported. */
export function addedParts(t: Messages, language: string, source: IptvSource): string[] {
  const text = t.sourceAdd
  const number = (n: number) => n.toLocaleString(language)
  return [
    source.options.liveTv ? text.channels(number(source.lineup.channels)) : '',
    source.options.movies ? text.movies(number(source.vod.shownMovies)) : '',
    source.options.series ? text.series(number(source.vod.shownSeries)) : '',
  ].filter(Boolean)
}

/**
 * Adds an IPTV source to a scope in two steps: the account (an M3U playlist or an Xtream Codes
 * account, with its guide), then which categories to import and how, from a preview of its list.
 */
export function IptvAddFlow({ scope, onAdded }: { scope: Scope; onAdded: (added: Addon) => void }) {
  const { t } = useI18n()
  const text = t.sourceAdd
  const [step, setStep] = useState<'account' | 'categories'>('account')
  const [name, setName] = useState('')
  const [kind, setKind] = useState<Kind>('m3u')
  const [account, setAccount] = useState(emptyAccount)
  const [guideUrl, setGuideUrl] = useState('')
  const [providerGuide, setProviderGuide] = useState(true)
  const [options, setOptions] = useState<IptvOptions>(defaultIptvOptions)
  // Each preview is tied to the account it was read from: a change reads it again.
  const [previewOf, setPreviewOf] = useState(0)
  const accountJSON: IptvAccount =
    kind === 'm3u'
      ? { kind, url: account.url.trim() }
      : {
          kind,
          server: account.server.trim(),
          username: account.username,
          password: account.password,
        }
  const mutation = useMutation({
    mutationFn: () =>
      addIptvSource(scope, {
        name: name.trim(),
        ...accountJSON,
        providerGuide: kind === 'xtream' && providerGuide,
        guideUrl: kind === 'xtream' && providerGuide ? '' : guideUrl.trim(),
        options,
      }),
    onSuccess: (added) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (old) =>
        old === undefined ? old : [...old, added],
      )
      onAdded(added)
    },
    onSettled: () => invalidateScope(scope),
  })

  function next(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.reset()
    setStep('categories')
  }

  function changeAccount(change: () => void) {
    mutation.reset()
    change()
    setPreviewOf((n) => n + 1)
    setOptions((current) => ({ ...current, excluded: [], vodExcluded: [] }))
  }

  return (
    <div className="flex flex-col gap-5">
      <p className="text-small text-ink-3">{text.iptvHelp}</p>
      <ol aria-label={text.steps} className="flex flex-wrap gap-2">
        {(['account', 'categories'] as const).map((id, index) => (
          <li
            key={id}
            aria-current={step === id ? 'step' : undefined}
            className={cx(
              'inline-flex h-7 items-center gap-2 rounded-field border px-2.5 text-small font-medium',
              step === id ? 'border-accent/55 bg-s3 text-ink' : 'border-line-2 text-ink-3',
            )}
          >
            <span className="figures">{index + 1}</span>
            {id === 'account' ? text.stepAccount : text.stepImport}
          </li>
        ))}
      </ol>
      {step === 'account' ? (
        <form onSubmit={next} noValidate className="flex flex-col gap-5">
          <Field label={text.name}>
            <TextInput
              value={name}
              onValue={(value) => {
                mutation.reset()
                setName(value)
              }}
              maxLength={64}
              autoComplete="off"
              required
            />
          </Field>
          <div className="flex flex-col gap-2">
            <span className="text-control font-medium text-ink">{text.account}</span>
            <Segmented<Kind>
              label={text.account}
              value={kind}
              onChange={(value) => changeAccount(() => setKind(value))}
              options={[
                { value: 'm3u', label: text.m3u },
                { value: 'xtream', label: text.xtream },
              ]}
              className="self-start"
            />
          </div>
          <AccountFields
            kind={kind}
            values={account}
            onChange={(patch) =>
              changeAccount(() => setAccount((current) => ({ ...current, ...patch })))
            }
            editing={false}
          />
          {kind === 'xtream' && (
            <Checkbox
              label={text.providerGuide}
              help={text.providerGuideHelp}
              checked={providerGuide}
              onChange={setProviderGuide}
            />
          )}
          {(kind === 'm3u' || !providerGuide) && (
            <Field label={text.guideUrl} help={text.guideUrlHint}>
              <TextInput
                type="url"
                inputMode="url"
                value={guideUrl}
                onValue={(value) => {
                  mutation.reset()
                  setGuideUrl(value)
                }}
                placeholder="https://…/guide.xml.gz"
                autoComplete="off"
                spellCheck={false}
                mono
              />
            </Field>
          )}
          <div>
            <Button
              type="submit"
              variant="primary"
              disabled={
                name.trim() === '' ||
                (kind === 'm3u' ? account.url.trim() === '' : account.server.trim() === '')
              }
            >
              {text.next}
            </Button>
          </div>
        </form>
      ) : (
        <div className="flex flex-col gap-5">
          <SourceOptions
            value={options}
            onChange={(patch) => {
              mutation.reset()
              setOptions((current) => ({ ...current, ...patch }))
            }}
            queryKey={['iptv-preview', scope, previewOf]}
            load={(by, signal) => previewNewSource(scope, accountJSON, by, signal)}
          />
          {mutation.isError && <FieldError>{errorMessage(t, mutation.error)}</FieldError>}
          <div className="flex flex-wrap gap-2">
            <Button
              variant="primary"
              loading={mutation.isPending}
              disabled={!optionsValid(options)}
              onClick={() => mutation.mutate()}
            >
              {text.import}
            </Button>
            <Button
              variant="secondary"
              disabled={mutation.isPending}
              onClick={() => setStep('account')}
            >
              {text.back}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

/** Changes a source's name and account; its categories and channels are on its own page. */
export function IptvEditForm({
  scope,
  addon,
  onSaved,
  onCancel,
}: {
  scope: Scope
  addon: Addon
  onSaved: (updated: Addon) => void
  onCancel: () => void
}) {
  const { t } = useI18n()
  const kind = addon.kind as Kind
  const [name, setName] = useState(addon.name)
  const [account, setAccount] = useState(emptyAccount)
  const mutation = useMutation({
    mutationFn: (patch: IptvSourcePatch) => updateIptvSource(scope, addon.id, patch),
    onSuccess: onSaved,
    onSettled: () => invalidateScope(scope),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const patch: IptvSourcePatch = { name: name.trim() }
    if (kind === 'm3u' && account.url.trim() !== '') patch.url = account.url.trim()
    if (
      kind === 'xtream' &&
      (account.server.trim() !== '' || account.username !== '' || account.password !== '')
    ) {
      Object.assign(patch, {
        server: account.server.trim(),
        username: account.username,
        password: account.password,
      })
    }
    mutation.mutate(patch)
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <Field label={t.sourceAdd.name}>
        <TextInput
          value={name}
          onValue={setName}
          maxLength={64}
          autoComplete="off"
          required
          autoFocus
        />
      </Field>
      <AccountFields
        kind={kind}
        values={account}
        onChange={(patch) => setAccount((current) => ({ ...current, ...patch }))}
        editing
      />
      <p className="text-small text-ink-3">{t.sources.iptvHint}</p>
      {mutation.isError && <FieldError>{errorMessage(t, mutation.error)}</FieldError>}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" variant="primary" loading={mutation.isPending}>
          {t.common.save}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          {t.common.cancel}
        </Button>
      </div>
    </form>
  )
}
