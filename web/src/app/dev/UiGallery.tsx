import {
  ArrowClockwiseIcon,
  ArrowRightIcon,
  BroadcastIcon,
  ChatTextIcon,
  DeviceMobileIcon,
  DotsThreeIcon,
  FolderIcon,
  InfoIcon,
  MagnifyingGlassIcon,
  MusicNotesIcon,
  PencilSimpleIcon,
  PlusIcon,
  PuzzlePieceIcon,
  StopIcon,
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
  AlertTile,
  ArtShelf,
  ArtTile,
  Avatar,
  Badge,
  Block,
  Button,
  ButtonLink,
  Checkbox,
  ConfirmDialog,
  Count,
  DockSlotContext,
  EmptyState,
  Field,
  FigureTile,
  IconButton,
  IconTile,
  InlineError,
  Kbd,
  Menu,
  MenuItem,
  MenuSeparator,
  Modal,
  modKey,
  MoveButtons,
  Notice,
  NumberInput,
  PageHeader,
  Panel,
  PanelFooter,
  PanelSection,
  ProgressBar,
  RelativeTime,
  Row,
  RowList,
  SaveBar,
  SecretField,
  SectionNav,
  Segmented,
  Select,
  SettingRow,
  Skeleton,
  SkeletonRows,
  SkeletonText,
  StatusPill,
  SuggestInput,
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
      <p className="text-micro text-ink-3">{name}</p>
      <div className="flex flex-wrap items-start gap-3">{children}</div>
    </div>
  )
}

const swatches = [
  ['bg', 'bg-bg'],
  ['room', 'bg-room'],
  ['s1', 'bg-s1'],
  ['s1-solid', 'bg-s1-solid'],
  ['s2', 'bg-s2'],
  ['s3', 'bg-s3'],
  ['s4', 'bg-s4'],
  ['raised', 'bg-raised'],
  ['focus', 'bg-focus'],
  ['on-focus', 'bg-on-focus'],
  ['on-focus-2', 'bg-on-focus-2'],
  ['accent', 'bg-accent'],
  ['accent-hover', 'bg-accent-hover'],
  ['link', 'bg-link'],
  ['link-lit', 'bg-link-lit'],
  ['ok', 'bg-ok'],
  ['warn', 'bg-warn'],
  ['danger', 'bg-danger'],
] as const

/** Forty categories, as a large IPTV line-up has: enough for the Select's search. */
const galleryGroups = [
  'Actualités',
  'Animation',
  'Arts',
  'Belgique',
  'Cinéma',
  'Comédie',
  'Cuisine',
  'Découverte',
  'Documentaires',
  'Éducation',
  'Enfants',
  'Espagne',
  'Été',
  'Football',
  'Généraliste',
  'Histoire',
  'Info locale',
  'Italie',
  'Jeunesse',
  'Kiosque',
  'Langues',
  'Maison',
  'Météo',
  'Musique',
  'Nature',
  'Opéra',
  'Outre-mer',
  'Policier',
  'Portugal',
  'Québec',
  'Radio',
  'Régions',
  'Science',
  'Séries',
  'Sport',
  'Suisse',
  'Téléréalité',
  'Théâtre',
  'Voyage',
  'Zen',
]

const galleryGenres = [
  'Action',
  'Animation',
  'Comédie',
  'Drame',
  'Horror',
  'Science-Fiction',
  'Thriller',
]

/** A made-up poster, drawn here so the gallery needs no file. */
function poster(top: string, bottom: string, word: string) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 560"><defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="${top}"/><stop offset="1" stop-color="${bottom}"/></linearGradient></defs><rect width="400" height="560" fill="url(#g)"/><circle cx="290" cy="150" r="56" fill="#fff" fill-opacity=".75"/><path d="M0 420 L120 330 L230 400 L400 300 L400 560 L0 560Z" fill="#000" fill-opacity=".35"/><text x="36" y="500" font-family="sans-serif" font-size="58" font-weight="700" fill="#fff">${word}</text></svg>`
  return `data:image/svg+xml,${encodeURIComponent(svg)}`
}

