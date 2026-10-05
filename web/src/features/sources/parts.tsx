import { useState } from 'react'
import type { Addon } from '@/api'
import { useI18n } from '@/i18n'
import { IconTile, StatusPill } from '@/ui'
import { kindIcon } from './model'

/**
 * The source's kind icon, covered by its logo once that loads. Logos are third-party images: when
 * one is missing, broken or blocked by the page's security policy, the icon stays, so nothing moves.
 */
export function SourceTile({ addon, selected = false }: { addon: Addon; selected?: boolean }) {
  const [state, setState] = useState<'loading' | 'loaded' | 'failed'>('loading')
  const src =
    addon.logo !== null && addon.logo.startsWith('https://') && state !== 'failed'
      ? addon.logo
      : null
  return (
    <span className="relative inline-grid shrink-0">
      <IconTile icon={kindIcon(addon)} selected={selected} />
      {src !== null && (
        <img
          key={src}
          src={src}
          alt=""
          width={40}
          height={40}
          loading="lazy"
          decoding="async"
          referrerPolicy="no-referrer"
          onLoad={() => setState('loaded')}
          onError={() => setState('failed')}
          className={`absolute inset-0 size-10 rounded-row border border-line-2 bg-s3 object-contain p-1 ${
            state === 'loaded' ? 'opacity-100' : 'opacity-0'
          }`}
        />
      )}
    </span>
  )
}

/** Whether a source works: on, off, or its last IPTV download failed. Always an icon and a word. */
export function SourceStatus({ addon }: { addon: Addon }) {
  const { t } = useI18n()
  if (!addon.enabled) return <StatusPill tone="muted">{t.sources.off}</StatusPill>
  if (addon.source !== null && addon.source.error !== '')
    return <StatusPill tone="danger">{t.sources.failed}</StatusPill>
  return <StatusPill tone="ok">{t.sources.active}</StatusPill>
}
