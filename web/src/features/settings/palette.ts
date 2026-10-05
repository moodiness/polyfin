import { settingsPath, settingsSections } from '@/app/navigation'
import { registerPaletteSource } from '@/app/palette/registry'
import enNav from '@/i18n/en/nav'
import enSettings from '@/i18n/en/settings'
import frNav from '@/i18n/fr/nav'
import frSettings from '@/i18n/fr/settings'
import { settingEntries } from './catalog'

// Every setting, for administrators, opening its section at the setting. The words of both
// languages find it, whichever the interface uses.
registerPaletteSource('settings-entries', ({ t, user }) => {
  if (!user.isAdministrator) return []
  return settingEntries.map((entry) => {
    const section = settingsSections.find((item) => item.id === entry.section)!
    return {
      id: `settings:${entry.section}:${entry.anchor}`,
      group: 'settings' as const,
      label: entry.label(t),
      hint: `${t.nav.settings} › ${t.nav.settingsSections[section.key]}`,
      keywords: [
        entry.label(enSettings),
        entry.label(frSettings),
        enNav.nav.settingsSections[section.key],
        frNav.nav.settingsSections[section.key],
        ...(entry.keywords ?? []),
      ],
      icon: section.icon,
      to: settingsPath(entry.section, entry.anchor),
    }
  })
})
