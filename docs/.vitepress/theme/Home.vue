<script setup lang="ts">
import { withBase } from 'vitepress'
import { onBeforeUnmount, ref, type Directive } from 'vue'
import FacetFrame from './FacetFrame.vue'
import FeatureViewer from './FeatureViewer.vue'
import Icon from './Icon.vue'
import TourScreen from './TourScreen.vue'
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

/** The shelf's two rows of poster art, each written twice so that it loops. */
const shelf = [
  [
    'amber-road',
    'northwave',
    'ocean-life',
    'echo-valley',
    'paper-moons',
    'glass-garden',
    'night-train',
    'silver-coast',
  ],
  [
    'low-tide',
    'long-field',
    'paper-moons-sq',
    'amber-road-sq',
    'northwave',
    'echo-valley',
    'ocean-life',
    'glass-garden',
    'silver-coast',
  ],
]

const tour = ref<InstanceType<typeof TourScreen>>()
const copied = ref(false)
let copiedTimer: ReturnType<typeof setTimeout> | undefined

async function copyCommands() {
  await navigator.clipboard.writeText(commands.join('\n'))
  copied.value = true
  clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => (copied.value = false), 2000)
}

/** Sections below the hero rise into place as they reach the screen. */
const revealer =
  typeof IntersectionObserver === 'undefined'
    ? undefined
    : new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (!entry.isIntersecting) continue
            entry.target.classList.add('in')
            revealer?.unobserve(entry.target)
          }
        },
        { rootMargin: '0px 0px -6% 0px' },
      )

const vReveal: Directive<HTMLElement> = {
  mounted(element) {
    element.classList.add('rv')
    revealer?.observe(element)
  },
}

onBeforeUnmount(() => {
  revealer?.disconnect()
  clearTimeout(copiedTimer)
})
</script>

