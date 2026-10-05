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
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n'
import { Badge, Button, Field, FieldError, Select, Switch, TextInput, useToast } from '@/ui'

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
  /** Shows a Close button that calls it, for settings opened from a list. */
  onClose?: () => void
  /** Heading level of the "Addon settings" title, so the page keeps a correct outline. */
  titleAs?: 'h2' | 'h3'
  className?: string
}

/**
 * The settings an Eclipse addon declares, edited and saved as a whole. It has no frame: put it in
 * a Panel (or a PanelSection of one). Saving updates the scope's cached addons and confirms with a
 * toast; errors are shown under their field, and the server's under the buttons.
 */
export function AddonSettings({
  scope,
  addon,
  music,
  onClose,
  titleAs: Title = 'h3',
  className,
}: AddonSettingsProps) {
  const { language, t } = useI18n()
  const toast = useToast()
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
      toast(t.music.saved)
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
    <section aria-labelledby={titleId} className={className}>
      <Title id={titleId} className="text-[15px] font-semibold tracking-[-0.01em] text-ink">
        {t.music.settingsTitle}
      </Title>
      <p className="mt-1 max-w-[70ch] text-small text-ink-3">{t.music.settingsHelp}</p>
      {music.settings.length === 0 ? (
        <p className="mt-4 text-control text-ink-2">{t.music.noSettings}</p>
      ) : (
        <form onSubmit={submit} noValidate className="mt-5 flex flex-col gap-5">
          {music.settings.map((setting) => (
            <SettingField
              key={setting.key}
              setting={setting}
              value={values[setting.key] ?? ''}
              error={touched ? (problems[setting.key] ?? undefined) : undefined}
              onValue={(value) => set(setting.key, value)}
            />
          ))}
          {save.isError && <FieldError>{errorMessage(t, save.error)}</FieldError>}
          <div className="flex flex-wrap gap-2">
            <Button type="submit" variant="primary" loading={save.isPending}>
              {t.common.save}
            </Button>
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
            {onClose && (
              <Button variant="ghost" onClick={onClose}>
                {t.ui.close}
              </Button>
            )}
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
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p id={`${id}-label`} className="text-control font-medium text-ink">
            {label}
          </p>
          <p id={`${id}-help`} className="mt-1 text-small text-ink-3">
            {[setting.help, defaults].filter(Boolean).join(' ')}
          </p>
        </div>
        <Switch
          checked={value === 'true'}
          onChange={(checked) => onValue(String(checked))}
          labelledBy={`${id}-label`}
          describedById={`${id}-help`}
          stateText
        />
      </div>
    )
  }
  if (setting.type === 'select') {
    return (
      <Field label={label} help={[setting.help, defaults].filter(Boolean).join(' ')}>
        <Select
          value={value}
          options={setting.options.map((option) => ({
            value: option.value,
            label: option.label || option.value,
          }))}
          onValue={onValue}
        />
      </Field>
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
  if (setting.type === 'number') {
    // The text is kept as typed, so an invalid entry stays visible with its error.
    return (
      <Field label={label} help={help} error={error}>
        <TextInput
          value={value}
          onValue={onValue}
          inputMode="decimal"
          placeholder={setting.placeholder || undefined}
          mono
          className="max-w-[200px]"
        />
      </Field>
    )
  }
  return (
    <Field label={label} help={help} error={error}>
      <TextInput
        value={value}
        onValue={onValue}
        placeholder={setting.placeholder || undefined}
        maxLength={limit}
        autoComplete="off"
      />
    </Field>
  )
}
