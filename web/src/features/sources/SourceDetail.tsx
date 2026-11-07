import {
  ArrowsClockwiseIcon,
  DotsThreeIcon,
  LinkSimpleIcon,
  PencilSimpleIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  deleteAddon,
  queryClient,
  queryKeys,
  refreshAddon,
  updateAddon,
  type Addon,
  type Scope,
} from '@/api'
import { IptvSource } from '@/features/iptv/IptvSource'
import { catalogTypeLabel, errorMessage, languageOptions, stremioLabel } from '@/format'
import { useI18n } from '@/i18n'
import {
  AlertTile,
  Avatar,
  Badge,
  Button,
  ConfirmDialog,
  Field,
  FieldError,
  IconButton,
  Menu,
  MenuItem,
  MenuSeparator,
  Modal,
  MoveButtons,
  Notice,
  RelativeTime,
  SettingRow,
  TextInput,
  useToast,
} from '@/ui'
import { AddonSettings, MusicBadge } from './AddonSettings'
import { IptvEditForm } from './IptvForms'
import {
  computer,
  LocalFolderEditForm,
  LocalFolderFigures,
  UnmatchedFiles,
  useFolderScanFollowed,
  useServerSystem,
} from './LocalFolders'
import {
  invalidateScope,
  isIptv,
  lastTime,
  replaceCachedAddon,
  useSourceToggle,
  type Entry,
} from './model'
import { Fact, Facts, Group, SourceMark, SourceStatus } from './parts'
import { SourceUsedBy } from './UsedBy'

/** Who owns a source of the list: nothing for the server's, else the owner's initial and name. */
export function OwnerLabel({ entry, selfId }: { entry: Entry; selfId: string }) {
  const { t } = useI18n()
  if (entry.owner === null) return null
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5">
      <Avatar name={entry.owner.name} size="sm" />
      <span>
        {entry.owner.id === selfId ? t.sources.yours : t.sources.ownedBy(entry.owner.name)}
      </span>
    </span>
  )
}

/**
 * One source's page, beside the list of sources: its head (mark, name, kind, state, Refresh and
 * the other actions), then, for an IPTV source, its six parts; for an addon or a folder, who uses
 * it and where it stands in the order, the language of its descriptions, its settings, its files
 * and its details. Another user's source (`entry.scope === null`) is shown without actions.
 */
