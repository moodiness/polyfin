import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import {
  fetchAddonPreferences,
  fetchAddons,
  fetchStatus,
  queryClient,
  queryKeys,
  saveAddonPreferences,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  Field,
  InlineError,
  Notice,
  Panel,
  Row,
  RowList,
  SettingRow,
  SkeletonRows,
  StatusPill,
  TextInput,
  TextLink,
  useToast,
} from '@/ui'
import { MyPlayback, titleOf, useLiveSessions } from './NowPlaying'
import { RoomLight } from './RoomLight'

/** Jellyfin apps show 6 digits, Silo’s TV apps 8, as on the Quick Connect page. */
const shortLength = 6
const longLength = 8
/** How long 6 digits wait for more before they are looked up: they may begin 8. */
const typingPause = 700

/**
 * A member's Home: the greeting, their own playback as a wide art tile whose poster tints the top
 * of the page while they play, then Quick Connect on the left, and My sources and My account on
 * the right, with the server's status under them.
 */
export function MemberHome({ name }: { name: string }) {
  const { t } = useI18n()
  const text = t.dashboard.member
  const sessions = useLiveSessions()
  const [litId, setLitId] = useState<string | null>(null)
  const playing = sessions.data ?? []
  const lit = playing.find((session) => session.id === litId) ?? playing[0]
  const posters = [
    ...new Set(
      playing.flatMap((session) => (session.item?.posterId ? [session.item.posterId] : [])),
    ),
  ]
  return (
    <>
      <RoomLight posters={posters} lit={lit?.item?.posterId || null} />
      <PageLayout
        greeting
        title={t.dashboard.overview.greeting(new Date().getHours(), name)}
        lede={
          playing.length === 1 && lit && !lit.download
            ? text.watching(titleOf(lit, t), lit.device.name)
            : text.lede
        }
      >
        <MyPlayback onLight={setLitId} />
        <div className="grid items-start gap-14 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] max-xl:gap-12 max-md:gap-11">
          <QuickConnect />
          <div className="flex flex-col gap-16 max-md:gap-11">
            <MySources />
            <MyAccount />
          </div>
        </div>
      </PageLayout>
    </>
  )
}

/**
 * Quick Connect on Home: the code well and its three steps. Once the code is whole (8 digits, or
 * 6 that stay so for a moment) or Enter is pressed, the Quick Connect page opens with it, which
 * finds the device and approves it.
 */
function QuickConnect() {
  const { t } = useI18n()
  const text = t.quickConnect
  const navigate = useNavigate()
  const [code, setCode] = useState('')

  useEffect(() => {
    if (code.length === longLength) {
      void navigate(`/me/quick-connect?code=${code}`)
      return
    }
    if (code.length !== shortLength) return
    const timer = setTimeout(() => void navigate(`/me/quick-connect?code=${code}`), typingPause)
    return () => clearTimeout(timer)
  }, [code, navigate])

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (code.length === shortLength || code.length === longLength) {
      void navigate(`/me/quick-connect?code=${code}`)
    }
  }

  return (
    <Panel title={text.title} description={text.description} className="lg:px-1.5">
      <form onSubmit={submit} className="max-w-[360px]">
        <Field label={text.code} help={text.codeHint}>
          <TextInput
            value={code}
            onValue={(value) => setCode(value.replace(/\D/g, '').slice(0, longLength))}
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}([0-9]{2})?"
            maxLength={longLength}
            placeholder="000000"
            mono
            className="[&_input]:text-[26px] [&_input]:tracking-[0.32em] max-md:[&_input]:text-[22px]"
          />
        </Field>
      </form>
      <ol
        aria-label={text.stepsLabel}
        className="mt-8 grid grid-cols-3 gap-3 max-md:grid-cols-[minmax(0,1fr)]"
      >
        {text.steps.map((step, index) => (
          <li
            key={step}
            className="min-w-0 rounded-[22px] bg-s1 px-5 pt-5 pb-[22px] text-[17px] text-ink-2 max-md:flex max-md:items-start max-md:gap-3.5 max-md:py-4 max-md:text-[16px]"
          >
            <span
              aria-hidden="true"
              className="figures mb-3 grid size-[34px] shrink-0 place-items-center rounded-full bg-s2 text-[16px] font-bold text-ink max-md:mb-0"
            >
              {index + 1}
            </span>
            {step}
          </li>
        ))}
      </ol>
    </Panel>
  )
}

