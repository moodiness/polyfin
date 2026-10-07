<script setup lang="ts">
import { withBase } from 'vitepress'
import { ref } from 'vue'
import FeatureViewer from './FeatureViewer.vue'
import HeroMedia from './HeroMedia.vue'
import Icon from './Icon.vue'
import { apps, extras, repository } from './home'
import { icons } from './icons'

/** The README's quick start. */
const commands = [
  'git clone https://github.com/moodiness/polyfin.git',
  'cd polyfin',
  'cp .env.example .env',
  '# set POSTGRES_PASSWORD in .env: openssl rand -hex 24 makes one',
  'docker compose up -d',
]

const copied = ref(false)
let copiedTimer: ReturnType<typeof setTimeout> | undefined

async function copyCommands() {
  await navigator.clipboard.writeText(commands.join('\n'))
  copied.value = true
  clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => (copied.value = false), 2000)
}
</script>

<template>
  <div class="pf-home">
    <div class="glow" aria-hidden="true"></div>

    <section class="container hero">
      <h1>Your addons and IPTV, in any Jellyfin app.</h1>
      <p class="lead">
        Polyfin turns Stremio addons, music addons and IPTV into a self-hosted, Jellyfin-compatible
        server, with real accounts and transcoding.
      </p>
      <div class="actions">
        <a class="button primary" href="#install">Install</a>
        <a class="button" :href="withBase('/docs/')">Read the docs</a>
      </div>
      <HeroMedia class="hero-media" />
      <p class="apps">
        Works with
        <template v-for="(app, index) in apps" :key="app">
          <strong>{{ app }}</strong
          >{{ index < apps.length - 1 ? ', ' : '' }}
        </template>
        and the <strong>official Jellyfin apps</strong>.
        <a :href="withBase('/docs/jellyfin-compatibility')">Compatibility</a>
      </p>
    </section>

    <section class="container section" aria-labelledby="features-title">
      <h2 id="features-title">Everything your apps expect from a server.</h2>
      <p class="section-lead">
        Libraries, accounts, transcoding, Live TV and music, all through the Jellyfin API.
      </p>
      <FeatureViewer class="viewer" />
    </section>

    <section class="container section" aria-labelledby="extras-title">
      <h2 id="extras-title">Also built in</h2>
      <ul class="extras">
        <li v-for="extra in extras" :key="extra.name">
          <a :href="withBase(extra.link)">
            <span class="extra-icon"><Icon :svg="extra.icon" /></span>
            <span class="extra-name">{{ extra.name }}</span>
            <span class="extra-text">{{ extra.text }}</span>
          </a>
        </li>
      </ul>
    </section>

    <section id="install" class="container section install" aria-labelledby="install-title">
      <div class="install-copy">
        <h2 id="install-title">Run it with Docker Compose.</h2>
        <p class="section-lead">
          Docker with Compose v2 is all it needs. The image is published for linux/amd64 and
          linux/arm64.
        </p>
        <ol class="steps">
          <li>Open <code>http://&lt;server&gt;:8096/admin/</code>.</li>
          <li>
            Enter the one-time setup code from <code>docker compose logs polyfin</code> to create
            the administrator.
          </li>
          <li>Continue with <a :href="withBase('/docs/getting-started')">Getting started</a>.</li>
        </ol>
        <p class="more">
          Unraid, GPUs, pinned versions and builds from source are in the
          <a :href="withBase('/docs/installation')">installation guide</a>.
        </p>
      </div>
      <div class="code">
        <div class="code-bar">
          <span>Shell</span>
          <button type="button" class="copy" @click="copyCommands">
            <Icon :svg="copied ? icons.check : icons.copy" :size="14" />
            {{ copied ? 'Copied' : 'Copy' }}
          </button>
        </div>
        <pre><code><template v-for="line in commands" :key="line"><span :class="{ comment: line.startsWith('#') }">{{ line }}</span>{{ '\n' }}</template></code></pre>
      </div>
      <p class="note">
        <Icon :svg="icons.info" :size="18" />
        <span>
          Polyfin is in early development. Expect changes between releases, and report problems in
          the <a :href="`${repository}/issues`">issues</a>.
        </span>
      </p>
    </section>

    <footer class="container footer">
      <div class="footer-top">
        <a class="brand" :href="withBase('/')">
          <img :src="withBase('/polyfin.svg')" alt="" width="24" height="24" />
          Polyfin
        </a>
        <nav class="footer-links" aria-label="Footer">
          <a :href="withBase('/docs/')">Docs</a>
          <a :href="repository">GitHub</a>
          <a :href="`${repository}/releases`">Releases</a>
          <a :href="`${repository}/blob/main/SECURITY.md`">Security</a>
        </nav>
      </div>
      <p class="legal">
        Polyfin does not host, store, or distribute any content. It only relays what the addons and
        services configured by its operator provide. You are solely responsible for the addons and
        services you configure and for complying with the laws that apply to you.
      </p>
      <p class="legal">
        Polyfin is an independent project, not affiliated with or endorsed by Jellyfin or Stremio.
        Released under the <a :href="`${repository}/blob/main/LICENSE`">MIT License</a>.
      </p>
    </footer>
  </div>
