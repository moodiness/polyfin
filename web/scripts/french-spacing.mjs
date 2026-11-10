// Sets French punctuation apart with a narrow no-break space (U+202F), so
// that a line never breaks before the sign: a plain space or a no-break
// space (U+00A0) before :, ;, ?, ! and », and after «, becomes U+202F,
// written \u202f. A sign with no space before it gets none ("10:30"), and
// a sign a letter or a digit follows is syntax, not punctuation, and keeps
// its space ("Ajoutez :port").
//
// It rewrites only the contents of strings, never code: the string literals
// and the text of the template literals of the admin app's French messages
// (src/i18n/fr), the strings of the French dictionaries (the fr properties)
// of the web player's script (internal/server/webclient.js), then the
// French texts of the Go code, through go generate ./internal/i18n (see
// TestFrenchSpacing), and the minified script through go generate
// ./internal/server. Running it again changes nothing.
//
// It needs the compiler API of TypeScript 5, as i18n-check.mjs does. Run it
// from web/, after a change of French messages, as
//   npx --yes --package=typescript@5.9.3 -- node scripts/french-spacing.mjs
//
// i18n-check.mjs fails on a French message the script would change, and
// TestFrenchSpacing on a Go text.
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, readdirSync, realpathSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const root = path.resolve(web, '..')
const french = path.join(web, 'src', 'i18n', 'fr')
const webClient = path.join(root, 'internal', 'server', 'webclient.js')

// typescript is TypeScript 5's compiler API, from the tsc first on PATH, as
// npx --package=typescript@5.9.3 puts it. script names the script to run so.
export function typescript(script) {
  const dir = (process.env.PATH ?? '')
    .split(path.delimiter)
    .find((d) => existsSync(path.join(d, 'tsc')))
  if (dir) {
    const ts = createRequire(realpathSync(path.join(dir, 'tsc')))('typescript')
    if (ts.version.startsWith('5.')) return ts
  }
  console.error(
    `${script} needs TypeScript 5 first on PATH: npx --yes --package=typescript@5.9.3 -- node scripts/${script}`,
  )
  process.exit(2)
}

// A space as a string's source may write it: plain, no-break (the
// character or its escapes), or narrow as the character or \u202F rather
// than \u202f.
const space = String.raw`(?: |\u00a0|\\u00[aA]0|\\x[aA]0|\u202f|\\u202F)`
// A sign a letter or a digit follows is not punctuation but syntax, as in
// "Ajoutez :port", and keeps its space.
const sign = String.raw`(?:[:;?!](?![\p{L}\p{N}])|»)`
const before = new RegExp(String.raw`(?<!\\)${space}(?=${sign})`, 'gu')
const after = new RegExp(String.raw`«${space}`, 'gu')

// narrow rewrites the source of a string, its quotes included, and counts
// the spaces it changed.
function narrow(source) {
  let count = 0
  const result = source
    .replace(before, () => (count++, String.raw`\u202f`))
    .replace(after, () => (count++, String.raw`«\u202f`))
  return { result, count }
}