/** "Open", with the shelf's name for screen readers: "Open My sources". */
function OpenLink({ to, name }: { to: string; name: string }) {
  const { t } = useI18n()
  return (
    <TextLink to={to}>
      {t.dashboard.overview.open}
      <span className="sr-only"> {name}</span>
    </TextLink>
  )
}

/**
 * My sources on Home: whether the member uses the server's sources, saved at once as on My
 * sources, and their own sources, which open My sources.
 */
function MySources() {
  const { t } = useI18n()
  const text = t.dashboard.member
  const toast = useToast()
  const preferences = useQuery({
    queryKey: queryKeys.addonPreferences,
    queryFn: ({ signal }) => fetchAddonPreferences(signal),
  })
  const mine = useQuery({
    queryKey: queryKeys.addons('me'),
    queryFn: ({ signal }) => fetchAddons('me', signal),
  })
  const mutation = useMutation({
    mutationFn: saveAddonPreferences,
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.addonPreferences, saved)
      toast(t.mySources.preferenceSaved)
    },
  })

  let shared
  if (preferences.isPending)
    shared = (
      <div role="status" aria-label={t.common.loading}>
        <SkeletonRows rows={1} />
      </div>
    )
  else if (preferences.isError)
    shared = (
      <InlineError onRetry={() => void preferences.refetch()} retrying={preferences.isRefetching}>
        {errorMessage(t, preferences.error)}
      </InlineError>
    )
  // The server's sources give the ratings parental control hides titles by.
  else if (preferences.data.parentalControl) shared = <Notice>{t.mySources.parentalControl}</Notice>
  // The server or the member's own permission turned their own sources off.
  else if (!preferences.data.personalAddons)
    shared = <Notice tone="warn">{t.mySources.personalAddonsOff}</Notice>
  else
    shared = (
      <SettingRow
        kind="switch"
        label={t.mySources.useShared}
        help={t.mySources.useSharedHelp}
        checked={preferences.data.useSharedAddons}
        disabled={mutation.isPending}
        error={mutation.isError ? errorMessage(t, mutation.error) : undefined}
        onChange={(useSharedAddons) => mutation.mutate({ useSharedAddons })}
      />
    )

  return (
    <Block title={t.nav.mySources} aside={<OpenLink to="/me/sources" name={t.nav.mySources} />}>
      <div className="grid grid-cols-[minmax(0,1fr)] gap-2">
        {shared}
        <RowList aria-label={t.mySources.listTitle}>
          <Row
            to="/me/sources"
            title={t.mySources.listTitle}
            trailing={mine.data !== undefined ? text.sourceCount(mine.data.length) : undefined}
          />
        </RowList>
      </div>
    </Block>
  )
}

/** My account on Home: four of its sections as rows, and the server's status under them. */
function MyAccount() {
  const { t } = useI18n()
  const sections = t.account.sections
  const rows = [
    { anchor: 'metadata-language', title: sections.metadataLanguage },
    { anchor: 'web-theme', title: sections.webTheme },
    { anchor: 'devices', title: sections.devices },
    { anchor: 'password', title: sections.password },
  ]
  return (
    <Block title={t.nav.myAccount} aside={<OpenLink to="/me/account" name={t.nav.myAccount} />}>
      <RowList aria-label={t.nav.myAccount}>
        {rows.map((row) => (
          <Row key={row.anchor} to={`/me/account#${row.anchor}`} title={row.title} />
        ))}
      </RowList>
      <StatusLine />
    </Block>
  )
}

/** The server's status in one line, refreshed every 10 seconds: its database, its version. */
function StatusLine() {
  const { t } = useI18n()
  const status = useQuery({
    queryKey: queryKeys.status,
    queryFn: ({ signal }) => fetchStatus(signal),
    refetchInterval: 10_000,
  })
  if (status.data === undefined) {
    return status.isError ? (
      <InlineError
        className="mt-4"
        onRetry={() => void status.refetch()}
        retrying={status.isFetching}
      >
        {errorMessage(t, status.error)}
      </InlineError>
    ) : null
  }
  const ready = status.data.database === 'ready'
  return (
    <p className="figures mt-[18px] flex flex-wrap items-center gap-x-[22px] gap-y-1.5 px-1.5 text-[16.5px] text-ink-3">
      <span className="flex items-center gap-2">
        {t.status.title}
        {t.common.colon}
        <StatusPill tone={ready ? 'ok' : 'warn'}>
          {ready ? t.status.databaseReady : t.status.databaseUnavailable}
        </StatusPill>
      </span>
      <span>
        {t.status.version} {status.data.version}
      </span>
    </p>
  )
}
