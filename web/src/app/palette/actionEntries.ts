import { LinkIcon, PlusIcon } from '@phosphor-icons/react'
import enNav from '@/i18n/en/nav'
import { registerPaletteSource } from './registry'

// What an administrator does most often, by its verb: adding a source and inviting someone. Each
// opens its page with the panel or dialog that does it (`?add=source`, `?action=invite`); a page
// that does not read its parameter yet opens as it is. The English verbs find them too, whichever
// the interface uses.
registerPaletteSource('actions', ({ t, user }) => {
  if (!user.isAdministrator) return []
  const verbs = t.palette.verbs
  return [
    {
      id: 'action:add-source',
      group: 'pages' as const,
      label: t.sources.add,
      hint: t.nav.sources,
      keywords: [...verbs.addSource, ...enNav.palette.verbs.addSource],
      icon: PlusIcon,
      to: '/sources?add=source',
    },
    {
      id: 'action:invite',
      group: 'pages' as const,
      label: t.invites.create,
      hint: t.nav.users,
      keywords: [...verbs.invite, ...enNav.palette.verbs.invite],
      icon: LinkIcon,
      to: '/users?action=invite',
    },
  ]
})