<template>
  <div class="pf-home">
    <section id="top" class="hero" aria-labelledby="hero-title">
      <svg
        class="backdrop"
        viewBox="0 0 1440 900"
        preserveAspectRatio="xMidYMid slice"
        aria-hidden="true"
      >
        <polygon points="760,900 1180,120 1440,640 1440,900" />
        <polygon class="g2" points="1180,120 1440,-40 1440,640" />
        <polygon class="g3" points="560,900 900,560 1000,900" />
      </svg>
      <div class="wrap hero-grid">
        <div class="hero-copy">
          <p class="eyebrow">
            <span class="facets" aria-hidden="true"
              ><i class="violet"></i><i class="indigo"></i><i class="periwinkle"></i
              ><i class="blue"></i><i class="cyan"></i></span
            >Self-hosted, Jellyfin compatible
          </p>
          <h1 id="hero-title">Your addons and IPTV, in <span>any Jellyfin app.</span></h1>
          <p class="lead">
            Polyfin turns Stremio addons, music addons and IPTV into a self-hosted,
            Jellyfin-compatible server, with real accounts and transcoding.
          </p>
          <div class="actions">
            <a class="pf-button primary" href="#install"
              >Install<span class="knob"><Icon :svg="icons.arrowRight" :size="16" /></span
            ></a>
            <a class="pf-button ghost" :href="withBase('/docs/')">Read the docs</a>
            <button type="button" class="tour-link" @click="tour?.open()">
              <span class="play" aria-hidden="true"></span>Watch the tour
              <span class="duration">1:23</span>
            </button>
          </div>
        </div>
        <FacetFrame />
      </div>

      <div class="wrap works" aria-label="Works with">
        <span class="label">Works with</span>
        <ul class="apps">
          <li v-for="app in apps" :key="app">{{ app }}</li>
          <li class="official">and the official Jellyfin apps</li>
        </ul>
        <a class="compatibility" :href="withBase('/docs/jellyfin-compatibility')"
          >Compatibility <Icon :svg="icons.arrowUpRight" :size="15"
        /></a>
      </div>
    </section>

    <section id="how" class="section" aria-labelledby="how-title">
      <div class="wrap">
        <div v-reveal class="head">
          <p class="eyebrow">
            <span class="pf-tri" style="--facet: var(--pf-violet)"></span>How it works
          </p>
          <h2 id="how-title">From your sources to every screen.</h2>
          <p class="section-lead">
            Jellyfin apps can't play your addons. Polyfin is the server in between: it speaks the
            Jellyfin API on one side and your sources on the other.
          </p>
        </div>
        <div class="flow">
          <article v-reveal class="node" style="--facet: var(--pf-violet)">
            <span class="corner" aria-hidden="true"></span>
            <p class="kind">In</p>
            <h3>Your sources</h3>
            <ul class="ticks">
              <li>Stremio addons</li>
              <li>Music addons</li>
              <li>IPTV playlists and Xtream Codes accounts</li>
              <li>Local folders and network shares</li>
            </ul>
          </article>
          <div class="chain" aria-hidden="true">
            <i class="violet"></i><i class="indigo"></i><i class="periwinkle"></i>
          </div>
          <article v-reveal class="node hub">
            <img class="mark" :src="withBase('/polyfin.svg')" alt="" width="104" height="104" />
            <h3>Polyfin</h3>
            <p>
              Catalogs become libraries, streams become versions of a title, IPTV becomes Live TV.
              Accounts, transcoding and a guide come with it.
            </p>
          </article>
          <div class="chain second" aria-hidden="true">
            <i class="periwinkle"></i><i class="blue"></i><i class="cyan"></i>
          </div>
          <article v-reveal class="node" style="--facet: var(--pf-cyan)">
            <span class="corner" aria-hidden="true"></span>
            <p class="kind">Out</p>
            <h3>Any Jellyfin app</h3>
            <ul class="ticks">
              <li>Phones, tablets, TVs and computers</li>
              <li>The built-in web client</li>
              <li>DLNA players with no Jellyfin app</li>
              <li>Signed in as they would to Jellyfin</li>
            </ul>
          </article>
        </div>
      </div>
    </section>

    <section id="tour" class="section" aria-labelledby="tour-title">
      <div class="wrap">
        <div v-reveal class="head split">
          <div>
            <p class="eyebrow">
              <span class="pf-tri" style="--facet: var(--pf-indigo)"></span>The tour
            </p>
            <h2 id="tour-title">See it in the apps you already use.</h2>
          </div>
          <p class="section-lead">
            Phones, TVs and the web, one server behind them. A minute and a half, start to finish.
          </p>
        </div>
        <TourScreen ref="tour" v-reveal />
      </div>
    </section>

    <section id="features" class="section" aria-labelledby="features-title">
      <div class="wrap">
        <div v-reveal class="head split">
          <div>
            <p class="eyebrow">
              <span class="pf-tri" style="--facet: var(--pf-periwinkle)"></span>Now showing
            </p>
            <h2 id="features-title">Everything your apps expect from a server.</h2>
          </div>
          <p class="section-lead">
            Libraries, accounts, transcoding, Live TV and music, all through the Jellyfin API. Pick
            one, or let them play in turn.
          </p>
        </div>
        <FeatureViewer v-reveal />
      </div>
    </section>

    <section class="section shelf" aria-labelledby="shelf-title">
      <div class="wrap">
        <div v-reveal class="head split">
          <div>
            <p class="eyebrow">
              <span class="pf-tri" style="--facet: var(--pf-blue)"></span>Your catalogs
            </p>
            <h2 id="shelf-title">On the shelf, with their artwork.</h2>
          </div>
          <p class="section-lead">
            Movies, series, music and audiobooks arrive with their artwork, resume points and
            favorites, the way your apps already show them.
          </p>
        </div>
      </div>
      <div class="rows" aria-hidden="true">
        <div v-for="(row, index) in shelf" :key="index" class="row">
          <div class="track">
            <img
              v-for="(poster, at) in [...row, ...row]"
              :key="at"
              :src="withBase(`/posters/${poster}.jpg`)"
              alt=""
              width="164"
              height="219"
              loading="lazy"
            />
          </div>
        </div>
      </div>
    </section>

    <section id="built-in" class="section" aria-labelledby="built-title">
      <div class="wrap">
        <div v-reveal class="head split">
          <div>
            <p class="eyebrow">
              <span class="pf-tri" style="--facet: var(--pf-blue)"></span>Eight more
            </p>
            <h2 id="built-title">Also built in.</h2>
          </div>
          <p class="section-lead">
            The parts a home server needs once more than one person watches. Each one has its page
            in the docs.
          </p>
        </div>
        <ul class="bento">
          <li
            v-for="(extra, index) in extras"
            :key="extra.name"
            v-reveal
            :class="[
              extra.still ? 'media' : extra.paths ? 'paths' : 'small',
              { wide: index === 0 || index === 7 },
            ]"
            :style="{ '--facet': `var(--pf-${extra.facet})` }"
          >
            <a :href="withBase(extra.link)">
              <span v-if="extra.still" class="shot">
                <img
                  :src="withBase(extra.still.src)"
                  :alt="extra.still.alt"
                  :style="{
                    '--scale': extra.still.scale,
                    '--origin': extra.still.origin,
                    '--position': extra.still.position ?? '50% 0',
                  }"
                  loading="lazy"
                  width="1200"
                  height="672"
                />
                <span class="corner-tab"></span>
              </span>
              <span class="body">
                <span class="tile" :class="{ 'on-cyan': extra.facet === 'cyan' }"
                  ><Icon :svg="extra.icon" :size="22"
                /></span>
                <span class="copy">
                  <b>{{ extra.name }}</b>
                  <span>{{ extra.text }}</span>
                </span>
                <span class="go"><Icon :svg="icons.arrowUpRight" :size="18" /></span>
              </span>
              <span v-if="extra.paths" class="urls" aria-hidden="true">
                <span v-for="url in extra.paths" :key="url.path" class="url">
                  <span class="pf-tri" :style="{ '--facet': `var(--pf-${url.facet})` }"></span>
                  <span
                    >server:8096<em>{{ url.path }}</em></span
                  >
                  <small>{{ url.label }}</small>
                </span>
              </span>
            </a>
          </li>
        </ul>
      </div>
    </section>

    <section id="install" class="section install" aria-labelledby="install-title">
      <div class="wrap">
        <div class="install-grid">
          <div v-reveal>
            <p class="eyebrow">
              <span class="pf-tri" style="--facet: var(--pf-cyan)"></span>Install
            </p>
            <h2 id="install-title">Run it with Docker Compose.</h2>
            <p class="section-lead">
              Docker with Compose v2 is all it needs. The image is published for linux/amd64 and
              linux/arm64.
            </p>
            <ol class="steps">
              <li>
                <span class="n">1</span
                ><span>Open <code>http://&lt;server&gt;:8096/admin/</code>.</span>
              </li>
              <li>
                <span class="n">2</span
                ><span
                  >Enter the one-time setup code from <code>docker compose logs polyfin</code> to
                  create the administrator.</span
                >
              </li>
              <li>
                <span class="n">3</span
                ><span
                  >Continue with
                  <a class="underline" :href="withBase('/docs/getting-started')">Getting started</a
                  >.</span
                >
              </li>
            </ol>
            <p class="more">
              Unraid, GPUs, pinned versions and builds from source are in the
              <a class="underline" :href="withBase('/docs/installation')">installation guide</a>.
            </p>
          </div>

          <div v-reveal class="install-side">
            <div class="terminal">
              <div class="terminal-bar">
                <span>Shell</span>
                <button
                  type="button"
                  class="copy-button"
                  :class="{ done: copied }"
                  @click="copyCommands"
                >
                  <Icon :svg="copied ? icons.check : icons.copy" :size="15" />
                  {{ copied ? 'Copied' : 'Copy' }}
                </button>
              </div>
              <pre><code><template v-for="line in commands" :key="line"><span :class="line.startsWith('#') ? 'comment' : 'command'">{{ line }}</span>{{ '\n' }}</template></code></pre>
            </div>

            <a class="desktop" :href="withBase('/docs/desktop')">
              <span class="desktop-mark"><img :src="withBase('/polyfin.svg')" alt="" /></span>
              <span class="desktop-text">
                <small>Rather not run Docker?</small>
                <b>Polyfin Desktop</b>
                <span
                  >The same server on Windows, macOS or Linux. An icon in the tray, or the menu bar
                  on macOS, starts and stops it.</span
                >
              </span>
              <span class="systems" aria-hidden="true">
                <Icon :svg="icons.windowsLogo" :size="19" />
                <Icon :svg="icons.appleLogo" :size="19" />
                <Icon :svg="icons.linuxLogo" :size="19" />
              </span>
              <span class="go"><Icon :svg="icons.arrowUpRight" :size="18" /></span>
            </a>
          </div>
        </div>

        <p v-reveal class="note">
          <Icon :svg="icons.info" :size="20" />
          <span
            >Polyfin is in early development. Expect changes between releases, and report problems
            in the <a class="underline" :href="`${repository}/issues`">issues</a>.</span
          >
        </p>
      </div>
    </section>

    <footer class="footer">
      <div class="wrap">
        <div class="footer-top">
          <div>
            <a class="brand" :href="withBase('/')"
              ><img :src="withBase('/polyfin.svg')" alt="" width="34" height="34" />Polyfin</a
            >
            <p>Your addons and IPTV, in any Jellyfin app.</p>
          </div>
          <nav class="footer-links" aria-label="Footer">
            <a :href="withBase('/docs/')">Docs</a>
            <a :href="repository">GitHub</a>
            <a :href="`${repository}/releases`">Releases</a>
            <a :href="`${repository}/discussions`">Community</a>
            <a :href="`${repository}/blob/main/SECURITY.md`">Security</a>
          </nav>
        </div>
        <div class="rule" aria-hidden="true">
          <i class="violet"></i><i class="indigo"></i><i class="periwinkle"></i><i class="blue"></i
          ><i class="cyan"></i>
        </div>
        <p class="legal">
          Polyfin does not host, store, or distribute any content. It only relays what the addons
          and services configured by its operator provide. You are solely responsible for the addons
          and services you configure and for complying with the laws that apply to you.
        </p>
        <p class="legal">
          Polyfin is an independent project, not affiliated with or endorsed by Jellyfin or Stremio.
          Released under the
          <a class="underline" :href="`${repository}/blob/main/LICENSE`">MIT License</a>.
        </p>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.pf-home {
  position: relative;
  overflow-x: clip;
  color: var(--pf-ink-2);
  font-size: 16px;
  line-height: 1.6;
}

