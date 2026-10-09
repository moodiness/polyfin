import {
  ArrowDownIcon,
  ArrowRightIcon,
  ArrowUpIcon,
  ArrowsClockwiseIcon,
  DotsThreeIcon,
  GearSixIcon,
  LinkSimpleIcon,
  PencilSimpleIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import { useId, useState, type FormEvent, type ReactNode } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  deleteAddon,
  queryClient,
  queryKeys,
  refreshAddon,
  updateAddon,
  type Addon,
  type IptvSource,
  type Scope,
} from '@/api'
import { errorMessage, stremioLabel } from '@/format'
import { useI18n } from '@/i18n'
import {
  Avatar,
  Badge,
  Button,
  ButtonLink,
  ConfirmDialog,
  cx,
  Field,
  FieldError,
  IconButton,
  Menu,
  MenuItem,
  MenuSeparator,
  Notice,
  Panel,
  PanelSection,
  Switch,
  TextInput,
  useToast,
  RelativeTime,
} from '@/ui'
import { AddonSettings, MusicBadge } from './AddonSettings'
import { IptvEditForm } from './IptvForms'
import {
  LocalFolderEditForm,
  LocalFolderFigures,
  UnmatchedFiles,
  useFolderScanFollowed,
} from './LocalFolders'
import {
  invalidateScope,
  isIptv,
  lastTime,
  replaceCachedAddon,
  sourcePath,
  type Entry,
} from './model'
import { SourceStatus, SourceTile } from './parts'

/** The tag of a source's kind, as in the list. */
export function KindBadge({ addon }: { addon: Addon }) {
  const { t } = useI18n()
  return <Badge>{t.sources.tag[addon.kind]}</Badge>
}

/** Who owns a source of the list: nothing for the server's, else the owner's initial and name. */
export function OwnerLabel({ entry, selfId }: { entry: Entry; selfId: string }) {
  const { t } = useI18n()
  if (entry.owner === null) return null
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 text-ink-2">
      <Avatar name={entry.owner.name} size="sm" />
      <span>
        {entry.owner.id === selfId ? t.sources.yours : t.sources.ownedBy(entry.owner.name)}
      </span>
    </span>
  )
}

type Editor = 'none' | 'edit' | 'settings'

/**
 * Everything about one source and every action on it: turn it on or off, move it, refresh it,
 * replace its address or edit its account, its music settings, scan a local folder and link its
 * unmatched files, remove it. `mode="panel"` is the detail beside the list; `mode="page"` is the
 * source's own page, with the settings and the unmatched files always open. Another user's source
 * (`entry.scope === null`) is shown without actions.
 */
