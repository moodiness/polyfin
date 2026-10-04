import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  clearMapping,
  setMapping,
  type CatalogTarget,
  type GuideMapping,
  type MappingItem,
  type Scope,
} from '@/api'
import GuidePicker from '@/components/lineup/GuidePicker'
import { Badge, Notice } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n/en'

/** How a channel's mapping reads: the guide channel, or none, and whether it was set by hand. */
export function MappingText({ mapping }: { mapping: GuideMapping | null }) {
  const { t } = useI18n()
  const text = t.lineup.mapping
  if (mapping === null || mapping.guideChannelId === null) {
    return (
      <span className="inline-flex flex-wrap items-center gap-1.5">
        <span className="text-muted">{text.none}</span>
        {mapping?.manual && <Badge tone="fin">{text.manualNone}</Badge>}
      </span>
    )
  }
  return (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-1.5">
      <span className="text-zinc-100">{mapping.guideChannelName ?? mapping.guideChannelId}</span>
      <span className="font-mono text-xs break-all text-muted">{mapping.guideChannelId}</span>
      <Badge tone={mapping.manual ? 'fin' : 'muted'}>
        {mapping.manual ? text.manual : text.automatic}
      </Badge>
    </span>
  )
}

/** The actions on one channel's mapping: choose a guide channel, pin "no guide", or go back to automatic. */
export function MappingControls({
  scope,
  target,
  channelId,
  channelName,
  mapping,
  onChanged,
}: {
  scope: Scope
  target: CatalogTarget
  channelId: string
  channelName: string
  mapping: GuideMapping | null
  onChanged: (item: MappingItem) => void
}) {
  const { t } = useI18n()
  const text = t.lineup.mapping
  const [picking, setPicking] = useState(false)
  const set = useMutation({
    mutationFn: (guide: { guideId: string; guideChannelId: string } | null) =>
      setMapping(scope, target, channelId, guide),
    onSuccess: (item) => {
      setPicking(false)
      onChanged(item)
    },
  })
  const clear = useMutation({
    mutationFn: () => clearMapping(scope, target, channelId),
    onSuccess: onChanged,
  })
  const busy = set.isPending || clear.isPending
  return (
    <div className="contents">
      <div className="flex flex-wrap gap-1.5">
        <button
          type="button"
          className={rowButton}
          aria-expanded={picking}
          aria-label={text.chooseLabel(channelName)}
          disabled={busy}
          onClick={() => setPicking((open) => !open)}
        >
          {text.choose}
        </button>
        {!(mapping?.manual && mapping.guideChannelId === null) && (
          <button
            type="button"
            className={rowButton}
            disabled={busy}
            aria-label={text.noGuideLabel(channelName)}
            onClick={() => set.mutate(null)}
          >
            {text.noGuide}
          </button>
        )}
        {mapping?.manual && (
          <button
            type="button"
            className={rowButton}
            disabled={busy}
            aria-label={text.automaticLabel(channelName)}
            onClick={() => clear.mutate()}
          >
            {text.backToAutomatic}
          </button>
        )}
      </div>
      {(set.isError || clear.isError) && (
        <div className="w-full">
          <Notice kind="error">{errorMessage(t, set.error ?? clear.error)}</Notice>
        </div>
      )}
      {picking && (
        <div className="w-full">
          <GuidePicker
            scope={scope}
            target={target}
            channelName={channelName}
            onPick={(channel) =>
              set.mutate({ guideId: channel.guideId, guideChannelId: channel.id })
            }
            onCancel={() => setPicking(false)}
          />
        </div>
      )}
    </div>
  )
}

/** The words of a mapping, for a message saying what changed. */
export function mappingWords(t: Messages, item: MappingItem): string {
  const mapping = item.mapping
  if (mapping === null || mapping.guideChannelId === null) return t.lineup.mapping.none
  return mapping.guideChannelName ?? mapping.guideChannelId
}

const rowButton =
  'inline-flex min-h-9 items-center rounded-lg border border-line bg-ink px-3 text-xs font-medium text-white transition-colors hover:border-fin-4 disabled:cursor-progress disabled:opacity-60'