export function SourceDetail({
  entry,
  selfId,
  index,
  count,
  section,
  onMove,
  moveError,
  onRemoved,
}: {
  entry: Entry
  selfId: string
  /** Its place among the sources of its scope, which `onMove` changes. */
  index: number
  count: number
  /** The part of an IPTV source's page the address names; its Summary without one. */
  section?: string
  onMove?: (to: number) => void
  /** The failure of the last move, shown under Order. */
  moveError?: string
  onRemoved?: () => void
}) {
  const { language, t } = useI18n()
  const text = t.sources
  const toast = useToast()
  const { addon, scope } = entry
  const iptv = isIptv(addon)
  const folder = addon.folder
  useFolderScanFollowed(addon)
  const system = useServerSystem()
  const [editing, setEditing] = useState(false)
  const [confirming, setConfirming] = useState(false)

  const fallbackScope: Scope = scope ?? 'shared'
  const toggle = useSourceToggle(fallbackScope, addon.id)
  const refresh = useMutation({
    mutationFn: () => refreshAddon(fallbackScope, addon.id),
    onSuccess: (updated) => {
      replaceCachedAddon(fallbackScope, updated)
      // An IPTV source answers with its state: a failed download is told by its error code.
      const failed = updated.source?.error ?? ''
      if (failed !== '') {
        toast(t.iptv.errors[failed] ?? t.errors.generic, { tone: 'danger' })
      } else {
        toast(
          folder !== null
            ? t.localFolders.scanStarted(updated.name)
            : iptv
              ? t.lineup.summary.refreshed
              : text.refreshed(updated.name),
        )
      }
    },
    onSettled: () => invalidateScope(fallbackScope),
  })
  const remove = useMutation({
    mutationFn: () => deleteAddon(fallbackScope, addon.id),
    onSuccess: () => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(fallbackScope), (old) =>
        old?.filter((item) => item.id !== addon.id),
      )
      setConfirming(false)
      toast(text.removed(addon.name))
      onRemoved?.()
    },
    onSettled: () => invalidateScope(fallbackScope),
  })

  const kindLine = [
    folder !== null && folder.share !== ''
      ? t.localFolders.locations[folder.share]
      : text.kindName[addon.kind],
    !iptv && folder === null && addon.version !== '' ? `v${addon.version}` : '',
    folder !== null ? t.localFolders.kinds[folder.kind] : '',
  ]
    .filter(Boolean)
    .join(' · ')
  const when = lastTime(addon)
  const editLabel = folder !== null ? t.localFolders.edit : iptv ? text.edit : text.replace
  const refreshLabel =
    folder !== null ? t.localFolders.scan : iptv ? t.lineup.summary.refresh : text.refresh

  const actions = scope !== null && (
    <div className="flex shrink-0 items-center gap-2 max-md:w-full">
      <Button
        icon={ArrowsClockwiseIcon}
        loading={refresh.isPending || (folder?.scanning ?? false)}
        title={iptv ? t.lineup.summary.refreshHelp : undefined}
        aria-label={
          folder !== null
            ? t.localFolders.scanLabel(addon.name)
            : iptv
              ? undefined
              : text.refreshLabel(addon.name)
        }
        onClick={() => {
          toggle.reset()
          refresh.reset()
          refresh.mutate()
        }}
      >
        {iptv && refresh.isPending ? t.lineup.summary.refreshing : refreshLabel}
      </Button>
      <Menu
        label={text.moreActions(addon.name)}
        align="end"
        trigger={(props) => (
          <IconButton {...props} label={text.moreActions(addon.name)} icon={DotsThreeIcon} />
        )}
      >
        <MenuItem
          icon={iptv || folder !== null ? PencilSimpleIcon : LinkSimpleIcon}
          onSelect={() => setEditing(true)}
        >
          {editLabel}
        </MenuItem>
        <MenuSeparator />
        <MenuItem icon={TrashIcon} danger onSelect={() => setConfirming(true)}>
          {text.remove}
        </MenuItem>
      </Menu>
    </div>
  )

  // Who uses it and where it stands: the rows an IPTV source's Summary shows too.
  const forWhom = scope !== null && (
    <Group title={text.forWhom}>
      <div className="grid grid-cols-[minmax(0,1fr)] gap-2">
        <SettingRow
          kind="switch"
          label={text.enabled}
          help={text.enabledHelp}
          checked={toggle.isPending ? (toggle.variables ?? addon.enabled) : addon.enabled}
          disabled={toggle.isPending}
          error={toggle.isError ? errorMessage(t, toggle.error) : undefined}
          onChange={(enabled) => {
            toggle.reset()
            refresh.reset()
            toggle.mutate(enabled)
          }}
        />
        {scope === 'shared' && <SourceUsedBy key={addon.id} addon={addon} />}
        {onMove && count > 1 && (
          <OrderRow
            name={addon.name}
            index={index}
            count={count}
            onMove={onMove}
            error={moveError}
          />
        )}
      </div>
    </Group>
  )

  const failure = refresh.isError && (
    <div className="mt-4">
      <FieldError>{errorMessage(t, refresh.error)}</FieldError>
    </div>
  )

  return (
    <article aria-labelledby={`source-${addon.id}`} className="min-w-0">
      <header className="flex items-start gap-3.5 max-md:flex-wrap">
        <SourceMark addon={addon} />
        <div className="min-w-0 flex-1">
          <h2 id={`source-${addon.id}`} className="text-h2 hyphenate text-ink">
            {addon.name}
          </h2>
          <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-control text-ink-2">
            <span>{kindLine}</span>
            {addon.music !== null && <MusicBadge music={addon.music} />}
            <SourceStatus addon={addon} />
            {when !== null && (
              <span className="text-ink-3">
                <RelativeTime iso={when} />
              </span>
            )}
          </p>
        </div>
        {actions}
      </header>
      {failure}

      {iptv && scope !== null ? (
        <IptvSource
          scope={scope}
          addon={addon}
          section={section}
          forWhom={forWhom}
          retry={
            <Button
              icon={ArrowsClockwiseIcon}
              loading={refresh.isPending}
              onClick={() => {
                toggle.reset()
                refresh.reset()
                refresh.mutate()
              }}
            >
              {refresh.isPending ? t.lineup.summary.refreshing : t.lineup.summary.refresh}
            </Button>
          }
        />
      ) : (
        <>
          {scope === null && (
            <Notice className="mt-8">{text.readOnly(entry.owner?.name ?? '')}</Notice>
          )}
          {addon.source !== null && addon.source.error !== '' && (
            <AlertTile
              tone="danger"
              title={t.iptv.errors[addon.source.error] ?? t.errors.generic}
              className="mt-8"
            />
          )}
          {forWhom}
          {scope === 'shared' && (addon.kind === 'stremio' || addon.language !== '') && (
            <Group title={text.descriptions}>
              <AddonLanguage addon={addon} />
            </Group>
          )}
          {scope !== null && addon.music !== null && (
            <AddonSettings key={addon.id} scope={scope} addon={addon} music={addon.music} />
          )}
          {folder !== null && <LocalFolderFigures folder={folder} />}
          {scope !== null && folder !== null && folder.kind !== 'homevideos' && (
            <UnmatchedFiles addon={addon} folder={folder} />
          )}
          <Group title={text.details}>
            <Facts>
              {entry.owner !== null && (
                <Fact label={text.owner}>
                  <OwnerLabel entry={entry} selfId={selfId} />
                </Fact>
              )}
              {!iptv && folder === null && addon.description !== '' && (
                <Fact label={text.description} wide>
                  {addon.description}
                </Fact>
              )}
              <Fact
                wide
                label={
                  folder !== null
                    ? folder.share === ''
                      ? t.localFolders.path[computer(system)]
                      : t.localFolders.address
                    : iptv
                      ? text.address
                      : text.manifestUrl
                }
              >
                <span className="font-mono text-body break-all">{addon.manifestUrl}</span>
              </Fact>
              {folder !== null && folder.share === 's3' && (
                <Fact label={t.localFolders.endpoint} wide>
                  <span className="font-mono text-body break-all">{folder.endpoint}</span>
                </Fact>
              )}
              {folder !== null && folder.share === 'rclone' && (
                <Fact label={t.localFolders.rcloneRemote} wide>
                  <span className="font-mono text-body break-all">{folder.rcloneRemote}</span>
                </Fact>
              )}
              {folder !== null && folder.share !== '' && folder.user !== '' && (
                <Fact
                  label={folder.share === 's3' ? t.localFolders.accessKey : t.localFolders.user}
                >
                  <span className="break-all">{folder.user}</span>
                </Fact>
              )}
              {!iptv && folder === null && addon.version !== '' && (
                <Fact label={text.version}>{addon.version}</Fact>
              )}
              {!iptv && folder === null && addon.resources.length > 0 && (
                <Fact label={text.provides}>
                  <span className="flex flex-wrap justify-end gap-1.5">
                    {addon.resources.map((resource) => (
                      <Badge key={resource}>{stremioLabel(t.stremioResources, resource)}</Badge>
                    ))}
                  </span>
                </Fact>
              )}
              {!iptv && folder === null && addon.types.length > 0 && (
                <Fact label={text.types}>
                  <span className="flex flex-wrap justify-end gap-1.5">
                    {addon.types.map((type) => (
                      <Badge key={type} tone="accent">
                        {catalogTypeLabel(t, type)}
                      </Badge>
                    ))}
                  </span>
                </Fact>
              )}
              {!iptv && folder === null && (
                <>
                  <Fact label={text.catalogs}>{text.catalogCount(addon.catalogCount)}</Fact>
                  <Fact label={text.lastRefresh}>
                    <RelativeTime iso={addon.refreshedAt} />
                  </Fact>
                </>
              )}
              {folder !== null && (
                <>
                  <Fact label={t.localFolders.holds}>{t.localFolders.kinds[folder.kind]}</Fact>
                  <Fact label={t.localFolders.lastScan}>
                    {folder.scannedAt === null ? (
                      t.localFolders.never
                    ) : (
                      <RelativeTime iso={folder.scannedAt} />
                    )}
                  </Fact>
                  {folder.error !== '' && folder.checkedAt !== null && (
                    <Fact label={t.localFolders.lastAttempt}>
                      <RelativeTime iso={folder.checkedAt} />
                    </Fact>
                  )}
                </>
              )}
              {addon.source !== null && (
                <>
                  <Fact label={text.channels}>
                    {addon.source.options.liveTv
                      ? text.shownOf(
                          addon.source.lineup.shownChannels.toLocaleString(language),
                          addon.source.lineup.channels.toLocaleString(language),
                        )
                      : text.notImported}
                  </Fact>
                  {(addon.source.options.movies || addon.source.options.series) && (
                    <Fact label={text.vod}>
                      {text.vodShort(
                        addon.source.vod.shownMovies.toLocaleString(language),
                        addon.source.vod.shownSeries.toLocaleString(language),
                      )}
                    </Fact>
                  )}
                  <Fact label={text.lastFetch}>
                    {addon.source.fetchedAt === null ? (
                      text.never
                    ) : (
                      <RelativeTime iso={addon.source.fetchedAt} />
                    )}
                  </Fact>
                  {addon.source.nextAt !== null && (
                    <Fact label={text.nextFetch}>
                      <RelativeTime iso={addon.source.nextAt} />
                    </Fact>
                  )}
                </>
              )}
            </Facts>
          </Group>
        </>
      )}

      {scope !== null && (
        <Modal open={editing} onClose={() => setEditing(false)} title={editLabel} width={640}>
          <div className="p-5">
            {folder !== null ? (
              <LocalFolderEditForm
                addon={addon}
                folder={folder}
                onSaved={(updated) => {
                  replaceCachedAddon(scope, updated)
                  setEditing(false)
                  toast(t.localFolders.saved)
                }}
                onCancel={() => setEditing(false)}
              />
            ) : iptv ? (
              <IptvEditForm
                scope={scope}
                addon={addon}
                onSaved={(updated) => {
                  replaceCachedAddon(scope, updated)
                  setEditing(false)
                  toast(text.saved)
                }}
                onCancel={() => setEditing(false)}
              />
            ) : (
              <ReplaceForm
                scope={scope}
                addon={addon}
                onDone={(replaced) => {
                  setEditing(false)
                  if (replaced) toast(text.replaced)
                }}
              />
            )}
          </div>
        </Modal>
      )}

      {scope !== null && (
        <ConfirmDialog
          open={confirming}
          onClose={() => {
            setConfirming(false)
            remove.reset()
          }}
          onConfirm={() => remove.mutate()}
          title={text.removeTitle(addon.name)}
          confirmLabel={text.remove}
          busy={remove.isPending}
          error={remove.isError ? errorMessage(t, remove.error) : undefined}
        >
          {scope === 'shared' ? text.removeShared : text.removeMine}
        </ConfirmDialog>
      )}
    </article>
  )
}

