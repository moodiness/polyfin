import { useId, useState } from 'react'
import {
  BooksIcon,
  BroadcastIcon,
  CaretDownIcon,
  CaretUpIcon,
  CheckIcon,
  DatabaseIcon,
  GlobeIcon,
  KeyIcon,
  PuzzlePieceIcon,
  UsersIcon,
  type Icon,
} from '@phosphor-icons/react'
import { useQueries } from '@tanstack/react-query'
import {
  fetchAddons,
  fetchInvites,
  fetchLibraries,
  fetchSettings,
  fetchUsers,
  queryKeys,
  type Addon,
  type Health,
  type HealthProblem,
  type Invite,
  type Library,
  type Settings,
  type User,
} from '@/api'
import { settingsPath } from '@/app/navigation'
import { useHealth, useHealthProblems } from '@/features/system/problems'
import { useI18n } from '@/i18n'
import { Block, Button, cx, IconTile, Panel, Row, RowList, StatusPill } from '@/ui'

/** The steps of a good setup, in the order the card lists them. */
type StepId = 'metadata' | 'library' | 'users' | 'backups' | 'secretKey' | 'address' | 'discovery'

type Step = { id: StepId; done: boolean; to: string }

/** What the steps are read from: the server's state as Health and the admin pages tell it. */
type GetStartedState = {
  health: Health
  problems: HealthProblem[]
  settings: Settings
  addons: Addon[]
  libraries: Library[]
  users: User[]
  invites: Invite[]
}

/** The Health problems that tell HTTPS is not served: its port taken, its certificate unread. */
const httpsDown: HealthProblem['code'][] = ['https_port', 'https_files', 'https_renewal']

/** The Health warnings about local network discovery. */
const discoveryDown: HealthProblem['code'][] = ['discovery_port', 'discovery_address']

/**
 * Each step, done or not, from the signals Health reads:
 * - a metadata addon is an enabled Stremio addon of the server with the `meta` resource, as
 *   Health's warning about metadata languages counts them;
 * - backups are on while Health tells how they go (`backup` is null when they are off);
 * - POLYFIN_SECRET_KEY is set when Health's Stored keys say the keys are encrypted;
 * - HTTPS counts while it is on and Health finds no problem that stops it being served;
 * - discovery counts while it answers, with Address on the local network set, and Health finds
 *   no warning about it. Without that address, each answer gives the address the system routes
 *   from, which in a container on a bridge network apps cannot reach, and Health only finds out
 *   once an app has asked.
 */
function getStartedSteps(state: GetStartedState): Step[] {
  const { health, problems, settings } = state
  const httpsProblem = problems.find((problem) => httpsDown.includes(problem.code))
  const discoveryProblem = problems.find((problem) => discoveryDown.includes(problem.code))
  return [
    {
      id: 'metadata',
      done: state.addons.some(
        (addon) => addon.enabled && addon.kind === 'stremio' && addon.resources.includes('meta'),
      ),
      to: '/sources',
    },
    {
      id: 'library',
      done: state.libraries.some((library) => library.enabled),
      to: '/libraries',
    },
    {
      id: 'users',
      done: state.users.length > 1 || state.invites.some((invite) => invite.state === 'active'),
      to: '/users',
    },
    { id: 'backups', done: health.backup !== null, to: settingsPath('backups', 'backups') },
    { id: 'secretKey', done: health.secrets?.encrypted === true, to: '/system/health#secrets' },
    {
      id: 'address',
      done:
        settings.publicAddress !== '' ||
        (settings.httpsMode !== 'off' && httpsProblem === undefined),
      to: httpsProblem?.to ?? settingsPath('security', 'https'),
    },
    {
      id: 'discovery',
      done:
        settings.localDiscovery && settings.localAddress !== '' && discoveryProblem === undefined,
      to:
        discoveryProblem?.to ??
        settingsPath('general', settings.localDiscovery ? 'local-address' : 'local-discovery'),
    },
  ]
}

const icons: Record<StepId, Icon> = {
  metadata: PuzzlePieceIcon,
  library: BooksIcon,
  users: UsersIcon,
  backups: DatabaseIcon,
  secretKey: KeyIcon,
  address: GlobeIcon,
  discovery: BroadcastIcon,
}

/** Remembers in this browser that the card was dismissed. */
const dismissedKey = 'polyfin.getStarted.dismissed'

function readDismissed(): boolean {
  try {
    return window.localStorage.getItem(dismissedKey) === '1'
  } catch {
    return false
  }
}

