import type { Owner } from '@/api'
import { icons } from '@/components/icons'
import { useI18n } from '@/i18n'

/** Whose a dashboard row is: the server's, or a user's own. */
export default function OwnerChip({ owner }: { owner: Owner }) {
  const { t } = useI18n()
  return owner === null ? (
    <span className="inline-flex items-center rounded-md border border-line px-1.5 py-0.5 text-xs font-medium whitespace-nowrap text-muted">
      {t.dashboard.health.owner.server}
    </span>
  ) : (
    <span className="inline-flex items-center gap-1 rounded-md border border-fin-4/40 bg-fin-2/10 px-1.5 py-0.5 text-xs font-medium whitespace-nowrap text-zinc-100">
      <icons.account className="size-3.5 text-fin-5" />
      {t.dashboard.health.owner.user(owner.name)}
    </span>
  )
}
