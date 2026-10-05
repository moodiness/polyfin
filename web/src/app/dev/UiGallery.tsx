import {
  ArrowClockwiseIcon,
  ArrowRightIcon,
  BroadcastIcon,
  DeviceMobileIcon,
  DotsThreeIcon,
  MagnifyingGlassIcon,
  MusicNotesIcon,
  PencilSimpleIcon,
  PlusIcon,
  PuzzlePieceIcon,
  TelevisionIcon,
  TrashIcon,
  TrayIcon,
  UserCircleIcon,
} from '@phosphor-icons/react'
import { useState, type ReactNode } from 'react'
import { PageLayout } from '@/app/PageLayout'
import { settingsSections } from '@/app/navigation'
import { useI18n } from '@/i18n'
import {
  Avatar,
  Badge,
  Block,
  Button,
  ButtonLink,
  Checkbox,
  ConfirmDialog,
  Count,
  Drawer,
  EmptyState,
  Field,
  IconButton,
  IconTile,
  InlineError,
  Kbd,
  Menu,
  MenuItem,
  MenuSeparator,
  modKey,
  Notice,
  NumberInput,
  Panel,
  PanelFooter,
  PanelSection,
  ProgressBar,
  Row,
  RowList,
  SaveBar,
  SecretField,
  SectionNav,
  Segmented,
  Select,
  Skeleton,
  SkeletonRows,
  SkeletonText,
  StatusPill,
  Switch,
  Table,
  Tabs,
  Textarea,
  TextInput,
  TextLink,
  Tooltip,
  useToast,
} from '@/ui'

/** A labelled specimen of the gallery. */
function Specimen({ name, children }: { name: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-3">
      <p className="font-mono text-micro text-ink-3">{name}</p>
      <div className="flex flex-wrap items-start gap-3">{children}</div>
    </div>
  )
}

const swatches = [
  ['bg', 'bg-bg'],
  ['s1', 'bg-s1'],
  ['s2', 'bg-s2'],
  ['s3', 'bg-s3'],
  ['s4', 'bg-s4'],
  ['accent', 'bg-accent'],
  ['accent-press', 'bg-accent-press'],
  ['link', 'bg-link'],
  ['ok', 'bg-ok'],
  ['warn', 'bg-warn'],
  ['danger', 'bg-danger'],
  ['brand', 'bg-brand'],
] as const

/**
 * `/dev/ui`, in development only: every component of `src/ui/` in its states, for building pages
 * and for review. Not translated; not in production builds.
 */
