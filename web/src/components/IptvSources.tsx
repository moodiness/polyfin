import { useId, useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  addIptvSource,
  queryClient,
  queryKeys,
  updateIptvSource,
  type Addon,
  type IptvSource,
  type IptvSourcePatch,
  type Scope,
} from '@/api'
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

type Kind = 'm3u' | 'xtream'

const fieldClass =
  'mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white placeholder:text-zinc-500'

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

/** Adds an IPTV source to a scope: an M3U playlist or an Xtream Codes account, with its guide. */
export function IptvAddForm({ scope }: { scope: Scope }) {
  const { t } = useI18n()
  const kindId = useId()
  const [name, setName] = useState('')
  const [kind, setKind] = useState<Kind>('m3u')
  const [account, setAccount] = useState(emptyAccount)
  const [guideUrl, setGuideUrl] = useState('')
  const [providerGuide, setProviderGuide] = useState(true)
  const mutation = useMutation({
    mutationFn: () =>
      addIptvSource(scope, {
        name: name.trim(),
        kind,
        ...(kind === 'm3u'
          ? { url: account.url.trim() }
          : {
              server: account.server.trim(),
              username: account.username,
              password: account.password,
            }),
        providerGuide: kind === 'xtream' && providerGuide,
        guideUrl: kind === 'xtream' && providerGuide ? '' : guideUrl.trim(),
      }),
    onSuccess: (added) => {
      setName('')
      setAccount(emptyAccount)
      setGuideUrl('')
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (old) =>
        old === undefined ? old : [...old, added],
      )
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) }),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate()
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <p className="text-sm text-muted">{t.iptv.addHelp}</p>
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
          onChange={(event) => {
            mutation.reset()
            setKind(event.target.value as Kind)
          }}
          className={fieldClass}
        >
          <option value="m3u">{t.iptv.kindM3u}</option>
          <option value="xtream">{t.iptv.kindXtream}</option>
        </select>
      </div>
      <AccountFields
        kind={kind}
        values={account}
        onChange={(patch) => {
          mutation.reset()
          setAccount((current) => ({ ...current, ...patch }))
        }}
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
      {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
      {mutation.isSuccess && (
        <Notice kind="success">
          {t.iptv.added(mutation.data.name, mutation.data.source?.channels ?? 0)}
        </Notice>
      )}
      <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
        {mutation.isPending ? t.iptv.adding : t.iptv.add}
      </button>
    </form>
  )
}

/** How a source's list was fetched, as rows of the addon's description list. */
export function IptvSourceDetails({ source }: { source: IptvSource }) {
  const { t } = useI18n()
  return (
    <>
      <dt className="text-muted">{t.iptv.channels}</dt>
      <dd className="text-zinc-200">
        {t.iptv.channelCount(source.channels)}
        {source.includedGroups !== null &&
          ` · ${t.iptv.groupsShown(source.includedGroups.length, source.groups.length)}`}
      </dd>
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

/** Changes a source: its name, its account, and the groups whose channels it shows. */
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
  const source = addon.source as IptvSource
  const kind = addon.kind as Kind
  const [name, setName] = useState(addon.name)
  const [account, setAccount] = useState(emptyAccount)
  const [all, setAll] = useState(source.includedGroups === null)
  const [chosen, setChosen] = useState(
    () => new Set(source.includedGroups ?? source.groups.map((group) => group.name)),
  )
  const [filter, setFilter] = useState('')
  const filterId = useId()
  const mutation = useMutation({
    mutationFn: (patch: IptvSourcePatch) => updateIptvSource(scope, addon.id, patch),
    onSuccess: onSaved,
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) }),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const patch: IptvSourcePatch = {
      name: name.trim(),
      groups: all
        ? null
        : source.groups.map((group) => group.name).filter((group) => chosen.has(group)),
    }
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

  const needle = filter.trim().toLocaleLowerCase()
  const shown = source.groups.filter(
    (group) => needle === '' || group.name.toLocaleLowerCase().includes(needle),
  )

  return (
    <form
      id={id}
      onSubmit={submit}
      noValidate
      className="mt-4 space-y-4 rounded-xl border border-line bg-ink/40 p-4"
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
      <fieldset className="space-y-3">
        <legend className="text-sm font-medium text-zinc-200">{t.iptv.groups}</legend>
        <Checkbox
          label={t.iptv.allGroups}
          help={t.iptv.allGroupsHelp}
          checked={all}
          onChange={setAll}
        />
        {!all && (
          <>
            <div>
              <label htmlFor={filterId} className="block text-xs font-medium text-zinc-300">
                {t.iptv.filterGroups}
              </label>
              <input
                id={filterId}
                type="search"
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                autoComplete="off"
                className={fieldClass}
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                className={buttonSecondary}
                onClick={() =>
                  setChosen((current) => new Set([...current, ...shown.map((group) => group.name)]))
                }
              >
                {t.iptv.checkShown}
              </button>
              <button
                type="button"
                className={buttonSecondary}
                onClick={() =>
                  setChosen((current) => {
                    const next = new Set(current)
                    for (const group of shown) next.delete(group.name)
                    return next
                  })
                }
              >
                {t.iptv.uncheckShown}
              </button>
            </div>
            <p className="text-xs text-muted">
              {t.iptv.groupsShown(
                source.groups.filter((group) => chosen.has(group.name)).length,
                source.groups.length,
              )}
            </p>
            <ul className="max-h-80 space-y-1 overflow-y-auto rounded-lg border border-line p-2">
              {shown.map((group) => (
                <li key={group.name}>
                  <Checkbox
                    label={`${group.name === '' ? t.iptv.noGroup : group.name} (${group.channels})`}
                    checked={chosen.has(group.name)}
                    onChange={(checked) =>
                      setChosen((current) => {
                        const next = new Set(current)
                        if (checked) next.add(group.name)
                        else next.delete(group.name)
                        return next
                      })
                    }
                  />
                </li>
              ))}
            </ul>
          </>
        )}
      </fieldset>
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