.wrap {
  width: 100%;
  max-width: calc(var(--pf-content-width) + 2 * var(--gutter));
  margin-inline: auto;
  padding-inline: var(--gutter);
  --gutter: clamp(20px, 4vw, 48px);
}

h1,
h2,
h3 {
  margin: 0;
  font-family: var(--pf-display);
  font-weight: 700;
  color: var(--pf-ink);
  text-wrap: balance;
}

p {
  margin: 0;
  text-wrap: pretty;
}

code {
  padding: 2px 7px;
  border-radius: 6px;
  background: var(--pf-s3);
  font-family: var(--vp-font-family-mono);
  font-size: 0.86em;
  color: var(--pf-ink);
  white-space: nowrap;
}

.underline {
  color: var(--pf-ink);
  text-decoration: underline;
  text-decoration-color: rgb(34 211 238 / 0.55);
  text-decoration-thickness: 1.5px;
  text-underline-offset: 3px;
  transition: text-decoration-color 0.3s var(--pf-ease);
}

.underline:hover {
  text-decoration-color: var(--pf-cyan);
}

.eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 22px;
  font: 500 12.5px/1 var(--vp-font-family-mono);
  letter-spacing: 0.02em;
  color: var(--pf-mute);
}

.facets {
  display: inline-flex;
  gap: 3px;
}

