// Checks the admin app's messages, and fails on a finding:
// - every English message is read by the code outside the message files;
// - every other language has the keys English has and no other, its lists
//   as long and its functions taking as many arguments.
//
// It needs the compiler API of TypeScript 5, which TypeScript 7, the admin
// app's, no longer has. Run it from web/ as
//   npx --yes --package=typescript@5.9.3 -- node scripts/i18n-check.mjs
// so that TypeScript 5's tsc comes first on PATH.
//
// A message counts as read when code reads it by name (t.users.title, a
// destructuring, t['users']), or when code takes an object holding it
// whole: indexes it with a variable (t.iptv.errors[code]), passes it,
// spreads it or keeps it in another variable first. The errors the server
// sends by code are read that way.
import { existsSync, readdirSync, realpathSync } from 'node:fs'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const i18nDir = path.join(web, 'src', 'i18n') + path.sep
const english = path.join(i18nDir, 'en') + path.sep
const languages = readdirSync(i18nDir, { withFileTypes: true })
  .filter((e) => e.isDirectory())
  .map((e) => e.name)
// The area files; index.ts only merges them for one language's import().
const areas = readdirSync(english)
  .filter((f) => f.endsWith('.ts') && f !== 'index.ts')
  .map((f) => f.slice(0, -3))

function typescript() {
  const dir = (process.env.PATH ?? '')
    .split(path.delimiter)
    .find((d) => existsSync(path.join(d, 'tsc')))
  if (dir) {
    const ts = createRequire(realpathSync(path.join(dir, 'tsc')))('typescript')
    if (ts.version.startsWith('5.')) return ts
  }
  console.error(
    'i18n-check needs TypeScript 5 first on PATH: npx --yes --package=typescript@5.9.3 -- node scripts/i18n-check.mjs',
  )
  process.exit(2)
}

// unread lists the English messages no code reads, as "file:line key".
function unread() {
  const ts = typescript()
  const config = ts.parseJsonConfigFileContent(
    ts.readConfigFile(path.join(web, 'tsconfig.json'), ts.sys.readFile).config,
    ts.sys,
    web,
  )
  const program = ts.createProgram(config.fileNames, config.options)
  const checker = program.getTypeChecker()

  // Each property of an English file, and the property holding each object.
  const props = new Map()
  const owners = new Map()
  const unwrap = (x) => {
    while (ts.isAsExpression(x) || ts.isSatisfiesExpression(x) || ts.isParenthesizedExpression(x))
      x = x.expression
    return x
  }
  function declare(literal, prefix, owner) {
    owners.set(literal, owner)
    for (const p of literal.properties) {
      if (!p.name) continue
      const key = prefix + p.name.getText()
      const value = ts.isPropertyAssignment(p) ? unwrap(p.initializer) : null
      const object = value && ts.isObjectLiteralExpression(value)
      const sf = p.getSourceFile()
      const line = sf.getLineAndCharacterOfPosition(p.getStart()).line + 1
      props.set(p, {
        key,
        at: `${path.relative(web, sf.fileName)}:${line}`,
        leaf: !object,
        read: false,
        whole: false,
        owner,
      })
      if (object) declare(value, key + '.', p)
    }
  }
  for (const sf of program.getSourceFiles()) {
    if (!sf.fileName.startsWith(english)) continue
    const name = path.basename(sf.fileName, '.ts')
    for (const statement of sf.statements) {
      if (!ts.isVariableStatement(statement)) continue
      for (const d of statement.declarationList.declarations) {
        const value = d.initializer && unwrap(d.initializer)
        if (d.name.getText() === name && value && ts.isObjectLiteralExpression(value))
          declare(value, '', null)
      }
    }
  }

  const declarationsOf = (symbol) => {
    if (!symbol) return []
    const symbols = [symbol, ...checker.getRootSymbols(symbol)]
    return symbols.flatMap((s) => s.declarations ?? []).filter((d) => props.has(d))
  }
  const read = (symbol) => {
    for (const d of declarationsOf(symbol)) props.get(d).read = true
  }
  const takeWhole = (d) => {
    if (d) props.get(d).whole = true
  }
  // The properties an expression may stand for, through variables,
  // conditionals and ?? or ||.
  function standsFor(x, depth = 0) {
    if (!x || depth > 5) return []
    x = unwrap(ts.isNonNullExpression(x) ? x.expression : x)
    if (ts.isPropertyAccessExpression(x)) return declarationsOf(checker.getSymbolAtLocation(x.name))
    if (ts.isElementAccessExpression(x))
      return declarationsOf(checker.getSymbolAtLocation(x.argumentExpression))
    if (ts.isIdentifier(x)) {
      const d = checker.getSymbolAtLocation(x)?.valueDeclaration
      return d && ts.isVariableDeclaration(d) && d.initializer
        ? standsFor(d.initializer, depth + 1)
        : []
    }
    if (ts.isConditionalExpression(x))
      return [...standsFor(x.whenTrue, depth + 1), ...standsFor(x.whenFalse, depth + 1)]
    if (ts.isBinaryExpression(x))
      return [...standsFor(x.left, depth + 1), ...standsFor(x.right, depth + 1)]
    return []
  }
  // Whether n, an expression, is only a step to a member or an alias.
  function passedThrough(n) {
    const p = n.parent
    return (
      (ts.isPropertyAccessExpression(p) && p.expression === n) ||
      (ts.isElementAccessExpression(p) &&
        p.expression === n &&
        ts.isStringLiteral(p.argumentExpression)) ||
      (ts.isVariableDeclaration(p) && p.initializer === n) ||
      ts.isTypeQueryNode(p) ||
      ts.isQualifiedName(p) ||
      ts.isPropertyAssignment(p) ||
      ts.isImportSpecifier(p)
    )
  }

  const source = path.join(web, 'src') + path.sep
  for (const sf of program.getSourceFiles()) {
    const file = sf.fileName
    if (!file.startsWith(source)) continue
    // The message files themselves are not readers; the rest of src/i18n
    // reads by name only, as it assembles the languages whole.
    if (file.startsWith(i18nDir) && path.dirname(file) + path.sep !== i18nDir) continue
    const byNameOnly = file.startsWith(i18nDir)
    ts.forEachChild(sf, function visit(n) {
      if (ts.isPropertyAccessExpression(n)) read(checker.getSymbolAtLocation(n.name))
      if (ts.isBindingElement(n) && ts.isObjectBindingPattern(n.parent)) {
        const type = checker.getTypeAtLocation(n.parent)
        const key = (n.propertyName ?? n.name).getText()
        for (const t of type.isUnion() ? type.types : [type]) read(t.getProperty(key))
      }
      if (ts.isElementAccessExpression(n) && ts.isStringLiteral(n.argumentExpression)) {
        const type = checker.getTypeAtLocation(n.expression)
        for (const t of type.isUnion() ? type.types : [type])
          read(t.getProperty(n.argumentExpression.text))
      }
      if (
        !byNameOnly &&
        (ts.isIdentifier(n) || ts.isPropertyAccessExpression(n)) &&
        !(ts.isPropertyAccessExpression(n.parent) && n.parent.name === n) &&
        !passedThrough(n)
      ) {
        const type = checker.getTypeAtLocation(n)
        for (const t of type.isUnion() ? type.types : [type]) {
          for (const d of t.getSymbol()?.declarations ?? [])
            if (owners.has(d)) takeWhole(owners.get(d))
        }
        for (const d of standsFor(n)) takeWhole(d)
      }
      ts.forEachChild(n, visit)
    })
  }

  const takenWhole = (p) => {
    for (let c = p; c; c = c.owner ? props.get(c.owner) : null) if (c.whole) return true
    return false
  }
  return [...props.values()]
    .filter((p) => p.leaf && !p.read && !takenWhole(p))
    .map((p) => `${p.at} ${p.key}`)
}

