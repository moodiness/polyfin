<script setup lang="ts">
import { withBase } from 'vitepress'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watchEffect } from 'vue'
import Icon from './Icon.vue'
import SoundControl from './SoundControl.vue'
import { features } from './home'
import { icons } from './icons'
import { heard, level } from './sound'

/** From 12 s, each feature's video ends on Polyfin's logo: the viewer moves to the next one there. The teaser (ad) plays to its end. */
const END = 12

const current = ref(0)
const video = ref<HTMLVideoElement>()
/** Whether the viewer plays when it is on screen: not with reduced motion, nor once paused. */
const autoplay = ref(false)
const playing = ref(false)
const feature = computed(() => features[current.value])
const number = computed(() => String(current.value + 1).padStart(2, '0'))
let visible = false
let progressBar: HTMLElement | undefined
let frame = 0
let observer: IntersectionObserver | undefined

onMounted(() => {
  autoplay.value = !matchMedia('(prefers-reduced-motion: reduce)').matches
  observer = new IntersectionObserver(
    ([entry]) => {
      visible = entry.isIntersecting
      sync()
    },
    { threshold: 0.5 },
  )
  observer.observe(video.value!)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  cancelAnimationFrame(frame)
})

watchEffect(() => {
  const element = video.value
  if (!element) return
  element.muted = heard.value !== 'features'
  element.volume = level.value
})

function play() {
  video.value?.play().catch((error: DOMException) => {
    // A browser that refuses to play muted videos on its own: show the poster and the play button.
    if (error.name === 'NotAllowedError') autoplay.value = false
  })
}

function sync() {
  if (autoplay.value && visible && !document.querySelector('.pf-theater[open]')) play()
  else video.value?.pause()
}

/** Shows a feature; it plays if the viewer does. */
function show(index: number) {
  current.value = (index + features.length) % features.length
  void nextTick(sync)
}

/** A feature picked in the list plays, even with reduced motion. */
function choose(index: number) {
  autoplay.value = true
  current.value = index
  void nextTick(play)
}

function togglePlay() {
  autoplay.value = video.value!.paused
  if (autoplay.value) play()
  else video.value!.pause()
}

function setProgressBar(element: unknown) {
  if (element instanceof HTMLElement) progressBar = element
}

function tick() {
  const time = video.value!.currentTime
  const end = feature.value.slug === 'ad' ? video.value!.duration || 30 : END
  if (feature.value.slug !== 'ad' && time >= END) return show(current.value + 1)
  progressBar?.style.setProperty('--progress', String(Math.min(1, time / end)))
  frame = requestAnimationFrame(tick)
}

function onPlay() {
  playing.value = true
  autoplay.value = true
  cancelAnimationFrame(frame)
  frame = requestAnimationFrame(tick)
}

function onPause() {
  playing.value = false
  cancelAnimationFrame(frame)
}
</script>

<template>
  <div class="viewer" :style="{ '--facet': `var(--pf-${feature.facet})` }">
    <div class="pf-media player">
      <span class="facet-back" aria-hidden="true"></span>
      <div class="clip">
        <video
          ref="video"
          :src="withBase(`/videos/polyfin-${feature.slug}.mp4`)"
          :poster="withBase(`/videos/polyfin-${feature.slug}.jpg`)"
          muted
          playsinline
          preload="none"
          :aria-label="`${feature.name}: ${feature.pitch}`"
          @play="onPlay"
          @pause="onPause"
          @ended="show(current + 1)"
        ></video>
        <span
          class="facet-tab"
          :class="{ 'dark-ink': feature.facet === 'cyan' || feature.facet === 'blue' }"
          aria-hidden="true"
          ><span class="number">{{ number }}</span></span
        >
        <button
          v-if="!autoplay && !playing"
          type="button"
          class="big-play"
          :aria-label="`Play ${feature.name}`"
          @click="choose(current)"
        >
          <span><Icon :svg="icons.play" :size="26" /></span>
        </button>
        <div class="controls">
          <button
            type="button"
            class="round"
            :aria-label="playing ? 'Pause' : 'Play'"
            @click="togglePlay"
          >
            <Icon :svg="playing ? icons.pause : icons.play" :size="17" />
          </button>
          <SoundControl owner="features" />
        </div>
      </div>
    </div>

    <ol class="episodes">
      <li
        v-for="(item, index) in features"
        :key="item.slug"
        :class="{ active: index === current }"
        :style="{ '--facet': `var(--pf-${item.facet})` }"
      >
        <button
          type="button"
          class="episode"
          :aria-expanded="index === current"
          :aria-controls="`pf-episode-${item.slug}`"
          @click="choose(index)"
        >
          <span class="tile" :class="{ 'on-cyan': item.facet === 'cyan' }"
            ><Icon :svg="item.icon" :size="19"
          /></span>
          <span class="name">{{ item.name }}</span>
          <span class="index">{{ String(index + 1).padStart(2, '0') }}</span>
        </button>
        <div :id="`pf-episode-${item.slug}`" class="body">
          <div>
            <p class="pitch">{{ item.pitch }}</p>
            <p class="text">{{ item.text }}</p>
            <a
              class="docs"
              :href="withBase(item.docs.link)"
              :aria-label="`${item.docs.title}, in the docs`"
              :tabindex="index === current ? undefined : -1"
            >
              <span class="pf-tri"></span>docs/<b>{{ item.docs.link.slice(6).split('#')[0] }}</b>
              <Icon :svg="icons.arrowUpRight" :size="14" />
            </a>
            <span
              v-if="index === current"
              :ref="setProgressBar"
              class="progress"
              aria-hidden="true"
            ></span>
          </div>
        </div>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.viewer {
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(300px, 1fr);
  gap: clamp(32px, 4vw, 56px);
  align-items: start;
}