.facets i,
.rule i {
  width: 9px;
  height: 9px;
  clip-path: polygon(0 100%, 50% 0, 100% 100%);
}

.violet {
  background: var(--pf-violet);
}
.indigo {
  background: var(--pf-indigo);
}
.periwinkle {
  background: var(--pf-periwinkle);
}
.blue {
  background: var(--pf-blue);
}
.cyan {
  background: var(--pf-cyan);
}

/* Reveal: sections below the hero rise into place. */
.rv {
  transition:
    opacity 0.9s var(--pf-ease),
    transform 0.9s var(--pf-ease),
    filter 0.9s var(--pf-ease);
}

.rv:not(.in) {
  opacity: 0;
  transform: translateY(28px);
  filter: blur(6px);
}

/* Hero */

.hero {
  position: relative;
  isolation: isolate;
  padding-top: 120px;
  overflow: clip;
}

.backdrop {
  position: absolute;
  inset: 0;
  z-index: -1;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.backdrop polygon {
  fill: #0e1024;
}

.backdrop .g2 {
  fill: #111330;
}

.backdrop .g3 {
  fill: #0c0e1f;
}

.hero-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  align-items: center;
  gap: 40px;
  min-height: calc(min(100dvh, 960px) - 196px);
}

h1 {
  max-width: 10.5em;
  font-size: clamp(40px, 5.05vw, 78px);
  line-height: 0.99;
  letter-spacing: -0.048em;
}

