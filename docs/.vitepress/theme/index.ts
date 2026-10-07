import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme-without-fonts'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import './style.css'
import Home from './Home.vue'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('PolyfinHome', Home)
  },
} satisfies Theme
