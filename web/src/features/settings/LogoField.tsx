import { useRef, useState } from 'react'
import { useI18n } from '@/i18n'
import { Button } from '@/ui'

/** The largest logo, decoded (as the server's MaxBrandLogoBytes). */
const MAX_BYTES = 1 << 20
const TYPES = ['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/svg+xml']

/**
 * The web player's logo: an image file read into a data URL, shown as it
 * will be, or empty for Jellyfin's own.
 */
export default function LogoField({
  value,
  onValue,
}: {
  value: string
  onValue: (value: string) => void
}) {
  const { t } = useI18n()
  const s = t.settings
  const input = useRef<HTMLInputElement>(null)
  const [problem, setProblem] = useState('')
  const choose = (file: File | undefined) => {
    setProblem('')
    if (!file) return
    if (!TYPES.includes(file.type)) {
      setProblem(s.brandLogoType)
      return
    }
    if (file.size > MAX_BYTES) {
      setProblem(s.brandLogoSize)
      return
    }
    const reader = new FileReader()
    reader.onload = () => onValue(String(reader.result))
    reader.readAsDataURL(file)
  }
  return (
    <div className="flex flex-wrap items-center gap-4">
      <div className="flex h-16 w-48 items-center justify-center rounded-field border border-line-2 bg-s2 p-2">
        {value ? (
          <img src={value} alt="" className="max-h-full max-w-full object-contain" />
        ) : (
          <span className="text-small text-ink-3">{s.brandLogoNone}</span>
        )}
      </div>
      <input
        ref={input}
        type="file"
        accept={TYPES.join(',')}
        className="hidden"
        onChange={(event) => {
          choose(event.target.files?.[0])
          event.target.value = ''
        }}
      />
      <Button onClick={() => input.current?.click()}>{s.brandLogoChoose}</Button>
      {value && (
        <Button variant="ghost" onClick={() => onValue('')}>
          {s.brandLogoRemove}
        </Button>
      )}
      {problem && <p className="w-full text-small text-danger">{problem}</p>}
    </div>
  )
}
