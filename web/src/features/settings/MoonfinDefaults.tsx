import { ArrowsClockwiseIcon, CopyIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useMemo, useState } from 'react'
import {
  applyMoonfinDefaults,
  copyMoonfinDefaults,
  fetchMoonfinDefaults,
  moonfinProfiles,
  queryClient,
  queryKeys,
  saveMoonfinDefaults,
  type MoonfinDefaults as Defaults,
  type MoonfinProfile,
  type MoonfinSettings,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Button,
  ConfirmDialog,
  Field,
  FieldError,
  InlineError,
  Modal,
  Segmented,
  Select,
  SkeletonRows,
  Tabs,
  useToast,
} from '@/ui'
import CodeEditor from './CodeEditor'
import type { SideEdits } from './SectionForm'

/** The largest defaults of a profile the server takes, in bytes of JSON. */
const maxBytes = 256 * 1024

/** A profile's settings as the editor shows them. */
function jsonText(settings: MoonfinSettings | undefined): string {
  return settings === undefined || Object.keys(settings).length === 0
    ? '{}'
    : JSON.stringify(settings, null, 2)
}

/** The text of the editor as settings, or null when it is not a JSON object. */
function parseSettings(text: string): MoonfinSettings | null {
  try {
    const value: unknown = JSON.parse(text)
    return typeof value === 'object' && value !== null && !Array.isArray(value)
      ? (value as MoonfinSettings)
      : null
  } catch {
    return null
  }
}

/** The number of settings a profile sets. */
const settingCount = (defaults: Defaults, profile: MoonfinProfile) =>
  Object.keys(defaults.profiles[profile] ?? {}).length

/**
 * Settings › Moonfin › Moonfin's defaults, under Serve Moonfin's companion: the settings Moonfin's
 * apps take for what a user never set, by profile, each with how many it sets. A profile's
 * settings are taken from an account's synced ones, or edited as JSON, which the section's dock
 * counts and saves with the settings (`side`). Apply to users writes the saved defaults into every
 * synced user's settings, merged or in place of theirs, once confirmed, and tells how many users
 * were updated.
 */
export default function MoonfinDefaults({ side }: { side: (edits: SideEdits | null) => void }) {
  const { t } = useI18n()
  const text = t.settings.moonfinDefaults
  const names = t.account.moonfin.profiles
  const toast = useToast()
  const panelId = useId()
  const [profile, setProfile] = useState<MoonfinProfile>('global')
  // The edited text of each profile, kept while another profile is shown.
  const [drafts, setDrafts] = useState<Partial<Record<MoonfinProfile, string>>>({})
  // The profiles whose text the last save found not to be a JSON object.
  const [invalid, setInvalid] = useState<ReadonlySet<MoonfinProfile>>(new Set())
  const [taking, setTaking] = useState(false)
  const [applying, setApplying] = useState(false)
  const defaults = useQuery({
    queryKey: queryKeys.moonfinDefaults,
    queryFn: ({ signal }) => fetchMoonfinDefaults(signal),
  })
  const data = defaults.data

  /** Shows the server's answer, dropping the draft of the profile it saved. */
  function adopt(answer: Defaults, saved: MoonfinProfile) {
    queryClient.setQueryData<Defaults>(queryKeys.moonfinDefaults, answer)
    setDrafts(({ [saved]: _, ...others }) => others)
  }

  // The edited profiles, for the dock: counted, checked, saved one after the other, discarded.
  const edits = useMemo<SideEdits | null>(() => {
    if (data === undefined) return null
    const changed = moonfinProfiles.filter(
      (name) => drafts[name] !== undefined && drafts[name] !== jsonText(data.profiles[name]),
    )
    return {
      changes: changed.length,
      check: () => {
        const wrong = changed.filter((name) => parseSettings(drafts[name] ?? '') === null)
        setInvalid(new Set(wrong))
        if (wrong.length > 0 && !wrong.includes(profile)) setProfile(wrong[0])
        return wrong.length === 0
      },
      save: async () => {
        for (const name of changed) {
          const settings = parseSettings(drafts[name] ?? '')
          if (settings === null) continue
          const answer = await saveMoonfinDefaults(name, settings)
          queryClient.setQueryData<Defaults>(queryKeys.moonfinDefaults, answer)
          setDrafts(({ [name]: _, ...others }) => others)
        }
      },
      discard: () => {
        setDrafts({})
        setInvalid(new Set())
      },
    }
  }, [data, drafts, profile])
  useEffect(() => side(edits), [side, edits])
  useEffect(() => () => side(null), [side])

  if (defaults.isPending) return <SkeletonRows rows={3} label={t.common.loading} />
  if (defaults.isError) {
    return (
      <InlineError onRetry={() => void defaults.refetch()} retrying={defaults.isFetching}>
        {errorMessage(t, defaults.error)}
      </InlineError>
    )
  }
  const loaded = defaults.data
  const draft = drafts[profile] ?? jsonText(loaded.profiles[profile])
  const anyDirty = (edits?.changes ?? 0) > 0

  return (
    <div className="flex flex-col gap-4" data-testid="moonfin-defaults">
      <div>
        <h3 className="text-h3 text-ink max-md:text-[1.125rem]">{text.title}</h3>
        <p className="mt-1 max-w-[66ch] text-small text-ink-2">{text.help}</p>
      </div>
      <Tabs
        variant="underline"
        label={text.profiles}
        value={profile}
        onChange={(id) => setProfile(id as MoonfinProfile)}
        items={moonfinProfiles.map((name) => ({
          id: name,
          label: names[name],
          count: settingCount(loaded, name),
          panelId,
        }))}
      />
      <div id={panelId} role="tabpanel" aria-label={names[profile]} className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-control text-ink-2" data-testid="moonfin-defaults-count">
            {text.count(settingCount(loaded, profile))}
          </p>
          <Button icon={CopyIcon} onClick={() => setTaking(true)}>
            {text.take}
          </Button>
        </div>
        <CodeEditor
          label={text.editor(names[profile])}
          help={text.editorHelp}
          value={draft}
          onValue={(value) => {
            setDrafts((current) => ({ ...current, [profile]: value }))
            setInvalid((current) => {
              const next = new Set(current)
              next.delete(profile)
              return next
            })
          }}
          maxBytes={maxBytes}
          rows={12}
          error={invalid.has(profile) ? text.notObject : undefined}
        />
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-line pt-4">
        <Button icon={ArrowsClockwiseIcon} disabled={anyDirty} onClick={() => setApplying(true)}>
          {text.apply}
        </Button>
        <p className="max-w-[60ch] text-small text-ink-2">
          {anyDirty ? text.saveFirst : text.applyHelp}
        </p>
      </div>
      {taking && (
        <TakeDialog
          defaults={loaded}
          profile={profile}
          onClose={() => setTaking(false)}
          onCopied={(answer) => {
            adopt(answer, profile)
            setInvalid((current) => {
              const next = new Set(current)
              next.delete(profile)
              return next
            })
            setTaking(false)
            toast(text.copied(names[profile]))
          }}
        />
      )}
      <ApplyDialog open={applying} onClose={() => setApplying(false)} />
    </div>
  )
}

