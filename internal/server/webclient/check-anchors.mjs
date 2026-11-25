// Checks the anchors of the web player script: what each of its files says
// it relies on in jellyfin-web, in the list under its description (see
// docs/development.md). Usage, from the repository's root:
//
//   node internal/server/webclient/check-anchors.mjs [folder [file...]]
//
// Without a folder, it reads the lists, fails on one it cannot read, and
// counts the anchors. With the folder of jellyfin-web's built files, such
// as the one scripts/packaging/files.sh takes from the Dockerfile's
// jellyfin-web stage (dist/packaging/files/jellyfin-web), it also looks
// for each anchor there, and names each file with the anchors it misses:
// what a new jellyfin-web broke. Files named after the folder, such as
// 02-versions.js, are checked alone. It exits 1 when anchors are missing,
// 2 when a list cannot be read or the folder holds no jellyfin-web.
//
// An anchor is a line "//   kind: value" of the list, which starts with
// "// Anchors in jellyfin-web:" ("// Anchors in jellyfin-web: none." for a
// file that relies on nothing). A built file is found to hold it as its
// kind says:
//   route     a route of jellyfin-web's router, such as details or
//             dashboard/users: a string, its leading #, ! or / aside, that
//             is the route or starts with it and then /, ? or #;
//   selector  a CSS selector the script looks for: each class, id and
//             attribute it names is found, its values in quotes aside;
//   class     a class jellyfin-web gives elements: a word of the built
//             files, or one MUI makes from a component and a slot, such as
//             MuiAppBar-root, from the strings MuiAppBar and root;
//   id, property
//             a word of the built files: an id jellyfin-web gives an
//             element, a property it gives an object or an element;
//   global    a global of jellyfin-web, then the members the script uses,
//             such as ApiClient.getItems: each a word of the built files;
//   event     the name of an event jellyfin-web fires: a string;
//   storage   a key or a database jellyfin-web keeps in the browser: a
//             word of the built files;
//   text      a text found as it is in the built files.
// Words are made of letters, digits, _, $ and -, so that a class such as
// emby-select-withcolor is one; strings are those in quotes in the built
// scripts.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const heading = '// Anchors in jellyfin-web:'
const kinds = ['route', 'selector', 'class', 'id', 'property', 'global', 'event', 'storage', 'text']

// files lists the script's files under folder, relative to it, sorted.
function files(folder, extensions) {
  return readdirSync(folder, { recursive: true })
    .filter((name) => extensions.some((extension) => name.endsWith(extension)))
    .filter((name) => statSync(path.join(folder, name)).isFile())
    .sort()
}

// declared reads the list of each file of the script: [{file, anchors}],
// anchors being [{kind, value, line}]. It throws on a list it cannot read.
function declared(only) {
  const names = files(here, ['.js']).filter((file) => only.length === 0 || only.includes(file))
  if (names.length < only.length) throw new Error(`no such file among ${only.join(', ')}`)
  return names.map((file) => {
    const lines = readFileSync(path.join(here, file), 'utf8').split('\n')
    const at = lines.findIndex((line) => line.startsWith(heading))
    if (at < 0) throw new Error(`${file} has no line "${heading}"`)
    const rest = lines[at].slice(heading.length)
    if (rest === ' none.') return { file, anchors: [] }
    if (rest !== '') throw new Error(`${file}:${at + 1}: "${heading}" is followed by "${rest}"`)
    const anchors = []
    for (let i = at + 1; i < lines.length && lines[i].startsWith('//   '); i++) {
      const match = /^\/\/ {3}([a-z]+): (\S.*)$/.exec(lines[i])
      if (!match || !kinds.includes(match[1]))
        throw new Error(`${file}:${i + 1}: not an anchor, "kind: value" of a kind among ${kinds.join(', ')}`)
      anchors.push({ kind: match[1], value: match[2].trim(), line: i + 1 })
    }
    if (anchors.length === 0) throw new Error(`${file}:${at + 1}: no anchor follows "${heading}"`)
    return { file, anchors }
  })
}