// strings lists the strings of a file that hold French text, with the key
// of the property each belongs to.
function strings(ts, file) {
  const sf = ts.createSourceFile(
    file,
    readFileSync(file, 'utf8'),
    ts.ScriptTarget.Latest,
    true,
    file.endsWith('.js') ? ts.ScriptKind.JS : ts.ScriptKind.TS,
  )
  const isText = (n) =>
    ts.isStringLiteral(n) ||
    ts.isNoSubstitutionTemplateLiteral(n) ||
    ts.isTemplateHead(n) ||
    ts.isTemplateMiddle(n) ||
    ts.isTemplateTail(n)
  // Strings that are code: names of properties, operands of comparisons,
  // cases, indexes, modules and types.
  const isCode = (n) => {
    const p = n.parent
    return (
      (ts.isPropertyAssignment(p) && p.name === n) ||
      (ts.isBinaryExpression(p) &&
        [
          ts.SyntaxKind.EqualsEqualsEqualsToken,
          ts.SyntaxKind.ExclamationEqualsEqualsToken,
          ts.SyntaxKind.EqualsEqualsToken,
          ts.SyntaxKind.ExclamationEqualsToken,
          ts.SyntaxKind.InKeyword,
        ].includes(p.operatorToken.kind)) ||
      ts.isCaseClause(p) ||
      (ts.isElementAccessExpression(p) && p.argumentExpression === n) ||
      ts.isImportDeclaration(p) ||
      ts.isExportDeclaration(p) ||
      ts.isLiteralTypeNode(p)
    )
  }
  // The path of properties to n, as t reads a message (versions.orderNone);
  // in webclient.js, from the variable holding them (words.fr.none).
  const key = (n) => {
    const names = []
    for (let p = n.parent; p; p = p.parent) {
      if ((ts.isPropertyAssignment(p) || ts.isMethodDeclaration(p)) && p.name)
        names.unshift(ts.isIdentifier(p.name) ? p.name.text : p.name.getText().slice(1, -1))
      if (ts.isVariableDeclaration(p)) {
        if (file.endsWith('.js') && ts.isIdentifier(p.name)) names.unshift(p.name.text)
        break
      }
    }
    return names.join('.')
  }
  // In webclient.js, only the values of the fr properties are French.
  const inFrench = file.endsWith('.js')
    ? (n) => {
        for (let p = n.parent; p; p = p.parent)
          if (ts.isPropertyAssignment(p) && p.name.getText() === 'fr') return true
        return false
      }
    : () => true
  const found = []
  ts.forEachChild(sf, function visit(n) {
    if (isText(n) && !isCode(n) && inFrench(n)) found.push({ node: n, key: key(n) })
    ts.forEachChild(n, visit)
  })
  return { sf, found }
}

// frenchFiles are the files whose strings frenchSpacing reads.
const frenchFiles = () => [
  ...readdirSync(french)
    .filter((f) => f.endsWith('.ts') && f !== 'index.ts')
    .sort()
    .map((f) => path.join(french, f)),
  webClient,
]

// frenchSpacing lists the French strings the script would change, as
// { at, key, source }, source being the string as written: those with a
// plain or a no-break space where a narrow one belongs, or with a narrow
// one written as the character or \u202F; with write, it rewrites them and
// returns the number of spaces changed by file instead.
export function frenchSpacing(ts, write = false) {
  const findings = []
  const changed = new Map()
  for (const file of frenchFiles()) {
    const { sf, found } = strings(ts, file)
    let text = sf.text
    let count = 0
    // From the end, so that the positions before stay right.
    for (const { node, key } of found.reverse()) {
      const start = node.getStart(sf)
      const { result, count: n } = narrow(text.slice(start, node.end))
      if (n === 0) continue
      count += n
      if (write) text = text.slice(0, start) + result + text.slice(node.end)
      else
        findings.unshift({
          at: `${path.relative(web, file)}:${sf.getLineAndCharacterOfPosition(start).line + 1}`,
          key,
          source: text.slice(start, node.end),
        })
    }
    if (count > 0) {
      changed.set(path.relative(root, file), count)
      if (write) writeFileSync(file, text)
    }
  }
  return write ? changed : findings
}

if (process.argv[1] && realpathSync(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const changed = frenchSpacing(typescript('french-spacing.mjs'), true)
  for (const [file, count] of changed) console.log(`${file}: ${count} spaces`)
  // An escape is longer than the space it writes: the admin app's
  // formatting may break a line anew.
  const messages = [...changed.keys()].filter((f) => f.startsWith('web' + path.sep))
  if (messages.length > 0)
    execFileSync(path.join(web, 'node_modules', '.bin', 'prettier'), ['--write', ...messages], {
      cwd: root,
      stdio: 'inherit',
    })
  const go = (pkg) => execFileSync('go', ['generate', pkg], { cwd: root, stdio: 'inherit' })
  go('./internal/i18n')
  if (changed.has(path.relative(root, webClient))) go('./internal/server')
}