/**
 * Home's Get started panel: how many steps of a good setup are done, the next one to do, and all
 * of them, each done or not and linking to its page, on demand. It shows once every signal is
 * read, and goes away once every step is done, or once dismissed.
 */
export function GetStarted() {
  const [dismissed, setDismissed] = useState(readDismissed)
  if (dismissed) return null
  return (
    <Steps
      onDismiss={() => {
        try {
          window.localStorage.setItem(dismissedKey, '1')
        } catch {
          // Without storage, the card goes away until the page is loaded again.
        }
        setDismissed(true)
      }}
    />
  )
}

function Steps({ onDismiss }: { onDismiss: () => void }) {
  const { t } = useI18n()
  const text = t.dashboard.getStarted
  const [all, setAll] = useState(false)
  const listId = useId()
  const health = useHealth()
  const problems = useHealthProblems()
  const [settings, addons, libraries, users, invites] = useQueries({
    queries: [
      { queryKey: queryKeys.settings, queryFn: ({ signal }) => fetchSettings(signal) },
      {
        queryKey: queryKeys.addons('shared'),
        queryFn: ({ signal }) => fetchAddons('shared', signal),
      },
      {
        queryKey: queryKeys.libraries('shared'),
        queryFn: ({ signal }) => fetchLibraries('shared', signal),
      },
      { queryKey: queryKeys.users, queryFn: ({ signal }) => fetchUsers(signal) },
      { queryKey: queryKeys.invites, queryFn: ({ signal }) => fetchInvites(signal) },
    ],
  })
  // Until every signal is read, nothing: a step is never told open while it is only unknown.
  if (
    health.data === undefined ||
    problems.data === undefined ||
    settings.data === undefined ||
    addons.data === undefined ||
    libraries.data === undefined ||
    users.data === undefined ||
    invites.data === undefined
  ) {
    return null
  }
  const steps = getStartedSteps({
    health: health.data,
    problems: problems.data,
    settings: settings.data,
    addons: addons.data,
    libraries: libraries.data,
    users: users.data,
    invites: invites.data,
  })
  const open = steps.filter((step) => !step.done)
  if (open.length === 0) return null
  const next = open[0]!
  return (
    <Block
      title={text.title}
      count={open.length}
      aside={
        <Button variant="ghost" size="sm" onClick={onDismiss}>
          {text.dismiss}
        </Button>
      }
    >
      <Panel as="div">
        <p className="text-h3 text-ink">
          {text.progress(steps.length - open.length, steps.length)}
        </p>
        {/* A count filling from the left, as the words above say it ("4 of 7 steps done"). */}
        <div aria-hidden="true" className="mt-3 grid grid-flow-col gap-1">
          {steps.map((step, index) => (
            <i
              key={step.id}
              className={cx(
                'h-1.5 rounded-full',
                index < steps.length - open.length ? 'bg-focus' : 'bg-s3',
              )}
            />
          ))}
        </div>
        {/* The next step is a tile of its own, inside the panel's padding. */}
        <RowList aria-label={text.next} className="mx-0! mt-3.5">
          <Row
            to={next.to}
            leading={<IconTile icon={icons[next.id]} />}
            title={
              <span className="block whitespace-normal">
                <small className="block text-micro font-normal text-ink-3">{text.next}</small>
                {text.steps[next.id].title}
              </span>
            }
            meta={<span className="min-w-0 whitespace-normal">{text.steps[next.id].help}</span>}
            className="bg-s2"
          />
        </RowList>
        <div className="mt-3">
          <Button
            variant="ghost"
            size="sm"
            icon={all ? CaretUpIcon : CaretDownIcon}
            aria-expanded={all}
            aria-controls={listId}
            onClick={() => setAll(!all)}
            className="-ml-3"
          >
            {all ? text.hideSteps : text.showSteps(steps.length)}
          </Button>
        </div>
        {all && (
          <div id={listId} className="mt-3">
            <RowList compact aria-label={text.title}>
              {steps.map((step) => (
                <Row
                  key={step.id}
                  to={step.to}
                  leading={<IconTile icon={step.done ? CheckIcon : icons[step.id]} size="sm" />}
                  // The steps' words wrap, rather than being cut, on a phone.
                  title={<span className="whitespace-normal">{text.steps[step.id].title}</span>}
                  muted={step.done}
                  trailing={
                    <StatusPill tone={step.done ? 'ok' : 'muted'}>
                      {step.done ? text.done : text.todo}
                    </StatusPill>
                  }
                />
              ))}
            </RowList>
          </div>
        )}
      </Panel>
    </Block>
  )
}
