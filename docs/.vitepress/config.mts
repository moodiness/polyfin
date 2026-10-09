import { existsSync, readFileSync, statSync } from 'node:fs'
import { dirname, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, type DefaultTheme } from 'vitepress'

const repo = 'https://github.com/moodiness/polyfin'
const site = 'https://moodiness.github.io/polyfin/'
const docsDir = fileURLToPath(new URL('..', import.meta.url))
const repoDir = resolve(docsDir, '..')

/** docs/README.md is the documentation's index on GitHub: each of its sections is a sidebar group. */
function sidebar(): DefaultTheme.SidebarItem[] {
  const items: DefaultTheme.SidebarItem[] = [{ text: 'Overview', link: '/docs/' }]
  for (const line of readFileSync(resolve(docsDir, 'README.md'), 'utf8').split('\n')) {
    const section = /^## (.+)/.exec(line)
    const page = /^\| \[(.+?)\]\(([\w-]+)\.md\) \|/.exec(line)
    if (section) items.push({ text: section[1], items: [] })
    else if (page && items.length > 1) {
      items.at(-1)!.items!.push({ text: page[1], link: `/docs/${page[2]}` })
    }
  }
  return items
}

/** The first paragraph of a page, as plain text, for search engines and link previews. */
function firstParagraph(file: string): string | undefined {
  const block = readFileSync(resolve(docsDir, file), 'utf8')
    .split(/\n\s*\n/)
    .find((text) => /^[A-Z]/.test(text.trim()))
  return block
    ?.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    .replace(/[`*]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

export default defineConfig({
  lang: 'en-US',
  title: 'Polyfin',
  description:
    'A self-hosted, Jellyfin-compatible server for Stremio addons, music addons and IPTV, with real user accounts and transcoding.',
  base: '/polyfin/',
  cleanUrls: true,
  appearance: 'force-dark',
  sitemap: { hostname: site },

  // The landing page stays at the root. The documentation, written to be read on GitHub too,
  // is served under /docs/, with docs/README.md as its home.
  rewrites: (page) =>
    page === 'index.md' ? page : `docs/${page === 'README.md' ? 'index.md' : page}`,

  markdown: {
    // Links in the documentation are relative so that they work on GitHub. Those that leave docs/
    // open the file on GitHub, and those to docs/README.md open the documentation's home.
    config(md) {
      md.core.ruler.push('polyfin_repository_links', (state) => {
        const source: string = state.env.realPath ?? state.env.path
        for (const link of state.tokens.flatMap((token) => token.children ?? [])) {
          const href = link.type === 'link_open' ? link.attrGet('href') : null
          if (!href || /^(?:[a-z][\w+.-]*:|\/|#)/i.test(href)) continue
          const hashAt = href.indexOf('#')
          const path = hashAt < 0 ? href : href.slice(0, hashAt)
          const hash = hashAt < 0 ? '' : href.slice(hashAt)
          const target = resolve(dirname(source), decodeURIComponent(path))
          const fromDocs = relative(docsDir, target)
          if (fromDocs === 'README.md') {
            link.attrSet('href', `index.md${hash}`)
          } else if (fromDocs.startsWith('..')) {
            if (!existsSync(target)) {
              throw new Error(`${relative(repoDir, source)} links to ${href}, which does not exist`)
            }
            const kind = statSync(target).isDirectory() ? 'tree' : 'blob'
            const fromRepo = relative(repoDir, target).split(sep).join('/')
            link.attrSet('href', `${repo}/${kind}/main/${fromRepo}${hash}`)
          }
        }
      })
    },
  },

  head: [
    ['link', { rel: 'icon', href: '/polyfin/polyfin.ico', sizes: 'any' }],
    ['link', { rel: 'icon', href: '/polyfin/polyfin.svg', type: 'image/svg+xml' }],
    ['link', { rel: 'apple-touch-icon', href: '/polyfin/polyfin.png' }],
    ['meta', { name: 'theme-color', content: '#08080e' }],
    ['meta', { property: 'og:site_name', content: 'Polyfin' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:image', content: `${site}polyfin-social.png` }],
    ['meta', { property: 'og:image:width', content: '1280' }],
    ['meta', { property: 'og:image:height', content: '640' }],
    ['meta', { property: 'og:image:alt', content: 'Polyfin' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
  ],

  transformPageData(pageData) {
    if (pageData.frontmatter.layout !== 'home' && !pageData.frontmatter.description) {
      pageData.description = firstParagraph(pageData.filePath) ?? pageData.description
    }
  },

  transformHead({ pageData, title, description }) {
    const url = site + pageData.relativePath.replace(/(^|\/)index\.md$/, '$1').replace(/\.md$/, '')
    return [
      ['link', { rel: 'canonical', href: url }],
      ['meta', { property: 'og:url', content: url }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
    ]
  },

  themeConfig: {
    logo: { src: '/polyfin.svg', width: 28, height: 28, alt: '' },
    nav: [
      { text: 'Docs', link: '/docs/', activeMatch: '^/docs/' },
      { text: 'Releases', link: `${repo}/releases` },
      { text: 'Community', link: `${repo}/discussions` },
    ],
    socialLinks: [{ icon: 'github', link: repo, ariaLabel: 'Polyfin on GitHub' }],
    sidebar: { '/docs/': sidebar() },
    outline: { level: [2, 3] },
    editLink: { pattern: `${repo}/edit/main/docs/:path`, text: 'Edit this page on GitHub' },
    search: { provider: 'local' },
    notFound: {
      quote: 'This page does not exist, or it has moved.',
      linkText: 'Back to Polyfin',
    },
  },
})
