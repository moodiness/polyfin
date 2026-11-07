import { fileURLToPath, URL } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const apiTarget = process.env.API_PROXY_TARGET || 'http://127.0.0.1:8096'

/**
 * Each icon of `@phosphor-icons/react` holds its six weights; the app draws only `regular`, and
 * `bold` in Checkbox. The build keeps those two: an icon drawn in another weight would show
 * nothing, so a weight the app starts using joins the list.
 */
const iconWeights = ['regular', 'bold']

function onlyIconWeights(): Plugin {
  return {
    name: 'polyfin-icon-weights',
    apply: 'build',
    transform(code, id) {
      if (!/@phosphor-icons\/react\/dist\/defs\/\w+\.es\.js$/.test(id)) return null
      // A Map of [weight, drawing] entries, each starting on a line of its own.
      const start = code.indexOf('new Map([') + 'new Map(['.length
      const end = code.lastIndexOf('\n]);')
      const entries = code.slice(start, end).split(/,(?=\n {2}\[\n)/)
      const kept = entries.filter((entry) => iconWeights.includes(/"(\w+)"/.exec(entry)?.[1] ?? ''))
      if (start < 'new Map(['.length || end < 0 || kept.length !== iconWeights.length) {
        throw new Error(`${id}: not the icon weights' expected layout`)
      }
      return { code: code.slice(0, start) + kept.join(',') + code.slice(end), map: null }
    },
  }
}

export default defineConfig({
  base: '/admin/',
  plugins: [react(), tailwindcss(), onlyIconWeights()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    proxy: {
      '/admin/api': { target: apiTarget, changeOrigin: false },
      '/healthz': { target: apiTarget, changeOrigin: false },
      '/readyz': { target: apiTarget, changeOrigin: false },
      // Posters and other artwork the pages show, served by the Jellyfin API.
      '/Items': { target: apiTarget, changeOrigin: false },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rolldownOptions: {
      // Everything the entry imports statically in one file, rather than a file per icon or
      // module the pages share with it: each is loaded at start anyway, and each file costs a
      // request. The languages, pages and palette entries stay files of their own.
      output: { codeSplitting: { groups: [{ name: 'shell', tags: ['$initial'] }] } },
    },
  },
})
