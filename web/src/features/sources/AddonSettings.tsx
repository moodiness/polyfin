import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  queryClient,
  queryKeys,
  saveAddonSettings,
  type Addon,
  type AddonMusic,
  type AddonSetting,
  type Scope,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'
import { Badge, Button, SaveBar, SettingRow, useToast } from '@/ui'
import { Group } from './parts'

/** The badge of an Eclipse addon: what its tracks are. */
export function MusicBadge({ music }: { music: AddonMusic }) {
  const { t } = useI18n()
  return <Badge tone="accent">{t.music.content[music.contentType] ?? t.music.content.music}</Badge>
}

/** The value a setting has now: the one chosen, else its default. */
function currentValue(music: AddonMusic, setting: AddonSetting): string {
  return music.values[setting.key] ?? setting.default
}

/** Why a value cannot be saved, as the server checks it; null when it can. */
function problem(
  t: Messages,
  language: string,
  setting: AddonSetting,
  value: string,
): string | null {
  const number = (n: number) => n.toLocaleString(language)
  if (setting.type === 'text') {
    const limit = setting.maxLength > 0 ? setting.maxLength : 1000
    return [...value].length > limit ? t.music.tooLong(limit) : null
  }
  if (setting.type !== 'number') return null
  const parsed = Number(value.trim())
  if (value.trim() === '' || !Number.isFinite(parsed)) return t.common.enterNumber
  if (setting.min !== null && parsed < setting.min) return t.music.belowMin(number(setting.min))
  if (setting.max !== null && parsed > setting.max) return t.music.aboveMax(number(setting.max))
  if (setting.step !== null && setting.step > 0) {
    const steps = (parsed - (setting.min ?? 0)) / setting.step
    if (Math.abs(steps - Math.round(steps)) > 1e-9) return t.music.offStep(number(setting.step))
  }
  return null
}

/** What a number field accepts, in words. */
function numberHint(t: Messages, language: string, setting: AddonSetting): string {
  const number = (n: number) => n.toLocaleString(language)
  const bounds =
    setting.min !== null && setting.max !== null
      ? t.music.range(number(setting.min), number(setting.max))
      : setting.min !== null
        ? t.music.atLeast(number(setting.min))
        : setting.max !== null
          ? t.music.atMost(number(setting.max))
          : ''
  const step = setting.step !== null && setting.step > 0 ? t.music.stepOf(number(setting.step)) : ''
  return [bounds, step].filter(Boolean).join(' ')
}

/** How a setting's default reads in the setting's own terms. */
function defaultText(t: Messages, setting: AddonSetting): string {
  if (setting.type === 'toggle') {
    return t.music.defaultValue(setting.default === 'true' ? t.music.on : t.music.off)
  }
  if (setting.type === 'select') {
    const option = setting.options.find((o) => o.value === setting.default)
    return t.music.defaultValue(option?.label ?? setting.default)
  }
  return setting.default === '' ? t.music.defaultEmpty : t.music.defaultValue(setting.default)
}

export type AddonSettingsProps = {
  /** Whose addon it is: the server's, or the signed-in user's own. */
  scope: Scope
  addon: Addon
  /** The addon's `music`: its declared settings and the values chosen. */
  music: AddonMusic
  className?: string
}

/**
 * The settings an Eclipse addon declares, as setting rows saved as a whole through the unsaved
 * dock. Saving updates the scope's cached addons and confirms with a toast; errors are shown under
 * their row, and the server's in the dock.
 */
