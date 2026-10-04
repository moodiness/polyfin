import { useId, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  deleteAddon,
  fetchAddons,
  installAddon,
  orderAddons,
  queryClient,
  queryKeys,
  refreshAddon,
  updateAddon,
  type Addon,
  type Scope,
} from '@/api'
import {
  Badge,
  buttonPrimary,
  buttonSecondary,
  Card,
  Checkbox,
  ConfirmButton,
  Loading,
  MoveButtons,
  Notice,
  RelativeTime,
  TextField,
} from '@/components/ui'
import { errorMessage, stremioLabel } from '@/format'
import { useI18n } from '@/i18n'
import { IptvAddForm, IptvSourceDetails, IptvSourceEditor } from '@/components/IptvSources'
import { AddonSettingsPanel, MusicBadge } from '@/components/AddonSettings'

/**
 * Adding, changing, reordering or removing an addon can change the scope's libraries too, and a
 * failure (404, invalid_order) means our list is stale: refetch both lists after every change.
 */
function invalidateScope(scope: Scope) {
  void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
}

function replaceCachedAddon(scope: Scope, updated: Addon) {
  queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (old) =>
    old?.map((addon) => (addon.id === updated.id ? updated : addon)),
  )
}

/** Installs, orders, refreshes and removes the Stremio addons and IPTV sources of one scope. */
export default function AddonManager({ scope }: { scope: Scope }) {
  const { t } = useI18n()
  const addonsKey = queryKeys.addons(scope)
  const orderKey = ['addon-order', scope] as const
  const addons = useQuery({
    queryKey: addonsKey,
    queryFn: ({ signal }) => fetchAddons(scope, signal),
  })
  const [removedName, setRemovedName] = useState<string | null>(null)
  const [announcement, setAnnouncement] = useState('')

  // Moves are applied to the cache at once and sent one after another (same mutation scope), so
  // quick successive moves never race; the list is refetched once the last one is done.
  const order = useMutation({
    mutationKey: orderKey,
    scope: { id: `addon-order-${scope}` },
    mutationFn: (ids: string[]) => orderAddons(scope, ids),
    onSettled: () => {
      if (queryClient.isMutating({ mutationKey: orderKey }) === 1) invalidateScope(scope)
    },
  })

  function move(from: number, to: number) {
    const current = queryClient.getQueryData<Addon[]>(addonsKey)
    if (current === undefined || to < 0 || to >= current.length) return
    const next = [...current]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    void queryClient.cancelQueries({ queryKey: addonsKey })
    queryClient.setQueryData(addonsKey, next)
    setRemovedName(null)
    setAnnouncement(t.common.moved(moved.name, to + 1, next.length))
    order.mutate(next.map((addon) => addon.id))
  }

  return (
    <div className="space-y-6">
      <Card title={t.addons.installTitle}>
        <InstallForm scope={scope} />
      </Card>
      <Card title={t.iptv.addTitle}>
        <IptvAddForm scope={scope} />
      </Card>
      <Card title={scope === 'shared' ? t.addons.listTitleShared : t.addons.listTitleMine}>
        <div className="space-y-4">
          <p className="text-sm text-muted">{t.addons.listHelp}</p>
          <p className="sr-only" role="status">
            {announcement}
          </p>
          {removedName !== null && <Notice kind="success">{t.addons.removed(removedName)}</Notice>}
          {order.isError && <Notice kind="error">{errorMessage(t, order.error)}</Notice>}
          {addons.isPending ? (
            <Loading />
          ) : addons.isError ? (
            <Notice kind="error">{errorMessage(t, addons.error)}</Notice>
          ) : addons.data.length === 0 ? (
            <p className="text-sm text-muted">
              {scope === 'shared' ? t.addons.emptyShared : t.addons.emptyMine}
            </p>
          ) : (
            <ul className="divide-y divide-line rounded-xl border border-line">
              {addons.data.map((addon, index) => (
                <AddonRow
                  key={addon.id}
                  scope={scope}
                  addon={addon}
                  index={index}
                  count={addons.data.length}
                  onMove={(to) => move(index, to)}
                  onRemoved={() => setRemovedName(addon.name)}
                />
              ))}
            </ul>
          )}
        </div>
      </Card>
    </div>
  )
}