h1 span {
  color: var(--pf-cyan);
}

.lead {
  max-width: 31em;
  margin: 26px 0 36px;
  font-size: clamp(17px, 1.35vw, 19px);
  line-height: 1.6;
  color: var(--pf-ink-2);
}

.actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.tour-link {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  height: 48px;
  margin-left: 6px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 15px;
  font-weight: 500;
  color: var(--pf-ink);
  cursor: pointer;
}

.tour-link .play {
  width: 11px;
  height: 13px;
  background: var(--pf-cyan);
  clip-path: polygon(0 0, 100% 50%, 0 100%);
  transition: transform 0.5s var(--pf-ease);
}

.tour-link:hover .play {
  transform: translateX(3px);
}

.duration {
  font: 500 12.5px/1 var(--vp-font-family-mono);
  color: var(--pf-mute);
}

/* Works with */

.works {
  position: relative;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 14px 28px;
  margin-top: 64px;
  padding-block: 28px;
}

.works::before {
  content: '';
  position: absolute;
  top: 0;
  left: var(--gutter);
  right: var(--gutter);
  height: 1px;
  background: var(--pf-line);
}

.works .label {
  font: 500 12.5px/1 var(--vp-font-family-mono);
  color: var(--pf-mute-2);
}

.apps {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  gap: 8px 24px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.apps li {
  font: 600 16px/1.3 var(--pf-display);
  letter-spacing: -0.02em;
  color: var(--pf-mute);
}

.apps .official {
  color: var(--pf-ink);
}

.compatibility {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 14.5px;
  font-weight: 500;
  color: var(--pf-cyan);
  transition: gap 0.4s var(--pf-ease);
}

.compatibility:hover {
  gap: 9px;
}

/* Sections */

.section {
  padding-top: clamp(96px, 11vw, 156px);
}

.head {
  max-width: 780px;
  margin-bottom: clamp(44px, 5.5vw, 72px);
}

.head.split {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(0, 0.75fr);
  align-items: end;
  gap: 48px;
  max-width: none;
}

h2 {
  font-size: clamp(34px, 4.1vw, 60px);
  line-height: 1.03;
  letter-spacing: -0.045em;
}

.head.split h2 {
  max-width: 13em;
}

.section-lead {
  max-width: 34em;
  margin-top: 22px;
  font-size: 18px;
  line-height: 1.6;
  color: var(--pf-mute);
}

.head.split .section-lead {
  margin: 0 0 6px;
}

/* How it works */

.flow {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 96px minmax(0, 1.05fr) 96px minmax(0, 1fr);
  align-items: center;
}

.node {
  position: relative;
  height: 100%;
  padding: 30px 30px 22px;
  overflow: hidden;
  border-radius: 24px;
  background: var(--pf-s2);
}

.corner {
  position: absolute;
  top: 0;
  right: 0;
  width: 58px;
  height: 58px;
  background: var(--facet);
  clip-path: polygon(0 0, 100% 0, 100% 100%);
}

.kind {
  font: 500 12.5px/1 var(--vp-font-family-mono);
  color: var(--pf-mute-2);
}

.node h3 {
  margin: 12px 0 16px;
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.03em;
}

.ticks {
  margin: 0;
  padding: 0;
  list-style: none;
}

.ticks li {
  display: flex;
  align-items: baseline;
  gap: 12px;
  padding: 12px 0;
  border-top: 1px solid var(--pf-line);
  font-size: 15.5px;
  color: var(--pf-ink-2);
}

.ticks li::before {
  content: '';
  flex: none;
  width: 8px;
  height: 8px;
  background: var(--facet);
  clip-path: polygon(0 100%, 50% 0, 100% 100%);
  transform: translateY(-1px);
}

.hub {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 44px 34px 40px;
  background: var(--pf-s3);
  text-align: center;
}

.hub .mark {
  width: 104px;
  height: 104px;
  margin-bottom: 18px;
}

.hub h3 {
  margin: 0 0 12px;
  font-size: 30px;
  font-weight: 700;
}

.hub p {
  max-width: 24em;
  font-size: 15.5px;
  color: var(--pf-mute);
}

.chain {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 7px;
}

.chain i {
  width: 13px;
  height: 16px;
  clip-path: polygon(0 0, 100% 50%, 0 100%);
  opacity: 0.35;
  animation: pulse 2.4s var(--pf-ease) infinite;
}

.chain i:nth-child(2) {
  animation-delay: 0.25s;
}
.chain i:nth-child(3) {
  animation-delay: 0.5s;
}
.chain.second i:nth-child(1) {
  animation-delay: 0.75s;
}
.chain.second i:nth-child(2) {
  animation-delay: 1s;
}
.chain.second i:nth-child(3) {
  animation-delay: 1.25s;
}

@keyframes pulse {
  0%,
  100% {
    opacity: 0.35;
    transform: translateX(0);
  }
  30% {
    opacity: 1;
    transform: translateX(3px);
  }
}

/* Media frames: the tour and the player (style.css, .pf-media), placed here. */

#tour .head {
  margin-bottom: clamp(40px, 5vw, 60px);
}