export function AddonSettings({ scope, addon, music, className }: AddonSettingsProps) {
  const { language, t } = useI18n()
  const toast = useToast()
  const saved = Object.fromEntries(music.settings.map((s) => [s.key, currentValue(music, s)]))
  const [values, setValues] = useState<Record<string, string>>(saved)
  const [touched, setTouched] = useState(false)
  const save = useMutation({
    mutationFn: (next: Record<string, string>) => saveAddonSettings(scope, addon.id, next),
    onSuccess: (updated) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (list) =>
        list?.map((item) => (item.id === updated.id ? updated : item)),
      )
      // Libraries follow the addon's catalogs, which its settings may change.
      void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
      setTouched(false)
      // The server writes values back in its own form: the rows follow it, unchanged.
      const next = updated.music
      if (next !== null) {
        setValues(Object.fromEntries(next.settings.map((s) => [s.key, currentValue(next, s)])))
      }
      toast(t.music.saved)
    },
  })
  const problems = Object.fromEntries(
    music.settings.map((s) => [s.key, problem(t, language, s, values[s.key] ?? '')]),
  )
  const valid = Object.values(problems).every((p) => p === null)
  const changes = music.settings.filter((s) => (values[s.key] ?? '') !== saved[s.key]).length

  function set(key: string, value: string) {
    save.reset()
    setTouched(true)
    setValues((current) => ({ ...current, [key]: value }))
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setTouched(true)
    if (!valid) return
    // Numbers are sent trimmed; the server writes them back in its own form.
    save.mutate(
      Object.fromEntries(
        music.settings.map((s) => [
          s.key,
          s.type === 'number' ? values[s.key].trim() : values[s.key],
        ]),
      ),
    )
  }

  return (
    <Group title={t.music.settingsTitle} className={className}>
      <p className="mb-6 max-w-[66ch] text-small text-ink-2">{t.music.settingsHelp}</p>
      {music.settings.length === 0 ? (
        <p className="text-lead text-ink-2">{t.music.noSettings}</p>
      ) : (
        <form onSubmit={submit} noValidate>
          <div className="grid grid-cols-[minmax(0,1fr)] gap-2">
            {music.settings.map((setting) => (
              <SettingField
                key={setting.key}
                setting={setting}
                value={values[setting.key] ?? ''}
                saved={saved[setting.key]}
                error={touched ? (problems[setting.key] ?? undefined) : undefined}
                onValue={(value) => set(setting.key, value)}
              />
            ))}
          </div>
          <div className="mt-5">
            <Button
              variant="secondary"
              onClick={() => {
                save.reset()
                setTouched(true)
                setValues(Object.fromEntries(music.settings.map((s) => [s.key, s.default])))
              }}
            >
              {t.music.resetDefaults}
            </Button>
          </div>
          <SaveBar
            dirty={changes > 0}
            changes={changes}
            saving={save.isPending}
            onDiscard={() => {
              save.reset()
              setTouched(false)
              setValues(saved)
            }}
            error={save.isError ? errorMessage(t, save.error) : undefined}
          />
        </form>
      )}
    </Group>
  )
}

function SettingField({
  setting,
  value,
  saved,
  error,
  onValue,
}: {
  setting: AddonSetting
  value: string
  saved: string
  error: string | undefined
  onValue: (value: string) => void
}) {
  const { language, t } = useI18n()
  const label = setting.label || setting.key
  const defaults = defaultText(t, setting)

  if (setting.type === 'toggle') {
    return (
      <SettingRow
        kind="switch"
        label={label}
        help={[setting.help, defaults].filter(Boolean).join(' ') || undefined}
        checked={value === 'true'}
        saved={saved === 'true'}
        onChange={(checked) => onValue(String(checked))}
      />
    )
  }
  if (setting.type === 'select') {
    return (
      <SettingRow
        kind="choice"
        label={label}
        help={[setting.help, defaults].filter(Boolean).join(' ') || undefined}
        value={value}
        saved={saved}
        options={setting.options.map((option) => ({
          value: option.value,
          label: option.label || option.value,
        }))}
        onChange={onValue}
      />
    )
  }
  const limit = setting.maxLength > 0 ? setting.maxLength : undefined
  const help = [
    setting.help,
    setting.type === 'number'
      ? numberHint(t, language, setting)
      : limit
        ? t.music.maxLength(limit)
        : '',
    defaults,
  ]
    .filter(Boolean)
    .join(' ')
  // A number is kept as typed, so an invalid entry stays visible with its error.
  return (
    <SettingRow
      kind="text"
      label={label}
      help={help || undefined}
      error={error}
      value={value}
      saved={saved}
      onChange={onValue}
      placeholder={setting.placeholder || undefined}
      maxLength={limit}
      mono={setting.type === 'number'}
    />
  )
}