.number {
  position: absolute;
  top: 13px;
  left: 13px;
  font: 700 14px/1 var(--pf-display);
  letter-spacing: -0.01em;
  color: #fff;
}

/* Text on the blue and cyan facets is ink: white on them is below AA. */
.dark-ink .number,
.tile.on-cyan {
  color: var(--pf-on-cyan);
}

.big-play {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: grid;
  place-items: center;
  cursor: pointer;
}

.big-play > span {
  display: grid;
  place-items: center;
  width: 72px;
  height: 72px;
  padding-left: 3px;
  border-radius: 50%;
  background: var(--pf-cyan);
  color: var(--pf-on-cyan);
  transition: transform 0.5s var(--pf-ease);
}

.big-play:hover > span {
  transform: scale(1.05);
}

.big-play:focus-visible {
  outline-offset: -6px;
}

.controls {
  position: absolute;
  right: 16px;
  bottom: 16px;
  z-index: 3;
  display: flex;
  gap: 8px;
}

.round {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: rgb(10 11 22 / 0.62);
  color: var(--pf-ink);
  cursor: pointer;
  transition:
    background-color 0.3s var(--pf-ease),
    transform 0.4s var(--pf-ease);
}

.round:hover {
  background: rgb(10 11 22 / 0.85);
}

.round:active {
  transform: scale(0.94);
}

.episodes {
  margin: 0;
  padding: 0;
  list-style: none;
}

.episodes li {
  border-top: 1px solid var(--pf-line);
}

.episodes li:last-child {
  border-bottom: 1px solid var(--pf-line);
}

.episode {
  display: grid;
  grid-template-columns: 36px minmax(0, 1fr) auto;
  align-items: center;
  gap: 16px;
  width: 100%;
  padding: 15px 2px;
  text-align: left;
  cursor: pointer;
}

.episode:focus-visible {
  outline-offset: -2px;
}

.tile {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: var(--facet);
  color: #fff;
}

.name {
  font: 600 17px/1.2 var(--pf-display);
  letter-spacing: -0.025em;
  color: var(--pf-mute);
  transition: color 0.4s var(--pf-ease);
}

.episode:hover .name,
.active .name {
  color: var(--pf-ink);
}

.index {
  font: 500 12.5px/1 var(--vp-font-family-mono);
  color: var(--pf-mute-2);
}

.body {
  display: grid;
  grid-template-rows: 0fr;
  transition: grid-template-rows 0.7s var(--pf-ease);
}

.active .body {
  grid-template-rows: 1fr;
}

.body > div {
  overflow: hidden;
  padding-left: 52px;
}

.pitch {
  margin: 2px 0 10px;
  font: 600 21px/1.25 var(--pf-display);
  letter-spacing: -0.03em;
  color: var(--pf-ink);
}

.text {
  margin: 0;
  font-size: 15px;
  line-height: 1.6;
  color: var(--pf-mute);
}

.docs {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin: 16px 0 18px;
  padding: 8px 12px 8px 11px;
  border-radius: 999px;
  background: var(--pf-s2);
  font: 400 13px/1 var(--vp-font-family-mono);
  color: var(--pf-mute);
  text-decoration: none;
  transition:
    background-color 0.3s var(--pf-ease),
    color 0.3s var(--pf-ease);
}

.docs b {
  font-weight: 500;
  color: var(--pf-ink);
}

.docs .pf-tri {
  width: 9px;
  height: 9px;
  margin-right: 2px;
  transform: rotate(90deg);
}

.docs:hover {
  background: var(--pf-s3);
  color: var(--pf-ink);
}

.progress {
  display: block;
  height: 3px;
  margin-bottom: 18px;
  overflow: hidden;
  border-radius: 2px;
  background: var(--pf-line);
}

.progress::after {
  content: '';
  display: block;
  height: 100%;
  background: var(--facet);
  transform: scaleX(var(--progress, 0));
  transform-origin: left;
}

@media (max-width: 1100px) {
  .viewer {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