/* Shelf */

.shelf .head {
  margin-bottom: clamp(36px, 4.5vw, 56px);
}

.rows {
  display: grid;
  gap: 16px;
  mask-image: linear-gradient(90deg, transparent, #000 9%, #000 91%, transparent);
  -webkit-mask-image: linear-gradient(90deg, transparent, #000 9%, #000 91%, transparent);
}

.row {
  overflow: hidden;
}

.track {
  display: flex;
  gap: 16px;
  width: max-content;
  animation: drift 80s linear infinite;
}

.row + .row .track {
  animation-direction: reverse;
  animation-duration: 96s;
}

.track img {
  width: 164px;
  height: auto;
  aspect-ratio: 3 / 4;
  object-fit: cover;
  border-radius: 14px;
  background: var(--pf-s2);
}

.row + .row .track img {
  width: 136px;
}

@keyframes drift {
  to {
    transform: translateX(-50%);
  }
}

/* Also built in */

.bento {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 16px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.bento > li {
  grid-column: span 3;
}

.bento > .media,
.bento > .paths {
  grid-column: span 5;
}

.bento > .media.wide {
  grid-column: span 7;
}

.bento a {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  border-radius: 22px;
  background: var(--pf-s2);
  transition:
    background-color 0.4s var(--pf-ease),
    transform 0.6s var(--pf-ease);
}

.bento a:hover {
  background: var(--pf-s3);
}

.bento a:active {
  transform: scale(0.99);
}

.shot {
  position: relative;
  display: block;
  aspect-ratio: 16 / 8.4;
  overflow: hidden;
  background: #05060e;
}

.wide .shot {
  aspect-ratio: 16 / 7.2;
}

.shot img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: var(--position);
  transform: scale(var(--scale));
  transform-origin: var(--origin);
  transition: transform 1.4s var(--pf-ease);
}

.bento a:hover .shot img {
  transform: scale(calc(var(--scale) + 0.04));
}

.corner-tab {
  position: absolute;
  top: 0;
  left: 0;
  width: 58px;
  height: 58px;
  background: var(--facet);
  clip-path: polygon(0 0, 100% 0, 0 100%);
}

.body {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: start;
  gap: 18px;
  padding: 24px 26px 28px;
}

.tile {
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  border-radius: 12px;
  background: var(--facet);
  color: #fff;
}

.tile.on-cyan {
  color: var(--pf-on-cyan);
}

.copy b {
  display: block;
  margin-bottom: 7px;
  font: 600 19px/1.25 var(--pf-display);
  letter-spacing: -0.025em;
  color: var(--pf-ink);
}

.copy span {
  font-size: 15px;
  line-height: 1.58;
  color: var(--pf-mute);
}

.go {
  display: inline-flex;
  color: var(--pf-mute-2);
  transition:
    color 0.3s var(--pf-ease),
    transform 0.5s var(--pf-ease);
}

a:hover .go {
  color: var(--pf-ink);
  transform: translate(2px, -2px);
}

.small .body,
.paths .body {
  grid-template-columns: minmax(0, 1fr);
  gap: 24px;
}

.small .go,
.paths .go {
  position: absolute;
  top: 26px;
  right: 24px;
}

.urls {
  display: grid;
  gap: 8px;
  margin-top: auto;
  padding: 0 26px 28px;
}

.url {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 46px;
  padding: 0 16px;
  border-radius: 12px;
  background: var(--pf-bg);
  font: 400 14px/1 var(--vp-font-family-mono);
  color: var(--pf-mute);
  transition: background-color 0.4s var(--pf-ease);
}

.url .pf-tri {
  width: 9px;
  height: 9px;
  transform: rotate(90deg);
}

.url em {
  font-style: normal;
  color: var(--pf-ink);
}

.url small {
  margin-left: auto;
  font: 500 12.5px/1 var(--vp-font-family-base);
  color: var(--pf-mute-2);
}

a:hover .url {
  background: var(--pf-s1);
}

/* Install */

.install {
  padding-bottom: clamp(96px, 11vw, 150px);
}

.install-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.08fr);
  align-items: start;
  gap: clamp(40px, 5.5vw, 80px);
}