export function SourceDetail({
  entry,
  selfId,
  mode,
  index,
  count,
  onMove,
  onRemoved,
}: {
  entry: Entry
  selfId: string
  mode: 'panel' | 'page'
  /** Its place among the sources of its scope, which `onMove` changes. */
  index: number
  count: number
  onMove?: (to: number) => void
  onRemoved?: () => void
}) {
  const { language, t } = useI18n()
  const text = t.sources
  const toast = useToast()
  const { addon, scope } = entry
  const iptv = isIptv(addon)
  const folder = addon.folder
  useFolderScanFollowed(addon)
  const [editor, setEditor] = useState<Editor>('none')
  const [confirming, setConfirming] = useState(false)
  const editorId = useId()

  const fallbackScope: Scope = scope ?? 'shared'
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => updateAddon(fallbackScope, addon.id, { enabled }),
    onSuccess: (updated) => replaceCachedAddon(fallbackScope, updated),
    onSettled: () => invalidateScope(fallbackScope),
  })
  const refresh = useMutation({
    mutationFn: () => refreshAddon(fallbackScope, addon.id),
    onSuccess: (updated) => {
      replaceCachedAddon(fallbackScope, updated)
      toast(
        folder !== null
          ? t.localFolders.scanStarted(updated.name)
          : iptv
            ? text.refreshedList(updated.name)
            : text.refreshed(updated.name),
      )
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

  function resetFeedback() {
    toggle.reset()
    refresh.reset()
  }

  const kindLine = [
    text.kindName[addon.kind],
    !iptv && folder === null && addon.version !== '' ? `v${addon.version}` : '',
    folder !== null ? t.localFolders.kinds[folder.kind] : '',
  ]
    .filter(Boolean)
    .join(' · ')
  const when = lastTime(addon)

  const actions =
    scope === null ? undefined : (
      <>
        <Button
          variant="secondary"
          size="sm"
          icon={ArrowsClockwiseIcon}
          loading={refresh.isPending || (folder?.scanning ?? false)}
          aria-label={
            folder !== null ? t.localFolders.scanLabel(addon.name) : text.refreshLabel(addon.name)
          }
          onClick={() => {
            resetFeedback()
            refresh.mutate()
          }}
        >
          <span className="max-sm:sr-only">
            {folder !== null ? t.localFolders.scan : text.refresh}
          </span>
        </Button>
        <Menu
          label={text.moreActions(addon.name)}
          align="end"
          trigger={(props) => (
            <IconButton {...props} label={text.moreActions(addon.name)} icon={DotsThreeIcon} />
          )}
        >
          {mode === 'panel' && (
            <MenuItem icon={ArrowRightIcon} to={sourcePath(scope, addon.id)}>
              {folder !== null ? t.localFolders.openPage : iptv ? text.openSource : text.openPage}
            </MenuItem>
          )}
          <MenuItem
            icon={iptv || folder !== null ? PencilSimpleIcon : LinkSimpleIcon}
            onSelect={() => setEditor(editor === 'edit' ? 'none' : 'edit')}
          >
            {folder !== null ? t.localFolders.edit : iptv ? text.edit : text.replace}
          </MenuItem>
          {addon.music !== null && mode === 'panel' && (
            <MenuItem icon={GearSixIcon} onSelect={() => setEditor('settings')}>
              {text.settings}
            </MenuItem>
          )}
          <MenuSeparator />
          <MenuItem icon={TrashIcon} danger onSelect={() => setConfirming(true)}>
            {text.remove}
          </MenuItem>
        </Menu>
      </>
    )

  return (
    <Panel
      as="section"
      flush
      title={addon.name}
      titleAside={
        <>
          <KindBadge addon={addon} />
          {addon.music !== null && <MusicBadge music={addon.music} />}
        </>
      }
      description={
        <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span>{kindLine}</span>
          <SourceStatus addon={addon} />
          {when !== null && (
            <span className="text-ink-3">
              <RelativeTime iso={when} />
            </span>
          )}
        </span>
      }
      media={<SourceTile addon={addon} />}
      actions={actions}
    >
      {scope === null ? (
        <PanelSection>
          <Notice>{text.readOnly(entry.owner?.name ?? '')}</Notice>
        </PanelSection>
      ) : (
        <PanelSection>
          <div className="flex flex-wrap items-start justify-between gap-x-8 gap-y-5">
            <div className="flex min-w-0 flex-1 basis-[280px] items-start justify-between gap-4">
              <div className="min-w-0">
                <p id={`${editorId}-on`} className="text-control font-medium text-ink">
                  {text.enabled}
                </p>
                <p id={`${editorId}-on-help`} className="mt-1 text-small text-ink-3">
                  {text.enabledHelp}
                </p>
              </div>
              <Switch
                checked={addon.enabled}
                labelledBy={`${editorId}-on`}
                describedById={`${editorId}-on-help`}
                stateText={[text.active, text.off]}
                disabled={toggle.isPending}
                onChange={(enabled) => {
                  resetFeedback()
                  toggle.mutate(enabled)
                }}
              />
            </div>
            {onMove && count > 1 && (
              <div className="flex items-center gap-3">
                <div>
                  <p className="text-control font-medium text-ink">{text.order}</p>
                  <p className="mt-1 text-small text-ink-3">
                    <span className="figures">{text.position(index + 1, count)}</span>
                  </p>
                </div>
                <div className="flex gap-1">
                  <IconButton
                    label={text.moveUp(addon.name)}
                    icon={ArrowUpIcon}
                    size="sm"
                    disabled={index === 0}
                    onClick={() => onMove(index - 1)}
                  />
                  <IconButton
                    label={text.moveDown(addon.name)}
                    icon={ArrowDownIcon}
                    size="sm"
                    disabled={index === count - 1}
                    onClick={() => onMove(index + 1)}
                  />
                </div>
              </div>
            )}
          </div>
          {(toggle.isError || refresh.isError) && (
            <div className="mt-4 flex flex-col gap-2">
              {toggle.isError && <FieldError>{errorMessage(t, toggle.error)}</FieldError>}
              {refresh.isError && <FieldError>{errorMessage(t, refresh.error)}</FieldError>}
            </div>
          )}
        </PanelSection>
      )}

      {scope !== null && editor === 'edit' && (
        <PanelSection
          title={folder !== null ? t.localFolders.edit : iptv ? text.edit : text.replace}
        >
          {folder !== null ? (
            <LocalFolderEditForm
              addon={addon}
              folder={folder}
              onSaved={(updated) => {
                replaceCachedAddon(scope, updated)
                setEditor('none')
                toast(t.localFolders.saved)
              }}
              onCancel={() => setEditor('none')}
            />
          ) : iptv ? (
            <IptvEditForm
              scope={scope}
              addon={addon}
              onSaved={(updated) => {
                replaceCachedAddon(scope, updated)
                setEditor('none')
                toast(text.saved)
              }}
              onCancel={() => setEditor('none')}
            />
          ) : (
            <ReplaceForm
              scope={scope}
              addon={addon}
              onDone={(replaced) => {
                setEditor('none')
                if (replaced) toast(text.replaced)
              }}
            />
          )}
        </PanelSection>
      )}

      {scope !== null && addon.music !== null && (mode === 'page' || editor === 'settings') && (
        <PanelSection>
          <AddonSettings
            key={addon.id}
            scope={scope}
            addon={addon}
            music={addon.music}
            onClose={mode === 'panel' ? () => setEditor('none') : undefined}
          />
        </PanelSection>
      )}

      {addon.source !== null && <IptvFigures source={addon.source} />}
      {folder !== null && <LocalFolderFigures folder={folder} />}

      <PanelSection title={text.details}>
        <dl className="grid gap-x-6 gap-y-3 text-control sm:grid-cols-[minmax(140px,auto)_1fr]">
          {entry.owner !== null && (
            <Fact label={text.owner}>
              <OwnerLabel entry={entry} selfId={selfId} />
            </Fact>
          )}
          {!iptv && folder === null && addon.description !== '' && (
            <Fact label={text.description}>
              <span className="text-ink-2">{addon.description}</span>
            </Fact>
          )}
          <Fact
            label={folder !== null ? t.localFolders.path : iptv ? text.address : text.manifestUrl}
          >
            <span className="font-mono text-small break-all text-ink-2">{addon.manifestUrl}</span>
          </Fact>
          {!iptv && folder === null && addon.version !== '' && (
            <Fact label={text.version}>
              <span className="figures">{addon.version}</span>
            </Fact>
          )}
          {!iptv && folder === null && addon.resources.length > 0 && (
            <Fact label={text.provides}>
              <span className="flex flex-wrap gap-1">
                {addon.resources.map((resource) => (
                  <Badge key={resource}>{stremioLabel(t.stremioResources, resource)}</Badge>
                ))}
              </span>
            </Fact>
          )}
          {!iptv && folder === null && addon.types.length > 0 && (
            <Fact label={text.types}>
              <span className="flex flex-wrap gap-1">
                {addon.types.map((type) => (
                  <Badge key={type} tone="accent">
                    {stremioLabel(t.stremioTypes, type)}
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
        </dl>
        {scope !== null && (iptv || folder !== null) && mode === 'panel' && (
          <div className="mt-5">
            <ButtonLink to={sourcePath(scope, addon.id)} variant="primary" iconEnd={ArrowRightIcon}>
              {folder !== null ? t.localFolders.openPage : text.openSource}
            </ButtonLink>
          </div>
        )}
      </PanelSection>

      {scope !== null && folder !== null && mode === 'page' && (
        <UnmatchedFiles addon={addon} folder={folder} />
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
    </Panel>
  )
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-ink-3">{label}</dt>
      <dd className="min-w-0 text-ink">{children}</dd>
    </>
  )
}

/** An IPTV source's counts, side by side, and the error of its last download. */
function IptvFigures({ source }: { source: IptvSource }) {
  const { language, t } = useI18n()
  const text = t.sources
  const figures = [
    { label: text.figures.entries, value: source.channels, warn: false },
    { label: text.figures.shown, value: source.lineup.shownChannels, warn: false },
    { label: text.figures.mapped, value: source.lineup.mapped, warn: false },
    {
      label: text.figures.unmapped,
      value: source.lineup.unmapped,
      warn: source.lineup.unmapped > 0,
    },
  ]
  return (
    <PanelSection>
      <dl className="grid grid-cols-4 gap-y-4 max-sm:grid-cols-2">
        {figures.map((figure, i) => (
          <div
            key={figure.label}
            className={cx(
              'min-w-0 px-4 first:pl-0',
              i > 0 && 'border-l border-line',
              'max-sm:[&:nth-child(3)]:border-l-0 max-sm:[&:nth-child(3)]:pl-0',
            )}
          >
            <dt className="text-small text-ink-3">{figure.label}</dt>
            <dd
              className={cx(
                'figures mt-1.5 text-[19px] font-medium',
                figure.warn ? 'text-warn' : 'text-ink',
              )}
            >
              {figure.value.toLocaleString(language)}
            </dd>
          </div>
        ))}
      </dl>
      {source.error !== '' && (
        <Notice tone="danger" className="mt-5">
          {Object.hasOwn(t.iptv.errors, source.error)
            ? t.iptv.errors[source.error]
            : t.errors.generic}
        </Notice>
      )}
    </PanelSection>
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