</template>

<style scoped>
.pf-home {
  position: relative;
  overflow-x: clip;
}

.glow {
  position: absolute;
  top: calc(-1 * var(--vp-nav-height));
  left: 50%;
  width: 1800px;
  height: 1100px;
  transform: translateX(-50%);
  background:
    radial-gradient(40% 45% at 30% 30%, rgb(109 74 255 / 0.16), transparent 70%),
    radial-gradient(35% 40% at 72% 55%, rgb(34 211 238 / 0.07), transparent 70%);
  pointer-events: none;
}

.container {
  position: relative;
  max-width: calc(var(--pf-content-width) + 64px);
  margin: 0 auto;
  padding: 0 24px;
}

@media (min-width: 768px) {
  .container {
    padding: 0 32px;
  }
}

/* Hero */

.hero {
  padding-top: 56px;
}

h1 {
  max-width: 15em;
  margin: 0;
  font-size: clamp(36px, 6vw, 64px);
  font-weight: 600;
  line-height: 1.04;
  letter-spacing: -0.035em;
  color: var(--nuit-ink);
  text-wrap: balance;
}

.lead {
  max-width: 34em;
  margin: 20px 0 0;
  font-size: clamp(17px, 1.6vw, 19px);
  line-height: 1.55;
  color: var(--nuit-ink-2);
  text-wrap: pretty;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 32px;
}

.button {
  display: inline-flex;
  align-items: center;
  height: 44px;
  padding: 0 20px;
  border: 1px solid var(--nuit-line-3);
  border-radius: var(--nuit-radius-field);
  background: var(--nuit-s3);
  font-size: 15px;
  font-weight: 500;
  color: var(--nuit-ink);
  text-decoration: none;
  white-space: nowrap;
  transition:
    background-color 160ms var(--nuit-ease),
    transform 160ms var(--nuit-ease);
}

.button:hover {
  background: var(--nuit-s4);
}

.button.primary {
  border-color: transparent;
  background: var(--nuit-accent);
  color: #fff;
}

.button.primary:hover {
  background: var(--nuit-accent-press);
}

.button:active {
  transform: scale(0.98);
}

.button:focus-visible,
a:focus-visible {
  outline: 2px solid var(--nuit-link);
  outline-offset: 2px;
}

.hero-media {
  margin-top: 56px;
}

.apps {
  max-width: 60em;
  margin: 28px 0 0;
  font-size: 15px;
  line-height: 1.7;
  color: var(--nuit-ink-3);
}

.apps strong {
  font-weight: 500;
  color: var(--nuit-ink-2);
}

.apps a,
.section a,
.footer a {
  color: var(--nuit-link);
  text-decoration: none;
}

.apps a {
  margin-left: 8px;
  white-space: nowrap;
}

.apps a:hover,
.section a:hover {
  color: var(--nuit-link-hover);
}

/* Sections */

.section {
  padding-top: 120px;
}

h2 {
  margin: 0;
  font-size: clamp(28px, 3.4vw, 40px);
  font-weight: 600;
  line-height: 1.1;
  letter-spacing: -0.03em;
  color: var(--nuit-ink);
}

.section-lead {
  max-width: 36em;
  margin: 14px 0 0;
  font-size: 17px;
  line-height: 1.55;
  color: var(--nuit-ink-2);
}

.viewer {
  margin-top: 48px;
}

/* Also built in */

.extras {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  margin: 40px -16px 0;
  padding: 0;
  list-style: none;
}

.extras a {
  display: grid;
  grid-template-columns: 36px minmax(0, 1fr);
  grid-template-rows: auto 1fr;
  column-gap: 14px;
  row-gap: 4px;
  height: 100%;
  padding: 16px;
  border: 1px solid transparent;
  border-radius: var(--nuit-radius-row);
  color: inherit;
  transition:
    background-color 160ms var(--nuit-ease),
    border-color 160ms var(--nuit-ease);
}

.extras a:hover {
  border-color: var(--nuit-line);
  background: var(--nuit-s1);
}

.extra-icon {
  grid-row: span 2;
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border: 1px solid var(--nuit-line-2);
  border-radius: 10px;
  background: var(--nuit-s2);
  color: var(--nuit-ink-2);
}