/** Where a source stands among its scope's sources, "2 of 3", with the buttons that move it. */
function OrderRow({
  name,
  index,
  count,
  onMove,
  error,
}: {
  name: string
  index: number
  count: number
  onMove: (to: number) => void
  error?: ReactNode
}) {
  const { t } = useI18n()
  return (
    <div className="rounded-row bg-s1">
      <div className="flex min-h-[3.375rem] items-center gap-4 py-2 pr-3 pl-4">
        <span className="min-w-0 flex-1 text-h3 text-ink">{t.sources.order}</span>
        <span className="figures text-control text-ink-2">
          {t.sources.position(index + 1, count)}
        </span>
        <MoveButtons name={name} index={index} count={count} onMove={onMove} />
      </div>
      {error !== undefined && (
        <div className="px-4 pb-3">
          <FieldError>{error}</FieldError>
        </div>
      )}
    </div>
  )
}

/** The languages offered for a server addon's descriptions, as language codes. */
const descriptionLanguages = (
  'ar bg ca cs da de el en es fa fi fr he hi hr hu id it ja ko ms nb nl pl pt pt-BR ro ru sk sr sv ' +
  'th tr uk vi zh zh-TW'
).split(' ')

/**
 * Language of its descriptions, for a server's Stremio addon: the language it was configured in,
 * saved as it changes. None leaves it out of the users' metadata languages.
 */