// mismatches lists where another language's messages differ in shape from
// English's.
async function mismatches() {
  const flatten = (value, prefix, out) => {
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      for (const [k, v] of Object.entries(value)) flatten(v, prefix ? `${prefix}.${k}` : k, out)
    } else out.set(prefix, value)
    return out
  }
  const load = async (language, area) =>
    flatten(
      (await import(pathToFileURL(path.join(i18nDir, language, area + '.ts')).href)).default,
      '',
      new Map(),
    )
  const found = []
  for (const area of areas) {
    const en = await load('en', area)
    for (const language of languages.filter((l) => l !== 'en')) {
      const other = await load(language, area)
      const at = `${language}/${area}.ts`
      for (const [key, value] of en) {
        if (!other.has(key)) {
          found.push(`${at}: ${key} is missing`)
          continue
        }
        const theirs = other.get(key)
        if (Array.isArray(value) && (!Array.isArray(theirs) || theirs.length !== value.length))
          found.push(
            `${at}: ${key} has ${Array.isArray(theirs) ? theirs.length : 'no'} items, English ${value.length}`,
          )
        if (
          typeof value === 'function' &&
          (typeof theirs !== 'function' || theirs.length !== value.length)
        )
          found.push(
            `${at}: ${key} takes ${typeof theirs === 'function' ? theirs.length : 'no'} arguments, English ${value.length}`,
          )
      }
      for (const key of other.keys())
        if (!en.has(key)) found.push(`${at}: ${key} is not in English`)
    }
  }
  return found
}

const notRead = unread()
const different = await mismatches()
for (const line of notRead) console.log(`unread: ${line}`)
for (const line of different) console.log(`differs: ${line}`)
if (notRead.length || different.length) {
  console.error(
    `${notRead.length} English messages no code reads, ${different.length} differences between languages`,
  )
  process.exit(1)
}
