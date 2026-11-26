import type { ReactNode } from 'react'

/**
 * The Markdown of a message to Moonfin's apps, drawn as the apps draw it: CommonMark's headings,
 * paragraphs, lists, quotes, code and rules, with bold, italic, code and links inside them. Images
 * are left out, as the apps leave them out, and a link opens only an http or https address. It
 * builds elements, never HTML, so what the administrator types is never run as markup.
 */
export default function MessageMarkdown({ source }: { source: string }) {
  return <div className="space-y-2 break-words">{blocks(source.split(/\r?\n/), 'b')}</div>
}

/** Whether `url` is an http or https address with a host, the only links a message opens. */
export function webLink(url: string): boolean {
  try {
    const parsed = new URL(url.trim())
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && parsed.host !== ''
  } catch {
    return false
  }
}

const heading = /^ {0,3}(#{1,6})\s+(.*?)\s*#*\s*$/
const rule = /^ {0,3}([-*_])(\s*\1){2,}\s*$/
const fence = /^ {0,3}(```|~~~)/
const quote = /^ {0,3}>\s?/
const bullet = /^ {0,3}[-*+]\s+/
const numbered = /^ {0,3}(\d{1,9})[.)]\s+/

/** The size of each heading level, from the largest. */
const headingSizes = [
  'text-group',
  'text-h3',
  'text-h3',
  'text-body',
  'text-body',
  'text-small',
] as const

/** Draws lines as blocks, their keys starting with `key`. */
function blocks(lines: string[], key: string): ReactNode[] {
  const out: ReactNode[] = []
  let i = 0
  const k = () => `${key}-${out.length}`
  while (i < lines.length) {
    const line = lines[i]
    if (line.trim() === '') {
      i++
      continue
    }
    const open = fence.exec(line)
    if (open) {
      const code: string[] = []
      i++
      while (i < lines.length && !lines[i].trimStart().startsWith(open[1])) code.push(lines[i++])
      i++
      out.push(
        <pre
          key={k()}
          className="overflow-x-auto rounded-field bg-s2 px-3 py-2 font-mono text-small"
        >
          <code>{code.join('\n')}</code>
        </pre>,
      )
      continue
    }
    const title = heading.exec(line)
    if (title) {
      out.push(
        <p
          key={k()}
          role="heading"
          aria-level={title[1].length}
          className={`${headingSizes[title[1].length - 1]} font-semibold text-ink`}
        >
          {inline(title[2], k())}
        </p>,
      )
      i++
      continue
    }
    if (rule.test(line)) {
      out.push(<hr key={k()} className="border-ink/15" />)
      i++
      continue
    }
    if (quote.test(line)) {
      const quoted: string[] = []
      while (i < lines.length && quote.test(lines[i])) quoted.push(lines[i++].replace(quote, ''))
      out.push(
        <blockquote key={k()} className="border-l-2 border-ink/25 pl-3 text-ink-2">
          {blocks(quoted, k())}
        </blockquote>,
      )
      continue
    }
    const ordered = numbered.exec(line)
    if (ordered || bullet.test(line)) {
      const marker = ordered ? numbered : bullet
      const items: string[] = []
      while (i < lines.length && marker.test(lines[i])) {
        let item = lines[i++].replace(marker, '')
        // Lines indented under an item continue it.
        while (i < lines.length && /^\s{2,}\S/.test(lines[i])) item += ` ${lines[i++].trim()}`
        items.push(item)
      }
      const children = items.map((item, n) => <li key={n}>{inline(item, `${k()}-${n}`)}</li>)
      out.push(
        ordered ? (
          <ol key={k()} start={Number(ordered[1])} className="list-decimal space-y-1 pl-6">
            {children}
          </ol>
        ) : (
          <ul key={k()} className="list-disc space-y-1 pl-6">
            {children}
          </ul>
        ),
      )
      continue
    }
    // A paragraph runs to a blank line or another block; its lines join with spaces.
    const text: string[] = []
    while (
      i < lines.length &&
      lines[i].trim() !== '' &&
      (text.length === 0 ||
        !(
          fence.test(lines[i]) ||
          heading.test(lines[i]) ||
          quote.test(lines[i]) ||
          bullet.test(lines[i]) ||
          numbered.test(lines[i])
        ))
    ) {
      text.push(lines[i++].trim())
    }
    out.push(<p key={k()}>{inline(text.join(' '), k())}</p>)
  }
  return out
}

/** The inline marks, in the order they are looked for where two start at the same place. */
const marks: { pattern: RegExp; draw: (m: RegExpExecArray, key: string) => ReactNode }[] = [
  {
    pattern: /`([^`]+)`/,
    draw: (m, key) => (
      <code key={key} className="rounded-[0.3rem] bg-s2 px-1 font-mono text-small">
        {m[1]}
      </code>
    ),
  },
  // Images are left out.
  { pattern: /!\[[^\]]*\]\([^)]*\)/, draw: () => null },
  {
    pattern: /\[([^\]]+)\]\(\s*([^)\s]+)(?:\s+"[^"]*")?\s*\)/,
    draw: (m, key) => link(m[2], inline(m[1], key), key),
  },
  { pattern: /<(https?:\/\/[^>\s]+)>/, draw: (m, key) => link(m[1], m[1], key) },
  {
    pattern: /\*\*(?=\S)(.+?)(?<=\S)\*\*|__(?=\S)(.+?)(?<=\S)__/,
    draw: (m, key) => (
      <strong key={key} className="font-semibold">
        {inline(m[1] ?? m[2], key)}
      </strong>
    ),
  },
  {
    pattern: /\*(?=\S)([^*]+?)(?<=\S)\*|(?<!\w)_(?=\S)([^_]+?)(?<=\S)_(?!\w)/,
    draw: (m, key) => <em key={key}>{inline(m[1] ?? m[2], key)}</em>,
  },
]

/** A link to `url` when it is a web address, else its text alone. */
function link(url: string, text: ReactNode, key: string): ReactNode {
  if (!webLink(url)) return <span key={key}>{text}</span>
  return (
    <a
      key={key}
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      className="text-link underline underline-offset-2"
    >
      {text}
    </a>
  )
}

/** Draws the inline marks of `text`, their keys starting with `key`. */
function inline(text: string, key: string): ReactNode[] {
  const out: ReactNode[] = []
  let rest = text
  while (rest !== '') {
    let first: { at: number; m: RegExpExecArray; draw: (typeof marks)[number]['draw'] } | null =
      null
    for (const { pattern, draw } of marks) {
      const m = pattern.exec(rest)
      if (m && (first === null || m.index < first.at)) first = { at: m.index, m, draw }
    }
    if (first === null) {
      out.push(rest)
      break
    }
    if (first.at > 0) out.push(rest.slice(0, first.at))
    out.push(first.draw(first.m, `${key}-${out.length}`))
    rest = rest.slice(first.at + first.m[0].length)
  }
  return out
}
