import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react'
import { useState, type ReactNode } from 'react'
import type { Settings, WebTheme } from '@/api'
import { useI18n, type Messages } from '@/i18n'
import { Button, ConfirmDialog } from '@/ui'

/** The settings of Settings › Web player that Restore the defaults puts back. */
const restored = [
  'webTheme',
  'customCss',
  'customJs',
  'loginDisclaimer',
  'addVersionsToOpenPage',
  'versionBadges',
  'posterRatings',
] as const satisfies readonly (keyof Settings)[]

type Restored = (typeof restored)[number]

/** The defaults of the settings restored, from the server's bounds. */
function webPlayerDefaults(form: Settings): Pick<Settings, Restored> {
  const value = <K extends Restored>(name: K): Settings[K] =>
    (form.bounds[name]?.default ?? form[name]) as Settings[K]
  return {
    webTheme: value('webTheme'),
    customCss: value('customCss'),
    customJs: value('customJs'),
    loginDisclaimer: value('loginDisclaimer'),
    addVersionsToOpenPage: value('addVersionsToOpenPage'),
    versionBadges: value('versionBadges'),
    posterRatings: value('posterRatings'),
  }
}

/** Each setting the form holds away from its default, as the confirmation lists it. */
function changeLines(form: Settings, defaults: Pick<Settings, Restored>, t: Messages): string[] {
  const s = t.settings
  const text = s.webPlayerDefaults
  const labels: Record<Restored, string> = {
    webTheme: s.webTheme,
    customCss: s.customCss,
    customJs: s.customJs,
    loginDisclaimer: s.loginDisclaimer,
    addVersionsToOpenPage: s.addVersionsToOpenPage,
    versionBadges: s.versionBadges,
    posterRatings: s.posterRatings,
  }
  return restored
    .filter((name) => form[name] !== defaults[name])
    .map((name) => {
      const value = defaults[name]
      if (typeof value === 'boolean') return text.value(labels[name], value ? t.ui.on : t.ui.off)
      if (name === 'webTheme') {
        return text.value(labels[name], s.webThemeChoices[value as WebTheme])
      }
      return value === '' ? text.emptied(labels[name]) : text.value(labels[name], value)
    })
}

/**
 * Settings › Web player › Restore the defaults, after the section's other buttons (`children`):
 * asks first, listing each setting of the section that changes and what stays, then puts
 * Polyfin's defaults in the form, which the save bar saves or discards as any change. Off, with
 * why, while every setting is at its default.
 */
export default function WebPlayerDefaults({
  form,
  update,
  children,
}: {
  form: Settings
  update: (patch: Partial<Settings>) => void
  children?: ReactNode
}) {
  const { t } = useI18n()
  const text = t.settings.webPlayerDefaults
  const [asking, setAsking] = useState(false)
  const defaults = webPlayerDefaults(form)
  const changes = changeLines(form, defaults, t)
  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        {children}
        <Button
          variant="secondary"
          icon={ArrowCounterClockwiseIcon}
          disabled={changes.length === 0}
          onClick={() => setAsking(true)}
        >
          {text.button}
        </Button>
      </div>
      <p className="mt-2 max-w-[60ch] text-small text-ink-2">
        {changes.length === 0 ? text.none : text.help}
      </p>
      <ConfirmDialog
        open={asking}
        onClose={() => setAsking(false)}
        onConfirm={() => {
          setAsking(false)
          update(defaults)
        }}
        title={text.title}
        confirmLabel={text.confirm}
      >
        <div className="space-y-3">
          <div>
            <p>{text.changes}</p>
            <ul className="mt-1 list-disc space-y-0.5 ps-5">
              {changes.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          </div>
          <p>{text.kept}</p>
          <p>{text.save}</p>
        </div>
      </ConfirmDialog>
    </>
  )
}