// builtFiles reads jellyfin-web's built files in folder: its words, its
// strings, and its whole text.
function builtFiles(folder) {
  const names = files(folder, ['.js', '.css', '.html', '.json'])
  if (!names.includes('index.html')) throw new Error(`${folder} holds no index.html of jellyfin-web`)
  const words = new Set()
  const strings = new Set()
  const texts = []
  for (const name of names) {
    const text = readFileSync(path.join(folder, name), 'utf8')
    texts.push(text)
    for (const word of text.match(/[\w$-]+/g) ?? []) words.add(word)
    if (!name.endsWith('.js')) continue
    for (const quoted of text.matchAll(/"((?:[^"\\\n]|\\.)*)"|'((?:[^'\\\n]|\\.)*)'|`((?:[^`\\$]|\\.)*)`/g))
      strings.add(quoted[1] ?? quoted[2] ?? quoted[3])
  }
  return { words, strings, text: texts.join('\n') }
}

// missing returns what of an anchor the built files lack: [] when they
// hold it, else the words or strings not found.
function missing(anchor, built) {
  const word = (w) => (built.words.has(w) ? [] : [w])
  const className = (name) => {
    if (built.words.has(name)) return []
    const mui = /^(Mui[A-Za-z]*)-([A-Za-z]+)$/.exec(name)
    if (mui && (mui[1] === 'Mui' || built.strings.has(mui[1])) && built.strings.has(mui[2])) return []
    return [name]
  }
  switch (anchor.kind) {
    case 'route': {
      const route = anchor.value.toLowerCase()
      for (const s of built.strings) {
        const plain = s.toLowerCase().replace(/^#?!?\/?/, '')
        if (plain === route || (plain.startsWith(route) && '/?#'.includes(plain[route.length]))) return []
      }
      return [anchor.value]
    }
    case 'selector': {
      const names = [...anchor.value.replace(/"[^"]*"|'[^']*'/g, '').matchAll(/([.#[])([\w-]+)/g)]
      if (names.length === 0) return [`${anchor.value} names no class, id or attribute`]
      return names.flatMap((m) => (m[1] === '.' ? className(m[2]) : word(m[2])))
    }
    case 'class':
      return className(anchor.value)
    case 'global':
      return anchor.value.split('.').flatMap(word)
    case 'event':
      return built.strings.has(anchor.value) ? [] : [anchor.value]
    case 'text':
      return built.text.includes(anchor.value) ? [] : [anchor.value]
    default:
      return word(anchor.value)
  }
}

function main() {
  let parts
  try {
    parts = declared(process.argv.slice(3))
  } catch (error) {
    console.error(`check-anchors: ${error.message}`)
    return 2
  }
  const count = parts.reduce((sum, part) => sum + part.anchors.length, 0)
  const folder = process.argv[2]
  if (!folder) {
    console.log(`${count} anchors declared by ${parts.length} files of the web player script.`)
    return 0
  }
  let built
  try {
    built = builtFiles(folder)
  } catch (error) {
    console.error(`check-anchors: ${error.message}`)
    return 2
  }
  const failures = []
  for (const part of parts) {
    const lacks = part.anchors
      .map((anchor) => ({ anchor, lacking: missing(anchor, built) }))
      .filter(({ lacking }) => lacking.length > 0)
    if (lacks.length > 0) failures.push({ part, lacks })
  }
  if (failures.length === 0) {
    console.log(`${count} anchors of ${parts.length} files of the web player script, all found in ${folder}.`)
    return 0
  }
  const lost = failures.reduce((sum, failure) => sum + failure.lacks.length, 0)
  console.error(`jellyfin-web in ${folder} lacks ${lost} of the ${count} anchors of the web player script:`)
  for (const { part, lacks } of failures) {
    console.error(`  internal/server/webclient/${part.file}:`)
    for (const { anchor, lacking } of lacks) {
      const detail = lacking.length === 1 && lacking[0] === anchor.value ? '' : ` (${lacking.join(', ')} not found)`
      console.error(`    line ${anchor.line}, ${anchor.kind}: ${anchor.value}${detail}`)
    }
  }
  return 1
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) process.exitCode = main()
