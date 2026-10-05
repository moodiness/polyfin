import { useId, useState, type FormEvent } from 'react'
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
import { Badge, buttonPrimary, buttonSecondary, Checkbox, Notice, TextField } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'

/** The badge of an Eclipse addon: what its tracks are. */
export function MusicBadge({ music }: { music: AddonMusic }) {
  const { t } = useI18n()
  return <Badge tone="fin">{t.music.content[music.contentType] ?? t.music.content.music}</Badge>
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
  if (value.trim() === '' || !Number.isFinite(parsed)) return t.music.notNumber
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

/** The settings an Eclipse addon declares, edited and saved as a whole. */
export function AddonSettingsPanel({
  scope,
  addon,
  music,
  onClose,
}: {
  scope: Scope
  addon: Addon
  music: AddonMusic
  onClose: () => void
}) {
  const { language, t } = useI18n()
  const titleId = useId()
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(music.settings.map((s) => [s.key, currentValue(music, s)])),
  )
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
    },
  })
  const problems = Object.fromEntries(
    music.settings.map((s) => [s.key, problem(t, language, s, values[s.key] ?? '')]),
  )
  const valid = Object.values(problems).every((p) => p === null)

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
    <section aria-labelledby={titleId} className="mt-4 rounded-xl border border-line bg-bg/60 p-4">
      <h3 id={titleId} className="font-semibold text-white">
        {t.music.settingsTitle}
      </h3>
      <p className="mt-1 max-w-prose text-xs text-muted">{t.music.settingsHelp}</p>
      {music.settings.length === 0 ? (
        <p className="mt-4 text-sm text-muted">{t.music.noSettings}</p>
      ) : (
        <form onSubmit={submit} noValidate className="mt-4 space-y-5">
          {music.settings.map((setting) => (
            <SettingField
              key={setting.key}
              setting={setting}
              value={values[setting.key] ?? ''}
              error={touched ? (problems[setting.key] ?? undefined) : undefined}
              onValue={(value) => set(setting.key, value)}
            />
          ))}
          {save.isError && <Notice kind="error">{errorMessage(t, save.error)}</Notice>}
          {save.isSuccess && <Notice kind="success">{t.music.saved}</Notice>}
          <div className="flex flex-wrap gap-2">
            <button type="submit" className={buttonPrimary} disabled={save.isPending}>
              {save.isPending ? t.common.saving : t.common.save}
            </button>
            <button
              type="button"
              className={buttonSecondary}
              onClick={() => {
                save.reset()
                setTouched(true)
                setValues(Object.fromEntries(music.settings.map((s) => [s.key, s.default])))
              }}
            >
              {t.music.resetDefaults}
            </button>
            <button type="button" className={buttonSecondary} onClick={onClose}>
              {t.users.close}
            </button>
          </div>
        </form>
      )}
    </section>
  )
}

function SettingField({
  setting,
  value,
  error,
  onValue,
}: {
  setting: AddonSetting
  value: string
  error: string | undefined
  onValue: (value: string) => void
}) {
  const { language, t } = useI18n()
  const id = useId()
  const label = setting.label || setting.key
  const defaults = defaultText(t, setting)

  if (setting.type === 'toggle') {
    return (
      <Checkbox
        label={label}
        help={[setting.help, defaults].filter(Boolean).join(' ')}
        checked={value === 'true'}
        onChange={(checked) => onValue(String(checked))}
      />
    )
  }
  if (setting.type === 'select') {
    return (
      <div>
        <label htmlFor={id} className="block text-sm font-medium text-zinc-200">
          {label}
        </label>
        <select
          id={id}
          value={value}
          onChange={(event) => onValue(event.target.value)}
          aria-describedby={`${id}-hint`}
          className="mt-1.5 block w-full rounded-lg border border-line bg-bg px-3 py-2 text-white"
        >
          {setting.options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label || option.value}
            </option>
          ))}
        </select>
        <p id={`${id}-hint`} className="mt-1 text-xs text-muted">
          {[setting.help, defaults].filter(Boolean).join(' ')}
        </p>
      </div>
    )
  }
  const limit = setting.maxLength > 0 ? setting.maxLength : undefined
  return (
    <TextField
      label={label}
      hint={[
        setting.help,
        setting.type === 'number'
          ? numberHint(t, language, setting)
          : limit
            ? t.music.maxLength(limit)
            : '',
        defaults,
      ]
        .filter(Boolean)
        .join(' ')}
      error={error}
      value={value}
      placeholder={setting.placeholder || undefined}
      onValue={onValue}
      {...(setting.type === 'number'
        ? {
            type: 'number',
            inputMode: 'decimal' as const,
            min: setting.min ?? undefined,
            max: setting.max ?? undefined,
            step: setting.step ?? 'any',
          }
        : { maxLength: limit, autoComplete: 'off' })}
    />
  )
}