.extra-name {
  align-self: center;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--nuit-ink);
}

.extra-text {
  font-size: 14px;
  line-height: 1.5;
  color: var(--nuit-ink-2);
}

/* Install */

.install {
  display: grid;
  grid-template-columns: minmax(0, 5fr) minmax(0, 7fr);
  column-gap: 56px;
  align-items: start;
}

.steps {
  margin: 28px 0 0;
  padding-left: 20px;
  list-style: decimal;
  font-size: 15px;
  line-height: 1.6;
  color: var(--nuit-ink-2);
}

.steps li + li {
  margin-top: 8px;
}

.steps li::marker {
  color: var(--nuit-ink-3);
  font-variant-numeric: tabular-nums;
}

code {
  padding: 2px 6px;
  border-radius: 6px;
  background: var(--nuit-line-2);
  font-family: var(--vp-font-family-mono);
  font-size: 0.875em;
  color: var(--nuit-ink);
}

.more {
  margin: 20px 0 0;
  font-size: 15px;
  line-height: 1.6;
  color: var(--nuit-ink-2);
}

.code {
  margin-top: 6px;
  border: 1px solid var(--nuit-line-2);
  border-radius: var(--nuit-radius-panel);
  background: var(--nuit-s1);
  overflow: hidden;
}

.code-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 44px;
  padding: 0 8px 0 20px;
  border-bottom: 1px solid var(--nuit-line);
  font-family: var(--vp-font-family-mono);
  font-size: 12px;
  color: var(--nuit-ink-3);
}

.copy {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 10px;
  border-radius: var(--nuit-radius-field);
  font-family: var(--vp-font-family-base);
  font-size: 13px;
  font-weight: 500;
  color: var(--nuit-ink-2);
  cursor: pointer;
  transition:
    background-color 160ms var(--nuit-ease),
    color 160ms var(--nuit-ease);
}

.copy:hover {
  background: var(--nuit-s3);
  color: var(--nuit-ink);
}

.copy:focus-visible {
  outline: 2px solid var(--nuit-link);
  outline-offset: 2px;
}

pre {
  margin: 0;
  padding: 20px;
  overflow-x: auto;
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
  line-height: 1.8;
  color: var(--nuit-ink);
}

pre code {
  padding: 0;
  background: none;
  font-size: inherit;
}

.comment {
  color: var(--nuit-ink-3);
}

.note {
  grid-column: 1 / -1;
  display: flex;
  gap: 10px;
  align-items: flex-start;
  margin: 40px 0 0;
  padding: 14px 16px;
  border: 1px solid var(--nuit-line-2);
  border-radius: var(--nuit-radius-row);
  background: var(--nuit-s1);
  font-size: 14px;
  line-height: 1.55;
  color: var(--nuit-ink-2);
}

.note .pf-icon {
  flex: none;
  margin-top: 1px;
  color: var(--nuit-link);
}

/* Footer */

.footer {
  margin-top: 120px;
  padding-top: 32px;
  padding-bottom: 56px;
}

.footer::before {
  content: '';
  position: absolute;
  top: 0;
  left: 24px;
  right: 24px;
  border-top: 1px solid var(--nuit-line-2);
}

.footer-top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--nuit-ink) !important;
}

.footer-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 24px;
  font-size: 14px;
}

.footer-links a {
  color: var(--nuit-ink-2);
}

.footer-links a:hover {
  color: var(--nuit-ink);
}

.legal {
  max-width: 52em;
  margin: 20px 0 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--nuit-ink-3);
}

.legal + .legal {
  margin-top: 8px;
}

/* Entrance: the hero rises, its parts 60 ms apart. */
@media (prefers-reduced-motion: no-preference) {
  .hero > * {
    animation: rise 520ms var(--nuit-ease) both;
  }
  .hero > :nth-child(2) {
    animation-delay: 60ms;
  }
  .hero > :nth-child(3) {
    animation-delay: 120ms;
  }
  .hero > :nth-child(n + 4) {
    animation-delay: 180ms;
  }
}

@keyframes rise {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
}

@media (max-width: 1099px) {
  .extras {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 959px) {
  .install {
    grid-template-columns: minmax(0, 1fr);
  }
  .code {
    margin-top: 32px;
  }
}

@media (max-width: 767px) {
  .hero {
    padding-top: 32px;
  }
  .hero-media {
    margin-top: 40px;
  }
  .section {
    padding-top: 88px;
  }
  pre {
    font-size: 13px;
  }
}

@media (max-width: 639px) {
  .extras {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (min-width: 768px) {
  .footer::before {
    left: 32px;
    right: 32px;
  }
}
</style>