function InstallForm({ scope }: { scope: Scope }) {
  const { t } = useI18n()
  const [manifestUrl, setManifestUrl] = useState('')
  const mutation = useMutation({
    mutationFn: (url: string) => installAddon(scope, url),
    onSuccess: (installed) => {
      setManifestUrl('')
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (old) =>
        old === undefined ? old : [...old, installed],
      )
    },
    onSettled: () => invalidateScope(scope),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate(manifestUrl.trim())
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <TextField
        label={t.addons.manifestUrl}
        hint={t.addons.manifestUrlHint}
        type="url"
        inputMode="url"
        value={manifestUrl}
        onValue={(value) => {
          mutation.reset()
          setManifestUrl(value)
        }}
        placeholder={t.addons.manifestUrlPlaceholder}
        autoComplete="off"
        spellCheck={false}
        required
      />
      {mutation.isError && <Notice kind="error">{errorMessage(t, mutation.error)}</Notice>}
      {mutation.isSuccess && (
        <Notice kind="success">{t.addons.installed(mutation.data.name)}</Notice>
      )}
      <button type="submit" className={buttonPrimary} disabled={mutation.isPending}>
        {mutation.isPending ? t.addons.installing : t.addons.install}
      </button>
    </form>
  )
}

function AddonRow({
  scope,
  addon,
  index,
  count,
  onMove,
  onRemoved,
}: {
  scope: Scope
  addon: Addon
  index: number
  count: number
  onMove: (to: number) => void
  onRemoved: () => void
}) {
  const { t } = useI18n()
  const formId = useId()
  const replaceToggle = useRef<HTMLButtonElement>(null)
  const [replacing, setReplacing] = useState(false)
  const [manifestUrl, setManifestUrl] = useState('')
  const [edited, setEdited] = useState(false)
  const settingsToggle = useRef<HTMLButtonElement>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)

  const toggle = useMutation({
    mutationFn: (enabled: boolean) => updateAddon(scope, addon.id, { enabled }),
    onSuccess: (updated) => replaceCachedAddon(scope, updated),
    onSettled: () => invalidateScope(scope),
  })
  const refresh = useMutation({
    mutationFn: () => refreshAddon(scope, addon.id),
    onSuccess: (updated) => replaceCachedAddon(scope, updated),
    onSettled: () => invalidateScope(scope),
  })
  const replace = useMutation({
    mutationFn: (url: string) => updateAddon(scope, addon.id, { manifestUrl: url }),
    onSuccess: (updated) => {
      replaceCachedAddon(scope, updated)
      closeReplace()
    },
    onSettled: () => invalidateScope(scope),
  })
  const remove = useMutation({
    mutationFn: () => deleteAddon(scope, addon.id),
    onSuccess: () => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (old) =>
        old?.filter((item) => item.id !== addon.id),
      )
      onRemoved()
    },
    onSettled: () => invalidateScope(scope),
  })
  const actions = [toggle, refresh, replace, remove]

  function resetFeedback() {
    for (const action of actions) action.reset()
    setEdited(false)
  }

  function closeReplace() {
    setReplacing(false)
    setManifestUrl('')
    replaceToggle.current?.focus()
  }

  function submitReplace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    replace.mutate(manifestUrl.trim())
  }

  const iptv = addon.source !== null

  return (
    <li className="p-4">
      <div className="flex gap-4">
        <AddonLogo key={addon.logo ?? ''} name={addon.name} logo={addon.logo} />
        <div className="min-w-0 flex-1 space-y-2">
          <h3 className="flex flex-wrap items-center gap-2 font-medium text-white">
            <span className="break-words">{addon.name}</span>
            {!iptv && addon.version !== '' && (
              <Badge tone="muted">{t.addons.version(addon.version)}</Badge>
            )}
            {iptv && (
              <Badge tone="fin">
                {addon.kind === 'xtream' ? t.iptv.kindXtream : t.iptv.kindM3u}
              </Badge>
            )}
            {addon.music !== null && <MusicBadge music={addon.music} />}
            {!addon.enabled && <Badge tone="danger">{t.addons.off}</Badge>}
          </h3>
          {!iptv && addon.description !== '' && (
            <p className="line-clamp-2 text-sm text-muted">{addon.description}</p>
          )}
          <dl className="grid gap-x-3 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
            <dt className="text-muted">{iptv ? t.iptv.address : t.addons.manifestUrl}</dt>
            <dd className="font-mono text-xs break-all text-zinc-200 sm:self-center">
              {addon.manifestUrl}
            </dd>
            {addon.source !== null && <IptvSourceDetails source={addon.source} />}
            {!iptv && addon.resources.length > 0 && (
              <>
                <dt className="text-muted">{t.addons.provides}</dt>
                <dd className="flex flex-wrap gap-1">
                  {addon.resources.map((resource) => (
                    <Badge key={resource} tone="muted">
                      {stremioLabel(t.stremioResources, resource)}
                    </Badge>
                  ))}
                </dd>
              </>
            )}
            {!iptv && addon.types.length > 0 && (
              <>
                <dt className="text-muted">{t.addons.types}</dt>
                <dd className="flex flex-wrap gap-1">
                  {addon.types.map((type) => (
                    <Badge key={type} tone="fin">
                      {stremioLabel(t.stremioTypes, type)}
                    </Badge>
                  ))}
                </dd>
              </>
            )}
            {!iptv && (
              <>
                <dt className="text-muted">{t.addons.catalogs}</dt>
                <dd className="text-zinc-200">{t.addons.catalogCount(addon.catalogCount)}</dd>
                <dt className="text-muted">{t.addons.lastRefresh}</dt>
                <dd className="text-zinc-200">
                  <RelativeTime iso={addon.refreshedAt} />
                </dd>
              </>
            )}
          </dl>
        </div>
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-3">
        <Checkbox
          label={t.addons.enabled}
          checked={addon.enabled}
          onChange={(enabled) => {
            resetFeedback()
            toggle.mutate(enabled)
          }}
        />
        <MoveButtons name={addon.name} index={index} count={count} onMove={onMove} />
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            className={buttonSecondary}
            disabled={refresh.isPending}
            aria-label={refresh.isPending ? undefined : t.addons.refreshLabel(addon.name)}
            onClick={() => {
              resetFeedback()
              refresh.mutate()
            }}
          >
            {refresh.isPending ? t.addons.refreshing : t.addons.refresh}
          </button>
          <button
            ref={replaceToggle}
            type="button"
            className={buttonSecondary}
            aria-label={iptv ? t.iptv.editLabel(addon.name) : t.addons.replaceLabel(addon.name)}
            aria-expanded={replacing}
            aria-controls={replacing ? formId : undefined}
            onClick={() => {
              resetFeedback()
              if (replacing) closeReplace()
              else setReplacing(true)
            }}
          >
            {iptv ? t.iptv.edit : t.addons.replace}
          </button>
          {addon.music !== null && (
            <button
              ref={settingsToggle}
              type="button"
              className={buttonSecondary}
              aria-label={t.music.settingsLabel(addon.name)}
              aria-expanded={settingsOpen}
              onClick={() => {
                resetFeedback()
                setSettingsOpen((open) => !open)
              }}
            >
              {t.music.settings}
            </button>
          )}
          <ConfirmButton
            label={t.addons.remove}
            busyLabel={t.addons.removing}
            message={
              scope === 'shared'
                ? t.addons.removeConfirmShared(addon.name)
                : t.addons.removeConfirmMine(addon.name)
            }
            busy={remove.isPending}
            onConfirm={() => {
              resetFeedback()
              remove.mutate()
            }}
          />
        </div>
      </div>

      {replacing && iptv && (
        <IptvSourceEditor
          scope={scope}
          addon={addon}
          id={formId}
          onSaved={(updated) => {
            replaceCachedAddon(scope, updated)
            closeReplace()
            setEdited(true)
          }}
          onCancel={closeReplace}
        />
      )}
      {settingsOpen && addon.music !== null && (
        <AddonSettingsPanel
          scope={scope}
          addon={addon}
          music={addon.music}
          onClose={() => {
            setSettingsOpen(false)
            settingsToggle.current?.focus()
          }}
        />
      )}
      {replacing && !iptv && (
        <form
          id={formId}
          onSubmit={submitReplace}
          noValidate
          className="mt-4 space-y-3 rounded-xl border border-line bg-ink/40 p-4"
        >
          <TextField
            label={t.addons.newManifestUrl}
            hint={`${t.addons.replaceHint} ${t.addons.manifestUrlHint}`}
            type="url"
            inputMode="url"
            value={manifestUrl}
            onValue={(value) => {
              replace.reset()
              setManifestUrl(value)
            }}
            placeholder={t.addons.manifestUrlPlaceholder}
            autoComplete="off"
            spellCheck={false}
            autoFocus
            required
          />
          {replace.isError && <Notice kind="error">{errorMessage(t, replace.error)}</Notice>}
          <div className="flex flex-wrap gap-2">
            <button type="submit" className={buttonPrimary} disabled={replace.isPending}>
              {replace.isPending ? t.addons.replacing : t.addons.replaceSubmit}
            </button>
            <button type="button" className={buttonSecondary} onClick={closeReplace}>
              {t.common.cancel}
            </button>
          </div>
        </form>
      )}

      <div className="mt-4 space-y-2 empty:hidden">
        {toggle.isError && <Notice kind="error">{errorMessage(t, toggle.error)}</Notice>}
        {refresh.isError && <Notice kind="error">{errorMessage(t, refresh.error)}</Notice>}
        {remove.isError && <Notice kind="error">{errorMessage(t, remove.error)}</Notice>}
        {refresh.isSuccess && <Notice kind="success">{t.addons.refreshed}</Notice>}
        {replace.isSuccess && <Notice kind="success">{t.addons.replaced}</Notice>}
        {edited && <Notice kind="success">{t.iptv.saved}</Notice>}
      </div>
    </li>
  )
}

/**
 * The addon's initial, covered by its logo once that loads. Logos are third-party images: when
 * one is missing, broken or blocked by the page's security policy, the initial stays visible in
 * the same fixed-size box, so nothing moves.
 */
function AddonLogo({ name, logo }: { name: string; logo: string | null }) {
  const [state, setState] = useState<'loading' | 'loaded' | 'failed'>('loading')
  const initial = Array.from(name.trim())[0]?.toLocaleUpperCase() ?? '?'
  const src = logo !== null && logo.startsWith('https://') && state !== 'failed' ? logo : null

  return (
    <div
      aria-hidden="true"
      className="relative grid size-12 shrink-0 place-items-center overflow-hidden rounded-xl border border-line bg-ink text-lg font-semibold text-fin-5"
    >
      {initial}
      {src !== null && (
        <img
          src={src}
          alt=""
          width={48}
          height={48}
          loading="lazy"
          decoding="async"
          referrerPolicy="no-referrer"
          onLoad={() => setState('loaded')}
          onError={() => setState('failed')}
          className={`absolute inset-0 size-full bg-ink object-contain p-1 ${
            state === 'loaded' ? 'opacity-100' : 'opacity-0'
          }`}
        />
      )}
    </div>
  )
}
