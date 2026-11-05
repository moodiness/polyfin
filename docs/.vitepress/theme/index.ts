import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme-without-fonts'
import { h } from 'vue'
import './style.css'
import Home from './Home.vue'
import Mermaid from './Mermaid.vue'
import NavInstall from './NavInstall.vue'
import { redirectMovedSections } from './redirects'

export default {
  extends: DefaultTheme,
  Layout: () =>
    h(DefaultTheme.Layout, null, {
      'nav-bar-content-after': () => h(NavInstall),
      'nav-screen-content-after': () => h(NavInstall, { screen: true }),
    }),
  async enhanceApp({ app, router, siteData }) {
    app.component('PolyfinHome', Home)
    app.component('PolyfinMermaid', Mermaid)
    await redirectMovedSections(router, siteData.value.base)
  },
} satisfies Theme