.steps {
  margin: 36px 0 0;
  padding: 0;
  list-style: none;
}

.steps li {
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr);
  align-items: baseline;
  gap: 16px;
  padding: 16px 0;
  border-top: 1px solid var(--pf-line);
  color: var(--pf-ink-2);
}

.steps .n {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--pf-s3);
  font: 600 13px/1 var(--pf-display);
  color: var(--pf-ink);
  transform: translateY(-2px);
}

.more {
  margin-top: 22px;
  font-size: 15px;
  color: var(--pf-mute);
}

.terminal {
  overflow: hidden;
  border-radius: 20px;
  background: var(--pf-s2);
}

.terminal-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 12px 12px 22px;
  border-bottom: 1px solid var(--pf-line);
  font: 500 12.5px/1 var(--vp-font-family-mono);
  color: var(--pf-mute-2);
}

.copy-button {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 34px;
  padding: 0 13px;
  border-radius: 999px;
  box-shadow: inset 0 0 0 1px var(--pf-line-2);
  font: 500 13px/1 var(--vp-font-family-base);
  color: var(--pf-ink);
  cursor: pointer;
  transition:
    background-color 0.3s var(--pf-ease),
    box-shadow 0.3s var(--pf-ease);
}

.copy-button:hover {
  background: rgb(178 184 255 / 0.06);
  box-shadow: inset 0 0 0 1px var(--pf-line-3);
}

.copy-button.done {
  color: var(--pf-cyan);
}

.terminal pre {
  margin: 0;
  padding: 22px 24px 26px;
  overflow-x: auto;
  font: 400 14px/1.9 var(--vp-font-family-mono);
  color: var(--pf-ink-2);
}

.terminal pre code {
  padding: 0;
  background: none;
  font-size: inherit;
  color: inherit;
  white-space: pre;
}

.terminal .command::before {
  content: '$ ';
  color: var(--pf-cyan);
}

.terminal .comment {
  color: var(--pf-mute-2);
}

.desktop {
  display: grid;
  grid-template-columns: 56px minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 18px;
  margin-top: 16px;
  padding: 22px 24px;
  border-radius: 20px;
  background: var(--pf-s2);
  transition: background-color 0.4s var(--pf-ease);
}

.desktop:hover {
  background: var(--pf-s3);
}

.desktop-mark {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  border-radius: 15px;
  background: var(--pf-s3);
}

.desktop:hover .desktop-mark {
  background: var(--pf-s4);
}

.desktop-mark img {
  width: 32px;
  height: 32px;
}

.desktop-text small {
  display: block;
  margin-bottom: 8px;
  font: 500 12.5px/1 var(--vp-font-family-mono);
  color: var(--pf-mute-2);
}

.desktop-text b {
  display: block;
  margin-bottom: 5px;
  font: 600 19px/1.2 var(--pf-display);
  letter-spacing: -0.025em;
  color: var(--pf-ink);
}

.desktop-text > span {
  font-size: 14.5px;
  line-height: 1.55;
  color: var(--pf-mute);
}

.systems {
  display: flex;
  gap: 10px;
  color: var(--pf-mute);
}

