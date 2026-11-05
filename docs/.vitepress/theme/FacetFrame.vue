<script setup lang="ts">
import { withBase } from 'vitepress'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from './Icon.vue'
import { icons } from './icons'

/**
 * The hero's picture: the logo's five facets, burst open around a phone showing a Polyfin
 * library. Each facet keeps its place in the mark: violet bottom left, indigo bottom right,
 * periwinkle in the middle, blue above it, the cyan spike top right. On load they fly in from
 * further out (ex, ey); then each follows the pointer at its own depth (d, in px at the edge).
 */
interface Layer {
  fill: string
  points: string
  depth: number
  ex: number
  ey: number
}

const behind: Layer[] = [
  { fill: '#3E8EF7', points: '26,318 172,92 306,236', depth: 10, ex: -70, ey: -90 },
  { fill: '#5B63F6', points: '150,380 506,322 330,590', depth: 7, ex: 40, ey: 50 },
  { fill: '#4C3FD9', points: '300,628 462,352 598,628', depth: 13, ex: 100, ey: 80 },
]
const front: Layer[] = [
  { fill: '#22D3EE', points: '410,196 566,44 514,288', depth: 22, ex: 110, ey: -120 },
  { fill: '#6D4AFF', points: '12,618 134,444 262,618', depth: 18, ex: -120, ey: 70 },
]

const posters = [
  'amber-road',
  'ocean-life',
  'paper-moons',
  'night-train',
  'glass-garden',
  'low-tide',
]

const art = ref<HTMLElement>()
const built = ref(false)
const live = ref(false)
let hero: HTMLElement | null = null
let frame = 0
let liveTimer: ReturnType<typeof setTimeout> | undefined

function onMove(event: PointerEvent) {
  const box = art.value!.getBoundingClientRect()
  const x = (event.clientX - (box.left + box.width / 2)) / (box.width * 0.9)
  const y = (event.clientY - (box.top + box.height / 2)) / (box.height * 0.9)
  cancelAnimationFrame(frame)
  frame = requestAnimationFrame(() => {
    art.value?.style.setProperty('--mx', Math.max(-1, Math.min(1, x)).toFixed(3))
    art.value?.style.setProperty('--my', Math.max(-1, Math.min(1, y)).toFixed(3))
  })
}

function onLeave() {
  art.value?.style.setProperty('--mx', '0')
  art.value?.style.setProperty('--my', '0')
}

onMounted(() => {
  requestAnimationFrame(() => requestAnimationFrame(() => (built.value = true)))
  liveTimer = setTimeout(() => (live.value = true), 1900)
  const still = matchMedia('(prefers-reduced-motion: reduce)').matches
  if (still || !matchMedia('(pointer: fine)').matches) return
  hero = art.value!.closest('.hero')
  hero?.addEventListener('pointermove', onMove)
  hero?.addEventListener('pointerleave', onLeave)
})

onBeforeUnmount(() => {
  hero?.removeEventListener('pointermove', onMove)
  hero?.removeEventListener('pointerleave', onLeave)
  cancelAnimationFrame(frame)
  clearTimeout(liveTimer)
})

const layerStyle = (layer: Layer) => ({
  '--d': layer.depth,
  '--ex': `${layer.ex}px`,
  '--ey': `${layer.ey}px`,
})
</script>

