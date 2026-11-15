// The large-library bench: seeds the bench database (or reuses it), replays the requests
// jellyfin-web sends for the home page, the live TV guide and search, cold and warm, prints the
// times, and exits with 1 when a screen's warm p95 passes its budget. See README.md.
import { readFile, writeFile } from 'node:fs/promises'
import {
  jellyfinClient,
  log,
  options,
  run,
  screensFile,
  sleep,
  startStage,
  viewer,
} from './lib.mjs'
import { percentile, replayScreen, requestName } from './replay.mjs'

await run(async () => {
  const values = options({
    runs: { type: 'string', default: '20' },
    'cold-runs': { type: 'string', default: '20' },
    'during-import': { type: 'boolean', default: false },
    requests: { type: 'boolean', default: false },
    json: { type: 'string' },
  })
  const file = JSON.parse(await readFile(screensFile, 'utf8'))
  const runs = Number(values.runs)
  const coldRuns = Number(values['cold-runs'])
  const results = { importing: null, settled: null }

  if (values['during-import']) values.fresh = true
  const stage = await startStage(values, {
    onImporting: values['during-import']
      ? ({ polyfin, settled, since }) =>
          measureWhileImporting(file, polyfin, settled, since, results)
      : undefined,
  })
  const { polyfin } = stage
  log(`Polyfin at ${polyfin.url}, pid ${polyfin.pid}`)

  const client = await signIn(polyfin)
  results.settled = { screens: {}, process: {} }
  const settled = results.settled
  for (const screen of file.screens) {
    settled.screens[screen.name] = { warm: [], cold: [] }
  }

  log(`Warm: ${runs} runs of each screen, after one to warm up`)
  await measured(polyfin, settled.process, 'warm', async () => {
    for (const screen of file.screens) {
      await replayScreen(screen, client)
      for (let i = 0; i < runs; i++) {
        settled.screens[screen.name].warm.push(await replayScreen(screen, client))
      }
    }
  })
  if (coldRuns > 0) {
    log(`Cold: ${coldRuns} runs of each screen, each right after a restart`)
    await measured(polyfin, settled.process, 'cold', async () => {
      for (let i = 0; i < coldRuns; i++) {
        for (const screen of file.screens) {
          await polyfin.restart()
          settled.screens[screen.name].cold.push(await replayScreen(screen, client))
        }
      }
    })
  }

  if (results.importing) {
    console.log('\nWhile the first import runs')
    report(file, results.importing, values.requests)
  }
  console.log('\nSettled')
  const failed = report(file, settled, values.requests, true)
  console.log(
    `\nPolyfin: ${describeProcess(settled.process.warm, 'warm')}` +
      (settled.process.cold ? `; ${describeProcess(settled.process.cold, 'cold')}` : ''),
  )
  if (values.json) {
    await writeFile(
      values.json,
      JSON.stringify({ seed: stage.state, screens: file.screens, results }, null, 2),
    )
    log(`Wrote ${values.json}`)
  }
  if (failed.length) {
    console.log(`\nOver budget: ${failed.join(', ')}`)
    return 1
  }
  return 0
})

/** jellyfin-web's client, signed in as the bench's viewer, for the replays. */
async function signIn(polyfin) {
  const client = await jellyfinClient(polyfin.url, viewer)
  return { ...client, base: polyfin.url }
}

/**
 * Replays the screens in turn while the first import runs, until it settles: the first run of
 * each is cold, Polyfin having just started, the next ones warm.
 */
async function measureWhileImporting(file, polyfin, settled, since, results) {
  const client = await signIn(polyfin)
  const importing = { screens: {}, process: {} }
  for (const screen of file.screens) importing.screens[screen.name] = { warm: [], cold: [] }
  results.importing = importing
  let done = false
  const imported = settled.then((result) => {
    done = true
    return result
  })
  await measured(polyfin, importing.process, 'warm', async () => {
    for (let round = 0; !done; round++) {
      for (const screen of file.screens) {
        if (done) break
        const result = await replayScreen(screen, client)
        result.at = (Date.now() - since) / 1000
        importing.screens[screen.name][round === 0 ? 'cold' : 'warm'].push(result)
      }
      await sleep(250)
    }
  })
  // The import ended when what it stores last changed: later runs are the settled server's.
  const { seconds } = await imported
  importing.seconds = seconds
  for (const runs of Object.values(importing.screens)) {
    runs.warm = runs.warm.filter((run) => run.at - run.ms / 1000 <= seconds)
  }
}

