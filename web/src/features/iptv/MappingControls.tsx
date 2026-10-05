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
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import { Badge, Button, FieldError } from '@/ui'
import GuidePicker from './GuidePicker'

/** How a channel's mapping reads: the guide channel, or none, and whether it was set by hand. */
export function MappingText({ mapping }: { mapping: GuideMapping | null }) {
  const { t } = useI18n()
  const text = t.lineup.mapping
  if (mapping === null || mapping.guideChannelId === null) {
    return (
      <span className="inline-flex flex-wrap items-center gap-1.5">
        <span className="text-ink-3">{text.none}</span>
        {mapping?.manual && <Badge tone="accent">{text.manualNone}</Badge>}
      </span>
    )
  }
  return (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
      <span className="text-ink">{mapping.guideChannelName ?? mapping.guideChannelId}</span>
      <span className="figures text-small break-all text-ink-3">{mapping.guideChannelId}</span>
      <Badge tone={mapping.manual ? 'accent' : 'neutral'}>
        {mapping.manual ? text.manual : text.automatic}
      </Badge>
    </span>
  )
}

/**
 * The actions on one channel's mapping: choose a guide channel, pin "no guide", or go back to
 * automatic. The picker opens under the actions, full width.
 */
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
    <>
      <div className="flex flex-wrap gap-1">
        <Button
          size="sm"
          aria-expanded={picking}
          aria-label={text.chooseLabel(channelName)}
          disabled={busy}
          onClick={() => setPicking((open) => !open)}
        >
          {text.choose}
        </Button>
        {!(mapping?.manual && mapping.guideChannelId === null) && (
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            aria-label={text.noGuideLabel(channelName)}
            onClick={() => set.mutate(null)}
          >
            {text.noGuide}
          </Button>
        )}
        {mapping?.manual && (
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            aria-label={text.automaticLabel(channelName)}
            onClick={() => clear.mutate()}
          >
            {text.backToAutomatic}
          </Button>
        )}
      </div>
      {(set.isError || clear.isError) && (
        <div className="w-full">
          <FieldError>{errorMessage(t, set.error ?? clear.error)}</FieldError>
        </div>
      )}
      {picking && (
        <div className="w-full">
          <GuidePicker
            scope={scope}
            target={target}
            channelName={channelName}
            busy={busy}
            onPick={(channel) =>
              set.mutate({ guideId: channel.guideId, guideChannelId: channel.id })
            }
            onCancel={() => setPicking(false)}
          />
        </div>
      )}
    </>
  )
}

/** The words of a mapping, for a message saying what changed. */
export function mappingWords(t: Messages, item: MappingItem): string {
  const mapping = item.mapping
  if (mapping === null || mapping.guideChannelId === null) return t.lineup.mapping.none
  return mapping.guideChannelName ?? mapping.guideChannelId
}
