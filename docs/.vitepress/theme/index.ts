import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme-without-fonts'
import { h } from 'vue'
import './style.css'
import Home from './Home.vue'
import NavInstall from './NavInstall.vue'

export default {
  extends: DefaultTheme,
  Layout: () =>
    h(DefaultTheme.Layout, null, {
      'nav-bar-content-after': () => h(NavInstall),
      'nav-screen-content-after': () => h(NavInstall, { screen: true }),
    }),
  enhanceApp({ app }) {
    app.component('PolyfinHome', Home)
  },
} satisfies Theme