/** Runs task, keeping Polyfin's processor use and peak memory under process[name]. */
async function measured(polyfin, process, name, task) {
  const samples = []
  let before = polyfin.usage()
  const started = performance.now()
  let cpu = 0
  let pid = polyfin.pid
  const sample = () => {
    try {
      const now = polyfin.usage()
      if (polyfin.pid !== pid) {
        // Restarted: the new process starts from zero.
        pid = polyfin.pid
        cpu += now.cpu
      } else {
        cpu += now.cpu - before.cpu
      }
      before = now
      samples.push(now.rss)
    } catch {
      // Restarting.
    }
  }
  const timer = setInterval(sample, 500)
  try {
    await task()
  } finally {
    clearInterval(timer)
    sample()
  }
  const seconds = (performance.now() - started) / 1000
  process[name] = {
    seconds,
    cpuSeconds: cpu,
    cores: cpu / seconds,
    rssPeakMb: Math.max(...samples),
    rssLastMb: samples.at(-1),
  }
}

function describeProcess(p, name) {
  if (!p) return ''
  return `${name}, ${p.cpuSeconds.toFixed(1)} s of CPU in ${p.seconds.toFixed(0)} s (${(p.cores * 100).toFixed(0)} % of a core), memory up to ${p.rssPeakMb.toFixed(0)} MB`
}

function ms(value) {
  return Number.isNaN(value) ? '' : value.toFixed(0)
}

function highest(numbers) {
  return numbers.length ? Math.max(...numbers) : NaN
}

/** Prints the screens' table, and each request's when asked; returns the screens over budget. */
function report(file, measured, requests, budgets = false) {
  const rows = []
  const failed = []
  for (const screen of file.screens) {
    const { warm, cold } = measured.screens[screen.name]
    const times = (list) => list.map((run) => run.ms)
    const warmP95 = percentile(times(warm), 95)
    const budget = screen.budget?.warmP95
    const over = budgets && budget && warmP95 > budget
    if (over) failed.push(screen.name)
    rows.push([
      screen.name,
      String(screen.waves.length),
      String(screen.waves.flat().length),
      ms(percentile(times(cold), 50)),
      ms(percentile(times(cold), 95)),
      ms(highest(times(cold))),
      ms(percentile(times(warm), 50)),
      ms(warmP95),
      ms(highest(times(warm))),
      String(warm.length),
      budgets ? (budget ? `${budget}${over ? ' OVER' : ''}` : '') : '',
    ])
  }
  table(
    [
      'Screen',
      'Waves',
      'Requests',
      'Cold p50',
      'Cold p95',
      'Cold max',
      'Warm p50',
      'Warm p95',
      'Warm max',
      'Runs',
      'Budget',
    ],
    rows,
  )
  if (requests) {
    const lines = []
    for (const screen of file.screens) {
      const { warm, cold } = measured.screens[screen.name]
      screen.waves.forEach((wave, w) => {
        for (const request of wave) {
          const of = (list) =>
            list.flatMap((run) => run.requests.filter((r) => r.index === request.index))
          const warmTimes = of(warm).map((r) => r.ms)
          const coldTimes = of(cold).map((r) => r.ms)
          const statuses = [...new Set([...of(warm), ...of(cold)].map((r) => r.status))]
          lines.push([
            screen.name,
            String(w + 1),
            requestName(request).slice(0, 90),
            ms(percentile(coldTimes, 50)),
            ms(percentile(coldTimes, 95)),
            ms(highest(coldTimes)),
            ms(percentile(warmTimes, 50)),
            ms(percentile(warmTimes, 95)),
            ms(highest(warmTimes)),
            statuses.join(','),
          ])
        }
      })
    }
    console.log('')
    table(
      [
        'Screen',
        'Wave',
        'Request',
        'Cold p50',
        'Cold p95',
        'Cold max',
        'Warm p50',
        'Warm p95',
        'Warm max',
        'Status',
      ],
      lines,
    )
  }
  return failed
}

/** Prints rows under headers, numbers aligned right. */
function table(headers, rows) {
  const widths = headers.map((header, i) =>
    Math.max(header.length, ...rows.map((row) => row[i].length)),
  )
  const line = (cells) =>
    cells
      .map((cell, i) => (/^[\d.]*$/.test(cell) ? cell.padStart(widths[i]) : cell.padEnd(widths[i])))
      .join('  ')
      .trimEnd()
  console.log(line(headers))
  console.log(widths.map((width) => '-'.repeat(width)).join('  '))
  for (const row of rows) console.log(line(row))
}