/**
 * Take from an account: an account with synced settings and one of its profiles, copied over the
 * defaults of `profile`, without the account's own API keys and hidden items.
 */
function TakeDialog({
  defaults,
  profile,
  onClose,
  onCopied,
}: {
  defaults: Defaults
  profile: MoonfinProfile
  onClose: () => void
  onCopied: (answer: Defaults) => void
}) {
  const { t } = useI18n()
  const text = t.settings.moonfinDefaults
  const names = t.account.moonfin.profiles
  const users = defaults.users.filter((user) => user.sync.profiles.length > 0)
  const [userId, setUserId] = useState(users[0]?.id ?? '')
  const user = users.find((candidate) => candidate.id === userId)
  const offered = moonfinProfiles.filter((name) => user?.sync.profiles.includes(name))
  const [chosen, setChosen] = useState<MoonfinProfile | null>(null)
  // The profile of the same name as the one shown, while the account has it.
  const from: MoonfinProfile | undefined =
    chosen !== null && offered.includes(chosen)
      ? chosen
      : offered.includes(profile)
        ? profile
        : offered[0]
  const copy = useMutation({
    mutationFn: (source: MoonfinProfile) => copyMoonfinDefaults(profile, userId, source),
    onSuccess: onCopied,
  })

  return (
    <Modal
      open
      onClose={onClose}
      title={text.takeTitle(names[profile])}
      width={520}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button
            variant="primary"
            icon={CopyIcon}
            disabled={user === undefined || from === undefined}
            loading={copy.isPending}
            onClick={() => {
              if (from !== undefined) copy.mutate(from)
            }}
          >
            {text.copy}
          </Button>
        </>
      }
    >
      <div className="space-y-5 p-5" data-testid="moonfin-defaults-take">
        <p className="text-body text-ink-2">{text.takeHelp(names[profile])}</p>
        {users.length === 0 ? (
          <p className="text-body text-ink-2">{text.noAccounts}</p>
        ) : (
          <>
            <Field label={text.account}>
              <Select
                value={userId}
                options={users.map((candidate) => ({
                  value: candidate.id,
                  label: candidate.name,
                  detail: t.account.moonfin.settings(candidate.sync.settings),
                }))}
                onValue={(id) => {
                  setUserId(id)
                  copy.reset()
                }}
              />
            </Field>
            <Field label={text.from}>
              <Select<MoonfinProfile>
                value={from ?? profile}
                options={offered.map((name) => ({ value: name, label: names[name] }))}
                onValue={(name) => {
                  setChosen(name)
                  copy.reset()
                }}
              />
            </Field>
          </>
        )}
        {copy.isError && <FieldError>{errorMessage(t, copy.error)}</FieldError>}
      </div>
    </Modal>
  )
}

/**
 * Apply to users: asks whether the saved defaults merge into each synced user's settings or take
 * their place, then tells how many users were updated.
 */
function ApplyDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useI18n()
  const text = t.settings.moonfinDefaults
  const toast = useToast()
  const [reset, setReset] = useState(false)
  const apply = useMutation({
    mutationFn: () => applyMoonfinDefaults(reset),
    onSuccess: (users) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.usersMoonfin })
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownMoonfin })
      void queryClient.invalidateQueries({ queryKey: queryKeys.moonfinDefaults })
      onClose()
      toast(text.applied(users), { tone: 'ok' })
    },
  })

  return (
    <ConfirmDialog
      open={open}
      onClose={() => {
        apply.reset()
        onClose()
      }}
      onConfirm={() => apply.mutate()}
      title={text.applyTitle}
      confirmLabel={text.applyConfirm}
      tone={reset ? 'danger' : 'primary'}
      busy={apply.isPending}
      error={apply.isError ? errorMessage(t, apply.error) : undefined}
    >
      <div className="space-y-3" data-testid="moonfin-defaults-apply">
        <p>{text.applyIntro}</p>
        <Segmented<'merge' | 'reset'>
          label={text.applyMode}
          value={reset ? 'reset' : 'merge'}
          options={[
            { value: 'merge', label: text.merge },
            { value: 'reset', label: text.reset },
          ]}
          onChange={(mode) => setReset(mode === 'reset')}
        />
        <p>{reset ? text.resetHelp : text.mergeHelp}</p>
      </div>
    </ConfirmDialog>
  )
}