function AddonLanguage({ addon }: { addon: Addon }) {
  const { language, t } = useI18n()
  const text = t.sources
  const toast = useToast()
  const save = useMutation({
    mutationFn: (chosen: string) => updateAddon('shared', addon.id, { language: chosen }),
    onSuccess: (updated) => {
      replaceCachedAddon('shared', updated)
      toast(text.languageSaved(updated.name))
    },
    onSettled: () => invalidateScope('shared'),
  })
  const codes =
    addon.language === '' || descriptionLanguages.includes(addon.language)
      ? descriptionLanguages
      : [...descriptionLanguages, addon.language]
  return (
    <SettingRow
      kind="choice"
      label={text.language}
      help={text.languageHelp}
      value={save.isPending ? (save.variables ?? addon.language) : addon.language}
      options={[{ value: '', label: text.languageNone }, ...languageOptions(codes, language)]}
      onChange={(chosen) => save.mutate(chosen)}
      disabled={save.isPending}
      error={save.isError ? errorMessage(t, save.error) : undefined}
    />
  )
}

/** A new manifest address for a Stremio or music addon, keeping its libraries. */
function ReplaceForm({
  scope,
  addon,
  onDone,
}: {
  scope: Scope
  addon: Addon
  onDone: (replaced: boolean) => void
}) {
  const { t } = useI18n()
  const text = t.sources
  const [manifestUrl, setManifestUrl] = useState('')
  const replace = useMutation({
    mutationFn: (url: string) => updateAddon(scope, addon.id, { manifestUrl: url }),
    onSuccess: (updated) => {
      replaceCachedAddon(scope, updated)
      onDone(true)
    },
    onSettled: () => invalidateScope(scope),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    replace.mutate(manifestUrl.trim())
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <Field
        label={text.newManifestUrl}
        help={`${text.replaceHint} ${t.sourceAdd.manifestUrlHint}`}
        error={replace.isError ? errorMessage(t, replace.error) : undefined}
      >
        <TextInput
          type="url"
          inputMode="url"
          value={manifestUrl}
          onValue={(value) => {
            replace.reset()
            setManifestUrl(value)
          }}
          placeholder={t.sourceAdd.manifestUrlPlaceholder}
          autoComplete="off"
          spellCheck={false}
          autoFocus
          required
          mono
        />
      </Field>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" variant="primary" loading={replace.isPending}>
          {text.replaceSubmit}
        </Button>
        <Button variant="ghost" onClick={() => onDone(false)}>
          {t.common.cancel}
        </Button>
      </div>
    </form>
  )
}
