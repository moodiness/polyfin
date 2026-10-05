import { HardDrivesIcon, UserIcon } from '@phosphor-icons/react'
import type { Owner } from '@/api'
import { Badge } from '@/ui'
import { useI18n } from '@/i18n'

/** Whose a row is: the server's, or a user's own. */
export default function OwnerChip({ owner }: { owner: Owner }) {
  const { t } = useI18n()
  return owner === null ? (
    <Badge className="gap-1">
      <HardDrivesIcon size={12} aria-hidden="true" />
      {t.system.health.owner.server}
    </Badge>
  ) : (
    <Badge tone="accent" className="gap-1">
      <UserIcon size={12} aria-hidden="true" />
      {t.system.health.owner.user(owner.name)}
    </Badge>
  )
}
