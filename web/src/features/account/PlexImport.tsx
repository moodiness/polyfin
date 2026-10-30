import { ArrowSquareInIcon, ArrowSquareOutIcon, SignInIcon } from '@phosphor-icons/react'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useEffectEvent, useState, type ReactNode } from 'react'
import {
  choosePlexProfile,
  dropPlexSignIn,
  pollPlexSignIn,
  startOwnImport,
  startPlexSignIn,
  type JellyfinImportStatus,
  type PlexAccount,
  type PlexSignIn,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  Checkbox,
  ExternalButtonLink,
  Field,
  Notice,
  PanelFooter,
  Select,
  TextInput,
} from '@/ui'

/** How often the server is asked whether Plex signed the user in. */
const signInPollMs = 2000

/**
 * The user signs in to Plex on Plex's own page, which the server asks plex.tv about every few
 * seconds; then they choose the profile of their Plex Home, when they have one, and the server,
 * and import their own watch data from it, with their Plex Watchlist unless they untick it. The
 * tokens Plex gives stay on the server: this page never sees them.
 */
export function PlexImportForm({
  header,
  onStarted,
}: {
  /** The choice of the kind of server, shown first. */
  header: ReactNode
  onStarted: (started: JellyfinImportStatus) => void
}) {
  const { t } = useI18n()
  const text = t.account.plexImport
  const [link, setLink] = useState<PlexSignIn | null>(null)
  const [account, setAccount] = useState<PlexAccount | null>(null)
  const [failure, setFailure] = useState<unknown>(null)
  const [profile, setProfile] = useState('')
  const [pin, setPin] = useState('')
  const [server, setServer] = useState('')
  const [watchlist, setWatchlist] = useState(true)

  function show(read: PlexAccount) {
    setAccount(read)
    setProfile(read.profile)
    setPin('')
    setServer((read.servers.find((s) => s.preselected) ?? read.servers[0])?.id ?? '')
  }

  const signIn = useMutation({
    mutationFn: startPlexSignIn,
    onSuccess: (created) => {
      setFailure(null)
      setAccount(null)
      setLink(created)
    },
  })
  const poll = useEffectEvent(() => pollPlexSignIn())
  const received = useEffectEvent((read: PlexAccount) => {
    setLink(null)
    show(read)
  })

  useEffect(() => {
    if (link === null) return
    let stopped = false
    let timer = 0
    const next = () => {
      timer = window.setTimeout(() => void once(), signInPollMs)
    }
    const once = async () => {
      try {
        const polled = await poll()
        if (stopped) return
        if ('pending' in polled) next()
        else received(polled)
      } catch (error) {
        if (stopped) return
        setLink(null)
        setFailure(error)
      }
    }
    next()
    return () => {
      stopped = true
      window.clearTimeout(timer)
    }
  }, [link])

  const switchProfile = useMutation({
    mutationFn: () => choosePlexProfile(profile, pin),
    onSuccess: show,
  })
  const cancel = useMutation({
    mutationFn: dropPlexSignIn,
    onSettled: () => {
      setLink(null)
      setAccount(null)
      setFailure(null)
    },
  })
  const start = useMutation({
    mutationFn: () => startOwnImport({ kind: 'plex', server, watchlist }),
    onSuccess: onStarted,
  })

  const chosen = account?.profiles.find((p) => p.id === profile)
  const profileChanged = account !== null && profile !== account.profile
  const error = signIn.isError
    ? errorMessage(t, signIn.error)
    : failure !== null
      ? errorMessage(t, failure)
      : switchProfile.isError
        ? errorMessage(t, switchProfile.error)
        : start.isError
          ? errorMessage(t, start.error)
          : undefined

  return (
    <>
      <div className="flex max-w-lg flex-col gap-5 px-6 py-6 max-sm:px-4">
        {header}
        {account === null && link === null && <p className="text-small text-ink-3">{text.help}</p>}
        {link !== null && (
          <div className="flex flex-col gap-4 rounded-row border border-line bg-bg px-[18px] py-4 max-sm:p-3.5">
            <p className="text-control text-ink">{text.approve}</p>
            <ExternalButtonLink
              variant="primary"
              href={link.link}
              target="_blank"
              rel="noopener noreferrer"
              iconEnd={ArrowSquareOutIcon}
              className="self-start"
            >
              {text.open}
            </ExternalButtonLink>
            <p role="status" className="text-small text-ink-3">
              {text.waiting}
            </p>
          </div>
        )}
        {account !== null && (
          <>
            <p role="status" className="text-control text-ink">
              {text.signedInAs(account.name)}
            </p>
            {account.profiles.length > 0 && (
              <Field label={text.profile} help={text.profileHelp}>
                <Select
                  value={profile}
                  options={account.profiles.map((p) => ({
                    value: p.id,
                    label: p.managed ? text.managed(p.name) : p.name,
                  }))}
                  onValue={(value) => {
                    switchProfile.reset()
                    setPin('')
                    setProfile(value)
                  }}
                />
              </Field>
            )}
            {profileChanged && chosen?.protected && (
              <Field label={text.pin} help={text.pinHelp}>
                <TextInput
                  type="password"
                  inputMode="numeric"
                  revealable
                  value={pin}
                  onValue={setPin}
                  autoComplete="off"
                />
              </Field>
            )}
            {profileChanged ? (
              <Button
                className="self-start"
                loading={switchProfile.isPending}
                disabled={chosen?.protected === true && pin === ''}
                onClick={() => switchProfile.mutate()}
              >
                {switchProfile.isPending ? text.switching : text.useProfile}
              </Button>
            ) : account.servers.length === 0 ? (
              <Notice>{text.noServers}</Notice>
            ) : (
              <>
                <Field label={text.server} help={text.serverHelp}>
                  <Select
                    value={server}
                    options={account.servers.map((s) => ({
                      value: s.id,
                      label: s.owned ? text.owned(s.name) : s.name,
                    }))}
                    onValue={(value) => {
                      start.reset()
                      setServer(value)
                    }}
                  />
                </Field>
                <Checkbox
                  checked={watchlist}
                  onChange={setWatchlist}
                  label={text.watchlist}
                  help={text.watchlistHelp}
                />
              </>
            )}
          </>
        )}
        {error && (
          <Notice tone="danger" live>
            {error}
          </Notice>
        )}
      </div>
      <PanelFooter>
        {account === null && link === null ? (
          <Button
            variant="primary"
            icon={SignInIcon}
            loading={signIn.isPending}
            onClick={() => {
              setFailure(null)
              signIn.mutate()
            }}
          >
            {signIn.isPending ? text.signingIn : text.signIn}
          </Button>
        ) : (
          <>
            <Button loading={cancel.isPending} onClick={() => cancel.mutate()}>
              {text.cancel}
            </Button>
            {account !== null && (
              <Button
                variant="primary"
                icon={ArrowSquareInIcon}
                loading={start.isPending}
                disabled={profileChanged || server === ''}
                onClick={() => start.mutate()}
              >
                {start.isPending ? t.account.serverImport.starting : t.account.serverImport.start}
              </Button>
            )}
          </>
        )}
      </PanelFooter>
    </>
  )
}
