// The admin app's bundle budget, checked on the build in `dist/`, or the folder given (`npm run
// check:bundle`, after `npm run build`). It fails when the JavaScript the first page loads, the
// entry file with the files it imports, passes its gzip budget, or when one file holds the
// messages of more than one language: each language is a file of its own, loaded alone.
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'

/** In gzip bytes, at the best level as the server sends them: 188,704 when #990 set it. */
const entryBudget = 200_000

const web = fileURLToPath(new URL('..', import.meta.url))
const dist = process.argv[2] ?? join(web, 'dist')
const failures = []

// The entry file, with every file it imports statically, and so on: what the first page loads
// before the language and the page's own file.
const html = readFileSync(join(dist, 'index.html'), 'utf8')
const initial = new Set(
  [...html.matchAll(/<script type="module"[^>]* src="\/admin\/assets\/([^"]+\.js)"/g)].map(
    (match) => match[1],
  ),
)
for (const file of initial) {
  const code = readFileSync(join(dist, 'assets', file), 'utf8')
  for (const match of code.matchAll(/(?:\bfrom|\bimport)\s*["']\.\/([^"']+\.js)["']/g)) {
    initial.add(match[1])
  }
}
if (initial.size === 0) failures.push('index.html loads no module script')
const entryBytes = [...initial].reduce(
  (sum, file) => sum + gzipSync(readFileSync(join(dist, 'assets', file)), { level: 9 }).length,
  0,
)
console.log(
  `Entry: ${initial.size} files, ${entryBytes.toLocaleString('en')} bytes gzip, budget ${entryBudget.toLocaleString('en')}`,
)
if (entryBytes > entryBudget) {
  failures.push(`the entry's JavaScript is ${entryBytes} bytes gzip, past its ${entryBudget}`)
}

// Each language's own words: the strings of its messages no other language has, plain enough to
// be found as written among the minified files' string literals.
const languages = readdirSync(join(web, 'src/i18n'), { withFileTypes: true })
  .filter((entry) => entry.isDirectory())
  .map((entry) => entry.name)
const areas = readdirSync(join(web, 'src/i18n/en')).filter((file) => file !== 'index.ts')
const strings = new Map()
for (const language of languages) {
  const found = new Set()
  const collect = (value) => {
    if (typeof value === 'string') found.add(value)
    else if (value && typeof value === 'object') Object.values(value).forEach(collect)
  }
  for (const area of areas) collect((await import(join(web, 'src/i18n', language, area))).default)
  strings.set(language, found)
}
const markers = new Map(
  languages.map((language) => [
    language,
    [...strings.get(language)].filter(
      (text) =>
        text.length >= 12 &&
        text.includes(' ') &&
        !/[\\`$'"\n]/.test(text) &&
        languages.every((other) => other === language || !strings.get(other).has(text)),
    ),
  ]),
)
// Under this many of a language's own words, a file only happens to share a phrase with it.
const enough = 5
const seen = new Set()
for (const file of readdirSync(join(dist, 'assets')).filter((name) => name.endsWith('.js'))) {
  const code = readFileSync(join(dist, 'assets', file), 'utf8')
  const literals = new Set(
    [...code.matchAll(/`([^`\\$]*)`|"([^"\\]*)"|'([^'\\]*)'/g)].map((m) => m[1] ?? m[2] ?? m[3]),
  )
  const held = languages.filter(
    (language) => markers.get(language).filter((text) => literals.has(text)).length >= enough,
  )
  held.forEach((language) => seen.add(language))
  if (held.length > 1) failures.push(`${file} holds ${held.join(', ')}`)
}
// A language found in no file means this check no longer recognizes the built messages.
for (const language of languages) {
  if (!seen.has(language)) failures.push(`no file holds ${language}, as far as this check sees`)
}
console.log(`Languages: ${[...seen].join(', ')} found`)

for (const failure of failures) console.error(`Bundle budget: ${failure}`)
process.exit(failures.length > 0 ? 1 : 0)
