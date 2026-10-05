import { useId, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
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
import { optionsValid, SourceOptions } from '@/components/lineup/ImportOptions'
import {
  buttonPrimary,
  buttonSecondary,
  Checkbox,
  Notice,
  RelativeTime,
  TextField,
} from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'
import { lineupPath } from '@/components/lineup/common'
import { fieldClass } from '@/components/lineup/shared'

type Kind = 'm3u' | 'xtream'

/** The account fields of a source: a playlist address, or an Xtream server and its login. */
function AccountFields({
  kind,
  values,
  onChange,
  editing,
}: {
  kind: Kind
  values: { url: string; server: string; username: string; password: string }
  onChange: (
    patch: Partial<{ url: string; server: string; username: string; password: string }>,
  ) => void
  editing: boolean
}) {
  const { t } = useI18n()
  if (kind === 'm3u') {
    return (
      <TextField
        label={t.iptv.playlistUrl}
        hint={editing ? t.iptv.keepHint : t.iptv.playlistUrlHint}
        type="url"
        inputMode="url"
        value={values.url}
        onValue={(url) => onChange({ url })}
        placeholder="https://…/playlist.m3u"
        autoComplete="off"
        spellCheck={false}
        required={!editing}
      />
    )
  }
  return (
    <>
      <TextField
        label={t.iptv.server}
        hint={editing ? t.iptv.keepHint : t.iptv.serverHint}
        type="url"
        inputMode="url"
        value={values.server}
        onValue={(server) => onChange({ server })}
        placeholder="https://…:8080"
        autoComplete="off"
        spellCheck={false}
        required={!editing}
      />
      <TextField
        label={t.iptv.username}
        value={values.username}
        onValue={(username) => onChange({ username })}
        autoComplete="off"
        spellCheck={false}
        required={!editing}
      />
      <TextField
        label={t.iptv.password}
        hint={editing ? t.iptv.passwordKeepHint : undefined}
        type="password"
        value={values.password}
        onValue={(password) => onChange({ password })}
        autoComplete="new-password"
        required={!editing}
      />
    </>
  )
}

const emptyAccount = { url: '', server: '', username: '', password: '' }

/**
 * Adds an IPTV source to a scope in two steps: the account (an M3U playlist or an Xtream Codes
 * account, with its guide), then which categories to import and how, from a preview of its list.
 */
export function IptvAddForm({ scope }: { scope: Scope }) {
  const { language, t } = useI18n()
  const kindId = useId()
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
      setName('')
      setAccount(emptyAccount)
      setGuideUrl('')
      setOptions(defaultIptvOptions)
      setStep('account')
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (old) =>
        old === undefined ? old : [...old, added],
      )
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) }),
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
    <div className="space-y-4">
      <p className="text-sm text-muted">{t.iptv.addHelp}</p>
      <ol aria-label={t.lineup.add.steps} className="flex flex-wrap gap-2 text-xs">
        {(['account', 'categories'] as const).map((name, index) => (
          <li
            key={name}
            aria-current={step === name ? 'step' : undefined}
            className={`rounded-md border px-2 py-1 font-medium ${
              step === name ? 'border-fin-3 bg-fin-2/20 text-white' : 'border-line text-muted'
            }`}
          >
            {index + 1}.{' '}
            {name === 'account' ? t.lineup.add.stepAccount : t.lineup.add.stepCategories}
          </li>
        ))}
      </ol>
      {step === 'account' ? (
        <form onSubmit={next} noValidate className="space-y-4">
          <TextField
            label={t.iptv.name}
            value={name}
            onValue={(value) => {
              mutation.reset()
              setName(value)
            }}
            maxLength={64}
            autoComplete="off"
            required
          />
          <div>
            <label htmlFor={kindId} className="block text-sm font-medium text-zinc-200">
              {t.iptv.kind}
            </label>
            <select
              id={kindId}
              value={kind}
              onChange={(event) => changeAccount(() => setKind(event.target.value as Kind))}
              className={fieldClass}
            >
              <option value="m3u">{t.iptv.kindM3u}</option>
              <option value="xtream">{t.iptv.kindXtream}</option>
            </select>
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
              label={t.iptv.providerGuide}
              help={t.iptv.providerGuideHelp}
              checked={providerGuide}
              onChange={setProviderGuide}
            />
          )}
          {(kind === 'm3u' || !providerGuide) && (
            <TextField
              label={t.iptv.guideUrl}
              hint={t.iptv.guideUrlHint}
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
            />
          )}
          {mutation.isSuccess && mutation.data.source !== null && (
            <Notice kind="success">
              {t.lineup.add.added(
                mutation.data.name,
                addedParts(t, language, mutation.data.source),
              )}
            </Notice>
          )}
          <button
            type="submit"
            className={buttonPrimary}
            disabled={
              name.trim() === '' ||
              (kind === 'm3u' ? account.url.trim() === '' : account.server.trim() === '')
            }
          >
            {t.lineup.add.next}
          </button>
        </form>
      ) : (
        <div className="space-y-5">
          <SourceOptions
            value={options}
            onChange={(patch) => {
              mutation.reset()
              setOptions((current) => ({ ...current, ...patch }))
            }}
            queryKey={['iptv-preview', scope, previewOf]}
            load={(by, signal) => previewNewSource(scope, accountJSON, by, signal)}
          />
          {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              className={buttonPrimary}
              disabled={mutation.isPending || !optionsValid(options)}
              onClick={() => mutation.mutate()}
            >
              {mutation.isPending ? t.lineup.add.importing : t.lineup.add.import}
            </button>
            <button
              type="button"
              className={buttonSecondary}
              disabled={mutation.isPending}
              onClick={() => setStep('account')}
            >
              {t.lineup.add.back}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

/** What a source just added brings: its channels, movies and series, as imported. */
function addedParts(t: Messages, language: string, source: IptvSource): string[] {
  const text = t.lineup.add
  const number = (n: number) => n.toLocaleString(language)
  return [
    source.options.liveTv ? text.channels(number(source.lineup.channels)) : '',
    source.options.movies ? text.movies(number(source.vod.shownMovies)) : '',
    source.options.series ? text.series(number(source.vod.shownSeries)) : '',
  ].filter(Boolean)
}

/** How a source's lists were fetched and what it imports, as rows of the addon's list. */
export function IptvSourceDetails({
  scope,
  id,
  source,
}: {
  scope: Scope
  id: string
  source: IptvSource
}) {
  const { language, t } = useI18n()
  const number = (n: number) => n.toLocaleString(language)
  return (
    <>
      <dt className="text-muted">{t.iptv.channels}</dt>
      <dd className="text-zinc-200">
        {source.options.liveTv
          ? t.lineup.summary.shownOf(
              number(source.lineup.shownChannels),
              number(source.lineup.channels),
            )
          : t.lineup.content.notImported}
        {' · '}
        <Link
          to={lineupPath(scope, id)}
          className="font-medium text-fin-5 underline decoration-fin-5/40 underline-offset-4 hover:decoration-fin-5"
        >
          {t.lineup.open}
        </Link>
      </dd>
      {(source.options.movies || source.options.series) && (
        <>
          <dt className="text-muted">{t.lineup.summary.vodTitle}</dt>
          <dd className="text-zinc-200">
            {t.lineup.summary.vodShort(
              number(source.vod.shownMovies),
              number(source.vod.shownSeries),
            )}
          </dd>
        </>
      )}
      <dt className="text-muted">{t.iptv.lastFetch}</dt>
      <dd className="text-zinc-200">
        {source.fetchedAt === null ? t.iptv.never : <RelativeTime iso={source.fetchedAt} />}
      </dd>
      {source.nextAt !== null && (
        <>
          <dt className="text-muted">{t.iptv.nextFetch}</dt>
          <dd className="text-zinc-200">
            <RelativeTime iso={source.nextAt} />
          </dd>
        </>
      )}
      {source.error !== '' && (
        <dd className="sm:col-span-2">
          <Notice kind="error">
            {Object.hasOwn(t.iptv.errors, source.error)
              ? t.iptv.errors[source.error]
              : t.errors.generic}
          </Notice>
        </dd>
      )}
    </>
  )
}

/** Changes a source's name and account; its categories and channels are on its line-up page. */
export function IptvSourceEditor({
  scope,
  addon,
  id,
  onSaved,
  onCancel,
}: {
  scope: Scope
  addon: Addon
  id: string
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
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) }),
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
    <form
      id={id}
      onSubmit={submit}
      noValidate
      className="mt-4 space-y-4 rounded-xl border border-line bg-bg/40 p-4"
    >
      <TextField
        label={t.iptv.name}
        value={name}
        onValue={setName}
        maxLength={64}
        autoComplete="off"
        required
      />
      <AccountFields
        kind={kind}
        values={account}
        onChange={(patch) => setAccount((current) => ({ ...current, ...patch }))}
        editing
      />
      <p className="text-xs text-muted">
        {t.lineup.editorHint}{' '}
        <Link
          to={lineupPath(scope, addon.id)}
          className="text-fin-5 underline decoration-fin-5/40 underline-offset-4 hover:decoration-fin-5"
        >
          {t.lineup.open}
        </Link>
      </p>
      {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
      <div className="flex flex-wrap gap-2">
        <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
          {mutation.isPending ? t.common.saving : t.common.save}
        </button>
        <button type="button" className={buttonSecondary} onClick={onCancel}>
          {t.common.cancel}
        </button>
      </div>
    </form>
  )
}