.note {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  max-width: 60em;
  margin-top: 56px;
  font-size: 15px;
  color: var(--pf-mute);
}

.note .pf-icon {
  margin-top: 1px;
  color: var(--pf-cyan);
}

/* Footer */

.footer {
  padding: 64px 0 56px;
  overflow: clip;
  background: var(--pf-s1);
}

.footer-top {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: flex-start;
  gap: 28px;
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  font: 700 24px/1 var(--pf-display);
  letter-spacing: -0.03em;
  color: var(--pf-ink);
}

.footer-top p {
  margin-top: 14px;
  font-size: 15px;
  color: var(--pf-mute);
}

.footer-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.footer-links a {
  padding: 10px 16px;
  border-radius: 999px;
  box-shadow: inset 0 0 0 1px var(--pf-line-2);
  font-size: 14px;
  font-weight: 500;
  color: var(--pf-ink-2);
  transition:
    color 0.3s var(--pf-ease),
    box-shadow 0.3s var(--pf-ease);
}

.footer-links a:hover {
  color: var(--pf-ink);
  box-shadow: inset 0 0 0 1px var(--pf-line-3);
}

.rule {
  display: flex;
  align-items: center;
  gap: 4px;
  margin: 48px 0 26px;
}

.rule i {
  width: 13px;
  height: 11px;
}

.rule::after {
  content: '';
  flex: 1;
  height: 1px;
  margin-left: 10px;
  background: var(--pf-line);
}

.legal {
  max-width: 68em;
  font-size: 13.5px;
  line-height: 1.65;
  color: var(--pf-mute-2);
}

.legal + .legal {
  margin-top: 10px;
}

/* Narrower screens */

@media (max-width: 1100px) {
  .hero-grid {
    grid-template-columns: minmax(0, 1fr);
    gap: 56px;
    min-height: 0;
  }

  .head.split {
    grid-template-columns: minmax(0, 1fr);
    gap: 0;
  }

  .head.split .section-lead {
    margin-top: 20px;
  }

  .flow {
    grid-template-columns: minmax(0, 1fr);
  }

  .chain {
    flex-direction: column;
    gap: 5px;
    height: 64px;
  }

  .chain i {
    rotate: 90deg;
  }

  .bento > li,
  .bento > .paths {
    grid-column: span 6;
  }

  .bento > .media,
  .bento > .media.wide {
    grid-column: span 12;
  }

  .install-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .hero {
    padding-top: 104px;
  }

  h1 {
    font-size: clamp(38px, 11vw, 50px);
  }

  .lead {
    margin: 20px 0 28px;
  }

  .actions .pf-button {
    flex: 1 1 auto;
    justify-content: center;
  }

  .actions .pf-button.primary {
    justify-content: space-between;
  }

  .tour-link {
    margin-left: 0;
  }

  .works {
    flex-direction: column;
    align-items: flex-start;
    gap: 14px;
    margin-top: 48px;
  }

  .apps {
    gap: 6px 16px;
  }

  .apps li {
    font-size: 15px;
  }

  h2 {
    font-size: clamp(32px, 9vw, 40px);
  }

  .section-lead {
    font-size: 16.5px;
  }

  .node {
    padding: 26px 22px 18px;
  }

  .bento > li,
  .bento > .paths {
    grid-column: span 12;
  }

  .body {
    gap: 14px;
    padding: 20px 20px 22px;
  }

  .small .body {
    grid-template-columns: 42px minmax(0, 1fr);
    column-gap: 16px;
    padding-right: 52px;
  }

  .small .go {
    top: 22px;
    right: 20px;
  }

  .urls {
    padding: 0 20px 22px;
  }

  .desktop {
    grid-template-columns: 48px minmax(0, 1fr) auto;
    padding: 20px;
  }

  .desktop-mark {
    width: 48px;
    height: 48px;
  }

  .systems {
    display: none;
  }

  .terminal pre {
    padding: 18px 18px 22px;
    font-size: 12.5px;
  }

  .track img {
    width: 120px;
  }

  .row + .row .track img {
    width: 104px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .rv:not(.in) {
    opacity: 1;
    transform: none;
    filter: none;
  }

  .track,
  .chain i {
    animation: none;
  }

  .chain i {
    opacity: 1;
  }
}
</style>