<template>
  <div
    ref="art"
    class="art"
    :class="{ built, live }"
    role="img"
    aria-label="The Polyfin library on a phone, framed by the five facets of the Polyfin logo"
  >
    <svg
      v-for="layer in behind"
      :key="layer.fill"
      class="facet"
      :style="layerStyle(layer)"
      viewBox="0 0 600 640"
      aria-hidden="true"
    >
      <polygon
        :points="layer.points"
        :fill="layer.fill"
        :stroke="layer.fill"
        stroke-width="9"
        stroke-linejoin="round"
      />
    </svg>

    <div class="facet phone" style="--d: 4; --ex: 0px; --ey: 30px" aria-hidden="true">
      <div class="screen">
        <div class="status">
          <b>9:41</b>
          <span class="signal"><i></i><i></i><i></i><i></i></span>
        </div>
        <div class="top">
          <span class="server"><img :src="withBase('/polyfin.svg')" alt="" />Polyfin</span>
          <span class="avatar">M</span>
        </div>
        <p class="heading">Library</p>
        <div class="tabs">
          <span class="on">Movies</span><span>Series</span><span>Collections</span>
        </div>
        <p class="row">Continue watching</p>
        <div class="resume">
          <img :src="withBase('/posters/northwave.jpg')" alt="" />
          <div class="resume-title"><b>Northwave</b><span>S1, E3 · 18 min left</span></div>
          <i class="bar"><i></i></i>
        </div>
        <p class="row">Movies <span>From your addons</span></p>
        <div class="grid">
          <img
            v-for="poster in posters"
            :key="poster"
            :src="withBase(`/posters/${poster}.jpg`)"
            alt=""
          />
        </div>
        <div class="tabbar">
          <span><Icon :svg="icons.house" size="2.3em" />Home</span>
          <span class="on"><Icon :svg="icons.squaresFour" size="2.3em" />Library</span>
          <span><Icon :svg="icons.magnifyingGlass" size="2.3em" />Search</span>
          <span><Icon :svg="icons.downloadSimple" size="2.3em" />Saved</span>
        </div>
      </div>
    </div>

    <svg
      v-for="layer in front"
      :key="layer.fill"
      class="facet front"
      :style="layerStyle(layer)"
      viewBox="0 0 600 640"
      aria-hidden="true"
    >
      <polygon
        :points="layer.points"
        :fill="layer.fill"
        :stroke="layer.fill"
        stroke-width="9"
        stroke-linejoin="round"
      />
    </svg>
  </div>
</template>

<style scoped>
.art {
  position: relative;
  justify-self: end;
  width: min(100%, 600px, calc((100dvh - 168px) * 600 / 640));
  aspect-ratio: 600 / 640;
  container-type: inline-size;
}

.facet {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  transform: translate3d(
    calc(var(--mx, 0) * var(--d) * 1px + var(--ox, 0px)),
    calc(var(--my, 0) * var(--d) * 1px + var(--oy, 0px)),
    0
  );
  transition:
    transform 1.5s var(--pf-spring),
    opacity 1.1s var(--pf-ease);
}

svg.facet {
  pointer-events: none;
}

.art:not(.built) .facet {
  --ox: var(--ex);
  --oy: var(--ey);
  opacity: 0;
}

.facet:nth-child(2) {
  transition-delay: 0.06s;
}

.facet:nth-child(3) {
  transition-delay: 0.12s;
}

.front {
  z-index: 2;
  transition-delay: 0.22s;
}

.art.live .facet {
  transition:
    transform 1s var(--pf-spring),
    opacity 0.6s var(--pf-ease);
  transition-delay: 0s;
}

.phone {
  inset: auto;
  left: 27.33%;
  top: 6.25%;
  z-index: 1;
  width: 45.33%;
  height: 86.875%;
  padding: 1.25cqw;
  border-radius: 7.4cqw;
  background: #04050d;
  box-shadow:
    inset 0 0 0 1px rgb(178 184 255 / 0.2),
    0 50px 90px -40px rgb(2 3 10 / 0.95);
}

.screen {
  position: relative;
  height: 100%;
  padding: 0 1.15em;
  overflow: hidden;
  border-radius: 6.2cqw;
  background: #0b0c1b;
  font-size: 2.06cqw;
  line-height: 1.3;
  color: var(--pf-ink-2);
}

.screen::before {
  content: '';
  position: absolute;
  top: 0.7em;
  left: 50%;
  width: 31%;
  height: 1.75em;
  margin-left: -15.5%;
  border-radius: 1em;
  background: #000;
}

.status {
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 3.3em;
  padding: 0 0.9em 0 1em;
  font-size: 0.92em;
  color: var(--pf-ink);
}