export default function UiGallery() {
  const { t } = useI18n()
  const toast = useToast()
  const [text, setText] = useState('')
  const [number, setNumber] = useState<number | null>(90)
  const [choice, setChoice] = useState<'fr' | 'en'>('fr')
  const [quality, setQuality] = useState('1080p')
  const [on, setOn] = useState(true)
  const [off, setOff] = useState(false)
  const [checked, setChecked] = useState(true)
  const [secret, setSecret] = useState<string | undefined>(undefined)
  const [savedSecret, setSavedSecret] = useState<string | undefined>(undefined)
  const [tab, setTab] = useState('summary')
  const [selected, setSelected] = useState('flux')
  const [search, setSearch] = useState('')
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [drawer, setDrawer] = useState(false)
  const [dirty, setDirty] = useState(true)
  const [saving, setSaving] = useState(false)

  return (
    <PageLayout
      title="Design system"
      lede="Every component of src/ui in its states. Development builds only."
      actions={
        <Button variant="primary" icon={PlusIcon}>
          Primary action
        </Button>
      }
      back={{ to: '/', label: t.nav.home }}
    >
      <Block title="Tokens">
        <div className="grid grid-cols-6 gap-3 max-md:grid-cols-3">
          {swatches.map(([name, className]) => (
            <div key={name} className="flex flex-col gap-1.5">
              <span className={`h-12 rounded-row border border-line-2 ${className}`} />
              <span className="font-mono text-micro text-ink-3">{name}</span>
            </div>
          ))}
        </div>
        <div className="mt-6 flex flex-col gap-2">
          <p className="text-h1">Heading 30</p>
          <p className="text-h2">Heading 22</p>
          <p className="text-h3">Heading 16</p>
          <p className="text-lead text-ink-2">Lead 15: what a page is for.</p>
          <p className="text-body">
            Body 14 in ink, <span className="text-ink-2">ink-2</span>,{' '}
            <span className="text-ink-3">ink-3</span>.
          </p>
          <p className="text-small text-ink-3">Small 13, for help under a field.</p>
          <p className="figures text-micro text-ink-2">
            figures 12: 21:42 · 192.168.1.31 · 0.15.0 · 1 373
          </p>
        </div>
      </Block>

      <Block title="Buttons">
        <div className="flex flex-col gap-5">
          <Specimen name="variants">
            <Button variant="primary">Save</Button>
            <Button>Replace</Button>
            <Button variant="ghost">Disconnect</Button>
            <Button variant="danger">Remove</Button>
          </Specimen>
          <Specimen name="icons, sm, disabled, loading">
            <Button icon={ArrowClockwiseIcon}>Refresh</Button>
            <Button variant="secondary" iconEnd={ArrowRightIcon}>
              Map the channels
            </Button>
            <Button size="sm" icon={PlusIcon}>
              Small
            </Button>
            <Button variant="primary" disabled>
              Disabled
            </Button>
            <Button variant="primary" loading>
              Saving…
            </Button>
            <ButtonLink to="/sources" icon={PuzzlePieceIcon}>
              ButtonLink
            </ButtonLink>
          </Specimen>
          <Specimen name="IconButton: md, sm, pressed, danger, disabled, loading">
            <IconButton label="More actions" icon={DotsThreeIcon} />
            <IconButton label="Edit" icon={PencilSimpleIcon} size="sm" />
            <IconButton label="Pressed" icon={PencilSimpleIcon} pressed />
            <IconButton label="Delete" icon={TrashIcon} danger />
            <IconButton label="Disabled" icon={TrashIcon} disabled />
            <IconButton label="Loading" icon={TrashIcon} loading />
          </Specimen>
        </div>
      </Block>

      <Block title="Fields">
        <div className="grid grid-cols-2 gap-x-8 gap-y-6 max-md:grid-cols-1">
          <Field label="Server name" help="Shown in apps and on the sign-in page.">
            <TextInput value={text} onValue={setText} placeholder="Polyfin" />
          </Field>
          <Field label="Filter" hideLabel>
            <TextInput
              icon={MagnifyingGlassIcon}
              size="sm"
              placeholder="Filter the countries"
              type="search"
            />
          </Field>
          <Field label="Name" error="Names must be 1 to 64 characters.">
            <TextInput defaultValue="" />
          </Field>
          <Field label="Disabled">
            <TextInput defaultValue="Read only here" disabled />
          </Field>
          <Field label="New password" help={t.common.passwordRule}>
            <TextInput type="password" revealable autoComplete="new-password" />
          </Field>
          <Field label="Marked as played after" help="From 50 to 100. 90 by default.">
            <NumberInput value={number} onValue={setNumber} min={50} max={100} suffix="%" />
          </Field>
          <Field label="Quality">
            <Select
              value={quality}
              onValue={setQuality}
              options={[
                { value: '4k', label: '4K' },
                { value: '1080p', label: '1080p' },
                { value: '720p', label: '720p', disabled: true },
              ]}
            />
          </Field>
          <Field label="Custom CSS" help="Applied to the web player.">
            <Textarea mono rows={3} defaultValue={'.skinHeader { opacity: .9 }'} />
          </Field>
        </div>
      </Block>

      <Block title="Choices">
        <div className="flex flex-col gap-5">
          <Specimen name="Switch: on, off, disabled, with its state written">
            <Switch label="On" checked={on} onChange={setOn} />
            <Switch label="Off" checked={off} onChange={setOff} />
            <Switch label="Disabled" checked disabled onChange={() => {}} />
            <Switch
              label="TheIntroDB"
              checked={on}
              onChange={setOn}
              stateText={['Active', 'Off']}
            />
            <Switch label="Similar titles" checked={off} onChange={setOff} stateText />
          </Specimen>
          <Specimen name="Checkbox: checked, unchecked, indeterminate, disabled, with help and aside">
            <div className="grid w-full max-w-[560px] grid-cols-2 gap-x-4 gap-y-3">
              <Checkbox
                label="France"
                aside={<span className="figures">1 204</span>}
                checked={checked}
                onChange={setChecked}
              />
              <Checkbox
                label="Portugal"
                aside={<span className="figures">690</span>}
                checked={false}
                onChange={() => {}}
              />
              <Checkbox label="Some countries" checked={false} indeterminate onChange={() => {}} />
              <Checkbox label="Disabled" checked disabled onChange={() => {}} />
              <Checkbox
                label="Hide from sign-in"
                help="Typed by name instead."
                checked={checked}
                onChange={setChecked}
              />
            </div>
          </Specimen>
          <Specimen name="Segmented">
            <Segmented
              label="Language"
              value={choice}
              onChange={setChoice}
              options={[
                { value: 'fr', label: 'FR' },
                { value: 'en', label: 'EN' },
              ]}
            />
            <Segmented
              label="Language, small"
              size="sm"
              value={choice}
              onChange={setChoice}
              options={[
                { value: 'fr', label: 'FR' },
                { value: 'en', label: 'EN' },
              ]}
            />
          </Specimen>
        </div>
      </Block>

      <Block title="SecretField">
        <div className="flex max-w-[640px] flex-col gap-8">
          <SecretField
            label="TheIntroDB key (optional)"
            help="Raises TheIntroDB's daily limit. Playback works without it."
            saved={false}
            value={secret}
            onValue={setSecret}
            status={null}
            actions={<Button disabled={secret === undefined}>Check and save</Button>}
          />
          <SecretField
            label="PublicMetaDB key"
            help="Adds PublicMetaDB as a source of markers."
            saved
            value={savedSecret}
            onValue={setSavedSecret}
            reveal={async () => 'pmdb_7f3a91c2e04b6d58'}
            status="Saved and checked on 2 October"
          />
          <SecretField
            label="MDBList key (read only)"
            saved
            value={undefined}
            replaceable={false}
            removable={false}
            reveal={() => Promise.reject(new Error('network'))}
            error="The server refused the key."
          />
        </div>
      </Block>

      <Block title="States">
        <div className="flex flex-col gap-5">
          <Specimen name="StatusPill">
            <StatusPill tone="ok">Active</StatusPill>
            <StatusPill tone="warn">Incomplete guide</StatusPill>
            <StatusPill tone="danger">Unreachable</StatusPill>
            <StatusPill tone="muted">Not configured</StatusPill>
            <StatusPill tone="live">Playing</StatusPill>
          </Specimen>
          <Specimen name="Badge and Count">
            <Badge>Stremio</Badge>
            <Badge tone="accent">Default</Badge>
            <Badge tone="ok">Set</Badge>
            <Badge tone="warn">Not read</Badge>
            <Badge tone="danger">Blocked</Badge>
            <Count>2</Count>
          </Specimen>
          <Specimen name="Notice">
            <div className="flex w-full flex-col gap-2">
              <Notice tone="warn" action={<TextLink to="/live-tv">Open the mapping</TextLink>}>
                More than eight channels out of ten have no guide.
              </Notice>
              <Notice tone="info">Applied at the next refresh of the source.</Notice>
              <Notice tone="ok">The key works.</Notice>
              <Notice tone="danger">The last download failed.</Notice>
            </div>
          </Specimen>
          <Specimen name="InlineError">
            <InlineError onRetry={() => toast('Retried', { tone: 'info' })} className="w-full">
              The administration API did not respond.
            </InlineError>
          </Specimen>
          <Specimen name="EmptyState">
            <EmptyState
              icon={TrayIcon}
              title="No sources yet"
              action={
                <Button variant="primary" icon={PlusIcon}>
                  Add a source
                </Button>
              }
              className="w-full"
            >
              Add a Stremio addon, a music addon or an IPTV account: their catalogs become
              libraries.
            </EmptyState>
          </Specimen>
          <Specimen name="Skeleton, SkeletonText, SkeletonRows">
            <div className="flex w-full flex-col gap-4">
              <Skeleton className="h-6 w-48" />
              <SkeletonText lines={3} className="max-w-md" />
              <SkeletonRows rows={2} />
            </div>
          </Specimen>
        </div>
      </Block>

      <Block title="Progress, avatars, keys, tooltip">
        <div className="flex flex-col gap-5">
          <Specimen name="ProgressBar: brand (playback), accent, neutral, warn, sizes">
            <div className="flex w-full max-w-[520px] flex-col gap-4">
              <ProgressBar
                value={0.3677}
                variant="brand"
                label="Dune"
                valueText="57 minutes of 2 hours 35"
              />
              <ProgressBar value={0.6} label="Task" />
              <ProgressBar value={0.32} variant="neutral" size="sm" label="Cache" />
              <ProgressBar value={0.92} variant="warn" size="lg" label="Disk" />
            </div>
          </Specimen>
          <Specimen name="Avatar, IconTile, Kbd, Tooltip">
            <Avatar name="Sam" size="sm" />
            <Avatar name="Julien" />
            <Avatar name="Lucas" size="lg" />
            <IconTile icon={BroadcastIcon} />
            <IconTile letters="Si" />
            <IconTile icon={TelevisionIcon} tone="warn" />
            <IconTile icon={DeviceMobileIcon} size="sm" />
            <Kbd>{modKey === '⌘' ? '⌘K' : 'Ctrl K'}</Kbd>
            <Kbd>↵</Kbd>
            <Tooltip content="Opens the Jellyfin web app in a new tab.">
              <Button variant="ghost">Hover or focus me</Button>
            </Tooltip>
          </Specimen>
        </div>
      </Block>

      <Block title="Lists" count={3} aside={<TextLink to="/system/logs">Open the log</TextLink>}>
        <div className="grid grid-cols-[minmax(0,432px)_minmax(0,1fr)] gap-6 max-lg:grid-cols-1">
          <RowList variant="separate" aria-label="Sources">
            <Row
              leading={PuzzlePieceIcon}
              title="Metadata"
              titleAside={<Badge>Stremio</Badge>}
              meta="Catalogs and metadata, 41 catalogs"
              trailing={
                <>
                  <StatusPill tone="ok">Active</StatusPill>
                  <span className="text-micro text-ink-3">18 min ago</span>
                </>
              }
              onClick={() => setSelected('metadata')}
              selected={selected === 'metadata'}
            />
            <Row
              leading={MusicNotesIcon}
              title="Music"
              titleAside={<Badge>Eclipse</Badge>}
              meta="Albums, artists and tracks"
              trailing={<StatusPill tone="ok">Active</StatusPill>}
              onClick={() => setSelected('flux')}
              selected={selected === 'flux'}
            />
            <Row
              leading={BroadcastIcon}
              title="My IPTV provider"
              titleAside={<Badge>Xtream</Badge>}
              meta={
                <>
                  <Avatar name="Lucas" size="sm" /> Lucas’s source
                </>
              }
              trailing={<StatusPill tone="warn">Incomplete guide</StatusPill>}
              onClick={() => setSelected('iptv')}
              selected={selected === 'iptv'}
            />
          </RowList>
          <RowList aria-label="Devices">
            <Row
              leading={TelevisionIcon}
              title="Living room Apple TV"
              meta="Infuse 8.1"
              trailing={<Button variant="ghost">Disconnect</Button>}
            />
            <Row
              leading={DeviceMobileIcon}
              title="Sam's iPhone"
              meta={<span className="figures">192.168.1.31</span>}
              trailing={<StatusPill tone="live">Playing</StatusPill>}
            />
            <Row leading={UserCircleIcon} title="Muted row" meta="Not configured" muted />
          </RowList>
        </div>
        <RowList variant="plain" className="mt-6" aria-label="Activity">
          <Row
            leading={<span className="figures w-14 text-micro text-ink-3">21:42</span>}
            title="Sam signed in on the living room Apple TV"
            trailing={<span className="text-small text-ink-3">Infuse 8.1</span>}
          />
          <Row
            leading={<span className="figures w-14 text-micro text-ink-3">20:15</span>}
            title="Metadata addon updated"
            trailing={<span className="text-small text-ink-3">41 catalogs</span>}
          />
        </RowList>
      </Block>

      <Block title="Table">
        <Table label="API keys">
          <thead>
            <tr>
              <th scope="col">App</th>
              <th scope="col">Created</th>
              <th scope="col" className="text-right">
                Requests
              </th>
              <th scope="col" className="w-0">
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td className="text-ink">Jellyseerr</td>
              <td>2 October</td>
              <td className="figures text-right">1 204</td>
              <td>
                <IconButton label="Delete Jellyseerr" icon={TrashIcon} size="sm" danger />
              </td>
            </tr>
            <tr>
              <td className="text-ink">Home Assistant</td>
              <td>14 September</td>
              <td className="figures text-right">38</td>
              <td>
                <IconButton label="Delete Home Assistant" icon={TrashIcon} size="sm" danger />
              </td>
            </tr>
          </tbody>
        </Table>
      </Block>

      <Block title="Navigation">
        <div className="flex flex-col gap-6">
          <Specimen name="Tabs: pill, links">
            <Tabs
              label="Content pages"
              items={[
                { id: 'sources', label: 'Sources', to: '/sources' },
                { id: 'libraries', label: 'Libraries', to: '/libraries' },
                { id: 'ui', label: 'This page', to: '/dev/ui' },
              ]}
            />
          </Specimen>
          <Specimen name="Tabs: underline, buttons (arrows move)">
            <Tabs
              label="Source sections"
              variant="underline"
              value={tab}
              onChange={setTab}
              className="w-full"
              items={[
                { id: 'summary', label: 'Summary' },
                { id: 'options', label: 'What to import' },
                { id: 'categories', label: 'Categories', count: 12 },
                { id: 'channels', label: 'Channels', count: 1691 },
              ]}
            />
          </Specimen>
          <Specimen name="SectionNav (a row of tabs below 768 px)">
            <div className="w-[216px] max-md:w-full">
              <SectionNav
                label="Settings sections"
                search={{ value: search, onChange: setSearch, placeholder: 'Search a setting' }}
                items={settingsSections.slice(0, 6).map((section) => ({
                  id: section.id,
                  label: t.nav.settingsSections[section.key],
                  icon: section.icon,
                  to: `#${section.id}`,
                }))}
                current="content"
              />
            </div>
          </Specimen>
        </div>
      </Block>

      <Block title="Panels">
        <Panel
          title="My IPTV provider"
          description="Xtream Codes, refreshed tonight at 03:30"
          media={<IconTile icon={BroadcastIcon} selected />}
          actions={
            <>
              <Button icon={ArrowClockwiseIcon}>Refresh</Button>
              <Menu
                label="More actions"
                align="end"
                trigger={(props) => (
                  <button
                    {...props}
                    aria-label="More actions"
                    className="inline-grid size-9 cursor-pointer place-items-center rounded-field text-ink-2 hover:bg-ink/8 hover:text-ink"
                  >
                    <DotsThreeIcon size={18} aria-hidden="true" />
                  </button>
                )}
              >
                <MenuItem icon={PencilSimpleIcon} onSelect={() => toast('Rename')}>
                  Rename
                </MenuItem>
                <MenuSeparator />
                <MenuItem icon={TrashIcon} danger onSelect={() => setConfirm(true)}>
                  Delete
                </MenuItem>
              </Menu>
            </>
          }
          flush
          footer={
            <PanelFooter note="Applied at the next refresh of the source.">
              <Button variant="ghost">Cancel</Button>
              <Button variant="primary" onClick={() => toast(t.ui.allSaved)}>
                Save
              </Button>
            </PanelFooter>
          }
        >
          <PanelSection
            title="Summary"
            description="What the source holds, and what Polyfin keeps."
          >
            <dl className="grid grid-cols-4 max-md:grid-cols-2 max-md:gap-y-4">
              {[
                ['Entries', '54 180'],
                ['Channels kept', '1 691'],
                ['With a guide', '318'],
                ['Without', '1 373'],
              ].map(([label, value], index) => (
                <div key={label} className="border-l border-line px-4 first:border-l-0 first:pl-0">
                  <dt className="text-[12.5px] text-ink-3">{label}</dt>
                  <dd
                    className={`figures mt-1.5 text-[20px] font-medium ${index === 3 ? 'text-warn' : ''}`}
                  >
                    {value}
                  </dd>
                </div>
              ))}
            </dl>
          </PanelSection>
          <PanelSection title="What to import">
            <p className="text-small text-ink-3">A second section, split by a hairline.</p>
          </PanelSection>
        </Panel>
      </Block>

      <Block title="Overlays">
        <Specimen name="ConfirmDialog, Drawer, Toast">
          <Button variant="danger" onClick={() => setConfirm(true)}>
            Delete the user sam
          </Button>
          <Button onClick={() => setDrawer(true)}>Open a drawer</Button>
          <Button onClick={() => toast('Settings saved.')}>Toast</Button>
          <Button onClick={() => toast('The last download failed.', { tone: 'danger' })}>
            Error toast
          </Button>
        </Specimen>
        <ConfirmDialog
          open={confirm}
          onClose={() => {
            setConfirm(false)
            setBusy(false)
          }}
          onConfirm={() => {
            setBusy(true)
            setTimeout(() => {
              setBusy(false)
              setConfirm(false)
              toast('sam was deleted.')
            }, 900)
          }}
          busy={busy}
          title="Delete the user sam?"
          confirmLabel="Delete"
        >
          Their devices are signed out and their own sources are removed. This cannot be undone.
        </ConfirmDialog>
        <Drawer
          open={drawer}
          onClose={() => setDrawer(false)}
          title="Edit the channel"
          footer={
            <>
              <Button variant="ghost" onClick={() => setDrawer(false)}>
                Cancel
              </Button>
              <Button variant="primary" onClick={() => setDrawer(false)}>
                Save
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-5 p-5">
            <Field label="Name">
              <TextInput defaultValue="Littoral 1" />
            </Field>
            <Field label="Number" help="Apps sort channels by it.">
              <NumberInput value={12} onValue={() => {}} />
            </Field>
          </div>
        </Drawer>
      </Block>

      <Block title="SaveBar">
        <form
          onSubmit={(event) => {
            event.preventDefault()
            setSaving(true)
            setTimeout(() => {
              setSaving(false)
              setDirty(false)
              toast(t.ui.allSaved)
            }, 900)
          }}
        >
          <Switch
            label="Dirty"
            checked={dirty}
            onChange={setDirty}
            stateText={['Changed', 'Saved']}
          />
          <SaveBar dirty={dirty} saving={saving} onDiscard={() => setDirty(false)} />
        </form>
      </Block>
    </PageLayout>
  )
}