const amberRoad = poster('#e6a446', '#4a2412', 'AMBER')
const nightTrain = poster('#2d6f7a', '#0d1d2a', 'NIGHT')
/** A square channel logo, for the uncropped art of an ArtTile. */
const channelLogo = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200"><rect width="200" height="200" rx="40" fill="#2f6fd6"/><text x="100" y="128" text-anchor="middle" font-family="sans-serif" font-size="80" font-weight="700" fill="#fff">TV1</text></svg>')}`

/** The settings of the SettingRow demo, as saved. */
const savedSettings = {
  conversion: true,
  conversions: 0,
  quality: 'original',
  category: 'group-5',
  timeout: 90,
  folder: '/data/cache/conversion',
  name: 'Polyfin',
  grouping: 'provider',
  cacheSize: null as number | null,
}

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
  const [group, setGroup] = useState('group-1')
  const [genre, setGenre] = useState('')
  const [genres, setGenres] = useState(['Horror'])
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
  const [modal, setModal] = useState(false)
  const [saving, setSaving] = useState(false)
  const [order, setOrder] = useState(['Popular', 'Trending', 'New releases'])
  const [saved, setSaved] = useState(savedSettings)
  const [form, setForm] = useState({ ...savedSettings, conversions: 2 })
  const [token, setToken] = useState<string | undefined>(undefined)
  const [access, setAccess] = useState(true)
  const [dock, setDock] = useState<HTMLElement | null>(null)
  const [times] = useState(() => {
    const now = Date.now()
    return {
      past: new Date(now - 18 * 60_000).toISOString(),
      future: new Date(now + 9 * 3_600_000).toISOString(),
    }
  })

  const update = (part: Partial<typeof form>) => setForm({ ...form, ...part })
  const changes =
    (Object.keys(saved) as (keyof typeof saved)[]).filter((key) => form[key] !== saved[key])
      .length + (token === undefined ? 0 : 1)

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
              <span className={`h-14 rounded-control ${className}`} />
              <span className="text-micro text-ink-3">{name}</span>
            </div>
          ))}
        </div>
        <div className="mt-8 flex flex-col gap-3">
          <p className="text-display">Display 56</p>
          <p className="text-h1">Page title 48</p>
          <p className="text-h2">Shelf title 27</p>
          <p className="text-group">Group title 25</p>
          <p className="text-area">Area words 24</p>
          <p className="text-h3">Row label 21</p>
          <p className="text-lead text-ink-2">Lead 20: what a page is for, and values.</p>
          <p className="text-body">
            Body 19 in ink, <span className="text-ink-2">ink-2</span>,{' '}
            <span className="text-ink-3">ink-3</span>.
          </p>
          <p className="text-control">Control 18, for buttons and fields.</p>
          <p className="text-small text-ink-2">Small 17, for help under a row.</p>
          <p className="figures text-micro text-ink-2">
            figures 15.5: 21:42 · 192.168.1.31 · 0.15.0 · 1 373
          </p>
        </div>
        <div className="mt-8 flex flex-wrap gap-3">
          <div className="rounded-row bg-s1 px-7 py-5 text-h3">A row</div>
          <div className="lit lift rounded-row px-7 py-5 text-h3 [--lift:1.012]">
            Lit <span className="text-lead text-ink-2">value</span>{' '}
            <span className="text-lead text-ok">Ready</span>
          </div>
          <div className="raised rounded-panel px-7 py-5 text-h3">Raised glass</div>
        </div>
      </Block>

      <Block title="Page header">
        <div className="flex flex-col gap-10">
          <Specimen name="PageHeader: title, lede, actions, sub-area words (nav)">
            <PageHeader
              className="w-full"
              title="Sources"
              lede="What Polyfin imports, and from where."
              actions={<Button icon={PlusIcon}>Add a source</Button>}
              nav={
                <Tabs
                  label="Content pages"
                  items={[
                    { id: 'sources', label: 'Sources', to: '/sources' },
                    { id: 'libraries', label: 'Libraries', to: '/libraries' },
                    { id: 'ui', label: 'This page', to: '/dev/ui' },
                  ]}
                />
              }
            />
          </Specimen>
          <Specimen name="PageHeader: greeting">
            <PageHeader
              greeting
              className="w-full"
              title="Good afternoon, Morgan"
              lede="2 playbacks under way, and 2 things to look at."
            />
          </Specimen>
        </div>
      </Block>

      <Block title="Buttons">
        <div className="flex flex-col gap-5">
          <Specimen name="variants (Tab to see them lit)">
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
          <Field label="Category" help="More than 12 options: the list starts with a search.">
            <Select
              value={group}
              onValue={setGroup}
              options={galleryGroups.map((name, index) => ({
                value: `group-${index + 1}`,
                label: name,
              }))}
            />
          </Field>
          <Field label="Quality group" error="Choose a group the user may play.">
            <Select
              value={quality}
              onValue={setQuality}
              options={[
                { value: '4k', label: '4K' },
                { value: '1080p', label: '1080p' },
              ]}
            />
          </Field>
          <Field label="Compact well" help="44 px, for dense toolbars and tables.">
            <TextInput
              size="compact"
              placeholder="Filter the channels"
              icon={MagnifyingGlassIcon}
            />
          </Field>
          <Field label="Compact select">
            <Select
              size="compact"
              value={quality}
              onValue={setQuality}
              options={[
                { value: '4k', label: '4K' },
                { value: '1080p', label: '1080p' },
              ]}
            />
          </Field>
          <Field label="Disabled select">
            <Select
              value="1080p"
              onValue={() => {}}
              disabled
              options={[{ value: '1080p', label: '1080p' }]}
            />
          </Field>
          <Field
            label="Genre"
            help={`Suggestions under the field; Enter adds what is typed. Blocked: ${genres.join(', ')}.`}
          >
            <SuggestInput
              value={genre}
              onValue={setGenre}
              suggestions={galleryGenres.filter((name) => !genres.includes(name))}
              onPick={(name) => {
                setGenres([...genres, name])
                setGenre('')
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && genre.trim() !== '') {
                  event.preventDefault()
                  setGenres([...genres, genre.trim()])
                  setGenre('')
                }
              }}
            />
          </Field>
          <div className="flex flex-col gap-2">
            <span className="text-control font-medium text-ink">In a scroll box</span>
            <div className="h-36 overflow-auto rounded-row bg-s1 p-4">
              <p className="mb-3 text-small text-ink-3">The list opens whole, over the box.</p>
              <Select
                aria-label="Quality in a scroll box"
                value={quality}
                onValue={setQuality}
                options={[
                  { value: '4k', label: '4K' },
                  { value: '1080p', label: '1080p' },
                  { value: '720p', label: '720p' },
                  { value: '480p', label: '480p' },
                ]}
              />
              <div className="h-24" />
            </div>
          </div>
          <Field label="Custom CSS" help="Applied to the web player.">
            <Textarea mono rows={3} defaultValue={'.skinHeader { opacity: .9 }'} />
          </Field>
        </div>
      </Block>

      <Block title="Choices">
        <div className="flex flex-col gap-5">
          <Specimen name="Switch: on, off, disabled, own words, no word">
            <Switch label="On" checked={on} onChange={setOn} />
            <Switch label="Off" checked={off} onChange={setOff} />
            <Switch label="Disabled" checked disabled onChange={() => {}} />
            <Switch
              label="TheIntroDB"
              checked={on}
              onChange={setOn}
              stateText={['Active', 'Off']}
            />
            <Switch label="Similar titles" checked={off} onChange={setOff} stateText={false} />
          </Specimen>
          <Specimen name="Checkbox: checked, unchecked, indeterminate, disabled, with help and aside">
            <div className="grid w-full max-w-[40rem] grid-cols-2 gap-x-4 gap-y-3 max-md:grid-cols-1">
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
          <Specimen name="Segmented: md, sm, compact">
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
            <Segmented
              label="Language, compact"
              size="compact"
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

      <Block title="Setting rows and the unsaved dock">
        <p className="mb-6 max-w-[62ch] text-small text-ink-2">
          Tab or click a row to light it and read its help; Enter opens its picker, the arrows
          choose, Esc closes. ↑ ↓ move between rows. Changed rows keep a dot and “was …”; the dock
          under the section list counts them. Access below saves at once, with no mark.
        </p>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            setSaving(true)
            setTimeout(() => {
              setSaving(false)
              setSaved(form)
              setToken(undefined)
              toast(t.ui.allSaved)
            }, 900)
          }}
          className="grid items-start gap-12 md:grid-cols-[18.75rem_minmax(0,1fr)] max-lg:md:grid-cols-[14.5rem_minmax(0,1fr)] max-md:block"
        >
          <DockSlotContext value={dock}>
            <div className="flex flex-col self-stretch max-md:contents">
              <SectionNav
                label="Settings sections"
                search={{ value: search, onChange: setSearch, placeholder: 'Search a setting' }}
                items={settingsSections.slice(0, 6).map((section) => ({
                  id: section.id,
                  label: t.nav.settingsSections[section.key],
                  icon: section.icon,
                  to: `#${section.id}`,
                }))}
                current="conversion"
              />
              <div ref={setDock} className="sticky bottom-7 mt-auto pt-10 max-md:hidden" />
            </div>
            <div className="min-w-0">
              <h3 className="mb-4 text-group">General</h3>
              <div className="grid grid-cols-[minmax(0,1fr)] gap-2">
                <SettingRow
                  kind="switch"
                  anchor="gallery-conversion"
                  label="Conversion (transcoding)"
                  help="When an app cannot play a file as it is, Polyfin converts it with FFmpeg."
                  checked={form.conversion}
                  saved={saved.conversion}
                  onChange={(conversion) => update({ conversion })}
                />
                <SettingRow
                  kind="number"
                  label="Video conversions at once (0 = no limit)"
                  help="Converting video is the heaviest work the server does. Once this many playbacks have their video converted, a new playback gets a version that needs no conversion. From 0 to 32; 0 by default."
                  value={form.conversions}
                  saved={saved.conversions}
                  min={0}
                  max={32}
                  format={(value) => (value === 0 ? 'No limit' : String(value))}
                  onChange={(conversions) => update({ conversions: conversions ?? 0 })}
                />
                <SettingRow
                  kind="choice"
                  label="Maximum quality of converted video"
                  help="Above it, converted video is scaled down."
                  value={form.quality}
                  saved={saved.quality}
                  options={[
                    { value: 'original', label: 'Original' },
                    { value: '2160', label: '4K' },
                    { value: '1080', label: '1080p' },
                    { value: '720', label: '720p' },
                    { value: '480', label: '480p' },
                  ]}
                  onChange={(quality) => update({ quality })}
                />
                <SettingRow
                  kind="choice"
                  label="Categories"
                  help="How the channels are grouped in apps. A long value wraps before the label does."
                  value={form.grouping}
                  saved={saved.grouping}
                  options={[
                    { value: 'provider', label: 'As the provider groups them' },
                    { value: 'country', label: 'By country' },
                    { value: 'none', label: 'In one list' },
                  ]}
                  onChange={(grouping) => update({ grouping })}
                />
                <SettingRow
                  kind="number"
                  label="Cache size"
                  detail="Saved empty: a change says “was Not set”"
                  help="From 1 to 500 GB. Empty keeps no limit."
                  value={form.cacheSize}
                  saved={saved.cacheSize}
                  min={1}
                  max={500}
                  unit="GB"
                  onChange={(cacheSize) => update({ cacheSize })}
                />
                <SettingRow
                  kind="choice"
                  label="Default category"
                  detail="More than 12 choices: a list with a search"
                  help="Channels without a category go there."
                  value={form.category}
                  saved={saved.category}
                  options={galleryGroups.map((name, index) => ({
                    value: `group-${index + 1}`,
                    label: name,
                  }))}
                  onChange={(category) => update({ category })}
                />
                <SettingRow
                  kind="number"
                  label="Analysis timeout"
                  help="From 10 to 3600 seconds. 90 by default."
                  value={form.timeout}
                  saved={saved.timeout}
                  min={10}
                  max={3600}
                  unit="s"
                  error={
                    form.timeout === null || form.timeout < 10 || form.timeout > 3600
                      ? 'From 10 to 3600 seconds.'
                      : undefined
                  }
                  onChange={(timeout) => update({ timeout: timeout ?? 0 })}
                />
                <SettingRow
                  kind="path"
                  label="Conversion folder"
                  help="Where converted pieces wait for the app. Needs a few gigabytes free."
                  value={form.folder}
                  saved={saved.folder}
                  onChange={(folder) => update({ folder })}
                />
                <SettingRow
                  kind="text"
                  label="Server name"
                  help="Shown in apps and on the sign-in page."
                  value={form.name}
                  saved={saved.name}
                  maxLength={64}
                  onChange={(name) => update({ name })}
                />
                <SettingRow
                  kind="secret"
                  label="Update tool token"
                  help="Sent to the update tool with each request."
                  saved
                  value={token}
                  onValue={setToken}
                  reveal={async () => 'upd_7f3a91c2e04b6d58'}
                />
                <SettingRow
                  kind="switch"
                  label="Hardware decoding"
                  detail="Disabled: no graphics card found"
                  help="Uses the graphics card to read video."
                  checked={false}
                  disabled
                  onChange={() => {}}
                />
              </div>
              <h3 className="mt-[3.25rem] mb-4 text-group">Access</h3>
              <div className="grid grid-cols-[minmax(0,1fr)] gap-2">
                <SettingRow
                  kind="switch"
                  label="May use Live TV"
                  help="Applies at once."
                  checked={access}
                  onChange={setAccess}
                  stateText={['Allowed', 'Not allowed']}
                >
                  A switch with no saved value saves at once and keeps no mark.
                </SettingRow>
              </div>
              <SaveBar
                dirty={changes > 0}
                saving={saving}
                changes={changes}
                onDiscard={() => {
                  setForm(saved)
                  setToken(undefined)
                }}
              />
            </div>
          </DockSlotContext>
        </form>
      </Block>

      <Block title="SecretField">
        <div className="flex max-w-[40rem] flex-col gap-8">
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

      <Block title="Tiles">
        <div className="flex flex-col gap-10">
          <Specimen name="ArtShelf and ArtTile: the lit tile opens wide, with a halo and its tools">
            <ArtShelf label="Playing now" className="w-full">
              <ArtTile
                poster={amberRoad}
                title="Amber Road"
                top={
                  <>
                    <Avatar name="Robin" size="sm" />
                    <span>
                      <b className="font-semibold text-ink">Robin</b> · Robin’s phone
                    </span>
                  </>
                }
                meta="Movie · 2023"
                tools={
                  <>
                    <IconButton label="Details" icon={InfoIcon} size="sm" />
                    <IconButton label="Send a message to Robin" icon={ChatTextIcon} size="sm" />
                    <IconButton label="Stop Robin’s playback" icon={StopIcon} size="sm" />
                  </>
                }
                detail="Polyfin repackages the file for the app, without re-encoding it."
                footer={
                  <ProgressBar
                    value={0.15}
                    variant="playback"
                    label="Amber Road"
                    valueText="15:37 of 1:44:00"
                  />
                }
              >
                <strong className="block text-[1.3125rem] font-bold text-ink">Remux</strong>
                Jellyfin iOS 10.11.0
              </ArtTile>
              <ArtTile
                poster={nightTrain}
                title="Night Train"
                top={
                  <>
                    <Avatar name="Sam" size="sm" />
                    <span>
                      <b className="font-semibold text-ink">Sam</b> · Living room TV
                    </span>
                  </>
                }
                meta="Movie · 2021"
                tools={<IconButton label="Stop Sam’s playback" icon={StopIcon} size="sm" />}
                detail="The app reads the file as it is."
                footer={
                  <ProgressBar
                    value={0.4}
                    variant="playback"
                    label="Night Train"
                    valueText="44:38 of 1:52:00"
                  />
                }
              >
                <strong className="block text-[1.3125rem] font-bold text-ink">Direct play</strong>
                Jellyfin Android TV 10.11.0
              </ArtTile>
            </ArtShelf>
          </Specimen>
          <Specimen name="ArtTile: fit contain (a channel logo), and the fallback of a poster that fails">
            <ArtShelf label="Live TV now" className="w-full">
              <ArtTile
                poster={channelLogo}
                fit="contain"
                title="Summit"
                meta="Channel 1 · Nature"
                footer={<ProgressBar value={0.5} variant="playback" label="Trail Notes" />}
              >
                Trail Notes, until 1:00 PM
              </ArtTile>
              <ArtTile
                poster="/admin/missing-poster.jpg"
                fallback="D"
                title="Deep Blue"
                meta="Channel 2 · Nature"
              >
                Ocean Life, until 12:30 PM
              </ArtTile>
            </ArtShelf>
          </Specimen>
          <Specimen name="AlertTile: danger, warn, info">
            <div className="grid w-full gap-2.5">
              <AlertTile
                tone="danger"
                title="Demo TV: the last programme guide fetch failed."
                cause="The guide’s address answered 404 three times."
                fix="Check the guide’s address in the source, or try again later."
                meanwhile="The channels keep the guide they have until tomorrow, 6:00 AM."
                actions={
                  <>
                    <Button icon={ArrowClockwiseIcon}>Try again</Button>
                    <ButtonLink to="/sources" variant="ghost" iconEnd={ArrowRightIcon}>
                      Open the source
                    </ButtonLink>
                  </>
                }
              />
              <AlertTile
                tone="warn"
                title="Little space left for the conversion folder: 3.8 GB free."
                actions={<Button icon={FolderIcon}>Choose another folder</Button>}
              />
              <AlertTile tone="info" title="A new version is ready: 0.62.0.">
                It restarts the server for a few seconds.
              </AlertTile>
            </div>
          </Specimen>
          <Specimen name="FigureTile: of N, unit, tone; and inside a Panel">
            <div className="grid w-full grid-cols-4 gap-2.5 max-md:grid-cols-2">
              <FigureTile value={4} total={4} label="Categories on" />
              <FigureTile value={5} total={5} label="Channels shown" />
              <FigureTile value={12} unit="Mbps" label="Sent to apps" />
              <FigureTile value={3} tone="warn" label="Without a guide" />
            </div>
            <Panel title="Bandwidth" className="w-full max-w-[35rem]">
              <div className="grid grid-cols-2 gap-6">
                <FigureTile value={12} unit="Mbps" label="Sent to apps" />
                <FigureTile value={54} unit="Mbps" label="Received from sources" />
              </div>
            </Panel>
          </Specimen>
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
              Add a Stremio addon, an Eclipse addon or an IPTV account: their catalogs become
              libraries.
            </EmptyState>
          </Specimen>
          <Specimen name="Skeleton, SkeletonText, SkeletonRows">
            <div className="flex w-full flex-col gap-4">
              <Skeleton className="h-6 w-48" />
              <SkeletonText lines={3} className="max-w-[33.25rem]" />
              <SkeletonRows rows={2} />
            </div>
          </Specimen>
        </div>
      </Block>

      <Block title="Progress, avatars, keys, tooltip">
        <div className="flex flex-col gap-5">
          <Specimen name="ProgressBar: playback, accent, neutral, warn, sizes">
            <div className="flex w-full max-w-[32.5rem] flex-col gap-4">
              <ProgressBar
                value={0.3677}
                variant="playback"
                label="Dune"
                valueText="57 minutes of 2 hours 35"
              />
              <ProgressBar value={0.6} label="Task" />
              <ProgressBar value={0.32} variant="neutral" size="sm" label="Cache" />
              <ProgressBar value={0.92} variant="warn" size="lg" label="Disk" />
            </div>
          </Specimen>
          <Specimen name="Avatar (sm, md, lg, xl), IconTile (sizes, tones), Kbd, Tooltip">
            <Avatar name="Sam" size="sm" />
            <Avatar name="Julien" />
            <Avatar name="Lucas" size="lg" />
            <Avatar name="Morgan" size="xl" />
            <IconTile icon={BroadcastIcon} size="lg" />
            <IconTile icon={BroadcastIcon} />
            <IconTile letters="Si" />
            <IconTile icon={DeviceMobileIcon} size="sm" />
            <IconTile icon={TelevisionIcon} tone="ok" />
            <IconTile icon={TelevisionIcon} tone="warn" />
            <IconTile icon={TelevisionIcon} tone="danger" />
            <Kbd>{modKey === '⌘' ? '⌘K' : 'Ctrl K'}</Kbd>
            <Kbd>↵</Kbd>
            <Tooltip content="Opens the Jellyfin web app in a new tab.">
              <Button variant="ghost">Hover or focus me</Button>
            </Tooltip>
          </Specimen>
        </div>
      </Block>

      <Block title="Rows" count={3} aside={<TextLink to="/system/logs">Open the log</TextLink>}>
        <div className="grid grid-cols-[minmax(0,26.25rem)_minmax(0,1fr)] gap-8 max-lg:grid-cols-1">
          <Specimen name="RowList separate: buttons, the open one lit (aria-current)">
            <RowList variant="separate" aria-label="Sources" className="w-full">
              <Row
                leading={PuzzlePieceIcon}
                title="Metadata"
                titleAside={<Badge>Stremio</Badge>}
                meta={
                  <>
                    <StatusPill tone="ok">Active</StatusPill>
                    <span>18 min ago</span>
                  </>
                }
                onClick={() => setSelected('metadata')}
                selected={selected === 'metadata'}
              />
              <Row
                leading={MusicNotesIcon}
                title="Music"
                titleAside={<Badge>Eclipse</Badge>}
                meta={<StatusPill tone="ok">Active</StatusPill>}
                onClick={() => setSelected('flux')}
                selected={selected === 'flux'}
              />
              <Row
                leading={BroadcastIcon}
                title="My IPTV provider"
                meta={<StatusPill tone="danger">Guide failed</StatusPill>}
                onClick={() => setSelected('iptv')}
                selected={selected === 'iptv'}
              />
            </RowList>
          </Specimen>
          <Specimen name="RowList boxed: links with a value and chevron, static rows, muted">
            <RowList aria-label="Server state" className="w-full">
              <Row
                title="Graphics card"
                trailing={
                  <>
                    <span>VideoToolbox</span>
                    <span className="text-micro text-ink-3">H.264, HEVC, tone mapping</span>
                  </>
                }
                to="/system/health"
              />
              <Row
                title="Database"
                trailing={
                  <span>
                    <span className="font-semibold text-ok">Ready</span>, 16 MB
                  </span>
                }
                to="/system/health"
              />
              <Row
                leading={TelevisionIcon}
                title="Living room Apple TV"
                meta="Infuse 8.1"
                trailing={<Button variant="ghost">Disconnect</Button>}
              />
              <Row leading={UserCircleIcon} title="Muted row" meta="Not configured" muted />
            </RowList>
          </Specimen>
        </div>
        <div className="mt-8 grid grid-cols-2 gap-8 max-lg:grid-cols-1">
          <Specimen name="RowList plain: a feed">
            <RowList variant="plain" aria-label="Activity" className="w-full">
              <Row
                leading={<span className="figures w-20 text-micro text-ink-3">12:07 PM</span>}
                title="Morgan signed in"
                to="/system/logs"
              />
              <Row
                leading={<span className="figures w-20 text-micro text-ink-3">12:05 PM</span>}
                title="Robin is playing Amber Road on Robin’s phone"
                to="/system/logs"
              />
            </RowList>
          </Specimen>
          <Specimen name="RowList compact: 56 px rows for long lists">
            <RowList compact aria-label="Channels" className="w-full">
              {['Summit', 'Deep Blue', 'Arena', 'Cinema One'].map((name, index) => (
                <Row
                  key={name}
                  leading={<span className="figures w-7 text-micro text-ink-3">0{index + 1}</span>}
                  title={name}
                  trailing="On now"
                  to="/live-tv"
                />
              ))}
            </RowList>
          </Specimen>
        </div>
      </Block>

      <Block title="Order and time">
        <div className="grid grid-cols-2 gap-6 max-lg:grid-cols-1">
          <Specimen name="MoveButtons (focus follows the item)">
            <RowList aria-label="Catalogs" className="w-full">
              {order.map((name, index) => (
                <Row
                  key={name}
                  title={name}
                  trailing={
                    <MoveButtons
                      name={name}
                      index={index}
                      count={order.length}
                      onMove={(to) =>
                        setOrder((old) => {
                          const next = old.filter((item) => item !== name)
                          next.splice(to, 0, name)
                          return next
                        })
                      }
                    />
                  }
                />
              ))}
            </RowList>
          </Specimen>
          <Specimen name="RelativeTime (full date on hover)">
            <span className="text-small text-ink-2">
              Updated <RelativeTime iso={times.past} />
            </span>
            <RelativeTime iso={times.future} className="text-small text-ink-3" />
          </Specimen>
        </div>
      </Block>

      <Block title="Table">
        <div className="flex flex-col gap-8">
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
                <td>Jellyseerr</td>
                <td>2 October</td>
                <td className="figures text-right">1 204</td>
                <td>
                  <IconButton label="Delete Jellyseerr" icon={TrashIcon} size="sm" danger />
                </td>
              </tr>
              <tr>
                <td>Home Assistant</td>
                <td>14 September</td>
                <td className="figures text-right">38</td>
                <td>
                  <IconButton label="Delete Home Assistant" icon={TrashIcon} size="sm" danger />
                </td>
              </tr>
            </tbody>
          </Table>
          <Table label="Log" compact caption="compact: 56 px rows">
            <thead>
              <tr>
                <th scope="col">Event</th>
                <th scope="col" className="text-right">
                  Time
                </th>
              </tr>
            </thead>
            <tbody>
              {['Morgan signed in', 'Demo TV refreshed', 'Backup done'].map((event, index) => (
                <tr key={event}>
                  <td>{event}</td>
                  <td className="figures text-right">12:0{7 - index} PM</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </div>
      </Block>

      <Block title="Navigation">
        <div className="flex flex-col gap-6">
          <Specimen name="Tabs: words, links (the sub-area words)">
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
                { id: 'options', label: 'Import options' },
                { id: 'categories', label: 'Categories', count: 12 },
                { id: 'channels', label: 'Channels', count: 1691 },
              ]}
            />
          </Specimen>
          <Specimen name="SectionNav: see the setting rows above (a strip below 768 px)">
            <p className="text-small text-ink-2">
              The section list with its search sits beside the setting rows, the dock under it.
            </p>
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
                  <IconButton {...props} label="More actions" icon={DotsThreeIcon} />
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
            <div className="grid grid-cols-4 gap-6 max-md:grid-cols-2">
              <FigureTile value="54 180" label="Entries" />
              <FigureTile value="1 691" label="Channels kept" />
              <FigureTile value="318" label="With a guide" />
              <FigureTile value="1 373" tone="warn" label="Without" />
            </div>
          </PanelSection>
          <PanelSection title="What to import">
            <RowList aria-label="What to import">
              <Row title="Live TV" trailing="1 691 channels" />
              <Row title="Movies" trailing="Off" />
            </RowList>
          </PanelSection>
        </Panel>
      </Block>

      <Block title="Overlays">
        <Specimen name="ConfirmDialog, Modal, Toast">
          <Button variant="danger" onClick={() => setConfirm(true)}>
            Delete the user sam
          </Button>
          <Button onClick={() => setModal(true)}>Open a modal</Button>
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
        <Modal
          open={modal}
          onClose={() => setModal(false)}
          title="Edit the channel"
          footer={
            <>
              <Button variant="ghost" onClick={() => setModal(false)}>
                Cancel
              </Button>
              <Button variant="primary" onClick={() => setModal(false)}>
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
            <Field label="Category">
              <Select
                value={group}
                onValue={setGroup}
                options={galleryGroups.map((name, index) => ({
                  value: `group-${index + 1}`,
                  label: name,
                }))}
              />
            </Field>
          </div>
        </Modal>
      </Block>
    </PageLayout>
  )
}