.status b {
  font-weight: 600;
}

.signal {
  display: flex;
  align-items: flex-end;
  gap: 0.18em;
  height: 0.9em;
}

.signal i {
  width: 0.24em;
  border-radius: 0.1em;
  background: var(--pf-ink);
}

.signal i:nth-child(1) {
  height: 35%;
}
.signal i:nth-child(2) {
  height: 55%;
}
.signal i:nth-child(3) {
  height: 78%;
}
.signal i:nth-child(4) {
  height: 100%;
}

.top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 0.5em;
}

.server {
  display: inline-flex;
  align-items: center;
  gap: 0.45em;
  padding: 0.38em 0.8em 0.38em 0.45em;
  border-radius: 1em;
  background: rgb(255 255 255 / 0.08);
  font-size: 0.8em;
  font-weight: 600;
  color: var(--pf-ink);
}

.server img {
  width: 1.35em;
  height: 1.35em;
}

.avatar {
  display: grid;
  place-items: center;
  width: 2.1em;
  height: 2.1em;
  border-radius: 50%;
  background: var(--pf-violet);
  font: 700 0.8em/1 var(--pf-display);
  color: #fff;
}

.heading {
  margin: 0.55em 0 0.5em;
  font: 700 2.05em/1.1 var(--pf-display);
  letter-spacing: -0.035em;
  color: var(--pf-ink);
}

.tabs {
  display: flex;
  gap: 0.4em;
}

.tabs span {
  padding: 0.42em 0.85em;
  border-radius: 1em;
  background: rgb(255 255 255 / 0.07);
  font-size: 0.74em;
  font-weight: 600;
  color: #cdd0e6;
}

.tabs .on {
  background: var(--pf-ink);
  color: #0b0c1b;
}

.row {
  margin: 1.15em 0 0.6em;
  font-size: 0.86em;
  font-weight: 600;
  color: var(--pf-ink);
}

.row span {
  margin-left: 0.35em;
  font-weight: 400;
  color: var(--pf-mute);
}

.resume {
  position: relative;
  aspect-ratio: 16 / 8.4;
  overflow: hidden;
  border-radius: 0.9em;
  background: #151733;
}

.resume img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: 50% 88%;
}

.resume::after {
  content: '';
  position: absolute;
  inset: 40% 0 0;
  background: linear-gradient(transparent, rgb(6 7 16 / 0.85));
}

.resume-title {
  position: absolute;
  left: 0.85em;
  bottom: 1.15em;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 0.15em;
}

.resume-title b {
  font: 700 0.98em/1.1 var(--pf-display);
  letter-spacing: -0.02em;
  color: #fff;
}

.resume-title span {
  font-size: 0.68em;
  color: #d6d8ea;
}

.bar {
  position: absolute;
  left: 0.85em;
  right: 0.85em;
  bottom: 0.6em;
  z-index: 1;
  height: 0.22em;
  border-radius: 1em;
  background: rgb(255 255 255 / 0.22);
}

.bar i {
  display: block;
  width: 64%;
  height: 100%;
  border-radius: inherit;
  background: var(--pf-cyan);
}

.grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 0.5em;
}

.grid img {
  width: 100%;
  aspect-ratio: 2 / 3;
  object-fit: cover;
  border-radius: 0.55em;
  background: #151733;
}

.tabbar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  justify-content: space-around;
  align-items: flex-start;
  height: 5.4em;
  padding-top: 0.85em;
  background: #0d0f21;
  box-shadow: 0 -1px 0 rgb(178 184 255 / 0.1);
}

.tabbar > span {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.3em;
  font-size: 0.62em;
  font-weight: 500;
  color: var(--pf-mute);
}

.tabbar .on {
  color: var(--pf-cyan);
}

@media (max-width: 1100px) {
  .art {
    justify-self: center;
    width: min(100%, 540px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .art:not(.built) .facet {
    --ox: 0px;
    --oy: 0px;
    opacity: 1;
  }
}
</style>
