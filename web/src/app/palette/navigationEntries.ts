import {
  adminSections,
  memberSections,
  accountPages,
  settingsPath,
  settingsSections,
} from '@/app/navigation'
import { registerPaletteSource } from './registry'

// Every page of the navigation, as the TopBar and its tabs offer them.
registerPaletteSource('navigation', ({ t, user }) => {
  const sections = user.isAdministrator ? [...adminSections, ...accountPages] : memberSections
  return sections.flatMap((section) =>
    section.pages
      ? section.pages.map((page) => ({
          id: `navigation:${page.id}`,
          group: 'pages' as const,
          label: page.label(t),
          hint: section.label(t),
          icon: page.icon,
          to: page.to,
        }))
      : [
          {
            id: `navigation:${section.id}`,
            group: 'pages' as const,
            label: section.label(t),
            icon: section.icon,
            to: section.to,
          },
        ],
  )
})

// The sections of Settings. Each setting inside them is the settings area's to register.
registerPaletteSource('settings-sections', ({ t, user }) =>
  user.isAdministrator
    ? settingsSections.map((section) => ({
        id: `settings:${section.id}`,
        group: 'settings' as const,
        label: t.nav.settingsSections[section.key],
        hint: t.nav.settings,
        keywords: [t.nav.settings],
        icon: section.icon,
        to: settingsPath(section.id),
      }))
    : [],
)
